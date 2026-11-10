package grantproof

import (
 "crypto/ed25519"
 "crypto/sha256"
 "errors"
 "testing"
 "time"
)

// restartDisk is deliberately a synthetic persistence model, not a filesystem
// implementation or proof of fsync, atomic rename, or crash durability.
type restartDisk struct {
 revision uint64
 unavailable bool
}

type restartFloor struct {
 disk *restartDisk
 failBefore bool
 failAfter bool
}

func (s *restartFloor) Floor(_, _, _ string) (uint64, error) {
 if s.disk == nil || s.disk.unavailable { return 0, ErrFloorUnavailable }
 return s.disk.revision, nil
}

func (s *restartFloor) CommitIfNotSuperseded(_, _, _ string, candidate uint64) error {
 if s.disk == nil || s.disk.unavailable || s.failBefore { return ErrFloorUnavailable }
 if candidate < s.disk.revision { return ErrFloorUnavailable }
 s.disk.revision = candidate // model of a completed durable commit, NOT a real disk write
 if s.failAfter { return ErrFloorUnavailable }
 return nil
}

func TestCheckpointRestartAndCommitFailureContract(t *testing.T) {
 seed := sha256.Sum256([]byte("synthetic-checkpoint-restart-contract"))
 key := ed25519.NewKeyFromSeed(seed[:])
 trusted := TrustedSource{SourceID:"source",KeyVersion:3,PublicKey:key.Public().(ed25519.PublicKey)}
 grant := Grant{SourceID:"source",AudienceID:"reader",ShareID:"share",SnapshotID:"snap",KeyVersion:3,GrantEpoch:7,NotBeforeUnix:100,ExpiresUnix:500,ReadOnly:true}
 message,err := Encode(grant); if err != nil { t.Fatal(err) }
 grantSig := ed25519.Sign(key,message)
 run := func(store CheckpointFloorStore, revision uint64) error {
  cp := SignedCheckpoint{Authority:CurrentAuthority{SourceID:"source",AudienceID:"reader",ShareID:"share",KeyVersion:3,GrantEpoch:7},Revision:revision,IssuedUnix:140,ExpiresUnix:200}
  msg,e := CheckpointMessage(cp); if e != nil { return e }
  return VerifyAndCommitCheckpoint(store,grant,grantSig,trusted,cp,ed25519.Sign(key,msg),20*time.Second,"snap",time.Unix(150,0))
 }
 t.Run("successful commit survives modeled restart and rejects rollback",func(t *testing.T){
  disk:=&restartDisk{revision:11}
  if err:=run(&restartFloor{disk:disk},13);err!=nil {t.Fatal(err)}
  restarted:=&restartFloor{disk:disk}
  if !errors.Is(run(restarted,12),ErrInvalid) {t.Fatal("accepted older signed checkpoint after restart")}
  if disk.revision!=13 {t.Fatalf("floor regressed: %d",disk.revision)}
 })
 t.Run("failure before persistence denies admission",func(t *testing.T){
  disk:=&restartDisk{revision:11}
  if !errors.Is(run(&restartFloor{disk:disk,failBefore:true},12),ErrFloorUnavailable) {t.Fatal("accepted failed commit")}
  if disk.revision!=11 {t.Fatal("failed commit changed durable model")}
  if err:=run(&restartFloor{disk:disk},12);err!=nil {t.Fatalf("recovery refused valid checkpoint: %v",err)}
 })
 t.Run("ambiguous failure after persistence still denies admission",func(t *testing.T){
  disk:=&restartDisk{revision:11}
  if !errors.Is(run(&restartFloor{disk:disk,failAfter:true},13),ErrFloorUnavailable) {t.Fatal("accepted ambiguous commit result")}
  if disk.revision!=13 {t.Fatal("model did not preserve committed floor")}
  if !errors.Is(run(&restartFloor{disk:disk},12),ErrInvalid) {t.Fatal("accepted rollback after ambiguous failure")}
 })
 t.Run("unavailable backing state fails closed",func(t *testing.T){
  disk:=&restartDisk{revision:11,unavailable:true}
  if !errors.Is(run(&restartFloor{disk:disk},12),ErrFloorUnavailable) {t.Fatal("accepted unavailable state")}
  if disk.revision!=11 {t.Fatal("unavailable state was modified")}
 })
}
