package checkpointgate

import (
 "bytes"
 "errors"
 "testing"
)

func TestDigestGrantLedgerCanonicalAndSensitive(t *testing.T) {
 grants:=[]GrantRecord{{ID:"b",PeerID:"peer2",ShareID:"share2",Epoch:2},{ID:"a",PeerID:"peer1",ShareID:"share1",Epoch:1}}
 tomb:=[]Revocation{{ID:"old-b",Epoch:4},{ID:"old-a",Epoch:3}}
 baseline,err:=DigestGrantLedger("store",7,grants,tomb)
 if err!=nil {t.Fatal(err)}
 reordered,err:=DigestGrantLedger("store",7,[]GrantRecord{grants[1],grants[0]},[]Revocation{tomb[1],tomb[0]})
 if err!=nil || baseline!=reordered {t.Fatalf("reordering changed digest: %v",err)}
 if grants[0].ID!="b" || tomb[0].ID!="old-b" {t.Fatal("mutated caller input")}
 check:=func(name string,store string,rev uint64,g []GrantRecord,r []Revocation){
  t.Helper()
  got,err:=DigestGrantLedger(store,rev,g,r)
  if err!=nil {t.Fatalf("%s: %v",name,err)}
  if bytes.Equal(got[:],baseline[:]) {t.Fatalf("%s: changed ledger did not change digest",name)}
 }
 check("store","other",7,grants,tomb)
 check("revision","store",8,grants,tomb)
 check("deleted grant","store",7,grants[:1],tomb)
 check("deleted tombstone","store",7,grants,tomb[:1])
 modified:=append([]GrantRecord(nil),grants...)
 modified[0].PeerID="other"
 check("peer identity","store",7,modified,tomb)
 modified=append([]GrantRecord(nil),grants...)
 modified[0].ShareID="other"
 check("share identity","store",7,modified,tomb)
 modified=append([]GrantRecord(nil),grants...)
 modified[0].Epoch=3
 check("grant epoch","store",7,modified,tomb)
 revoked:=append([]Revocation(nil),tomb...)
 revoked[0].Epoch=5
 check("revocation epoch","store",7,grants,revoked)
}

func TestDigestGrantLedgerRejectsAmbiguousAndInvalid(t *testing.T) {
 good:=GrantRecord{ID:"a",PeerID:"peer",ShareID:"share",Epoch:1}
 for _,tc:=range []struct{
  name string
  store string
  revision uint64
  grants []GrantRecord
  tombstones []Revocation
 }{
  {"empty store","",1,nil,nil},
  {"zero revision","store",0,nil,nil},
  {"empty grant ID","store",1,[]GrantRecord{{PeerID:"peer",ShareID:"share",Epoch:1}},nil},
  {"empty peer","store",1,[]GrantRecord{{ID:"a",ShareID:"share",Epoch:1}},nil},
  {"empty share","store",1,[]GrantRecord{{ID:"a",PeerID:"peer",Epoch:1}},nil},
  {"zero epoch","store",1,[]GrantRecord{{ID:"a",PeerID:"peer",ShareID:"share"}},nil},
  {"duplicate grant","store",1,[]GrantRecord{good,good},nil},
  {"duplicate tombstone","store",1,nil,[]Revocation{{ID:"old",Epoch:1},{ID:"old",Epoch:2}}},
  {"active revoked conflict","store",1,[]GrantRecord{good},[]Revocation{{ID:"a",Epoch:2}}},
  {"empty tombstone","store",1,nil,[]Revocation{{ID:"",Epoch:1}}},
  {"zero tombstone epoch","store",1,nil,[]Revocation{{ID:"old"}}},
 } {
  t.Run(tc.name,func(t *testing.T){
   digest,err:=DigestGrantLedger(tc.store,tc.revision,tc.grants,tc.tombstones)
   if !errors.Is(err,ErrInvalidLedger) || digest!=([32]byte{}) {t.Fatalf("expected invalid zero digest, got %x %v",digest,err)}
  })
 }
}

func TestDigestGrantLedgerSeparatesFieldBoundaries(t *testing.T) {
 a:=[]GrantRecord{{ID:"a:b",PeerID:"p",ShareID:"s",Epoch:1}}
 b:=[]GrantRecord{{ID:"a",PeerID:"b:p",ShareID:"s",Epoch:1}}
 x,err:=DigestGrantLedger("store",1,a,nil);if err!=nil {t.Fatal(err)}
 y,err:=DigestGrantLedger("store",1,b,nil);if err!=nil {t.Fatal(err)}
 if x==y {t.Fatal("field boundary collision")}
}
