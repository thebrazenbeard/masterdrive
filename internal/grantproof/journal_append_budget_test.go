package grantproof

import (
 "errors"
 "os"
 "path/filepath"
 "testing"
)

// The append helper must not acknowledge a write that makes a previously
// replayable journal exceed the total replay budget.
func TestJournalAppendRejectsAggregateBudgetOverflow(t *testing.T) {
 path := filepath.Join(t.TempDir(), "floor.journal")
 const maxJournalBytes int64 = 8 << 20
 f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0600)
 if err != nil { t.Fatal(err) }
 if err := f.Truncate(maxJournalBytes-1); err != nil { f.Close(); t.Fatal(err) }
 if err := f.Close(); err != nil { t.Fatal(err) }
 store := &JournalFingerprintFloors{Path:path}
 entry := journalEntry{Source:"source", Audience:"audience", Share:"share", Revision:1, Fingerprint:"0123456789012345678901234567890123456789012345678901234567890123"}
 if err := store.append(entry); !errors.Is(err, ErrFloorUnavailable) { t.Fatalf("append beyond budget must fail closed: %v", err) }
 info, err := os.Stat(path)
 if err != nil { t.Fatal(err) }
 if info.Size()!=maxJournalBytes-1 { t.Fatalf("failed append changed journal size: %d", info.Size()) }
}
