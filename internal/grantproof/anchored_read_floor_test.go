package grantproof

import (
    "errors"
    "os"
    "path/filepath"
    "testing"
)

func TestAnchoredReadFloorRejectsCompleteJournalRollback(t *testing.T) {
    path := filepath.Join(t.TempDir(), "journal")
    journal := &JournalFingerprintFloors{Path: path}
    anchor := &MemoryFingerprintFloors{} // test oracle only: NOT protected/durable
    first, second := [32]byte{1}, [32]byte{2}
    for _, store := range []interface{ Provision(string,string,string,uint64,[32]byte) error }{journal, anchor} {
        if err := store.Provision("s","a","share",1,first); err != nil { t.Fatal(err) }
    }
    gate := &AnchoredReadFloor{Journal:journal, Authority:anchor}
    if rev, fp, err := gate.FingerprintFloor("s","a","share"); err != nil || rev != 1 || fp != first {
        t.Fatalf("initial agreement: revision=%d error=%v", rev, err)
    }
    old, err := os.ReadFile(path); if err != nil { t.Fatal(err) }
    for _, store := range []interface{ CommitFingerprintIfCurrent(string,string,string,uint64,[32]byte) error }{journal, anchor} {
        if err := store.CommitFingerprintIfCurrent("s","a","share",2,second); err != nil { t.Fatal(err) }
    }
    if err := os.WriteFile(path,old,0600); err != nil { t.Fatal(err) }
    if _,_,err := gate.FingerprintFloor("s","a","share"); !errors.Is(err,ErrFloorUnavailable) {
        t.Fatalf("rollback must fail closed, got %v",err)
    }
}

func TestAnchoredReadFloorRejectsDivergenceAndMissingAuthority(t *testing.T) {
    first, other := [32]byte{1}, [32]byte{9}
    journal, anchor := &MemoryFingerprintFloors{}, &MemoryFingerprintFloors{}
    if err:=journal.Provision("s","a","share",1,first);err!=nil{t.Fatal(err)}
    gate:=&AnchoredReadFloor{Journal:journal,Authority:anchor}
    if _,_,err:=gate.FingerprintFloor("s","a","share");!errors.Is(err,ErrFloorUnavailable){t.Fatalf("missing authority: %v",err)}
    if err:=anchor.Provision("s","a","share",1,other);err!=nil{t.Fatal(err)}
    if _,_,err:=gate.FingerprintFloor("s","a","share");!errors.Is(err,ErrFloorUnavailable){t.Fatalf("equivocation: %v",err)}
    if err:=anchor.CommitFingerprintIfCurrent("s","a","share",2,first);err!=nil{t.Fatal(err)}
    if _,_,err:=gate.FingerprintFloor("s","a","share");!errors.Is(err,ErrFloorUnavailable){t.Fatalf("journal lag: %v",err)}
    if _,_,err:=(&AnchoredReadFloor{Journal:journal}).FingerprintFloor("s","a","share");!errors.Is(err,ErrFloorUnavailable){t.Fatalf("nil authority: %v",err)}
}
