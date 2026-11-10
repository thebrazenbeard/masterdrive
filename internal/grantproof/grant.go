// Package grantproof verifies a source-signed, read-only grant envelope.
// Verification does not establish revocation freshness, key provenance, or mount safety.
package grantproof

import (
 "crypto/ed25519"
 "encoding/binary"
 "errors"
 "time"
)

const domain = "masterdrive/grantproof/ed25519/v1\x00"

var ErrInvalid = errors.New("grantproof: invalid or unauthorized grant")

type Grant struct {
 SourceID string
 AudienceID string
 ShareID string
 SnapshotID string
 KeyVersion uint64
 GrantEpoch uint64
 NotBeforeUnix int64
 ExpiresUnix int64
 ReadOnly bool
}

type TrustedSource struct {
 SourceID string
 KeyVersion uint64
 PublicKey ed25519.PublicKey
}

// Encode produces a deterministic, length-delimited, domain-separated message.
func Encode(g Grant) ([]byte, error) {
 if g.SourceID == "" || g.AudienceID == "" || g.ShareID == "" || g.SnapshotID == "" || !g.ReadOnly || g.KeyVersion == 0 || g.GrantEpoch == 0 || g.ExpiresUnix <= g.NotBeforeUnix { return nil, ErrInvalid }
 out := []byte(domain)
 for _, s := range []string{g.SourceID,g.AudienceID,g.ShareID,g.SnapshotID} {
  if len(s) > 4096 { return nil, ErrInvalid }
  var b [4]byte; binary.BigEndian.PutUint32(b[:], uint32(len(s))); out=append(out,b[:]...); out=append(out,s...)
 }
 var b [8]byte
 for _, n := range []uint64{g.KeyVersion,g.GrantEpoch,uint64(g.NotBeforeUnix),uint64(g.ExpiresUnix)} {binary.BigEndian.PutUint64(b[:],n);out=append(out,b[:]...)}
 out=append(out,1)
 return out,nil
}

// Verify requires an independently pinned source key and caller-supplied trusted time.
// It does not consult a revocation ledger or determine whether the grant is the latest epoch.
func Verify(g Grant, signature []byte, trusted TrustedSource, audience, share, snapshot string, now time.Time) error {
 if trusted.SourceID == "" || trusted.SourceID != g.SourceID || trusted.KeyVersion != g.KeyVersion || len(trusted.PublicKey) != ed25519.PublicKeySize || audience == "" || share == "" || snapshot == "" || g.AudienceID != audience || g.ShareID != share || g.SnapshotID != snapshot {return ErrInvalid}
 if now.Unix() < g.NotBeforeUnix || now.Unix() >= g.ExpiresUnix {return ErrInvalid}
 msg,err:=Encode(g);if err!=nil{return err}
 if !ed25519.Verify(trusted.PublicKey,msg,signature){return ErrInvalid}
 return nil
}
