package mountcontract

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestGenerationJournalRejectsExternalTruncation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal")
	j, err := OpenGenerationJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	if err := j.Observe(9); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(path, 0); err != nil {
		t.Skipf("platform prevents concurrent truncation: %v", err)
	}
	if err := j.Observe(9); !errors.Is(err, ErrReadNotAuthorized) {
		t.Fatalf("unchanged generation after truncation: %v", err)
	}
	if err := j.Observe(10); !errors.Is(err, ErrReadNotAuthorized) {
		t.Fatalf("poisoned journal recovered: %v", err)
	}
}
