package mountcontract

import (
	"os"
	"path/filepath"
	"testing"
)

// This test deliberately demonstrates a security limitation, not an accepted
// authorization property. A restored valid journal can erase observations.
func TestJournalSnapshotRollbackIsNotDetected(t *testing.T) {
	p := filepath.Join(t.TempDir(), "journal")
	j, err := OpenGenerationJournal(p)
	if err != nil {
		t.Fatal(err)
	}
	if err = j.Observe(4); err != nil {
		t.Fatal(err)
	}
	if err = j.Close(); err != nil {
		t.Fatal(err)
	}
	snapshot, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	j, err = OpenGenerationJournal(p)
	if err != nil {
		t.Fatal(err)
	}
	if err = j.Observe(9); err != nil {
		t.Fatal(err)
	}
	if err = j.Close(); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(p, snapshot, 0600); err != nil {
		t.Fatal(err)
	}
	j, err = OpenGenerationJournal(p)
	if err != nil {
		t.Fatalf("snapshot rollback demonstration changed: %v", err)
	}
	defer j.Close()
	if err := j.Observe(4); err != nil {
		t.Fatalf("restored generation rejected: %v", err)
	}
}
