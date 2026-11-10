package grantproof

import (
 "errors"
 "os"
 "path/filepath"
 "testing"
)

func TestJournalRejectsPreexistingSymlinkWithoutTouchingTarget(t *testing.T) {
 dir:=t.TempDir()
 target:=filepath.Join(dir,"protected")
 if err:=os.WriteFile(target,[]byte("sentinel"),0600);err!=nil{t.Fatal(err)}
 journal:=filepath.Join(dir,"floor.journal")
 if err:=os.Symlink(target,journal);err!=nil{t.Skipf("symlink unavailable: %v",err)}
 store:=&JournalFingerprintFloors{Path:journal}
 var fp [32]byte
 if err:=store.Provision("source","audience","share",1,fp);!errors.Is(err,ErrFloorUnavailable){t.Fatalf("symlink journal accepted: %v",err)}
 b,err:=os.ReadFile(target);if err!=nil{t.Fatal(err)}
 if string(b)!="sentinel"{t.Fatalf("target modified: %q",b)}
}
func TestJournalRejectsPreexistingLockSymlink(t *testing.T) {
 dir:=t.TempDir()
 target:=filepath.Join(dir,"protected")
 if err:=os.WriteFile(target,[]byte("sentinel"),0600);err!=nil{t.Fatal(err)}
 journal:=filepath.Join(dir,"floor.journal")
 if err:=os.Symlink(target,journal+".lock");err!=nil{t.Skipf("symlink unavailable: %v",err)}
 store:=&JournalFingerprintFloors{Path:journal}
 var fp [32]byte
 if err:=store.Provision("source","audience","share",1,fp);!errors.Is(err,ErrFloorUnavailable){t.Fatalf("symlink lock accepted: %v",err)}
 if _,err:=os.Lstat(journal);!errors.Is(err,os.ErrNotExist){t.Fatalf("journal unexpectedly created: %v",err)}
}
