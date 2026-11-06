// Package mountcontract enforces version preconditions for future read-only mounts.
package mountcontract

import (
	"errors"
	"net/http"
	"strings"
)

var ErrUnsafeVersion = errors.New("unsafe source version")
var ErrVersionChanged = errors.New("source version changed")
var ErrAmbiguousVersion = errors.New("ambiguous source version")

// StrongETag accepts only a nonempty quoted strong HTTP entity tag.
func StrongETag(s string) bool {
	if len(s) < 3 || s[0] != '"' || s[len(s)-1] != '"' {
		return false
	}
	for i := 1; i < len(s)-1; i++ {
		c := s[i]
		if c < 0x21 || c == 0x22 || c == 0x7f {
			return false
		}
	}
	return true
}

// CheckReadVersion must pass before exposing bytes. Caller separately checks
// authorization, response status, range, length and peer identity.
func CheckReadVersion(pin string, h http.Header) error {
	if !StrongETag(pin) {
		return ErrUnsafeVersion
	}
	v := h.Values("ETag")
	if len(v) != 1 || strings.Contains(v[0], ",") {
		return ErrAmbiguousVersion
	}
	if !StrongETag(v[0]) {
		return ErrUnsafeVersion
	}
	if v[0] != pin {
		return ErrVersionChanged
	}
	return nil
}
