package grantproof

import (
    "errors"
    "testing"
)

type scriptedFloor struct {
    calls int
    revisions []uint64
    fingerprints [][32]byte
    failAt int
}

func (s *scriptedFloor) FingerprintFloor(_, _, _ string) (uint64, [32]byte, error) {
    s.calls++
    if s.failAt == s.calls {
        return 0, [32]byte{}, ErrFloorUnavailable
    }
    i := s.calls - 1
    if i >= len(s.revisions) { i = len(s.revisions)-1 }
    return s.revisions[i], s.fingerprints[i], nil
}

func TestAnchoredReadFloorRejectsObservedJournalDrift(t *testing.T) {
    old, next := [32]byte{1}, [32]byte{2}
    journal := &scriptedFloor{revisions: []uint64{1, 2}, fingerprints: [][32]byte{old, next}}
    authority := &scriptedFloor{revisions: []uint64{1, 1}, fingerprints: [][32]byte{old, old}}
    gate := &AnchoredReadFloor{Journal: journal, Authority: authority}
    if _, _, err := gate.FingerprintFloor("source", "audience", "share"); !errors.Is(err, ErrFloorUnavailable) {
        t.Fatalf("observed journal drift must fail closed: %v", err)
    }
    if journal.calls != 2 || authority.calls != 2 { t.Fatalf("expected two snapshots each, got %d and %d", journal.calls, authority.calls) }
}

func TestAnchoredReadFloorRejectsObservedAuthorityDrift(t *testing.T) {
    old, next := [32]byte{1}, [32]byte{2}
    journal := &scriptedFloor{revisions: []uint64{1, 1}, fingerprints: [][32]byte{old, old}}
    authority := &scriptedFloor{revisions: []uint64{1, 1}, fingerprints: [][32]byte{old, next}}
    gate := &AnchoredReadFloor{Journal: journal, Authority: authority}
    if _, _, err := gate.FingerprintFloor("source", "audience", "share"); !errors.Is(err, ErrFloorUnavailable) {
        t.Fatalf("same-revision authority mutation must fail closed: %v", err)
    }
}

func TestAnchoredReadFloorRejectsSecondReadFailure(t *testing.T) {
    fp := [32]byte{1}
    journal := &scriptedFloor{revisions: []uint64{1}, fingerprints: [][32]byte{fp}, failAt: 2}
    authority := &scriptedFloor{revisions: []uint64{1}, fingerprints: [][32]byte{fp}}
    if _, _, err := (&AnchoredReadFloor{Journal: journal, Authority: authority}).FingerprintFloor("s", "a", "share"); !errors.Is(err, ErrFloorUnavailable) {
        t.Fatalf("second read error must fail closed: %v", err)
    }
}

func TestAnchoredReadFloorAcceptsStableSnapshots(t *testing.T) {
    fp := [32]byte{3}
    journal := &scriptedFloor{revisions: []uint64{3}, fingerprints: [][32]byte{fp}}
    authority := &scriptedFloor{revisions: []uint64{3}, fingerprints: [][32]byte{fp}}
    rev, got, err := (&AnchoredReadFloor{Journal: journal, Authority: authority}).FingerprintFloor("s", "a", "share")
    if err != nil || rev != 3 || got != fp { t.Fatalf("stable agreement: rev=%d fp=%v err=%v", rev, got, err) }
}
