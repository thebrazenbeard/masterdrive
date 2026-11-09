package versionproof

import (
 "crypto/sha256"
 "crypto/subtle"
 "fmt"
 "io"
)

// VerifyStream checks a complete, bounded representation without buffering it.
// The caller must supply an independently trusted pin, a cancellation-aware
// reader, and transport deadlines. It does not authenticate the reader.
// maxBytes is a locally configured policy limit, not a peer-supplied value.
func VerifyStream(pin Pin, version string, reader io.Reader, maxBytes uint64) error {
 if reader == nil || pin.Version == "" || version == "" || pin.Version != version || pin.Size > maxBytes || pin.Size > uint64(^uint64(0)>>1) {
  return fmt.Errorf("%w: missing reader, version mismatch, or size exceeds limit", ErrInvalid)
 }
 h := sha256.New()
 // io.CopyN may spin forever on a broken Reader returning (0, nil).
 // Reject no-progress reads rather than treating them as transport liveness.
 var buf [32 * 1024]byte
 remaining := pin.Size
 terminalEOF := false
 for remaining > 0 {
  want := len(buf)
  if remaining < uint64(want) { want = int(remaining) }
  n, err := reader.Read(buf[:want])
  if n < 0 || n > want { return fmt.Errorf("%w: invalid reader count", ErrInvalid) }
  if n > 0 {
   _, _ = h.Write(buf[:n])
   remaining -= uint64(n)
  }
  // io.Reader permits returning data and io.EOF together. Consume the
  // bytes first, but only accept terminal EOF at the exact pinned length.
  if err != nil {
   if err != io.EOF || remaining != 0 { return fmt.Errorf("%w: short or failed stream: %v", ErrInvalid, err) }
   terminalEOF = true
  }
  if n == 0 { return fmt.Errorf("%w: reader made no progress", ErrInvalid) }
 }
 // A final (n>0, io.EOF) already establishes end-of-stream under
 // io.Reader's contract. Do not issue a second potentially blocking Read.
 if !terminalEOF {
  var extra [1]byte
  nExtra, probeErr := reader.Read(extra[:])
  if nExtra != 0 || probeErr != io.EOF {
   return fmt.Errorf("%w: trailing bytes or unverified end of stream", ErrInvalid)
  }
 }
 if subtle.ConstantTimeCompare(h.Sum(nil), pin.Digest[:]) != 1 {
  return fmt.Errorf("%w: digest mismatch", ErrInvalid)
 }
 return nil
}
