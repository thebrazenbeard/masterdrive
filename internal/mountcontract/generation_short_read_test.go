package mountcontract

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestJournalDigestRejectsShortRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal")
	if err := os.WriteFile(path, []byte("abc"), 0600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := journalDigest(f, 4); !errors.Is(err, io.EOF) {
		t.Fatalf("expected EOF on short read, got %v", err)
	}
	if _, err := journalDigest(f, -1); !errors.Is(err, ErrGenerationJournal) {
		t.Fatalf("expected invalid length rejection, got %v", err)
	}
	if _, err := journalDigest(f, 3); err != nil {
		t.Fatalf("exact read failed: %v", err)
	}
}
