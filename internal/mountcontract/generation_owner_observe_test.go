package mountcontract

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestGenerationJournalOwnerMarkerRemovalPoisonsObservations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "generation.log")
	j, err := OpenGenerationJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	if err := j.Observe(7); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path + ".owner"); err != nil {
		t.Skipf("platform does not allow removal of open owner marker: %v", err)
	}
	if err := j.Observe(8); !errors.Is(err, ErrReadNotAuthorized) {
		t.Fatalf("removed owner marker must deny observation: %v", err)
	}
	if err := j.Observe(9); !errors.Is(err, ErrReadNotAuthorized) {
		t.Fatalf("poisoned journal must stay denied: %v", err)
	}
}

func TestGenerationJournalClosedOwnerPoisonsObservations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "generation.log")
	j, err := OpenGenerationJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	if err := j.Observe(7); err != nil {
		t.Fatal(err)
	}
	if err := j.owner.Close(); err != nil {
		t.Fatal(err)
	}
	if err := j.Observe(8); !errors.Is(err, ErrReadNotAuthorized) {
		t.Fatalf("closed owner handle must deny observation: %v", err)
	}
	if err := j.Observe(9); !errors.Is(err, ErrReadNotAuthorized) {
		t.Fatalf("poisoned journal must stay denied: %v", err)
	}
}
