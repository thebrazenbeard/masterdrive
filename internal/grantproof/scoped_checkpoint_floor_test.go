package grantproof

import (
 "crypto/sha256"
 "errors"
 "testing"
)

type recordingFloor struct {
 reads, commits int
 source, audience, share string
 revision uint64
 fingerprint [32]byte
 err error
}
func (f *recordingFloor) FingerprintFloor(s,a,h string)(uint64,[32]byte,error){
 f.reads++;f.source=s;f.audience=a;f.share=h
 return f.revision,f.fingerprint,f.err
}
func (f *recordingFloor) CommitFingerprintIfCurrent(s,a,h string,r uint64,d [32]byte)error{
 f.commits++;f.source=s;f.audience=a;f.share=h
 return f.err
}

func TestScopedCheckpointFloorIsolation(t *testing.T){
 f:=&recordingFloor{revision:17,fingerprint:sha256.Sum256([]byte("fixture"))}
 s:=ScopedCheckpointFloor{Store:f,SourceID:"source",AudienceID:"reader",ShareID:"share"}
 r,d,err:=s.FingerprintFloor("source","reader","share")
 if err!=nil||r!=17||d!=f.fingerprint||f.reads!=1 {t.Fatal("valid scoped read failed")}
 for _,tc:=range []struct{name,src,aud,share string}{
  {"source","other","reader","share"},
  {"audience","source","other","share"},
  {"share","source","reader","other"},
  {"empty source","","reader","share"},
 }{
  t.Run(tc.name,func(t *testing.T){
   before:=f.reads
   if _,_,err:=s.FingerprintFloor(tc.src,tc.aud,tc.share);!errors.Is(err,ErrFloorUnavailable){t.Fatal("cross-scope read allowed")}
   if f.reads!=before{t.Fatal("cross-scope read reached backing store")}
   before=f.commits
   if err:=s.CommitFingerprintIfCurrent(tc.src,tc.aud,tc.share,18,d);!errors.Is(err,ErrFloorUnavailable){t.Fatal("cross-scope commit allowed")}
   if f.commits!=before{t.Fatal("cross-scope commit reached backing store")}
  })
 }
 if err:=s.CommitFingerprintIfCurrent("source","reader","share",0,d);!errors.Is(err,ErrFloorUnavailable)||f.commits!=0{t.Fatal("zero revision committed")}
 if err:=s.CommitFingerprintIfCurrent("source","reader","share",18,d);err!=nil||f.commits!=1{t.Fatal("valid scoped commit failed")}
}
func TestScopedCheckpointFloorMissingProvisioning(t *testing.T){
 for _,s:=range []ScopedCheckpointFloor{
  {},
  {Store:&recordingFloor{},AudienceID:"reader",ShareID:"share"},
  {Store:&recordingFloor{},SourceID:"source",ShareID:"share"},
  {Store:&recordingFloor{},SourceID:"source",AudienceID:"reader"},
 }{
  if _,_,err:=s.FingerprintFloor("source","reader","share");!errors.Is(err,ErrFloorUnavailable){t.Fatal("unprovisioned read allowed")}
  if err:=s.CommitFingerprintIfCurrent("source","reader","share",1,[32]byte{});!errors.Is(err,ErrFloorUnavailable){t.Fatal("unprovisioned commit allowed")}
 }
}
func TestScopedCheckpointFloorCommitErrorsFailClosed(t *testing.T){
 f:=&recordingFloor{err:errors.New("ambiguous disk failure")}
 s:=ScopedCheckpointFloor{Store:f,SourceID:"source",AudienceID:"reader",ShareID:"share"}
 if err:=s.CommitFingerprintIfCurrent("source","reader","share",18,[32]byte{});!errors.Is(err,ErrFloorUnavailable){t.Fatal("ambiguous commit acknowledged")}
 f.err=ErrCheckpointEquivocation
 if err:=s.CommitFingerprintIfCurrent("source","reader","share",18,[32]byte{});!errors.Is(err,ErrCheckpointEquivocation){t.Fatal("equivocation lost")}
}
