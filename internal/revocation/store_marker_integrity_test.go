package revocation

import (
    "os"
    "path/filepath"
    "strings"
    "testing"
)

// Marker validation is a fail-closed integrity check, not an authenticated
// rollback barrier: restoring a complete old directory remains possible.
func TestMalformedIdentityMarkerRejectedOnOpen(t *testing.T) {
    cases := []struct{name, contents string}{
        {"empty", ""},
        {"truncated", strings.Repeat("a", 63) + "\n"},
        {"nonhex", strings.Repeat("g", 64) + "\n"},
        {"missing-newline", strings.Repeat("a", 64)},
        {"extra-data", strings.Repeat("a", 64) + "\nextra"},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            dir := t.TempDir()
            if err := os.WriteFile(filepath.Join(dir, ".store-identity"), []byte(tc.contents), 0600); err != nil { t.Fatal(err) }
            if _, err := Open(dir); err == nil { t.Fatal("accepted malformed identity marker") }
        })
    }
}

func TestIdentityMarkerMutationDeniesExistingStore(t *testing.T) {
    for _, mutation := range []string{"remove", "replace"} {
        t.Run(mutation, func(t *testing.T) {
            dir := t.TempDir()
            store, err := Open(dir)
            if err != nil { t.Fatal(err) }
            marker := filepath.Join(dir, ".store-identity")
            if err := os.Remove(marker); err != nil { t.Fatal(err) }
            if mutation == "replace" {
                if err := os.WriteFile(marker, []byte(strings.Repeat("b", 64)+"\n"), 0600); err != nil { t.Fatal(err) }
            }
            token := strings.Repeat("m", 48)
            if err := store.Revoke(token); err == nil { t.Fatal("mutation accepted for revoke") }
            denied, err := store.Check(token)
            if !denied || err == nil { t.Fatalf("mutation did not fail closed: denied=%v err=%v", denied, err) }
        })
    }
}
