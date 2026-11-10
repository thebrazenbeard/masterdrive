package grantproof

import "time"

// CurrentAuthority is evidence that MUST be obtained from an independently
// authenticated, rollback-resistant source. This type does not provide that
// authority or freshness by itself.
type CurrentAuthority struct {
 SourceID string
 ShareID string
 AudienceID string
 KeyVersion uint64
 GrantEpoch uint64
 Revoked bool
}

// VerifyCurrent rejects stale grants, rotated keys and explicitly revoked
// relationships before verifying the signed grant. Callers must reacquire
// authenticated current authority for every read or use a separately reviewed
// bounded lease. Never treat caller-constructed CurrentAuthority as trusted.
func VerifyCurrent(g Grant, signature []byte, trusted TrustedSource, current CurrentAuthority, snapshot string, now time.Time) error {
 if current.Revoked || current.SourceID == "" || current.ShareID == "" || current.AudienceID == "" ||
  current.KeyVersion == 0 || current.GrantEpoch == 0 ||
  current.SourceID != g.SourceID || current.ShareID != g.ShareID ||
  current.AudienceID != g.AudienceID || current.KeyVersion != g.KeyVersion ||
  current.GrantEpoch != g.GrantEpoch || trusted.KeyVersion != current.KeyVersion {
  return ErrInvalid
 }
 return Verify(g, signature, trusted, current.AudienceID, current.ShareID, snapshot, now)
}
