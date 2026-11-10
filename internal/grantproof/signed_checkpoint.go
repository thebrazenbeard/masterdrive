package grantproof

import (
 "crypto/ed25519"
 "encoding/binary"
 "time"
)

// SignedCheckpoint is source-signed evidence of a relationship's current
// revocation state. A separately durable monotonic revision floor and trusted
// source key are mandatory; neither is established by this structure.
type SignedCheckpoint struct {
 Authority CurrentAuthority
 Revision uint64
 IssuedUnix int64
 ExpiresUnix int64
}

const checkpointDomain = "masterdrive/grantproof/checkpoint/v1\x00"

// CheckpointMessage encodes all authorization fields, including revocation,
// in a domain-separated and unambiguous signed message.
func CheckpointMessage(c SignedCheckpoint) ([]byte,error) {
 a:=c.Authority
 if a.SourceID=="" || a.ShareID=="" || a.AudienceID=="" ||
  a.KeyVersion==0 || a.GrantEpoch==0 || c.Revision==0 ||
  c.IssuedUnix<0 || c.ExpiresUnix<=c.IssuedUnix {return nil,ErrInvalid}
 out:=[]byte(checkpointDomain)
 for _,s:=range []string{a.SourceID,a.ShareID,a.AudienceID} {
  if len(s)>4096 {return nil,ErrInvalid}
  var b [4]byte
  binary.BigEndian.PutUint32(b[:],uint32(len(s)))
  out=append(out,b[:]...)
  out=append(out,s...)
 }
 var b [8]byte
 for _,v:=range []uint64{a.KeyVersion,a.GrantEpoch,c.Revision,uint64(c.IssuedUnix),uint64(c.ExpiresUnix)} {
  binary.BigEndian.PutUint64(b[:],v)
  out=append(out,b[:]...)
 }
 if a.Revoked {out=append(out,1)} else {out=append(out,0)}
 return out,nil
}

// VerifySignedCurrent checks a signed checkpoint and grant together. minRevision
// MUST be read from independently protected durable state, not the peer. A
// positive maxAge limits accepted checkpoint age; no offline grace is granted.
// Caller must atomically persist the accepted revision before permitting reads
// and recheck authorization for subsequent reads.
func VerifySignedCurrent(g Grant, grantSig []byte, trusted TrustedSource, c SignedCheckpoint, checkpointSig []byte, minRevision uint64, maxAge time.Duration, snapshot string, now time.Time) error {
 if minRevision==0 || maxAge<=0 || c.Revision<minRevision ||
  trusted.SourceID=="" || trusted.SourceID!=c.Authority.SourceID ||
  trusted.KeyVersion!=c.Authority.KeyVersion ||
  len(trusted.PublicKey)!=ed25519.PublicKeySize {return ErrInvalid}
 if now.Unix()<c.IssuedUnix || now.Unix()>=c.ExpiresUnix {return ErrInvalid}
 age:=now.Sub(time.Unix(c.IssuedUnix,0))
 if age<0 || age>maxAge {return ErrInvalid}
 msg,err:=CheckpointMessage(c)
 if err!=nil || !ed25519.Verify(trusted.PublicKey,msg,checkpointSig) {return ErrInvalid}
 return VerifyCurrent(g,grantSig,trusted,c.Authority,snapshot,now)
}
