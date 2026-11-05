package mountcontract

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestJournalRestartRollbackAndPoison(t *testing.T) {
	p := filepath.Join(t.TempDir(), "j")
	j, e := OpenGenerationJournal(p)
	if e != nil {
		t.Fatal(e)
	}
	for _, v := range []uint64{3, 8, 8} {
		if e = j.Observe(v); e != nil {
			t.Fatal(e)
		}
	}
	j.Close()
	j, e = OpenGenerationJournal(p)
	if e != nil {
		t.Fatal(e)
	}
	if e = j.Observe(7); !errors.Is(e, ErrReadNotAuthorized) {
		t.Fatalf("rollback: %v", e)
	}
	if e = j.Observe(9); !errors.Is(e, ErrReadNotAuthorized) {
		t.Fatalf("poison: %v", e)
	}
	j.Close()
}
func TestJournalTornRecordRejected(t *testing.T) {
	p := filepath.Join(t.TempDir(), "j")
	j, e := OpenGenerationJournal(p)
	if e != nil {
		t.Fatal(e)
	}
	if e = j.Observe(4); e != nil {
		t.Fatal(e)
	}
	j.Close()
	f, e := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		t.Fatal(e)
	}
	f.WriteString("9 bad")
	f.Close()
	if j, e = OpenGenerationJournal(p); !errors.Is(e, ErrGenerationJournal) {
		if j != nil {
			j.Close()
		}
		t.Fatalf("accepted torn record: %v", e)
	}
}
func TestJournalMaxGeneration(t *testing.T) {
	p := filepath.Join(t.TempDir(), "j")
	j, e := OpenGenerationJournal(p)
	if e != nil {
		t.Fatal(e)
	}
	if e = j.Observe(^uint64(0)); e != nil {
		t.Fatal(e)
	}
	j.Close()
	j, e = OpenGenerationJournal(p)
	if e != nil {
		t.Fatal(e)
	}
	if e = j.Observe(^uint64(0)); e != nil {
		t.Fatal(e)
	}
	j.Close()
}
