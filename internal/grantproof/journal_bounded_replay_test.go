package grantproof

import (
 "crypto/sha256"
 "errors"
 "os"
 "path/filepath"
 "strings"
 "testing"
)

// An attacker-controlled or corrupted journal must not force unbounded
// allocations while replaying a single record.
func TestJournalFingerprintFloorsRejectOversizedRecord(t *testing.T) {
 path := filepath.Join(t.TempDir(), "floor.log")
 j := &JournalFingerprintFloors{Path: path}
 fp := sha256.Sum256([]byte("baseline"))
 if err := j.Provision("source", "audience", "share", 1, fp); err != nil { t.Fatal(err) }
 f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
 if err != nil { t.Fatal(err) }
 if _, err := f.WriteString(strings.Repeat("x", 70*1024) + "\n"); err != nil { f.Close(); t.Fatal(err) }
 if err := f.Close(); err != nil { t.Fatal(err) }
 if _, _, err := j.FingerprintFloor("source", "audience", "share"); !errors.Is(err, ErrFloorUnavailable) { t.Fatalf("oversized journal read accepted: %v", err) }
 before, err := os.Stat(path)
 if err != nil { t.Fatal(err) }
 if err := j.CommitFingerprintIfCurrent("source", "audience", "share", 2, fp); !errors.Is(err, ErrFloorUnavailable) { t.Fatalf("oversized journal mutation accepted: %v", err) }
 after, err := os.Stat(path)
 if err != nil { t.Fatal(err) }
 if before.Size() != after.Size() { t.Fatal("rejected mutation changed journal") }
}
