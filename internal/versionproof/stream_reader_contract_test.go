package versionproof

import (
 "crypto/sha256"
 "errors"
 "io"
 "testing"
)

// scriptedReader returns explicit io.Reader outcomes, including adversarial
// combinations that ordinary bytes.Reader does not produce.
type scriptedStep struct { data string; err error }
type scriptedReader struct { steps []scriptedStep; calls int }
func (r *scriptedReader) Read(p []byte) (int,error) {
 if r.calls >= len(r.steps) { return 0, io.EOF }
 s:=r.steps[r.calls]; r.calls++
 return copy(p, s.data),s.err
}

func TestVerifyStreamReaderContractBoundaries(t *testing.T) {
 bad:=errors.New("synthetic read failure")
 cases:=[]struct{
  name string
  steps []scriptedStep
  wantValid bool
 }{
  {"terminal EOF after multiple reads",[]scriptedStep{{"ab",nil},{"cd",io.EOF},{"",io.EOF}},true},
  {"terminal EOF with full single read",[]scriptedStep{{"abcd",io.EOF},{"",io.EOF}},true},
  {"ordinary EOF after multiple reads",[]scriptedStep{{"ab",nil},{"cd",nil},{"",io.EOF}},true},
  {"EOF before pinned length",[]scriptedStep{{"ab",io.EOF}},false},
  {"non-EOF error with final bytes",[]scriptedStep{{"abcd",bad}},false},
  {"trailing byte on EOF probe",[]scriptedStep{{"abcd",nil},{"x",io.EOF}},false},
  {"probe returns zero nil",[]scriptedStep{{"abcd",nil},{"",nil}},false},
  {"zero-progress during body",[]scriptedStep{{"ab",nil},{"",nil}},false},
 }
 for _,tc:=range cases {
  t.Run(tc.name,func(t *testing.T){
   r:=&scriptedReader{steps:tc.steps}
   pin:=Pin{Version:"v1",Size:4,Digest:sha256.Sum256([]byte("abcd"))}
   err:=VerifyStream(pin,"v1",r,4)
   if tc.wantValid && err!=nil {t.Fatalf("expected valid, got %v",err)}
   if !tc.wantValid && !errors.Is(err,ErrInvalid) {t.Fatalf("expected ErrInvalid, got %v",err)}
  })
 }
}
