package grantproof

import (
 "crypto/sha256"
 "errors"
 "os"
 "path/filepath"
 "testing"
)

func TestJournalFingerprintFloorsRestartAndIsolation(t *testing.T) {
 path:=filepath.Join(t.TempDir(),"floor.log")
 a:=&JournalFingerprintFloors{Path:path}
 one:=sha256.Sum256([]byte("one"))
 two:=sha256.Sum256([]byte("two"))
 if err:=a.Provision("src","aud","share",1,one);err!=nil{t.Fatal(err)}
 if err:=a.Provision("src","other","share",5,two);err!=nil{t.Fatal(err)}
 if err:=a.CommitFingerprintIfCurrent("src","aud","share",2,two);err!=nil{t.Fatal(err)}
 b:=&JournalFingerprintFloors{Path:path}
 rev,fp,err:=b.FingerprintFloor("src","aud","share")
 if err!=nil || rev!=2 || fp!=two {t.Fatalf("restart floor: %d %v",rev,err)}
 rev,fp,err=b.FingerprintFloor("src","other","share")
 if err!=nil || rev!=5 || fp!=two {t.Fatalf("scope contamination: %d %v",rev,err)}
 if err:=b.CommitFingerprintIfCurrent("src","aud","share",1,one);!errors.Is(err,ErrFloorUnavailable){t.Fatalf("rollback: %v",err)}
 if err:=b.CommitFingerprintIfCurrent("src","aud","share",2,one);!errors.Is(err,ErrCheckpointEquivocation){t.Fatalf("fork: %v",err)}
 if err:=b.Provision("src","aud","share",1,one);!errors.Is(err,ErrFloorUnavailable){t.Fatalf("reset: %v",err)}
}

func TestJournalFingerprintFloorsCorruptionAndLockFailClosed(t *testing.T){
 path:=filepath.Join(t.TempDir(),"floor.log")
 j:=&JournalFingerprintFloors{Path:path}
 fp:=sha256.Sum256([]byte("fingerprint"))
 if err:=j.Provision("s","a","h",1,fp);err!=nil{t.Fatal(err)}
 lock:=path+".lock"
 if err:=os.WriteFile(lock,[]byte("occupied"),0600);err!=nil{t.Fatal(err)}
 if err:=j.CommitFingerprintIfCurrent("s","a","h",2,fp);!errors.Is(err,ErrFloorUnavailable){t.Fatalf("lock bypass: %v",err)}
 if err:=os.Remove(lock);err!=nil{t.Fatal(err)}
 f,err:=os.OpenFile(path,os.O_APPEND|os.O_WRONLY,0600);if err!=nil{t.Fatal(err)}
 if _,err=f.WriteString("{truncated");err!=nil{t.Fatal(err)}
 if err=f.Close();err!=nil{t.Fatal(err)}
 if _,_,err=j.FingerprintFloor("s","a","h");!errors.Is(err,ErrFloorUnavailable){t.Fatalf("corrupt journal accepted: %v",err)}
 if err=j.CommitFingerprintIfCurrent("s","a","h",2,fp);!errors.Is(err,ErrFloorUnavailable){t.Fatalf("corrupt journal mutation: %v",err)}
}
