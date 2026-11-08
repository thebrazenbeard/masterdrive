package revocation

import (
    "os"
    "path/filepath"
    "strings"
    "testing"
)

func TestRevocationPersistsAcrossReopen(t *testing.T) {
    dir:=t.TempDir()
    s,err:=Open(dir);if err!=nil{t.Fatal(err)}
    token:=strings.Repeat("a",48)
    revoked,err:=s.Check(token);if err!=nil||revoked{t.Fatalf("initial: %v %v",revoked,err)}
    if err:=s.Revoke(token);err!=nil{t.Fatal(err)}
    if err:=s.Revoke(token);err!=nil{t.Fatal(err)}
    reopened,err:=Open(dir);if err!=nil{t.Fatal(err)}
    revoked,err=reopened.Check(token);if err!=nil||!revoked{t.Fatalf("reopen: %v %v",revoked,err)}
    other:=strings.Repeat("b",48)
    revoked,err=reopened.Check(other);if err!=nil||revoked{t.Fatalf("other token: %v %v",revoked,err)}
    entries,err:=os.ReadDir(dir);if err!=nil{t.Fatal(err)}
    if len(entries)!=1||strings.Contains(entries[0].Name(),token){t.Fatalf("unexpected tombstone filenames: %v",entries)}
}

func TestMissingStoreFailsClosed(t *testing.T) {
    dir:=t.TempDir()
    s,err:=Open(dir);if err!=nil{t.Fatal(err)}
    if err:=os.Remove(dir);err!=nil{t.Fatal(err)}
    revoked,err:=s.Check(strings.Repeat("x",40))
    if err==nil||!revoked{t.Fatalf("missing directory must deny: %v %v",revoked,err)}
    if _,err:=Open(filepath.Join(dir,"missing"));err==nil{t.Fatal("missing store accepted")}
}

func TestRejectsShortTokenAndRelativeDirectory(t *testing.T) {
    if _,err:=Open("relative");err==nil{t.Fatal("relative path accepted")}
    s,err:=Open(t.TempDir());if err!=nil{t.Fatal(err)}
    if err:=s.Revoke("short");err==nil{t.Fatal("short token accepted")}
    revoked,err:=s.Check("short");if err==nil||!revoked{t.Fatal("short token not denied")}
}

func TestIncompleteAndCorruptTombstonesFailClosed(t *testing.T) {
    for _,payload:=range []string{"", "revok", "tampered\n"} {
        t.Run(strings.ReplaceAll(payload,"\n","_"),func(t *testing.T){
            s,err:=Open(t.TempDir());if err!=nil{t.Fatal(err)}
            token:=strings.Repeat("c",48)
            name,err:=s.filename(token);if err!=nil{t.Fatal(err)}
            if err:=os.WriteFile(name,[]byte(payload),0600);err!=nil{t.Fatal(err)}
            if err:=s.Revoke(token);err==nil{t.Fatal("corrupt preexisting tombstone accepted as idempotent")}
            revoked,err:=s.Check(token)
            if !revoked||err==nil{t.Fatalf("corrupt tombstone must fail closed: %v %v",revoked,err)}
        })
    }
}

func TestReplacedStoreDirectoryFailsClosed(t *testing.T) {
    parent:=t.TempDir()
    dir:=filepath.Join(parent,"ledger")
    if err:=os.Mkdir(dir,0700);err!=nil{t.Fatal(err)}
    s,err:=Open(dir);if err!=nil{t.Fatal(err)}
    if err:=os.Remove(dir);err!=nil{t.Fatal(err)}
    if err:=os.WriteFile(dir,[]byte("replacement"),0600);err!=nil{t.Fatal(err)}
    token:=strings.Repeat("d",48)
    if err:=s.Revoke(token);err==nil{t.Fatal("replaced store accepted for revoke")}
    revoked,err:=s.Check(token)
    if !revoked||err==nil{t.Fatalf("replaced store must fail closed: %v %v",revoked,err)}
}
