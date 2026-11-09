// Package admission provides a fail-closed, side-effect-free preflight predicate.
// It is not a mount, grant issuer, or source of trusted security evidence.
package admission

import ("errors"; "time")

var ErrDenied = errors.New("admission: missing, stale, or conflicting trusted evidence")

// Evidence must originate from independently authenticated, source-owned authorities.
// Merely populating these fields is NOT authentication.
type Evidence struct {
 PeerID, PairedPeerID, GrantPeerID, ShareID, GrantShareID string
 PeerKeyVersion, PairedKeyVersion uint64
 Revoked, GrantReadOnly bool
 GrantExpires time.Time
 GrantEpoch, CurrentPolicyEpoch uint64
 SnapshotVersion, PinnedVersion string
 SnapshotDigest, PinnedDigest [32]byte
 SnapshotSize, PinnedSize int64
 SourceRootConfined, ImmutableSnapshot, TransportAuthenticated, TransportDeadlineEnforced bool
}

// Check validates consistency and freshness only. It performs no I/O.
func Check(e Evidence, now time.Time) error {
 if now.IsZero() || e.PeerID == "" || e.ShareID == "" || e.PeerID != e.PairedPeerID || e.PeerID != e.GrantPeerID || e.ShareID != e.GrantShareID { return ErrDenied }
 if e.PeerKeyVersion == 0 || e.PeerKeyVersion != e.PairedKeyVersion || e.Revoked || !e.GrantReadOnly { return ErrDenied }
 if e.GrantEpoch == 0 || e.GrantEpoch != e.CurrentPolicyEpoch || e.GrantExpires.IsZero() || !now.Before(e.GrantExpires) { return ErrDenied }
 if e.SnapshotVersion == "" || e.SnapshotVersion != e.PinnedVersion || e.SnapshotSize < 0 || e.SnapshotSize != e.PinnedSize { return ErrDenied }
 if e.SnapshotDigest != e.PinnedDigest || e.SnapshotDigest == ([32]byte{}) { return ErrDenied }
 if !e.SourceRootConfined || !e.ImmutableSnapshot || !e.TransportAuthenticated || !e.TransportDeadlineEnforced { return ErrDenied }
 return nil
}
