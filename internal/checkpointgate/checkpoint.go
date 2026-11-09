// Package checkpointgate provides a fail-closed comparison between a locally
// observed revocation ledger checkpoint and independently trusted evidence.
// It does not authenticate either input or make filesystem reads atomic.
package checkpointgate

import (
 "crypto/subtle"
 "errors"
)

var ErrUntrustedCheckpoint = errors.New("checkpointgate: checkpoint mismatch or missing evidence")

// Checkpoint describes one complete, canonical ledger snapshot. Digest must
// be SHA-256 of a deterministic, domain-separated serialization of the full
// ledger including tombstones, not a hash supplied by an untrusted peer.
type Checkpoint struct {
 StoreID string
 Revision uint64
 Digest []byte
}

// VerifyExact rejects rollback, missing tombstones, and divergent ledgers
// when trusted was obtained independently of the mutable ledger being checked.
// An attacker able to rewrite both inputs defeats this check. Revision alone
// cannot prove a tombstone has not been deleted.
func VerifyExact(trusted, observed Checkpoint) error {
 if trusted.StoreID == "" || observed.StoreID == "" ||
  trusted.Revision == 0 || observed.Revision == 0 ||
  len(trusted.Digest) != 32 || len(observed.Digest) != 32 {
  return ErrUntrustedCheckpoint
 }
 if trusted.StoreID != observed.StoreID || trusted.Revision != observed.Revision ||
  subtle.ConstantTimeCompare(trusted.Digest, observed.Digest) != 1 {
  return ErrUntrustedCheckpoint
 }
 return nil
}
