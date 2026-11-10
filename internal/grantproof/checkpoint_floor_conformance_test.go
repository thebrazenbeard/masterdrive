package grantproof

import (
 "crypto/sha256"
 "errors"
 "sync"
 "testing"
)

// fingerprintFloorFixture describes an explicitly provisioned test store.
// Durable implementations should use a fresh isolated backing directory per
// invocation and run this same contract suite, plus restart/crash tests.
type fingerprintFloorFixture struct {
 store CheckpointFingerprintFloorStore
 provision func(source, audience, share string, revision uint64, digest [32]byte) error
}

// exerciseFingerprintFloorContract checks storage semantics, not authenticity,
// durability, or live read authorization.
func exerciseFingerprintFloorContract(t *testing.T, makeFixture func(t *testing.T) fingerprintFloorFixture) {
 t.Helper()
 t.Run("relationship keys cannot collide by concatenation", func(t *testing.T) {
  f := makeFixture(t)
  a := sha256.Sum256([]byte("relationship A"))
  b := sha256.Sum256([]byte("relationship B"))
  // Naive string concatenation maps both triples to "abcd".
  if err := f.provision("a", "bc", "d", 7, a); err != nil { t.Fatal(err) }
  if err := f.provision("ab", "c", "d", 11, b); err != nil { t.Fatal(err) }
  for _, tc := range []struct{ source, audience, share string; revision uint64; digest [32]byte }{
   {"a","bc","d",7,a}, {"ab","c","d",11,b},
  } {
   gotRevision, gotDigest, err := f.store.FingerprintFloor(tc.source,tc.audience,tc.share)
   if err != nil || gotRevision != tc.revision || gotDigest != tc.digest {
    t.Fatalf("cross-relationship alias: revision=%d digest=%x err=%v",gotRevision,gotDigest,err)
   }
  }
 })
 t.Run("unknown relationship fails closed", func(t *testing.T) {
  f := makeFixture(t)
  d := sha256.Sum256([]byte("unknown"))
  if _,_,err := f.store.FingerprintFloor("unprovisioned","reader","share"); err == nil { t.Fatal("unknown relationship readable") }
  if err := f.store.CommitFingerprintIfCurrent("unprovisioned","reader","share",1,d); err == nil { t.Fatal("unknown relationship writable") }
 })
 t.Run("concurrent same-revision forks cannot both commit", func(t *testing.T) {
  f := makeFixture(t)
  original := sha256.Sum256([]byte("original"))
  left := sha256.Sum256([]byte("left"))
  right := sha256.Sum256([]byte("right"))
  if err := f.provision("source","reader","share",10,original); err != nil { t.Fatal(err) }
  start := make(chan struct{})
  results := make(chan error,2)
  var wg sync.WaitGroup
  for _, digest := range [][32]byte{left,right} {
   wg.Add(1)
   go func(d [32]byte) {
    defer wg.Done()
    <-start
    results <- f.store.CommitFingerprintIfCurrent("source","reader","share",11,d)
   }(digest)
  }
  close(start)
  wg.Wait()
  close(results)
  successes, forks := 0,0
  for err := range results {
   switch {
   case err == nil: successes++
   case errors.Is(err,ErrCheckpointEquivocation): forks++
   default: t.Fatalf("unexpected concurrent commit error: %v",err)
   }
  }
  if successes != 1 || forks != 1 { t.Fatalf("same revision fork: successes=%d conflicts=%d",successes,forks) }
  revision, digest, err := f.store.FingerprintFloor("source","reader","share")
  if err != nil || revision != 11 || (digest != left && digest != right) {
   t.Fatalf("invalid winner: revision=%d digest=%x err=%v",revision,digest,err)
  }
  if err := f.store.CommitFingerprintIfCurrent("source","reader","share",10,original); err == nil {
   t.Fatal("stale checkpoint accepted after fork")
  }
 })
 t.Run("equal revision is idempotent but cannot reset", func(t *testing.T) {
  f := makeFixture(t)
  d := sha256.Sum256([]byte("stable"))
  other := sha256.Sum256([]byte("fork"))
  if err := f.provision("source","reader","share",4,d); err != nil { t.Fatal(err) }
  if err := f.store.CommitFingerprintIfCurrent("source","reader","share",4,d); err != nil { t.Fatalf("idempotence: %v",err) }
  if err := f.provision("source","reader","share",1,other); err == nil { t.Fatal("reprovision reset allowed") }
  if err := f.store.CommitFingerprintIfCurrent("source","reader","share",4,other); !errors.Is(err,ErrCheckpointEquivocation) {
   t.Fatalf("equal-revision equivocation: %v",err)
  }
 })
}

func TestMemoryFingerprintFloorsConformance(t *testing.T) {
 exerciseFingerprintFloorContract(t,func(t *testing.T) fingerprintFloorFixture {
  store := &MemoryFingerprintFloors{}
  return fingerprintFloorFixture{store:store,provision:store.Provision}
 })
}
