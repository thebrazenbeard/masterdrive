package revocation

import (
    "crypto/sha256"
    "encoding/hex"
    "os"
    "path/filepath"
    "strings"
    "testing"
)

// This counterexample deliberately records an unsafe result. It is NOT a
// revocation acceptance test: restoring an earlier complete ledger can
// resurrect a credential. Do not connect Store.Check to agent authorization.
func TestRollbackCounterexampleRevocationResurrection(t *testing.T) {
    dir := t.TempDir()
    token := strings.Repeat("r", 48)
    before, err := Open(dir)
    if err != nil { t.Fatal(err) }
    originalMarker, err := os.ReadFile(filepath.Join(dir, ".store-identity"))
    if err != nil { t.Fatal(err) }
    if err := before.Revoke(token); err != nil { t.Fatal(err) }
    revoked, err := before.Check(token)
    if err != nil || !revoked { t.Fatalf("expected revoked before rollback: %v %v", revoked, err) }

    // Model a rollback to the earlier empty store with the same marker.
    // An attacker controlling the backing directory could restore both.
    digest := sha256.Sum256([]byte(token))
    tombstone := filepath.Join(dir, hex.EncodeToString(digest[:])+".revoked")
    if err := os.Remove(tombstone); err != nil { t.Fatal(err) }
    marker, err := os.ReadFile(filepath.Join(dir, ".store-identity"))
    if err != nil { t.Fatal(err) }
    if string(marker) != string(originalMarker) { t.Fatal("unexpected marker change") }

    after, err := Open(dir)
    if err != nil { t.Fatal(err) }
    revoked, err = after.Check(token)
    if err != nil { t.Fatal(err) }
    if revoked { t.Fatal("counterexample changed: rollback no longer resurrects credential; replace this test with a fail-closed assertion") }
    t.Log("SECURITY GAP: old marker plus missing tombstone is accepted after reopen")
}
