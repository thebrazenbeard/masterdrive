package grantproof

import (
    "crypto/subtle"
    "errors"
)

// FingerprintFloorReader supplies a relationship-specific checkpoint floor.
// Its implementation and protection are outside this interface.
type FingerprintFloorReader interface {
    FingerprintFloor(source, audience, share string) (uint64, [32]byte, error)
}

// AnchoredReadFloor checks two successive snapshots from each source.
// This detects changes observed during comparison, but is NOT an atomic
// snapshot, a protected anti-rollback authority, or read/mount admission.
type AnchoredReadFloor struct {
    Journal FingerprintFloorReader
    Authority FingerprintFloorReader
}

func (a *AnchoredReadFloor) FingerprintFloor(source, audience, share string) (uint64, [32]byte, error) {
    if a == nil || a.Journal == nil || a.Authority == nil {
        return 0, [32]byte{}, ErrFloorUnavailable
    }
    jr, jf, je := a.Journal.FingerprintFloor(source, audience, share)
    ar, af, ae := a.Authority.FingerprintFloor(source, audience, share)
    jr2, jf2, je2 := a.Journal.FingerprintFloor(source, audience, share)
    ar2, af2, ae2 := a.Authority.FingerprintFloor(source, audience, share)
    if je != nil || ae != nil || je2 != nil || ae2 != nil ||
        jr == 0 || ar == 0 || jr != ar || jr != jr2 || ar != ar2 ||
        subtle.ConstantTimeCompare(jf[:], af[:]) != 1 ||
        subtle.ConstantTimeCompare(jf[:], jf2[:]) != 1 ||
        subtle.ConstantTimeCompare(af[:], af2[:]) != 1 {
        return 0, [32]byte{}, errors.Join(ErrFloorUnavailable, je, ae, je2, ae2)
    }
    return jr, jf, nil
}
