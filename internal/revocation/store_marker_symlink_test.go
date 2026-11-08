package revocation

import (
    "os"
    "path/filepath"
    "strings"
    "testing"
)

// A symlink to a valid identity must not establish store authority.
// Windows runners without symlink privilege skip only this fixture.
func TestOpenRejectsSymlinkIdentityMarker(t *testing.T) {
    dir := t.TempDir()
    if _, err := Open(dir); err != nil { t.Fatal(err) }
    marker := filepath.Join(dir, ".store-identity")
    target := filepath.Join(t.TempDir(), "copied-identity")
    b, err := os.ReadFile(marker)
    if err != nil { t.Fatal(err) }
    if err := os.WriteFile(target, b, 0600); err != nil { t.Fatal(err) }
    if err := os.Remove(marker); err != nil { t.Fatal(err) }
    if err := os.Symlink(target, marker); err != nil { t.Skipf("symlink fixture unavailable: %v", err) }
    if _, err := Open(dir); err == nil { t.Fatal("Open accepted symlink identity marker") }
}

func TestCheckRejectsIdentityMarkerSwappedToSymlink(t *testing.T) {
    dir := t.TempDir()
    store, err := Open(dir)
    if err != nil { t.Fatal(err) }
    marker := filepath.Join(dir, ".store-identity")
    target := filepath.Join(t.TempDir(), "copied-identity")
    b, err := os.ReadFile(marker)
    if err != nil { t.Fatal(err) }
    if err := os.WriteFile(target, b, 0600); err != nil { t.Fatal(err) }
    if err := os.Remove(marker); err != nil { t.Fatal(err) }
    if err := os.Symlink(target, marker); err != nil { t.Skipf("symlink fixture unavailable: %v", err) }
    revoked, err := store.Check(strings.Repeat("x", 48))
    if !revoked || err == nil { t.Fatalf("Check must fail closed: revoked=%v err=%v", revoked, err) }
}
