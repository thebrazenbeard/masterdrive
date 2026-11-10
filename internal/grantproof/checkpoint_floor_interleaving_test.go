package grantproof

import (
 "crypto/ed25519"
 "crypto/sha256"
 "errors"
 "sync"
 "testing"
 "time"
)

// interleavedFloor models an atomic revision floor, NOT durable storage.
// It forces a stale verifier to read the old floor before a newer
// checkpoint commits, exercising the compare-and-commit contract.
type interleavedFloor struct {
 mu sync.Mutex
 revision uint64
 firstRead chan struct{}
 releaseFirst chan struct{}
 once sync.Once
}

func (f *interleavedFloor) Floor(_, _, _ string) (uint64, error) {
 f.mu.Lock()
 r := f.revision
 f.mu.Unlock()
 blocked := false
 f.once.Do(func(){ blocked=true; close(f.firstRead) })
 if blocked { <-f.releaseFirst }
 return r, nil
}
func (f *interleavedFloor) CommitIfNotSuperseded(_, _, _ string, candidate uint64) error {
 f.mu.Lock()
 defer f.mu.Unlock()
 if candidate < f.revision { return ErrFloorUnavailable }
 f.revision = candidate
 return nil
}

func TestCheckpointConcurrentSupersessionDeniesStaleAdmission(t *testing.T) {
 seed := sha256.Sum256([]byte("synthetic-floor-race-contract"))
 private := ed25519.NewKeyFromSeed(seed[:])
 trusted := TrustedSource{SourceID:"source", KeyVersion:3, PublicKey:private.Public().(ed25519.PublicKey)}
 grant := Grant{SourceID:"source", AudienceID:"reader", ShareID:"share", SnapshotID:"snap", KeyVersion:3, GrantEpoch:7, NotBeforeUnix:100, ExpiresUnix:500, ReadOnly:true}
 gb,err:=Encode(grant); if err!=nil { t.Fatal(err) }
 grantSig:=ed25519.Sign(private,gb)
 makeCheckpoint:=func(rev uint64)(SignedCheckpoint,[]byte){
  cp:=SignedCheckpoint{Authority:CurrentAuthority{SourceID:"source",AudienceID:"reader",ShareID:"share",KeyVersion:3,GrantEpoch:7},Revision:rev,IssuedUnix:140,ExpiresUnix:200}
  b,e:=CheckpointMessage(cp);if e!=nil {t.Fatal(e)}
  return cp,ed25519.Sign(private,b)
 }
 old,oldSig:=makeCheckpoint(12)
 newer,newSig:=makeCheckpoint(13)
 store:=&interleavedFloor{revision:11,firstRead:make(chan struct{}),releaseFirst:make(chan struct{})}
 run:=func(cp SignedCheckpoint,sig []byte)error{
  return VerifyAndCommitCheckpoint(store,grant,grantSig,trusted,cp,sig,20*time.Second,"snap",time.Unix(150,0))
 }
 result:=make(chan error,1)
 go func(){result<-run(old,oldSig)}()
 <-store.firstRead
 if err:=run(newer,newSig);err!=nil {t.Fatalf("newer checkpoint rejected: %v",err)}
 close(store.releaseFirst)
 if err:=<-result; !errors.Is(err,ErrFloorUnavailable) {t.Fatalf("stale admission accepted or wrong error: %v",err)}
 store.mu.Lock(); final:=store.revision;store.mu.Unlock()
 if final!=13 {t.Fatalf("revision regressed: %d",final)}
}
