package mountcontract

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
)

var ErrGenerationJournal = errors.New("invalid generation journal")

// maxGenerationJournalBytes bounds observation storage until authenticated
// checkpoint/compaction authority exists. Exhaustion fails closed.
const maxGenerationJournalBytes int64 = 64 * 1024

// GenerationJournal is a single-process observation fence, NOT an authorization
// authority. Callers must ensure exclusive ownership, protected storage and
// independent anti-rollback protection; restored files cannot prove revocation.
type GenerationJournal struct {
	mu        sync.Mutex
	f         *os.File
	owner     *os.File
	ownerPath string
	highest   uint64
	size      int64
	digest    [32]byte
	poisoned  bool
}

func OpenGenerationJournal(path string) (*GenerationJournal, error) {
	ownerPath := path + ".owner"
	owner, e := os.OpenFile(ownerPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return nil, fmt.Errorf("journal ownership unavailable: %w", e)
	}
	f, e := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0600)
	if e != nil {
		owner.Close()
		os.Remove(ownerPath)
		return nil, e
	}
	j := &GenerationJournal{f: f, owner: owner, ownerPath: ownerPath}
	initialStat, statErr := f.Stat()
	if statErr != nil || initialStat.Size() > maxGenerationJournalBytes {
		j.Close()
		return nil, ErrGenerationJournal
	}
	s := bufio.NewScanner(f)
	var prior uint64
	for s.Scan() {
		fields := strings.Split(s.Text(), " ")
		if len(fields) != 2 {
			j.Close()
			return nil, ErrGenerationJournal
		}
		v, e := strconv.ParseUint(fields[0], 10, 64)
		sum := sha256.Sum256([]byte("masterdrive-generation-v1:" + fields[0]))
		if e != nil || v == 0 || v <= prior || strconv.FormatUint(v, 10) != fields[0] || fields[1] != hex.EncodeToString(sum[:]) {
			j.Close()
			return nil, ErrGenerationJournal
		}
		prior = v
	}
	if e := s.Err(); e != nil {
		j.Close()
		return nil, e
	}
	stat, e := f.Stat()
	if e != nil {
		j.Close()
		return nil, e
	}
	if stat.Size() > 0 {
		b := make([]byte, 1)
		_, e = f.ReadAt(b, stat.Size()-1)
		if e != nil || b[0] != '\n' {
			j.Close()
			return nil, ErrGenerationJournal
		}
	}
	j.highest = prior
	j.size = stat.Size()
	j.digest, e = journalDigest(f, j.size)
	if e != nil {
		j.Close()
		return nil, e
	}
	return j, nil
}
func (j *GenerationJournal) Observe(v uint64) error {
	if j == nil {
		return ErrReadNotAuthorized
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.poisoned || j.f == nil || v == 0 || v < j.highest {
		j.poisoned = true
		return ErrReadNotAuthorized
	}
	ownerInfo, ownerErr := j.owner.Stat()
	ownerAtPath, ownerPathErr := os.Stat(j.ownerPath)
	if ownerErr != nil || ownerPathErr != nil || !os.SameFile(ownerInfo, ownerAtPath) {
		j.poisoned = true
		return ErrReadNotAuthorized
	}
	stat, statErr := j.f.Stat()
	pathStat, pathErr := os.Stat(j.f.Name())
	if statErr != nil || pathErr != nil || !os.SameFile(stat, pathStat) || stat.Size() != j.size {
		j.poisoned = true
		return ErrReadNotAuthorized
	}
	digest, digestErr := journalDigest(j.f, j.size)
	if digestErr != nil || digest != j.digest {
		j.poisoned = true
		return ErrReadNotAuthorized
	}
	if v == j.highest {
		return nil
	}
	s := strconv.FormatUint(v, 10)
	sum := sha256.Sum256([]byte("masterdrive-generation-v1:" + s))
	record := fmt.Sprintf("%s %x\n", s, sum)
	if j.size > maxGenerationJournalBytes-int64(len(record)) {
		j.poisoned = true
		return ErrReadNotAuthorized
	}
	if n, e := writeJournalRecord(j.f, record); e != nil || n != len(record) {
		j.poisoned = true
		if e != nil {
			return e
		}
		return io.ErrShortWrite
	}
	if e := j.f.Sync(); e != nil {
		j.poisoned = true
		return e
	}
	j.highest = v
	j.size += int64(len(record))
	updatedDigest, digestErr := journalDigest(j.f, j.size)
	if digestErr != nil {
		j.poisoned = true
		return digestErr
	}
	j.digest = updatedDigest
	return nil
}

// writeJournalRecord rejects a short write even when the writer returns nil error.
// Kept separate to permit deterministic fault injection without touching real files.
func writeJournalRecord(w io.Writer, record string) (int, error) {
	n, err := io.WriteString(w, record)
	if err == nil && n != len(record) {
		return n, io.ErrShortWrite
	}
	return n, err
}

// journalDigest reads through the existing file handle without moving its append offset.
// This detects same-length external rewrites; it does not authenticate storage.
func journalDigest(f *os.File, size int64) ([32]byte, error) {
	var zero [32]byte
	h := sha256.New()
	if size < 0 {
		return zero, ErrGenerationJournal
	}
	if _, e := io.CopyN(h, io.NewSectionReader(f, 0, size), size); e != nil {
		return zero, e
	}
	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	return sum, nil
}

func (j *GenerationJournal) Close() error {
	if j == nil {
		return nil
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	j.poisoned = true
	if j.f == nil {
		return nil
	}
	e := j.f.Close()
	j.f = nil
	if j.owner != nil {
		ownerInfo, ownerStatErr := j.owner.Stat()
		if ownerStatErr != nil && e == nil {
			e = ownerStatErr
		}
		if closeErr := j.owner.Close(); e == nil {
			e = closeErr
		}
		j.owner = nil
		if e == nil {
			// Never remove a marker replaced by another process. This check
			// is not atomic and does not replace an OS-owned file lock.
			marker, statErr := os.Stat(j.ownerPath)
			if statErr != nil || !os.SameFile(ownerInfo, marker) {
				e = ErrGenerationJournal
			} else {
				e = os.Remove(j.ownerPath)
			}
		}
	}
	return e
}
