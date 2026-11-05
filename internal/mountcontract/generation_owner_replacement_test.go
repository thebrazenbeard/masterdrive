package mountcontract

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestJournalCloseDoesNotDeleteReplacedOwner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "generations")
	j, err := OpenGenerationJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	marker := path + ".owner"
	if err := os.Remove(marker); err != nil {
		_ = j.Close()
		t.Skipf("OS prevents replacement of open owner marker: %v", err)
	}
	if err := os.WriteFile(marker, []byte("foreign owner"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := j.Close(); !errors.Is(err, ErrGenerationJournal) {
		t.Fatalf("expected replacement rejection, got %v", err)
	}
	b, err := os.ReadFile(marker)
	if err != nil || string(b) != "foreign owner" {
		t.Fatalf("foreign marker lost: %q, %v", b, err)
	}
}
