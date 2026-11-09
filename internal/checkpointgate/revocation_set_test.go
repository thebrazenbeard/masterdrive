package checkpointgate

import (
 "errors"
 "testing"
)

func TestVerifyRevocationSet(t *testing.T) {
 entries := []Revocation{{ID:"grant-A",Epoch:1},{ID:"grant-B",Epoch:3}}
 digest, err := DigestRevocations("owner",9,entries)
 if err != nil { t.Fatal(err) }
 trusted := Checkpoint{StoreID:"owner",Revision:9,Digest:digest[:]}
 cases := []struct{
  name string
  checkpoint Checkpoint
  store string
  revision uint64
  entries []Revocation
  valid bool
 }{
  {"exact",trusted,"owner",9,entries,true},
  {"deleted tombstone",trusted,"owner",9,entries[:1],false},
  {"changed epoch",trusted,"owner",9,[]Revocation{{ID:"grant-A",Epoch:1},{ID:"grant-B",Epoch:4}},false},
  {"revision rollback",trusted,"owner",8,entries,false},
  {"store mismatch",trusted,"other",9,entries,false},
  {"untrusted digest",Checkpoint{StoreID:"owner",Revision:9,Digest:make([]byte,32)},"owner",9,entries,false},
  {"invalid duplicate",trusted,"owner",9,[]Revocation{{ID:"grant-A",Epoch:1},{ID:"grant-A",Epoch:2}},false},
  {"invalid empty",trusted,"owner",9,[]Revocation{{ID:"",Epoch:1}},false},
  {"missing trusted",Checkpoint{},"owner",9,entries,false},
 }
 for _,tc := range cases {
  t.Run(tc.name,func(t *testing.T){
   err:=VerifyRevocationSet(tc.checkpoint,tc.store,tc.revision,tc.entries)
   if tc.valid && err != nil { t.Fatalf("expected valid: %v",err) }
   if !tc.valid && !errors.Is(err,ErrUntrustedCheckpoint) { t.Fatalf("expected fail closed: %v",err) }
  })
 }
}

func TestVerifyRevocationSetDoesNotMutateEntries(t *testing.T) {
 entries:=[]Revocation{{ID:"z",Epoch:1},{ID:"a",Epoch:2}}
 digest,err:=DigestRevocations("owner",1,entries)
 if err!=nil { t.Fatal(err) }
 if err:=VerifyRevocationSet(Checkpoint{StoreID:"owner",Revision:1,Digest:digest[:]},"owner",1,entries);err!=nil { t.Fatal(err) }
 if entries[0].ID!="z" || entries[1].ID!="a" { t.Fatal("input mutated") }
}
