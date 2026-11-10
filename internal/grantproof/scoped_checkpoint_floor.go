package grantproof

import "errors"

// ScopedCheckpointFloor binds a floor store to one provisioned relationship.
// It prevents a caller from accidentally reading or committing another
// relationship's floor through a reused store handle. It is NOT a durable
// store, an identity authenticator, or a replacement for storage-side
// namespace isolation.
type ScopedCheckpointFloor struct {
 Store CheckpointFingerprintFloorStore
 SourceID, AudienceID, ShareID string
}

func (s ScopedCheckpointFloor) matches(source, audience, share string) bool {
 return s.Store != nil && s.SourceID != "" && s.AudienceID != "" && s.ShareID != "" &&
  source == s.SourceID && audience == s.AudienceID && share == s.ShareID
}

func (s ScopedCheckpointFloor) FingerprintFloor(source, audience, share string) (uint64, [32]byte, error) {
 if !s.matches(source, audience, share) {
  return 0, [32]byte{}, ErrFloorUnavailable
 }
 return s.Store.FingerprintFloor(source, audience, share)
}

func (s ScopedCheckpointFloor) CommitFingerprintIfCurrent(source, audience, share string, revision uint64, fingerprint [32]byte) error {
 if !s.matches(source, audience, share) {
  return ErrFloorUnavailable
 }
 if revision == 0 {
  return ErrFloorUnavailable
 }
 err := s.Store.CommitFingerprintIfCurrent(source, audience, share, revision, fingerprint)
 if err != nil {
  if errors.Is(err, ErrCheckpointEquivocation) { return ErrCheckpointEquivocation }
  return ErrFloorUnavailable
 }
 return nil
}
