package mountcontract

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGenerationJournalExclusiveOwner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal")
	first, err := OpenGenerationJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Observe(7); err != nil {
		t.Fatal(err)
	}
	if second, err := OpenGenerationJournal(path); err == nil {
		second.Close()
		t.Fatal("second writer acquired active journal")
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	next, err := OpenGenerationJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	if err := next.Observe(6); err == nil {
		t.Fatal("restart accepted rollback")
	}
}

func TestGenerationJournalStaleOwnerFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal")
	if err := os.WriteFile(path+".owner", []byte("stale"), 0600); err != nil {
		t.Fatal(err)
	}
	if j, err := OpenGenerationJournal(path); err == nil {
		j.Close()
		t.Fatal("stale ownership bypassed")
	}
}
