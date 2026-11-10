package grantproof

import (
 "crypto/sha256"
 "errors"
 "sync"
 "testing"
)

func TestMemoryFingerprintFloorsRelationshipIsolation(t *testing.T) {
 var store MemoryFingerprintFloors
 a := sha256.Sum256([]byte("A"))
 b := sha256.Sum256([]byte("B"))
 if err := store.Provision("source-A","reader-A","share-A",10,a); err != nil { t.Fatal(err) }
 if err := store.Provision("source-B","reader-B","share-B",20,b); err != nil { t.Fatal(err) }
 if err := store.CommitFingerprintIfCurrent("source-A","reader-A","share-A",11,b); err != nil { t.Fatal(err) }
 rev, fp, err := store.FingerprintFloor("source-B","reader-B","share-B")
 if err != nil || rev != 20 || fp != b { t.Fatalf("cross-relationship alias: %d %x %v",rev,fp,err) }
 if err := store.Provision("source-A","reader-A","share-A",1,a); !errors.Is(err,ErrFloorUnavailable) { t.Fatalf("reprovision reset: %v",err) }
 if _,_,err := store.FingerprintFloor("source-A","reader-A","unknown"); !errors.Is(err,ErrFloorUnavailable) { t.Fatalf("unknown accepted: %v",err) }
 if err := store.CommitFingerprintIfCurrent("source-A","reader-A","share-A",11,a); !errors.Is(err,ErrCheckpointEquivocation) { t.Fatalf("equal revision fork: %v",err) }
 if err := store.CommitFingerprintIfCurrent("source-A","reader-A","share-A",10,a); !errors.Is(err,ErrFloorUnavailable) { t.Fatalf("rollback accepted: %v",err) }
}

func TestMemoryFingerprintFloorsConcurrentSupersession(t *testing.T) {
 var store MemoryFingerprintFloors
 first := sha256.Sum256([]byte("first"))
 newer := sha256.Sum256([]byte("newer"))
 if err := store.Provision("s","a","h",1,first); err != nil { t.Fatal(err) }
 var wg sync.WaitGroup
 for i:=uint64(2); i<=100; i++ {
  wg.Add(1)
  go func(rev uint64) { defer wg.Done(); _ = store.CommitFingerprintIfCurrent("s","a","h",rev,newer) }(i)
 }
 wg.Wait()
 rev,_,err := store.FingerprintFloor("s","a","h")
 if err != nil || rev != 100 { t.Fatalf("lost max revision: %d %v",rev,err) }
 if err := store.CommitFingerprintIfCurrent("s","a","h",99,newer); !errors.Is(err,ErrFloorUnavailable) { t.Fatalf("stale writer admitted: %v",err) }
}

func TestMemoryFingerprintFloorsUnprovisionedAndZero(t *testing.T) {
 var store MemoryFingerprintFloors
 var nilStore *MemoryFingerprintFloors
 if err:=nilStore.Provision("s","a","h",1,[32]byte{}); !errors.Is(err,ErrFloorUnavailable) { t.Fatal(err) }
 if err:=store.Provision("","a","h",1,[32]byte{}); !errors.Is(err,ErrFloorUnavailable) { t.Fatal(err) }
 if err:=store.Provision("s","a","h",0,[32]byte{}); !errors.Is(err,ErrFloorUnavailable) { t.Fatal(err) }
 if err:=store.CommitFingerprintIfCurrent("s","a","h",1,[32]byte{}); !errors.Is(err,ErrFloorUnavailable) { t.Fatal(err) }
}
