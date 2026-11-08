package revocation

import (
    "os"
    "path/filepath"
    "strings"
    "testing"
)

// Missing identity on a populated ledger must not silently establish a new
// identity, even if the remaining tombstones appear structurally valid.
func TestOpenRejectsMissingMarkerInPopulatedLedger(t *testing.T) {
    dir := t.TempDir()
    store, err := Open(dir)
    if err != nil { t.Fatal(err) }
    token := strings.Repeat("t", 48)
    if err := store.Revoke(token); err != nil { t.Fatal(err) }
    marker := filepath.Join(dir, ".store-identity")
    if err := os.Remove(marker); err != nil { t.Fatal(err) }
    if _, err := Open(dir); err == nil {
        t.Fatal("Open reinitialized populated ledger after marker removal")
    }
    if _, err := os.Stat(marker); !os.IsNotExist(err) {
        t.Fatalf("rejected Open must not create replacement marker: %v", err)
    }
}

func TestOpenRejectsUnmarkedPopulatedDirectory(t *testing.T) {
    dir := t.TempDir()
    if err := os.WriteFile(filepath.Join(dir, "unrelated"), []byte("data"), 0600); err != nil { t.Fatal(err) }
    if _, err := Open(dir); err == nil { t.Fatal("Open accepted nonempty unmarked directory") }
}

func TestOpenBootstrapsEmptyOwnerProvisionedDirectory(t *testing.T) {
    dir := t.TempDir()
    if _, err := Open(dir); err != nil { t.Fatal(err) }
    if _, err := Open(dir); err != nil { t.Fatalf("reopen: %v", err) }
}
