package mountcontract

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestJournalRejectsNoncanonicalGenerationRecords(t *testing.T) {
	for _, generation := range []string{"01", "+1", "0001", "1\t"} {
		t.Run(fmt.Sprintf("%q", generation), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "journal")
			sum := sha256.Sum256([]byte("masterdrive-generation-v1:" + generation))
			if err := os.WriteFile(path, []byte(fmt.Sprintf("%s %x\n", generation, sum)), 0600); err != nil {
				t.Fatal(err)
			}
			j, err := OpenGenerationJournal(path)
			if j != nil {
				j.Close()
				t.Fatal("noncanonical generation accepted")
			}
			if !errors.Is(err, ErrGenerationJournal) {
				t.Fatalf("unexpected error: %v", err)
			}
			if _, err := os.Stat(path + ".owner"); !os.IsNotExist(err) {
				t.Fatalf("ownership marker leaked: %v", err)
			}
		})
	}
}
