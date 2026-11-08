// Package boundedread provides a bounded, fail-closed body reader for a single
// read-only range response. It does not authenticate peers or verify content hashes.
package boundedread

import (
 "errors"
 "io"
)

var ErrRangeBody = errors.New("boundedread: response body length mismatch")
var ErrLimit = errors.New("boundedread: requested range exceeds local limit")

// ReadExact reads a range into memory only when its declared size fits the
// caller's positive limit. A one-byte probe detects oversized responses.
// On any error, no partial response bytes are returned. The caller owns Close.
// Use with an HTTP transport configured to reject redirects and bound timeouts.
func ReadExact(body io.Reader, expected, limit int64) ([]byte, error) {
 if body == nil || expected <= 0 || limit <= 0 || expected > limit || expected > int64(int(^uint(0)>>1))-1 {
  return nil, ErrLimit
 }
 buf := make([]byte, int(expected))
 if _, err := io.ReadFull(body, buf); err != nil { return nil, errors.Join(ErrRangeBody, err) }
 var probe [1]byte
 n, err := body.Read(probe[:])
 if n != 0 || err == nil || err != io.EOF { 
  if err != nil && err != io.EOF { return nil, errors.Join(ErrRangeBody, err) }
  return nil, ErrRangeBody
 }
 return buf, nil
}
