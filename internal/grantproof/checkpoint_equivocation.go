package grantproof

import (
 "crypto/sha256"
 "crypto/subtle"
 "errors"
)

// ErrCheckpointEquivocation means two different checkpoint messages claim the
// same source/audience/share revision. It is NOT evidence that either is trusted.
var ErrCheckpointEquivocation = errors.New("grantproof: conflicting checkpoint at same revision")

// CheckpointFingerprint hashes the canonical, domain-separated signed message.
// It does not verify a signature, establish source-key trust, or persist a floor.
func CheckpointFingerprint(c SignedCheckpoint) ([32]byte, error) {
 message, err := CheckpointMessage(c)
 if err != nil { return [32]byte{}, err }
 return sha256.Sum256(message), nil
}

// RejectSameRevisionFork detects a fork only for the same relationship and
// revision. A caller must independently authenticate both records and retain
// the previously accepted fingerprint in protected durable state.
// Different revisions are NOT validated by this function.
func RejectSameRevisionFork(previous, candidate SignedCheckpoint) error {
 a, b := previous.Authority, candidate.Authority
 if a.SourceID == "" || a.AudienceID == "" || a.ShareID == "" ||
  a.SourceID != b.SourceID || a.AudienceID != b.AudienceID ||
  a.ShareID != b.ShareID || previous.Revision == 0 ||
  previous.Revision != candidate.Revision { return ErrInvalid }
 oldDigest, err := CheckpointFingerprint(previous)
 if err != nil { return err }
 newDigest, err := CheckpointFingerprint(candidate)
 if err != nil { return err }
 if subtle.ConstantTimeCompare(oldDigest[:], newDigest[:]) != 1 { return ErrCheckpointEquivocation }
 return nil
}
