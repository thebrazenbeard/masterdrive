package versionproof

import (
 "crypto/sha256"
 "io"
 "testing"
)

// terminalOnlyReader fails the test if verification reads again after a
// complete data+EOF response. A network reader could block on that call.
type terminalOnlyReader struct {
 data []byte
 calls int
}
func (r *terminalOnlyReader) Read(p []byte) (int, error) {
 r.calls++
 if r.calls > 1 { panic("redundant read after terminal EOF") }
 return copy(p,r.data),io.EOF
}
func TestVerifyStreamTerminalEOFDoesNotProbeAgain(t *testing.T) {
 for _, data := range []string{"a", "abcd"} {
  t.Run(data,func(t *testing.T){
   r:=&terminalOnlyReader{data:[]byte(data)}
   pin:=Pin{Version:"v1",Size:uint64(len(data)),Digest:sha256.Sum256([]byte(data))}
   if err:=VerifyStream(pin,"v1",r,1024); err!=nil { t.Fatal(err) }
   if r.calls!=1 { t.Fatalf("expected one Read, got %d",r.calls) }
  })
 }
}
