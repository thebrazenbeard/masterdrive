package checkpointgate

import (
    "bytes"
    "errors"
    "testing"
)

// FuzzDigestRevocationsCanonicality exercises arbitrary binary identities,
// ordering invariance, input immutability, and tombstone deletion at a fixed
// revision. It is a serialization test, not proof of trusted persistence.
func FuzzDigestRevocationsCanonicality(f *testing.F) {
    f.Add([]byte("alpha"), []byte("beta"), uint64(1), uint64(2))
    f.Add([]byte{0, ':', '/', 255}, []byte{0, ':', '/', 254}, uint64(1), uint64(256))
    f.Add([]byte("a"), []byte("a"), uint64(1), uint64(2))
    f.Fuzz(func(t *testing.T, first, second []byte, firstEpoch, secondEpoch uint64) {
        // Bound fuzz allocations and exclude invalid ledger inputs from the
        // positive canonicality property.
        if len(first) == 0 || len(second) == 0 || len(first) > 1024 ||
            len(second) > 1024 || firstEpoch == 0 || secondEpoch == 0 {
            return
        }
        entries := []Revocation{{ID: string(first), Epoch: firstEpoch}, {ID: string(second), Epoch: secondEpoch}}
        if bytes.Equal(first, second) {
            _, err := DigestRevocations("source", 7, entries)
            if !errors.Is(err, ErrInvalidLedger) {
                t.Fatalf("duplicate identity accepted: %v", err)
            }
            return
        }
        before := append([]Revocation(nil), entries...)
        digest, err := DigestRevocations("source", 7, entries)
        if err != nil {
            t.Fatal(err)
        }
        if entries[0] != before[0] || entries[1] != before[1] {
            t.Fatal("caller input mutated")
        }
        reversed := []Revocation{entries[1], entries[0]}
        reordered, err := DigestRevocations("source", 7, reversed)
        if err != nil || digest != reordered {
            t.Fatalf("ordering changed digest: %v", err)
        }
        withoutOne, err := DigestRevocations("source", 7, entries[:1])
        if err != nil || digest == withoutOne {
            t.Fatalf("tombstone deletion undetected: %v", err)
        }
        changedRevision, err := DigestRevocations("source", 8, entries)
        if err != nil || digest == changedRevision {
            t.Fatalf("revision change undetected: %v", err)
        }
    })
}
