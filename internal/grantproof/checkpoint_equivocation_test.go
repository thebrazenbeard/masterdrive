package grantproof

import (
 "errors"
 "testing"
)

func TestSameRevisionCheckpointEquivocation(t *testing.T) {
 original := SignedCheckpoint{
  Authority: CurrentAuthority{SourceID:"source", AudienceID:"reader", ShareID:"share", KeyVersion:3, GrantEpoch:7},
  Revision:12, IssuedUnix:140, ExpiresUnix:200,
 }
 if err:=RejectSameRevisionFork(original,original);err!=nil {t.Fatalf("identical checkpoint rejected: %v",err)}
 cases:=[]struct{name string; mutate func(*SignedCheckpoint); want error}{
  {"revocation changed",func(c *SignedCheckpoint){c.Authority.Revoked=true},ErrCheckpointEquivocation},
  {"epoch changed",func(c *SignedCheckpoint){c.Authority.GrantEpoch++},ErrCheckpointEquivocation},
  {"key rotated",func(c *SignedCheckpoint){c.Authority.KeyVersion++},ErrCheckpointEquivocation},
  {"issued changed",func(c *SignedCheckpoint){c.IssuedUnix++},ErrCheckpointEquivocation},
  {"expiry changed",func(c *SignedCheckpoint){c.ExpiresUnix++},ErrCheckpointEquivocation},
  {"different revision",func(c *SignedCheckpoint){c.Revision++},ErrInvalid},
  {"different audience",func(c *SignedCheckpoint){c.Authority.AudienceID="other"},ErrInvalid},
  {"different source",func(c *SignedCheckpoint){c.Authority.SourceID="other"},ErrInvalid},
  {"different share",func(c *SignedCheckpoint){c.Authority.ShareID="other"},ErrInvalid},
  {"invalid expiry",func(c *SignedCheckpoint){c.ExpiresUnix=c.IssuedUnix},ErrInvalid},
 }
 for _,tc:=range cases {t.Run(tc.name,func(t *testing.T){
  candidate:=original;tc.mutate(&candidate)
  if err:=RejectSameRevisionFork(original,candidate);!errors.Is(err,tc.want){t.Fatalf("got %v, want %v",err,tc.want)}
 })}
}

func TestCheckpointFingerprintDeterministicAndDomainSeparated(t *testing.T) {
 c:=SignedCheckpoint{Authority:CurrentAuthority{SourceID:"a",AudienceID:"b",ShareID:"c",KeyVersion:1,GrantEpoch:1},Revision:1,IssuedUnix:1,ExpiresUnix:2}
 a,err:=CheckpointFingerprint(c);if err!=nil{t.Fatal(err)}
 b,err:=CheckpointFingerprint(c);if err!=nil{t.Fatal(err)}
 if a!=b {t.Fatal("nondeterministic fingerprint")}
 c.Authority.Revoked=true
 changed,err:=CheckpointFingerprint(c);if err!=nil{t.Fatal(err)}
 if changed==a {t.Fatal("revocation did not change fingerprint")}
}
