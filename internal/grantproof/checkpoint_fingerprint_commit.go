package grantproof

import (
 "crypto/subtle"
 "errors"
 "time"
)

// CheckpointFingerprintFloorStore is a stricter, injected storage contract.
// A real implementation must atomically persist the revision and fingerprint,
// reject older revisions and conflicting equal revisions, and return only after
// durable commit. The caller must provision and protect the initial floor.
// This interface does not itself provide persistence or rollback resistance.
type CheckpointFingerprintFloorStore interface {
 FingerprintFloor(sourceID, audienceID, shareID string) (revision uint64, fingerprint [32]byte, err error)
 CommitFingerprintIfCurrent(sourceID, audienceID, shareID string, revision uint64, fingerprint [32]byte) error
}

// VerifyAndCommitCheckpointFingerprint validates both signatures, checks the
// previously committed fingerprint on equal revision, and requires an atomic
// revision+fingerprint commit before admission. It does not authorize future
// reads or qualify any concrete store.
func VerifyAndCommitCheckpointFingerprint(store CheckpointFingerprintFloorStore, g Grant, grantSig []byte, trusted TrustedSource, c SignedCheckpoint, checkpointSig []byte, maxAge time.Duration, snapshot string, now time.Time) error {
 if store == nil { return ErrFloorUnavailable }
 a := c.Authority
 if a.SourceID == "" || a.AudienceID == "" || a.ShareID == "" { return ErrInvalid }
 floor, prior, err := store.FingerprintFloor(a.SourceID, a.AudienceID, a.ShareID)
 if err != nil || floor == 0 { return ErrFloorUnavailable }
 if err := VerifySignedCurrent(g, grantSig, trusted, c, checkpointSig, floor, maxAge, snapshot, now); err != nil { return err }
 digest, err := CheckpointFingerprint(c)
 if err != nil { return err }
 if floor == c.Revision && subtle.ConstantTimeCompare(prior[:], digest[:]) != 1 {
  return ErrCheckpointEquivocation
 }
 // The store MUST repeat both comparisons atomically at commit time. A
 // concurrent writer may have advanced the floor since FingerprintFloor.
 if err := store.CommitFingerprintIfCurrent(a.SourceID, a.AudienceID, a.ShareID, c.Revision, digest); err != nil {
  if errors.Is(err, ErrCheckpointEquivocation) { return ErrCheckpointEquivocation }
  return ErrFloorUnavailable
 }
 return nil
}
