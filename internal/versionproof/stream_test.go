package versionproof

import (
 "bytes"
 "crypto/sha256"
 "errors"
 "io"
 "testing"
)

type noEOF struct{ io.Reader }
func (r noEOF) Read(p []byte)(int,error) { n,e:=r.Reader.Read(p);if e==io.EOF{return 0,nil};return n,e }
type failAfter struct{ remaining int }
func (r *failAfter) Read(p []byte)(int,error) { if r.remaining==0{return 0,errors.New("broken")};r.remaining--;p[0]='a';return 1,nil }

func TestVerifyStream(t *testing.T) {
 data:=[]byte("immutable snapshot")
 pin:=Pin{Size:uint64(len(data)),Digest:sha256.Sum256(data),Version:"v1"}
 cases:=[]struct{name string;pin Pin;version string;reader io.Reader;max uint64;ok bool}{
  {"exact",pin,"v1",bytes.NewReader(data),1024,true},
  {"short",pin,"v1",bytes.NewReader(data[:len(data)-1]),1024,false},
  {"extra",pin,"v1",bytes.NewReader(append(bytes.Clone(data),'x')),1024,false},
  {"changed",pin,"v1",bytes.NewReader([]byte("immutable snapshoT")),1024,false},
  {"wrong version",pin,"v2",bytes.NewReader(data),1024,false},
  {"over policy",pin,"v1",bytes.NewReader(data),3,false},
  {"nil",pin,"v1",nil,1024,false},
  {"no EOF proof",pin,"v1",noEOF{bytes.NewReader(data)},1024,false},
  {"read failure",pin,"v1",&failAfter{remaining:2},1024,false},
  {"overflow",Pin{Size:^uint64(0),Version:"v1"},"v1",bytes.NewReader(data),^uint64(0),false},
 }
 for _,tc:=range cases {t.Run(tc.name,func(t *testing.T){
  err:=VerifyStream(tc.pin,tc.version,tc.reader,tc.max)
  if tc.ok && err!=nil {t.Fatal(err)}
  if !tc.ok && !errors.Is(err,ErrInvalid) {t.Fatalf("expected invalid, got %v",err)}
 })}
}
