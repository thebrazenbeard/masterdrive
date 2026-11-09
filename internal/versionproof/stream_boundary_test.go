package versionproof

import (
 "bytes"
 "crypto/sha256"
 "errors"
 "io"
 "testing"
)

func TestVerifyStreamEmptyRepresentation(t *testing.T) {
 pin := Pin{Size: 0, Digest: sha256.Sum256(nil), Version: "empty-v1"}
 if err := VerifyStream(pin, pin.Version, bytes.NewReader(nil), 0); err != nil { t.Fatalf("valid empty representation rejected: %v", err) }
 if err := VerifyStream(pin, pin.Version, bytes.NewReader([]byte("x")), 0); !errors.Is(err, ErrInvalid) { t.Fatalf("trailing content accepted: %v", err) }
 bad := pin
 bad.Digest = sha256.Sum256([]byte("not empty"))
 if err := VerifyStream(bad, bad.Version, bytes.NewReader(nil), 0); !errors.Is(err, ErrInvalid) { t.Fatalf("incorrect empty digest accepted: %v", err) }
 if err := VerifyStream(pin, pin.Version, noEOF{bytes.NewReader(nil)}, 0); !errors.Is(err, ErrInvalid) { t.Fatalf("unproven EOF accepted: %v", err) }
}

type dataAndEOF struct{ sent bool }
func (r *dataAndEOF) Read(p []byte) (int, error) {
 if r.sent { return 0, io.EOF }
 r.sent = true
 p[0] = 'x'
 return 1, io.EOF
}
func TestVerifyStreamRejectsTrailingDataWithEOF(t *testing.T) {
 pin := Pin{Size: 0, Digest: sha256.Sum256(nil), Version: "v1"}
 if err := VerifyStream(pin, "v1", &dataAndEOF{}, 1); !errors.Is(err, ErrInvalid) { t.Fatalf("trailing data with EOF accepted: %v", err) }
}
