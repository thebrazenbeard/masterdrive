package boundedread

import (
 "crypto/sha256"
 "crypto/subtle"
 "errors"
 "io"
)

var ErrDigest = errors.New("boundedread: response digest mismatch")

// ReadExactSHA256 checks a bounded range against a trusted, independently
// authenticated SHA-256 digest of the exact requested bytes. It returns no
// bytes on a mismatch. A digest supplied by the same untrusted peer is not
// authentication; the caller must pin it from separate trusted authority.
// Like ReadExact, this does not enforce network deadlines or cancellation.
func ReadExactSHA256(body io.Reader, expected, limit int64, trustedDigest [sha256.Size]byte) ([]byte, error) {
 data, err := ReadExact(body, expected, limit)
 if err != nil { return nil, err }
 actual := sha256.Sum256(data)
 if subtle.ConstantTimeCompare(actual[:], trustedDigest[:]) != 1 {
  return nil, ErrDigest
 }
 return data, nil
}
