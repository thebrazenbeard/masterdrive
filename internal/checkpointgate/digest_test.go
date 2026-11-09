package checkpointgate

import (
 "bytes"
 "errors"
 "testing"
)

func TestDigestRevocationsCanonicalAndTamper(t *testing.T) {
 a:=[]Revocation{{ID:"grant-b",Epoch:4},{ID:"grant-a",Epoch:2}}
 original:=append([]Revocation(nil),a...)
 d,err:=DigestRevocations("store",9,a);if err!=nil{t.Fatal(err)}
 if a[0]!=original[0] || a[1]!=original[1] {t.Fatal("mutated caller order")}
 reversed:=[]Revocation{a[1],a[0]}
 same,err:=DigestRevocations("store",9,reversed);if err!=nil || d!=same{t.Fatal("ordering changed digest",err)}
 variants:=[]struct{name string; store string; rev uint64; entries []Revocation}{
  {"deleted tombstone","store",9,a[:1]},
  {"changed epoch","store",9,[]Revocation{{ID:"grant-b",Epoch:5},{ID:"grant-a",Epoch:2}}},
  {"changed revision","store",8,a},
  {"changed store","other",9,a},
  {"empty ledger","store",9,nil},
 }
 for _,tc:=range variants {t.Run(tc.name,func(t *testing.T){got,err:=DigestRevocations(tc.store,tc.rev,tc.entries);if err!=nil || bytes.Equal(got[:],d[:]){t.Fatalf("digest collision or error: %v",err)}})}
}
func TestDigestRevocationsRejectsInvalid(t *testing.T) {
 cases:=[]struct{store string;rev uint64;entries []Revocation}{
  {"",1,nil},{"store",0,nil},{"store",1,[]Revocation{{ID:"",Epoch:1}}},
  {"store",1,[]Revocation{{ID:"x",Epoch:0}}},
  {"store",1,[]Revocation{{ID:"x",Epoch:1},{ID:"x",Epoch:2}}},
 }
 for _,tc:=range cases {_,err:=DigestRevocations(tc.store,tc.rev,tc.entries);if !errors.Is(err,ErrInvalidLedger){t.Fatalf("expected rejection, got %v",err)}}
}
