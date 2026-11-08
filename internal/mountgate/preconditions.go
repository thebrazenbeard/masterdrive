// Package mountgate defines a conservative, pure preflight for a future
// read-only mount adapter. It neither authenticates peers nor opens files.
// Inputs are evidence supplied by a trusted caller, NOT self-attesting claims.
package mountgate

import (
    "crypto/subtle"
    "encoding/hex"
    "errors"
    "net/url"
    "strings"
)

var ErrUnqualified = errors.New("mountgate: read-only source not qualified")

// Evidence must be populated from independently verified transport, grant,
// and immutable source checks. Never populate these fields from peer metadata
// alone. A successful Check does not pin an OS handle or prevent later change.
type Evidence struct {
    Origin string
    TransportAuthenticated bool
    DevicePaired bool
    GrantCurrent bool
    ReadOnly bool
    ImmutableSnapshotVerified bool
    StrongETag string
    SHA256 string
    Length int64
}

// Pin identifies the exact representation the future mount intends to read.
// Revalidate after opening and on every ranged read; this is only preflight.
type Pin struct {
    StrongETag string
    SHA256 string
    Length int64
}

func validETag(v string) bool {
    if len(v) < 3 || v[0] != '"' || v[len(v)-1] != '"' { return false }
    for _, c := range v[1:len(v)-1] {
        if c < 0x21 || c > 0x7e || c == '"' || c == '\\' { return false }
    }
    return true
}

func validDigest(s string) bool {
    if len(s) != 64 { return false }
    b, err := hex.DecodeString(s)
    return err == nil && len(b) == 32 && strings.ToLower(s) == s
}

// Check rejects uncertain authority, mutable representations, insecure
// transport, and any version mismatch. It does not enable a mount.
func Check(e Evidence, p Pin) error {
    u, err := url.Parse(e.Origin)
    if err != nil || u.Scheme != "https" || u.Hostname() == "" ||
        u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
        u.Path != "" || u.Opaque != "" {
        return ErrUnqualified
    }
    if !e.TransportAuthenticated || !e.DevicePaired || !e.GrantCurrent ||
        !e.ReadOnly || !e.ImmutableSnapshotVerified {
        return ErrUnqualified
    }
    if e.Length < 0 || p.Length < 0 || e.Length != p.Length ||
        !validETag(e.StrongETag) || !validETag(p.StrongETag) ||
        !validDigest(e.SHA256) || !validDigest(p.SHA256) {
        return ErrUnqualified
    }
    if subtle.ConstantTimeCompare([]byte(e.StrongETag), []byte(p.StrongETag)) != 1 ||
        subtle.ConstantTimeCompare([]byte(e.SHA256), []byte(p.SHA256)) != 1 {
        return ErrUnqualified
    }
    return nil
}
