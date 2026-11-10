package grantproof

import (
 "crypto/ed25519"
 "crypto/sha256"
 "errors"
 "testing"
 "time"
)

type fakeFloor struct { floor uint64; readErr, commitErr error; reads, commits int; last uint64 }
func (f *fakeFloor) Floor(_,_,_ string)(uint64,error){ f.reads++;return f.floor,f.readErr }
func (f *fakeFloor) CommitIfNotSuperseded(_,_,_ string,v uint64)error {
 f.commits++;f.last=v
 if f.commitErr!=nil{return f.commitErr}
 if f.floor>v{return ErrFloorUnavailable}
 if v>f.floor {f.floor=v}
 return nil
}

func TestVerifyAndCommitCheckpoint(t *testing.T) {
 seed:=sha256.Sum256([]byte("synthetic-checkpoint-commit-only"))
 key:=ed25519.NewKeyFromSeed(seed[:])
 trusted:=TrustedSource{SourceID:"source",KeyVersion:3,PublicKey:key.Public().(ed25519.PublicKey)}
 grant:=Grant{SourceID:"source",AudienceID:"reader",ShareID:"share",SnapshotID:"snap",KeyVersion:3,GrantEpoch:7,NotBeforeUnix:100,ExpiresUnix:500,ReadOnly:true}
 gb,err:=Encode(grant);if err!=nil {t.Fatal(err)}
 gs:=ed25519.Sign(key,gb)
 cp:=SignedCheckpoint{Authority:CurrentAuthority{SourceID:"source",AudienceID:"reader",ShareID:"share",KeyVersion:3,GrantEpoch:7},Revision:12,IssuedUnix:140,ExpiresUnix:200}
 cb,err:=CheckpointMessage(cp);if err!=nil {t.Fatal(err)}
 cs:=ed25519.Sign(key,cb)
 run:=func(f CheckpointFloorStore,c SignedCheckpoint,sig []byte)error{
  return VerifyAndCommitCheckpoint(f,grant,gs,trusted,c,sig,20*time.Second,"snap",time.Unix(150,0))
 }
 t.Run("commit before admission",func(t *testing.T){
  f:=&fakeFloor{floor:11}
  if err:=run(f,cp,cs);err!=nil{t.Fatal(err)}
  if f.commits!=1 || f.last!=12 || f.floor!=12 {t.Fatalf("commit not durable: %+v",f)}
 })
 t.Run("no store",func(t *testing.T){if !errors.Is(run(nil,cp,cs),ErrFloorUnavailable){t.Fatal("accepted absent store")}})
 t.Run("unprovisioned floor",func(t *testing.T){
  f:=&fakeFloor{}
  if !errors.Is(run(f,cp,cs),ErrFloorUnavailable)||f.commits!=0{t.Fatal("accepted unprovisioned floor")}
 })
 t.Run("floor read error",func(t *testing.T){
  f:=&fakeFloor{floor:11,readErr:errors.New("read failed")}
  if !errors.Is(run(f,cp,cs),ErrFloorUnavailable)||f.commits!=0{t.Fatal("accepted failed read")}
 })
 t.Run("concurrent floor advancement",func(t *testing.T){
  f:=&fakeFloor{floor:11}
  // Simulate a second writer advancing the durable floor after Floor().
  // A real implementation must enforce this comparison atomically.
  f.floor=13
  if err:=f.CommitIfNotSuperseded("source","reader","share",12);err==nil{t.Fatal("accepted stale concurrent commit")}
 })
 t.Run("rollback",func(t *testing.T){
  f:=&fakeFloor{floor:13}
  if run(f,cp,cs)==nil||f.commits!=0{t.Fatal("accepted rollback")}
 })
 t.Run("signature failure",func(t *testing.T){
  f:=&fakeFloor{floor:11}
  bad:=append([]byte(nil),cs...);bad[0]^=1
  if run(f,cp,bad)==nil||f.commits!=0{t.Fatal("committed forged checkpoint")}
 })
 t.Run("revocation",func(t *testing.T){
  f:=&fakeFloor{floor:11};revoked:=cp;revoked.Authority.Revoked=true
  b,e:=CheckpointMessage(revoked);if e!=nil{t.Fatal(e)}
  if run(f,revoked,ed25519.Sign(key,b))==nil||f.commits!=0{t.Fatal("committed revoked grant")}
 })
 t.Run("commit failure denies",func(t *testing.T){
  f:=&fakeFloor{floor:11,commitErr:errors.New("disk full")}
  if !errors.Is(run(f,cp,cs),ErrFloorUnavailable)||f.commits!=1||f.floor!=11{t.Fatal("accepted failed durable commit")}
 })
}
