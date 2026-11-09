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
 n, err := io.CopyN(h, reader, int64(pin.Size))
 if err != nil || uint64(n) != pin.Size {
  return fmt.Errorf("%w: short or failed stream: %v", ErrInvalid, err)
 }
 var extra [1]byte
 nExtra, probeErr := reader.Read(extra[:])
 if nExtra != 0 || probeErr != io.EOF {
  return fmt.Errorf("%w: trailing bytes or unverified end of stream", ErrInvalid)
 }
 if subtle.ConstantTimeCompare(h.Sum(nil), pin.Digest[:]) != 1 {
  return fmt.Errorf("%w: digest mismatch", ErrInvalid)
 }
 return nil
}
