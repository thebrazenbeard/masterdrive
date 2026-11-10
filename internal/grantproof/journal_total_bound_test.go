package grantproof

import (
 "errors"
 "os"
 "path/filepath"
 "testing"
)

func TestJournalRejectsOversizedAggregateBeforeMutation(t *testing.T) {
 path := filepath.Join(t.TempDir(), "checkpoint.journal")
 const maxJournalBytes = 8 << 20
 // Many individually small records; the aggregate exceeds the replay budget.
 chunk := make([]byte, 4096)
 for i := range chunk { chunk[i] = 'x' }
 f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0600)
 if err != nil { t.Fatal(err) }
 for i:=0; i<maxJournalBytes/len(chunk)+1; i++ {
  if _,err=f.Write(chunk);err!=nil {f.Close();t.Fatal(err)}
 }
 if err=f.Close();err!=nil {t.Fatal(err)}
 before,err:=os.Stat(path);if err!=nil {t.Fatal(err)}
 store:= &JournalFingerprintFloors{Path:path}
 _,_,err=store.FingerprintFloor("source","audience","share")
 if !errors.Is(err,ErrFloorUnavailable) {t.Fatalf("read should fail closed: %v",err)}
 if err=store.CommitFingerprintIfCurrent("source","audience","share",2,[32]byte{1}); !errors.Is(err,ErrFloorUnavailable) {t.Fatalf("commit should fail closed: %v",err)}
 after,err:=os.Stat(path);if err!=nil {t.Fatal(err)}
 if before.Size()!=after.Size() {t.Fatalf("journal modified: %d -> %d",before.Size(),after.Size())}
}
