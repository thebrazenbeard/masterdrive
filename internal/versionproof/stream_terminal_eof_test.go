package versionproof

import (
 "crypto/sha256"
 "errors"
 "io"
 "testing"
)

// terminalEOFReader follows the io.Reader contract: data and EOF may
// be returned together. A subsequent read must still confirm EOF.
type terminalEOFReader struct {
 data []byte
 delivered bool
}
func (r *terminalEOFReader) Read(p []byte) (int, error) {
 if r.delivered { return 0, io.EOF }
 r.delivered = true
 return copy(p, r.data), io.EOF
}

func TestVerifyStreamTerminalEOF(t *testing.T) {
 for _, tc := range []struct {
  name string
  data string
  size uint64
  valid bool
 }{
  {"exact data and EOF", "abc", 3, true},
  {"premature EOF with data", "ab", 3, false},
  {"empty payload with EOF", "", 0, true},
 } {
  t.Run(tc.name, func(t *testing.T) {
   pin := Pin{Version:"v1", Size:tc.size, Digest:sha256.Sum256([]byte("abc"))}
   if tc.size == 0 { pin.Digest = sha256.Sum256(nil) }
   err := VerifyStream(pin, "v1", &terminalEOFReader{data:[]byte(tc.data)}, 8)
   if tc.valid && err != nil { t.Fatalf("expected valid stream, got %v", err) }
   if !tc.valid && !errors.Is(err, ErrInvalid) { t.Fatalf("expected ErrInvalid, got %v", err) }
  })
 }
}
