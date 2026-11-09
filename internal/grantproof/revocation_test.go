package grantproof

import (
 "crypto/ed25519"
 "crypto/sha256"
 "testing"
 "time"
)

func TestVerifyCurrent(t *testing.T) {
 seed := sha256.Sum256([]byte("synthetic-current-authority-only"))
 private := ed25519.NewKeyFromSeed(seed[:])
 g := Grant{SourceID:"source", AudienceID:"reader", ShareID:"share", SnapshotID:"snap", KeyVersion:2, GrantEpoch:5, NotBeforeUnix:100, ExpiresUnix:200, ReadOnly:true}
 payload,err:=Encode(g); if err!=nil {t.Fatal(err)}
 sig:=ed25519.Sign(private,payload)
 trusted:=TrustedSource{SourceID:"source",KeyVersion:2,PublicKey:private.Public().(ed25519.PublicKey)}
 current:=CurrentAuthority{SourceID:"source",AudienceID:"reader",ShareID:"share",KeyVersion:2,GrantEpoch:5}
 check:=func(c CurrentAuthority,grant Grant,s []byte)error{return VerifyCurrent(grant,s,trusted,c,"snap",time.Unix(150,0))}
 if err:=check(current,g,sig);err!=nil {t.Fatalf("valid grant rejected: %v",err)}
 cases:=[]struct{name string; mutate func(*CurrentAuthority)}{
  {"revoked",func(c *CurrentAuthority){c.Revoked=true}},
  {"epoch advanced",func(c *CurrentAuthority){c.GrantEpoch++}},
  {"epoch rolled back",func(c *CurrentAuthority){c.GrantEpoch--}},
  {"epoch missing",func(c *CurrentAuthority){c.GrantEpoch=0}},
  {"key rotated",func(c *CurrentAuthority){c.KeyVersion++}},
  {"key missing",func(c *CurrentAuthority){c.KeyVersion=0}},
  {"wrong source",func(c *CurrentAuthority){c.SourceID="other"}},
  {"wrong audience",func(c *CurrentAuthority){c.AudienceID="other"}},
  {"wrong share",func(c *CurrentAuthority){c.ShareID="other"}},
  {"missing source",func(c *CurrentAuthority){c.SourceID=""}},
  {"missing audience",func(c *CurrentAuthority){c.AudienceID=""}},
  {"missing share",func(c *CurrentAuthority){c.ShareID=""}},
 }
 for _,tc:=range cases {t.Run(tc.name,func(t *testing.T){c:=current;tc.mutate(&c);if check(c,g,sig)==nil {t.Fatal("accepted stale or inconsistent authority")}})}
 t.Run("bad signature",func(t *testing.T){bad:=append([]byte(nil),sig...);bad[0]^=1;if check(current,g,bad)==nil{t.Fatal("accepted bad signature")}})
 t.Run("tampered grant",func(t *testing.T){changed:=g;changed.SnapshotID="other";if check(current,changed,sig)==nil{t.Fatal("accepted altered grant")}})
 t.Run("expired",func(t *testing.T){if VerifyCurrent(g,sig,trusted,current,"snap",time.Unix(200,0))==nil{t.Fatal("accepted expired grant")}})
}
