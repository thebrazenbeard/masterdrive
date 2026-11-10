package grantproof

import (
    "errors"
    "testing"
)

// An adapter is allowed to return a plausible record even for malformed
// identity inputs. The gate must reject before it consults that adapter.
func TestAnchoredReadFloorRejectsEmptyIdentityWithoutReaderCalls(t *testing.T) {
    cases := []struct {
        name, source, audience, share string
    }{
        {"source", "", "audience", "share"},
        {"audience", "source", "", "share"},
        {"share", "source", "audience", ""},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            fp := [32]byte{7}
            journal := &scriptedFloor{revisions: []uint64{4}, fingerprints: [][32]byte{fp}}
            authority := &scriptedFloor{revisions: []uint64{4}, fingerprints: [][32]byte{fp}}
            rev, got, err := (&AnchoredReadFloor{Journal: journal, Authority: authority}).FingerprintFloor(tc.source, tc.audience, tc.share)
            if !errors.Is(err, ErrFloorUnavailable) || rev != 0 || got != ([32]byte{}) {
                t.Fatalf("malformed identity must fail closed: rev=%d fp=%v err=%v", rev, got, err)
            }
            if journal.calls != 0 || authority.calls != 0 {
                t.Fatalf("malformed identity reached readers: journal=%d authority=%d", journal.calls, authority.calls)
            }
        })
    }
}

func TestAnchoredReadFloorValidIdentityStillConsultsBothReaders(t *testing.T) {
    fp := [32]byte{9}
    journal := &scriptedFloor{revisions: []uint64{4}, fingerprints: [][32]byte{fp}}
    authority := &scriptedFloor{revisions: []uint64{4}, fingerprints: [][32]byte{fp}}
    rev, got, err := (&AnchoredReadFloor{Journal: journal, Authority: authority}).FingerprintFloor("source", "audience", "share")
    if err != nil || rev != 4 || got != fp || journal.calls != 2 || authority.calls != 2 {
        t.Fatalf("valid identity regression: rev=%d fp=%v err=%v calls=%d/%d", rev, got, err, journal.calls, authority.calls)
    }
}
