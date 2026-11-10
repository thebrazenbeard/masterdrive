package grantproof

import (
 "crypto/ed25519"
 "crypto/sha256"
 "crypto/subtle"
 "errors"
 "sync"
 "testing"
 "time"
)

// A deterministic in-memory fake, NOT a durable store.
type fingerprintFloorFake struct {
 mu sync.Mutex
 revision uint64
 digest [32]byte
 readErr, commitErr error
 beforeCommit func()
 commits int
}
func (f *fingerprintFloorFake) FingerprintFloor(_,_,_ string)(uint64,[32]byte,error) {
 f.mu.Lock(); defer f.mu.Unlock()
 return f.revision,f.digest,f.readErr
}
func (f *fingerprintFloorFake) CommitFingerprintIfCurrent(_,_,_ string,r uint64,d [32]byte)error {
 if f.beforeCommit!=nil { f.beforeCommit() }
 f.mu.Lock();defer f.mu.Unlock()
 f.commits++
 if f.commitErr!=nil{return f.commitErr}
 if f.revision>r{return ErrFloorUnavailable}
 if f.revision==r && subtle.ConstantTimeCompare(f.digest[:],d[:])!=1{return ErrCheckpointEquivocation}
 f.revision=r;f.digest=d
 return nil
}

func TestVerifyAndCommitCheckpointFingerprint(t *testing.T) {
 seed:=sha256.Sum256([]byte("fingerprint-floor-synthetic-only"))
 key:=ed25519.NewKeyFromSeed(seed[:])
 trusted:=TrustedSource{SourceID:"source",KeyVersion:3,PublicKey:key.Public().(ed25519.PublicKey)}
 grant:=Grant{SourceID:"source",AudienceID:"reader",ShareID:"share",SnapshotID:"snap",KeyVersion:3,GrantEpoch:7,NotBeforeUnix:100,ExpiresUnix:500,ReadOnly:true}
 gb,err:=Encode(grant);if err!=nil{t.Fatal(err)}
 gs:=ed25519.Sign(key,gb)
 cp:=SignedCheckpoint{Authority:CurrentAuthority{SourceID:"source",AudienceID:"reader",ShareID:"share",KeyVersion:3,GrantEpoch:7},Revision:12,IssuedUnix:140,ExpiresUnix:200}
 sign:=func(c SignedCheckpoint)[]byte{b,e:=CheckpointMessage(c);if e!=nil{t.Fatal(e)};return ed25519.Sign(key,b)}
 digest,err:=CheckpointFingerprint(cp);if err!=nil{t.Fatal(err)}
 run:=func(f CheckpointFingerprintFloorStore,c SignedCheckpoint,sig []byte)error{
  return VerifyAndCommitCheckpointFingerprint(f,grant,gs,trusted,c,sig,20*time.Second,"snap",time.Unix(150,0))
 }
 t.Run("new revision commits",func(t *testing.T){
  f:=&fingerprintFloorFake{revision:11}
  if err:=run(f,cp,sign(cp));err!=nil{t.Fatal(err)}
  if f.revision!=12||f.digest!=digest||f.commits!=1{t.Fatal("revision and fingerprint not committed together")}
 })
 t.Run("equal revision same message",func(t *testing.T){
  f:=&fingerprintFloorFake{revision:12,digest:digest}
  if err:=run(f,cp,sign(cp));err!=nil{t.Fatal(err)}
 })
 t.Run("equal revision fork rejected before commit",func(t *testing.T){
  f:=&fingerprintFloorFake{revision:12,digest:sha256.Sum256([]byte("other signed checkpoint"))}
  if !errors.Is(run(f,cp,sign(cp)),ErrCheckpointEquivocation)||f.commits!=0{t.Fatal("accepted fork")}
 })
 t.Run("concurrent equal revision fork rejected at commit",func(t *testing.T){
  f:=&fingerprintFloorFake{revision:11}
  f.beforeCommit=func(){f.mu.Lock();f.revision=12;f.digest=sha256.Sum256([]byte("concurrent conflicting message"));f.mu.Unlock()}
  if !errors.Is(run(f,cp,sign(cp)),ErrCheckpointEquivocation){t.Fatal("accepted concurrent fork")}
 })
 t.Run("concurrent newer revision rejected at commit",func(t *testing.T){
  f:=&fingerprintFloorFake{revision:11}
  f.beforeCommit=func(){f.mu.Lock();f.revision=13;f.mu.Unlock()}
  if !errors.Is(run(f,cp,sign(cp)),ErrFloorUnavailable){t.Fatal("accepted superseded checkpoint")}
 })
 t.Run("invalid signature never commits",func(t *testing.T){
  f:=&fingerprintFloorFake{revision:11}
  sig:=sign(cp);sig[0]^=1
  if run(f,cp,sig)==nil||f.commits!=0{t.Fatal("accepted forged evidence")}
 })
 t.Run("commit error denies",func(t *testing.T){
  f:=&fingerprintFloorFake{revision:11,commitErr:errors.New("disk failure")}
  if !errors.Is(run(f,cp,sign(cp)),ErrFloorUnavailable){t.Fatal("accepted failed commit")}
 })
 t.Run("missing or unprovisioned store denies",func(t *testing.T){
  if !errors.Is(run(nil,cp,sign(cp)),ErrFloorUnavailable){t.Fatal("accepted missing store")}
  if !errors.Is(run(&fingerprintFloorFake{},cp,sign(cp)),ErrFloorUnavailable){t.Fatal("accepted unprovisioned store")}
 })
 t.Run("read failure denies",func(t *testing.T){
  f:=&fingerprintFloorFake{revision:11,readErr:errors.New("read failure")}
  if !errors.Is(run(f,cp,sign(cp)),ErrFloorUnavailable)||f.commits!=0{t.Fatal("accepted failed floor read")}
 })
 t.Run("older revision denies",func(t *testing.T){
  f:=&fingerprintFloorFake{revision:13}
  if run(f,cp,sign(cp))==nil||f.commits!=0{t.Fatal("accepted rollback")}
 })
}
