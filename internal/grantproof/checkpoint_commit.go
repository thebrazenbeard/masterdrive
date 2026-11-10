package grantproof

import (
 "errors"
 "time"
)

// CheckpointFloorStore is an injected authority boundary, not an implementation.
// Floor must return an independently protected durable floor; CommitIfNotSuperseded must
// durably and atomically reject a stored revision greater than the candidate and persist the candidate,
// and return only after persistence. Errors must fail closed.
type CheckpointFloorStore interface {
 Floor(sourceID, audienceID, shareID string) (uint64, error)
 CommitIfNotSuperseded(sourceID, audienceID, shareID string, revision uint64) error
}

var ErrFloorUnavailable = errors.New("grantproof: durable checkpoint floor unavailable")

// VerifyAndCommitCheckpoint validates signed grant/checkpoint evidence and
// requires the monotonic floor to be committed BEFORE reporting admission.
// It does not perform I/O, authorize future reads, or authenticate the store.
// A trusted store and current per-read revocation checks are still required.
func VerifyAndCommitCheckpoint(store CheckpointFloorStore, g Grant, grantSig []byte, trusted TrustedSource, c SignedCheckpoint, checkpointSig []byte, maxAge time.Duration, snapshot string, now time.Time) error {
 if store == nil { return ErrFloorUnavailable }
 a := c.Authority
 if a.SourceID == "" || a.AudienceID == "" || a.ShareID == "" { return ErrInvalid }
 floor, err := store.Floor(a.SourceID, a.AudienceID, a.ShareID)
 if err != nil || floor == 0 { return ErrFloorUnavailable }
 if err := VerifySignedCurrent(g, grantSig, trusted, c, checkpointSig, floor, maxAge, snapshot, now); err != nil { return err }
 if err := store.CommitIfNotSuperseded(a.SourceID, a.AudienceID, a.ShareID, c.Revision); err != nil { return ErrFloorUnavailable }
 return nil
}
