package versionproof

import (
 "crypto/sha256"
 "errors"
 "io"
 "testing"
)

type zeroProgressReader struct{}
func (zeroProgressReader) Read(p []byte) (int,error) { return 0,nil }

type oneByteThenStall struct{ done bool }
func (r *oneByteThenStall) Read(p []byte) (int,error) {
 if !r.done { r.done=true; p[0]='x'; return 1,nil }
 return 0,nil
}

type invalidCountReader struct{}
func (invalidCountReader) Read(p []byte) (int,error) { return len(p)+1,nil }

func TestVerifyStreamRejectsNonprogressAndInvalidCounts(t *testing.T) {
 pin:=Pin{Version:"v1",Size:2,Digest:sha256.Sum256([]byte("xy"))}
 cases:=[]struct{name string;reader io.Reader}{
  {"zero progress",zeroProgressReader{}},
  {"partial then stall",&oneByteThenStall{}},
  {"invalid reader count",invalidCountReader{}},
 }
 for _,tc:=range cases { t.Run(tc.name,func(t *testing.T){
  if err:=VerifyStream(pin,"v1",tc.reader,2);!errors.Is(err,ErrInvalid) {
   t.Fatalf("expected fail-closed ErrInvalid, got %v",err)
  }
 }) }
}
