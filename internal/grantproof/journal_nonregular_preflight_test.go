package grantproof

import (
 "errors"
 "os"
 "path/filepath"
 "testing"
)

// Directory paths are portable non-regular fixtures on Windows and Linux.
// A FIFO/device fixture would be platform-specific and could block on open.
func TestJournalRejectsNonregularPathsBeforeOpen(t *testing.T) {
 for _, lockPath := range []bool{false,true} {
  name:="journal"
  if lockPath {name="lock"}
  t.Run(name,func(t *testing.T){
   dir:=t.TempDir()
   journal:=filepath.Join(dir,"floor.journal")
   blocked:=journal
   if lockPath {blocked+=".lock"}
   if err:=os.Mkdir(blocked,0700);err!=nil {t.Fatal(err)}
   store:=&JournalFingerprintFloors{Path:journal}
   var fp [32]byte
   if err:=store.Provision("source","audience","share",1,fp);!errors.Is(err,ErrFloorUnavailable){
    t.Fatalf("non-regular path accepted: %v",err)
   }
   info,err:=os.Lstat(blocked)
   if err!=nil || !info.IsDir(){t.Fatalf("fixture changed: %v, %v",info,err)}
   if lockPath {
    if _,err:=os.Lstat(journal);!errors.Is(err,os.ErrNotExist){t.Fatalf("journal created: %v",err)}
   }
  })
 }
}
