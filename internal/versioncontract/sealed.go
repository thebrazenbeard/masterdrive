// Package versioncontract contains source-only M0 primitives for future
// version-pinned reads. It is not wired into MasterDrive's agent/controller.
// Capturing bytes is not a native filesystem point-in-time snapshot.
package versioncontract

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

const MaxCaptureBytes int64 = 16 << 20

var (
	ErrTooLarge = errors.New("m0 capture exceeds byte limit")
	ErrStale    = errors.New("m0 representation differs from requested version")
)

// Sealed owns a private copy of bytes and exposes independent read handles.
// After successful capture, subsequent mutations of the original source do
// not affect its representation. This does NOT mean the captured bytes were
// taken atomically from a mutable filesystem or represent a real OS snapshot.
type Sealed struct {
	data []byte
	etag string
}

// Capture reads at most limit+1 bytes from an already-authorized source and
// retains a bounded in-memory copy. It returns no partial snapshot on errors.
// The caller must close/cancel blocking source readers; context cancellation
// can be checked between reads but cannot interrupt an arbitrary blocked Read.
// A production cache additionally needs global quotas and owner revocation.
func Capture(ctx context.Context, source io.Reader, limit int64) (*Sealed, error) {
	if ctx == nil || source == nil {
		return nil, errors.New("nil context or source")
	}
	if limit < 1 || limit > MaxCaptureBytes {
		return nil, fmt.Errorf("capture limit must be 1..%d", MaxCaptureBytes)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(&cancelReader{ctx: ctx, reader: source}, limit+1))
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, ErrTooLarge
	}
	sum := sha256.Sum256(data)
	return &Sealed{
		data: data,
		etag: `"m0-sha256-` + hex.EncodeToString(sum[:]) + `"`,
	}, nil
}

// ETag is a syntactically strong HTTP entity-tag for exactly the sealed bytes.
// Its unsalted digest discloses equality and permits known-content guessing;
// do not expose it on sensitive shares without a documented privacy policy.
func (s *Sealed) ETag() string { return s.etag }
func (s *Sealed) Size() int64  { return int64(len(s.data)) }

// Open succeeds only for the exact sealed representation. Each reader has
// independent seek position. There is no mutable reader handle to the source.
func (s *Sealed) Open(expectedETag string) (io.ReadSeekCloser, error) {
	if s == nil || expectedETag == "" || expectedETag != s.etag {
		return nil, ErrStale
	}
	return &sealedReader{Reader: bytes.NewReader(s.data)}, nil
}

type sealedReader struct{ *bytes.Reader }

func (*sealedReader) Close() error { return nil }

type cancelReader struct {
	ctx        context.Context
	reader     io.Reader
	emptyReads int
}

func (r *cancelReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := r.reader.Read(p)
	if canceled := r.ctx.Err(); canceled != nil {
		return 0, canceled
	}
	if len(p) > 0 && n == 0 && err == nil {
		r.emptyReads++
		if r.emptyReads >= 100 {
			return 0, io.ErrNoProgress
		}
	} else if n > 0 {
		r.emptyReads = 0
	}
	return n, err
}
