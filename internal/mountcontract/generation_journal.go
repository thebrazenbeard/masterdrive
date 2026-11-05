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

// GenerationJournal is a single-process observation fence, NOT an authorization
// authority. Callers must ensure exclusive ownership, protected storage and
// independent anti-rollback protection; restored files cannot prove revocation.
type GenerationJournal struct {
	mu        sync.Mutex
	f         *os.File
	owner     *os.File
	ownerPath string
	highest   uint64
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
		if e != nil || v == 0 || v <= prior || fields[1] != hex.EncodeToString(sum[:]) {
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
	if v == j.highest {
		return nil
	}
	s := strconv.FormatUint(v, 10)
	sum := sha256.Sum256([]byte("masterdrive-generation-v1:" + s))
	if _, e := io.WriteString(j.f, fmt.Sprintf("%s %x\n", s, sum)); e != nil {
		j.poisoned = true
		return e
	}
	if e := j.f.Sync(); e != nil {
		j.poisoned = true
		return e
	}
	j.highest = v
	return nil
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
		if closeErr := j.owner.Close(); e == nil {
			e = closeErr
		}
		j.owner = nil
		if e == nil {
			e = os.Remove(j.ownerPath)
		}
	}
	return e
}
