package versioncontract

import (
	"bytes"
	"errors"
	"io"
)

var ErrInvalidRange = errors.New("m0 range is outside sealed representation")

// OpenRange returns exactly length bytes from the requested immutable sealed
// representation. It never falls back to a different version or a live file.
// This is a source-only primitive: the caller MUST separately authorize the
// host/share/path and recheck revocation before every open and stream.
func (s *Sealed) OpenRange(expectedETag string, offset, length int64) (io.ReadCloser, error) {
	if s == nil || expectedETag == "" || expectedETag != s.etag {
		return nil, ErrStale
	}
	size := int64(len(s.data))
	// Subtract only after validating offset, avoiding offset+length overflow.
	if offset < 0 || length < 0 || offset > size || length > size-offset {
		return nil, ErrInvalidRange
	}
	return io.NopCloser(io.NewSectionReader(bytes.NewReader(s.data), offset, length)), nil
}
