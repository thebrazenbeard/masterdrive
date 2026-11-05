package mountcontract

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestGenerationJournalRejectsSameLengthRewrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "generation.log")
	j, err := OpenGenerationJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	if err := j.Observe(9); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteAt([]byte("8"), 0); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if err := j.Observe(9); !errors.Is(err, ErrReadNotAuthorized) {
		t.Fatalf("same-length rewrite accepted: %v", err)
	}
	if err := j.Observe(10); !errors.Is(err, ErrReadNotAuthorized) {
		t.Fatalf("poisoned journal resumed: %v", err)
	}
}
