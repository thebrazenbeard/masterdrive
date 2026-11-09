package checkpointgate

import (
    "bytes"
    "errors"
    "strings"
    "testing"
)

// A canonical digest must not confuse a delimiter inside an identity with
// the boundary between two identities or with a revision/epoch field.
func TestDigestRevocationsLengthPrefixAndEpochBoundaries(t *testing.T) {
    cases := []struct {
        name string
        left []Revocation
        right []Revocation
    }{
        {"embedded separator", []Revocation{{ID: "a:b", Epoch: 1}}, []Revocation{{ID: "a", Epoch: 1}, {ID: "b", Epoch: 1}}},
        {"embedded NUL", []Revocation{{ID: "a\x00b", Epoch: 1}}, []Revocation{{ID: "a", Epoch: 1}, {ID: "b", Epoch: 1}}},
        {"epoch boundary", []Revocation{{ID: "a", Epoch: 1}}, []Revocation{{ID: "a", Epoch: 256}}},
        {"identity boundary", []Revocation{{ID: "ab", Epoch: 1}}, []Revocation{{ID: "a", Epoch: 1}}},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            left, err := DigestRevocations("source", 7, tc.left)
            if err != nil { t.Fatal(err) }
            right, err := DigestRevocations("source", 7, tc.right)
            if err != nil { t.Fatal(err) }
            if bytes.Equal(left[:], right[:]) { t.Fatal("distinct ledgers share digest") }
        })
    }
}

func TestDigestRevocationsLengthLimits(t *testing.T) {
    for _, tc := range []struct {
        name string
        store string
        entries []Revocation
    }{
        {"oversize store", strings.Repeat("s", 65536), nil},
        {"oversize tombstone ID", "source", []Revocation{{ID: strings.Repeat("i", 65536), Epoch: 1}}},
    } {
        t.Run(tc.name, func(t *testing.T) {
            digest, err := DigestRevocations(tc.store, 1, tc.entries)
            if !errors.Is(err, ErrInvalidLedger) { t.Fatalf("expected invalid ledger, got %v", err) }
            if digest != ([32]byte{}) { t.Fatal("invalid input returned nonzero digest") }
        })
    }
}

func TestVerifyExactDetectsTombstoneDeletionAtSameRevision(t *testing.T) {
    entries := []Revocation{{ID: "grant-a", Epoch: 1}, {ID: "grant-b", Epoch: 2}}
    original, err := DigestRevocations("source", 10, entries)
    if err != nil { t.Fatal(err) }
    deleted, err := DigestRevocations("source", 10, entries[:1])
    if err != nil { t.Fatal(err) }
    trusted := Checkpoint{StoreID: "source", Revision: 10, Digest: original[:]}
    observed := Checkpoint{StoreID: "source", Revision: 10, Digest: deleted[:]}
    if !errors.Is(VerifyExact(trusted, observed), ErrUntrustedCheckpoint) {
        t.Fatal("tombstone deletion with unchanged revision was accepted")
    }
    // This positive control only proves byte comparison, not provenance.
    if err := VerifyExact(trusted, trusted); err != nil { t.Fatal(err) }
}
