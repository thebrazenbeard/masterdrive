package grantproof

import (
 "crypto/sha256"
 "testing"
)

// aliasingFloor deliberately violates the storage-side relationship namespace
// contract. This is a passing counterexample, NOT a secure implementation.
type aliasingFloor struct {
 revision uint64
 fingerprint [32]byte
}
func (f *aliasingFloor) FingerprintFloor(_, _, _ string) (uint64, [32]byte, error) {
 return f.revision, f.fingerprint, nil
}
func (f *aliasingFloor) CommitFingerprintIfCurrent(_, _, _ string, revision uint64, fingerprint [32]byte) error {
 if revision < f.revision { return ErrFloorUnavailable }
 if revision == f.revision && fingerprint != f.fingerprint { return ErrCheckpointEquivocation }
 f.revision, f.fingerprint = revision, fingerprint
 return nil
}

// TestScopedFloorDoesNotPreventBackingStoreAlias documents why checking the
// adapter's relationship fields cannot establish storage-side isolation.
// Two correctly scoped adapters share a broken backend which ignores its keys.
// A commit through one relationship changes the other's observed floor.
func TestScopedFloorDoesNotPreventBackingStoreAlias(t *testing.T) {
 old := sha256.Sum256([]byte("initial checkpoint"))
 newer := sha256.Sum256([]byte("unrelated relationship checkpoint"))
 backing := &aliasingFloor{revision: 10, fingerprint: old}
 a := ScopedCheckpointFloor{Store: backing, SourceID: "source-A", AudienceID: "reader-A", ShareID: "share-A"}
 b := ScopedCheckpointFloor{Store: backing, SourceID: "source-B", AudienceID: "reader-B", ShareID: "share-B"}
 if err := a.CommitFingerprintIfCurrent("source-A", "reader-A", "share-A", 11, newer); err != nil {
  t.Fatalf("fixture commit: %v", err)
 }
 gotRevision, gotFingerprint, err := b.FingerprintFloor("source-B", "reader-B", "share-B")
 if err != nil { t.Fatalf("fixture read: %v", err) }
 if gotRevision != 11 || gotFingerprint != newer {
  t.Fatalf("expected to demonstrate backing-store alias, got revision %d", gotRevision)
 }
 // The adapter still rejects direct cross-relationship calls. The defect is
 // entirely behind the adapter, so an adapter-only test cannot qualify it.
 if _, _, err := a.FingerprintFloor("source-B", "reader-B", "share-B"); err != ErrFloorUnavailable {
  t.Fatalf("adapter unexpectedly accepted cross-scope read: %v", err)
 }
}
