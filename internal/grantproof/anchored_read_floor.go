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

// AnchoredReadFloor compares a journal snapshot against an independently
// supplied floor before exposing it. This is a read-only consistency gate,
// NOT an anti-rollback authority or permission to admit real reads/mounts.
// Both stores must agree exactly; unavailable, stale and divergent states fail closed.
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
    if je != nil || ae != nil || jr == 0 || ar == 0 || jr != ar ||
        subtle.ConstantTimeCompare(jf[:], af[:]) != 1 {
        return 0, [32]byte{}, errors.Join(ErrFloorUnavailable, je, ae)
    }
    return jr, jf, nil
}
