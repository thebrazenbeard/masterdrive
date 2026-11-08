package revocation

import (
    "os"
    "testing"
    "strings"
)

// TestDeletedTombstoneResurrectsCredential is a deliberately passing
// counterexample, NOT a security acceptance test. A store with no trusted
// external checkpoint cannot distinguish a never-revoked token from a
// previously revoked token whose tombstone was deleted.
func TestDeletedTombstoneResurrectsCredential(t *testing.T) {
    dir := t.TempDir()
    store, err := Open(dir)
    if err != nil { t.Fatal(err) }
    token := strings.Repeat("d", 48)
    if err := store.Revoke(token); err != nil { t.Fatal(err) }
    if revoked, err := store.Check(token); err != nil || !revoked {
        t.Fatalf("expected revoked before deletion: revoked=%v err=%v", revoked, err)
    }
    name, err := store.filename(token)
    if err != nil { t.Fatal(err) }
    if err := os.Remove(name); err != nil { t.Fatal(err) }
    // Both an already-open handle and a reopened store are vulnerable.
    if revoked, err := store.Check(token); err != nil || revoked {
        t.Fatalf("counterexample changed for live store: revoked=%v err=%v", revoked, err)
    }
    reopened, err := Open(dir)
    if err != nil { t.Fatal(err) }
    if revoked, err := reopened.Check(token); err != nil || revoked {
        t.Fatalf("counterexample changed after reopen: revoked=%v err=%v", revoked, err)
    }
}
