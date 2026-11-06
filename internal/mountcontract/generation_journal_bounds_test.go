package mountcontract

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestJournalOversizeOpenFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(maxGenerationJournalBytes + 1); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	j, err := OpenGenerationJournal(path)
	if j != nil {
		j.Close()
		t.Fatal("oversized journal accepted")
	}
	if !errors.Is(err, ErrGenerationJournal) {
		t.Fatalf("got %v", err)
	}
}

func TestJournalExhaustionPoisonsWithoutWriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal")
	j, err := OpenGenerationJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	var n uint64
	for n = 1; n < 2000; n++ {
		err = j.Observe(n)
		if err != nil {
			break
		}
	}
	if !errors.Is(err, ErrReadNotAuthorized) {
		t.Fatalf("exhaustion result: %v", err)
	}
	stat, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if stat.Size() > maxGenerationJournalBytes {
		t.Fatalf("journal exceeded cap: %d", stat.Size())
	}
	if err := j.Observe(n + 1); !errors.Is(err, ErrReadNotAuthorized) {
		t.Fatalf("poison not persistent: %v", err)
	}
}
