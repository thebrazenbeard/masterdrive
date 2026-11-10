package grantproof

import (
 "crypto/sha256"
 "errors"
 "sync"
 "testing"
)

// This suite exercises namespace isolation under mutation, rather than only
// inspecting provisioned values. It is not a persistence or host test.
func TestMemoryFingerprintFloorsCrossScopeMutation(t *testing.T) {
 store := &MemoryFingerprintFloors{}
 type scope struct { source, audience, share string }
 scopes := []scope{
  {"a","bc","d"}, {"ab","c","d"},
  {"source","audience","share"}, {"source","audience","share/child"},
  {"source","other-audience","share"}, {"other-source","audience","share"},
 }
 for i, s := range scopes {
  d := sha256.Sum256([]byte{byte(i), 0})
  if err := store.Provision(s.source,s.audience,s.share,1,d); err != nil { t.Fatal(err) }
 }
 const rounds = 64
 var wg sync.WaitGroup
 errs := make(chan error,len(scopes))
 for i, s := range scopes {
  wg.Add(1)
  go func(i int,s scope) {
   defer wg.Done()
   for rev:=uint64(2);rev<=rounds;rev++ {
    digest:=sha256.Sum256([]byte{byte(i),byte(rev)})
    if err:=store.CommitFingerprintIfCurrent(s.source,s.audience,s.share,rev,digest);err!=nil { errs<-err;return }
   }
  }(i,s)
 }
 wg.Wait()
 close(errs)
 for err:=range errs { t.Fatalf("independent relationship mutation failed: %v",err) }
 for i,s:=range scopes {
  rev,digest,err:=store.FingerprintFloor(s.source,s.audience,s.share)
  want:=sha256.Sum256([]byte{byte(i),rounds})
  if err!=nil || rev!=rounds || digest!=want { t.Fatalf("scope %d corrupted: rev=%d err=%v",i,rev,err) }
  stale:=sha256.Sum256([]byte{byte(i),1})
  if err:=store.CommitFingerprintIfCurrent(s.source,s.audience,s.share,rounds-1,stale);!errors.Is(err,ErrFloorUnavailable) {
   t.Fatalf("scope %d accepted rollback: %v",i,err)
  }
 }
}

func TestMemoryFingerprintFloorsInvalidScopeNeverMutates(t *testing.T) {
 store:=&MemoryFingerprintFloors{}
 original:=sha256.Sum256([]byte("original"))
 if err:=store.Provision("source","audience","share",9,original);err!=nil { t.Fatal(err) }
 for _,s:=range [][3]string{{"","audience","share"},{"source","","share"},{"source","audience",""}} {
  if err:=store.Provision(s[0],s[1],s[2],10,original);!errors.Is(err,ErrFloorUnavailable) { t.Fatalf("invalid provision: %v",err) }
  if err:=store.CommitFingerprintIfCurrent(s[0],s[1],s[2],10,original);!errors.Is(err,ErrFloorUnavailable) { t.Fatalf("invalid commit: %v",err) }
  if _,_,err:=store.FingerprintFloor(s[0],s[1],s[2]);!errors.Is(err,ErrFloorUnavailable) { t.Fatalf("invalid read: %v",err) }
 }
 rev,digest,err:=store.FingerprintFloor("source","audience","share")
 if err!=nil || rev!=9 || digest!=original { t.Fatalf("valid scope changed: %d %v",rev,err) }
}
