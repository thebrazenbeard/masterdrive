package checkpointgate

// VerifyRevocationSet recomputes the observed tombstone digest from the
// supplied entries instead of trusting a caller-provided observed digest.
// trusted MUST be independently authenticated and rollback-resistant.
//
// This checks ONLY tombstones. It is not a complete grant-ledger checkpoint,
// does not authenticate trusted, and MUST NOT authorize pairing, reads or mounts.
func VerifyRevocationSet(trusted Checkpoint, storeID string, revision uint64, entries []Revocation) error {
 digest, err := DigestRevocations(storeID, revision, entries)
 if err != nil {
  return ErrUntrustedCheckpoint
 }
 return VerifyExact(trusted, Checkpoint{
  StoreID: storeID,
  Revision: revision,
  Digest: digest[:],
 })
}
