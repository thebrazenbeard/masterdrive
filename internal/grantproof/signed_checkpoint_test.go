package grantproof

import (
 "crypto/ed25519"
 "crypto/sha256"
 "testing"
 "time"
)

func TestVerifySignedCurrent(t *testing.T) {
 seed:=sha256.Sum256([]byte("synthetic-signed-checkpoint-not-production"))
 key:=ed25519.NewKeyFromSeed(seed[:])
 trusted:=TrustedSource{SourceID:"source",KeyVersion:3,PublicKey:key.Public().(ed25519.PublicKey)}
 grant:=Grant{SourceID:"source",AudienceID:"reader",ShareID:"share",SnapshotID:"snap",KeyVersion:3,GrantEpoch:7,NotBeforeUnix:100,ExpiresUnix:500,ReadOnly:true}
 signGrant:=func(g Grant)[]byte {t.Helper();b,e:=Encode(g);if e!=nil {t.Fatal(e)};return ed25519.Sign(key,b)}
 c:=SignedCheckpoint{Authority:CurrentAuthority{SourceID:"source",AudienceID:"reader",ShareID:"share",KeyVersion:3,GrantEpoch:7},Revision:11,IssuedUnix:140,ExpiresUnix:200}
 signCheckpoint:=func(v SignedCheckpoint)[]byte {t.Helper();b,e:=CheckpointMessage(v);if e!=nil {t.Fatal(e)};return ed25519.Sign(key,b)}
 check:=func(v SignedCheckpoint,cs []byte,gs []byte,floor uint64,age time.Duration,now int64)error{
  return VerifySignedCurrent(grant,gs,trusted,v,cs,floor,age,"snap",time.Unix(now,0))
 }
 validSig:=signCheckpoint(c);grantSig:=signGrant(grant)
 if e:=check(c,validSig,grantSig,11,20*time.Second,150);e!=nil {t.Fatalf("valid evidence rejected: %v",e)}
 cases:=[]struct{name string; mutate func(*SignedCheckpoint)}{
  {"revoked",func(v *SignedCheckpoint){v.Authority.Revoked=true}},
  {"epoch advance",func(v *SignedCheckpoint){v.Authority.GrantEpoch++}},
  {"key rotation",func(v *SignedCheckpoint){v.Authority.KeyVersion++}},
  {"wrong source",func(v *SignedCheckpoint){v.Authority.SourceID="other"}},
  {"wrong audience",func(v *SignedCheckpoint){v.Authority.AudienceID="other"}},
  {"wrong share",func(v *SignedCheckpoint){v.Authority.ShareID="other"}},
  {"revision change",func(v *SignedCheckpoint){v.Revision++}},
  {"issued change",func(v *SignedCheckpoint){v.IssuedUnix++}},
  {"expiry change",func(v *SignedCheckpoint){v.ExpiresUnix++}},
 }
 for _,tc:=range cases {t.Run(tc.name+" signature binding",func(t *testing.T){
  v:=c;tc.mutate(&v)
  if check(v,validSig,grantSig,11,20*time.Second,150)==nil {t.Fatal("accepted tampered checkpoint")}
 })}
 t.Run("signed revoked",func(t *testing.T){v:=c;v.Authority.Revoked=true;if check(v,signCheckpoint(v),grantSig,11,20*time.Second,150)==nil {t.Fatal("accepted signed revocation")}})
 t.Run("signed epoch change",func(t *testing.T){v:=c;v.Authority.GrantEpoch++;if check(v,signCheckpoint(v),grantSig,11,20*time.Second,150)==nil {t.Fatal("accepted superseded grant")}})
 t.Run("rollback floor",func(t *testing.T){if check(c,validSig,grantSig,12,20*time.Second,150)==nil {t.Fatal("accepted rollback")}})
 t.Run("missing floor",func(t *testing.T){if check(c,validSig,grantSig,0,20*time.Second,150)==nil {t.Fatal("accepted missing durable floor")}})
 t.Run("missing age limit",func(t *testing.T){if check(c,validSig,grantSig,11,0,150)==nil {t.Fatal("accepted unlimited freshness")}})
 t.Run("stale",func(t *testing.T){if check(c,validSig,grantSig,11,5*time.Second,150)==nil {t.Fatal("accepted stale checkpoint")}})
 t.Run("future issued",func(t *testing.T){if check(c,validSig,grantSig,11,20*time.Second,139)==nil {t.Fatal("accepted future checkpoint")}})
 t.Run("expired",func(t *testing.T){if check(c,validSig,grantSig,11,100*time.Second,200)==nil {t.Fatal("accepted expired checkpoint")}})
 t.Run("bad checkpoint signature",func(t *testing.T){bad:=append([]byte(nil),validSig...);bad[0]^=1;if check(c,bad,grantSig,11,20*time.Second,150)==nil {t.Fatal("accepted forged signature")}})
 t.Run("bad grant signature",func(t *testing.T){bad:=append([]byte(nil),grantSig...);bad[0]^=1;if check(c,validSig,bad,11,20*time.Second,150)==nil {t.Fatal("accepted forged grant")}})
 t.Run("other source key",func(t *testing.T){other:=ed25519.NewKeyFromSeed(make([]byte,ed25519.SeedSize));b,_:=CheckpointMessage(c);if check(c,ed25519.Sign(other,b),grantSig,11,20*time.Second,150)==nil {t.Fatal("accepted untrusted signer")}})
}
