package grantproof

import (
    "os"
    "path/filepath"
    "testing"
)

// This test deliberately documents an unresolved security failure, not a
// desirable authorization property. A complete valid historical journal can
// be restored without triggering checksum or revision validation.
func TestJournalCompleteRollbackCounterexample(t *testing.T) {
    path := filepath.Join(t.TempDir(), "floor.journal")
    store := &JournalFingerprintFloors{Path: path}
    var first, second [32]byte
    first[0] = 1
    second[0] = 2
    if err := store.Provision("source", "audience", "share", 1, first); err != nil {
        t.Fatal(err)
    }
    historical, err := os.ReadFile(path)
    if err != nil { t.Fatal(err) }
    if err := store.CommitFingerprintIfCurrent("source", "audience", "share", 2, second); err != nil {
        t.Fatal(err)
    }
    revision, fingerprint, err := store.FingerprintFloor("source", "audience", "share")
    if err != nil || revision != 2 || fingerprint != second {
        t.Fatalf("expected committed revision 2: revision=%d error=%v", revision, err)
    }
    // Simulate a filesystem attacker replacing the entire journal while no
    // cooperative lock is held. The record checksum remains valid.
    if err := os.WriteFile(path, historical, 0600); err != nil { t.Fatal(err) }
    reopened := &JournalFingerprintFloors{Path: path}
    revision, fingerprint, err = reopened.FingerprintFloor("source", "audience", "share")
    if err != nil { t.Fatal(err) }
    if revision != 1 || fingerprint != first {
        t.Fatalf("counterexample changed: revision=%d fingerprint=%x", revision, fingerprint)
    }
    t.Log("KNOWN VULNERABILITY: a complete historical journal is accepted after rollback; no independent monotonic authority exists")
}
