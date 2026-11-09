// Package versionproof validates complete-file content against a trusted version pin.
package versionproof

import (
 "crypto/sha256"
 "crypto/subtle"
 "errors"
 "fmt"
)

var ErrInvalid = errors.New("versionproof: invalid evidence")

// Pin is obtained from an independently authenticated source, never from the read response.
type Pin struct { Size uint64; Digest [32]byte; Version string }

// VerifyWhole checks a complete representation against a previously trusted pin.
// It does not establish peer identity, source immutability or grant validity.
func VerifyWhole(pin Pin, version string, content []byte) error {
 if pin.Version == "" || version == "" || pin.Version != version { return fmt.Errorf("%w: version mismatch", ErrInvalid) }
 if uint64(len(content)) != pin.Size { return fmt.Errorf("%w: size mismatch", ErrInvalid) }
 digest := sha256.Sum256(content)
 if subtle.ConstantTimeCompare(digest[:], pin.Digest[:]) != 1 { return fmt.Errorf("%w: digest mismatch", ErrInvalid) }
 return nil
}

// CheckRangeBounds validates overflow-safe half-open offsets. It does not
// prove integrity of a range or permit an untrusted peer to select the pin.
func CheckRangeBounds(pin Pin, offset, length uint64) error {
 if pin.Version == "" || length == 0 || offset >= pin.Size || length > pin.Size-offset { return ErrInvalid }
 return nil
}
