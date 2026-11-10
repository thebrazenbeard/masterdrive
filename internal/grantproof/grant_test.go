package grantproof

import (
 "crypto/ed25519"
 "crypto/sha256"
 "testing"
 "time"
)

func TestSignedGrant(t *testing.T) {
 seed:=sha256.Sum256([]byte("synthetic-test-key-only"))
 private:=ed25519.NewKeyFromSeed(seed[:]);pub:=private.Public().(ed25519.PublicKey)
 g:=Grant{SourceID:"node-a",AudienceID:"node-b",ShareID:"share",SnapshotID:"snapshot",KeyVersion:2,GrantEpoch:3,NotBeforeUnix:100,ExpiresUnix:200,ReadOnly:true}
 msg,err:=Encode(g);if err!=nil{t.Fatal(err)}
 sig:=ed25519.Sign(private,msg)
 trust:=TrustedSource{SourceID:"node-a",KeyVersion:2,PublicKey:pub}
 verify:=func(g Grant,sig []byte,tr TrustedSource,a,s,ss string,at int64)error{return Verify(g,sig,tr,a,s,ss,time.Unix(at,0))}
 if err:=verify(g,sig,trust,"node-b","share","snapshot",100);err!=nil{t.Fatal(err)}
 if err:=verify(g,sig,trust,"node-b","share","snapshot",199);err!=nil{t.Fatal(err)}
 cases:=[]struct{name string; mutate func(*Grant);at int64}{
 {"wrong audience",func(g *Grant){g.AudienceID="attacker"},150},
 {"wrong share",func(g *Grant){g.ShareID="other"},150},
 {"wrong snapshot",func(g *Grant){g.SnapshotID="other"},150},
 {"rotated key",func(g *Grant){g.KeyVersion++},150},
 {"epoch change",func(g *Grant){g.GrantEpoch++},150},
 {"write access",func(g *Grant){g.ReadOnly=false},150},
 {"expired",func(g *Grant){},200},
 {"not yet valid",func(g *Grant){},99},
 {"empty identity",func(g *Grant){g.SourceID=""},150},
 {"invalid validity",func(g *Grant){g.ExpiresUnix=g.NotBeforeUnix},150},
 }
 for _,tc:=range cases{t.Run(tc.name,func(t *testing.T){changed:=g;tc.mutate(&changed);if verify(changed,sig,trust,"node-b","share","snapshot",tc.at)==nil{t.Fatal("accepted invalid grant")}})}
 t.Run("wrong trusted key",func(t *testing.T){other:=sha256.Sum256([]byte("other key"));tr:=trust;tr.PublicKey=ed25519.NewKeyFromSeed(other[:]).Public().(ed25519.PublicKey);if verify(g,sig,tr,"node-b","share","snapshot",150)==nil{t.Fatal("accepted")}})
 t.Run("tampered signature",func(t *testing.T){bad:=append([]byte(nil),sig...);bad[0]^=1;if verify(g,bad,trust,"node-b","share","snapshot",150)==nil{t.Fatal("accepted")}})
 t.Run("wrong expected audience",func(t *testing.T){if verify(g,sig,trust,"other","share","snapshot",150)==nil{t.Fatal("accepted")}})
 t.Run("field ambiguity",func(t *testing.T){a:=g;a.SourceID="node";a.AudienceID="-anode-b";x,_:=Encode(a);if string(x)==string(msg){t.Fatal("ambiguous encoding")}})
}
