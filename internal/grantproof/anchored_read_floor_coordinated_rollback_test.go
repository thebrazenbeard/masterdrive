package grantproof

import (
    "os"
    "path/filepath"
    "testing"
)

// This deliberately PASSING counterexample demonstrates that two unprotected
// filesystem copies are not an independent anti-rollback authority.
func TestAnchoredReadFloorAcceptsCoordinatedHistoricalRollback(t *testing.T) {
    dir := t.TempDir()
    journalPath := filepath.Join(dir, "journal")
    authorityPath := filepath.Join(dir, "authority")
    journal := &JournalFingerprintFloors{Path: journalPath}
    authority := &JournalFingerprintFloors{Path: authorityPath}
    oldFP, revokedFP := [32]byte{1}, [32]byte{2}
    for _, store := range []*JournalFingerprintFloors{journal, authority} {
        if err := store.Provision("source", "audience", "share", 1, oldFP); err != nil {
            t.Fatal(err)
        }
    }
    oldJournal, err := os.ReadFile(journalPath)
    if err != nil { t.Fatal(err) }
    oldAuthority, err := os.ReadFile(authorityPath)
    if err != nil { t.Fatal(err) }
    for _, store := range []*JournalFingerprintFloors{journal, authority} {
        if err := store.CommitFingerprintIfCurrent("source", "audience", "share", 2, revokedFP); err != nil {
            t.Fatal(err)
        }
    }
    gate := &AnchoredReadFloor{Journal: journal, Authority: authority}
    if rev, fp, err := gate.FingerprintFloor("source", "audience", "share"); err != nil || rev != 2 || fp != revokedFP {
        t.Fatalf("current agreement: rev=%d fp=%x err=%v", rev, fp, err)
    }
    // Adversary replaces both stores with complete, internally valid history.
    // This is a disposable test directory, never an operator's real authority.
    if err := os.WriteFile(journalPath, oldJournal, 0600); err != nil { t.Fatal(err) }
    if err := os.WriteFile(authorityPath, oldAuthority, 0600); err != nil { t.Fatal(err) }
    reopened := &AnchoredReadFloor{
        Journal: &JournalFingerprintFloors{Path: journalPath},
        Authority: &JournalFingerprintFloors{Path: authorityPath},
    }
    rev, fp, err := reopened.FingerprintFloor("source", "audience", "share")
    if err != nil || rev != 1 || fp != oldFP {
        t.Fatalf("expected demonstration of coordinated rollback acceptance: rev=%d fp=%x err=%v", rev, fp, err)
    }
}
