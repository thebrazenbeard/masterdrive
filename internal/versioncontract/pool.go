package versioncontract

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
)

// ErrBudgetExhausted means the caller must wait, release an existing snapshot,
// or explicitly increase its owner-approved resource allowance.
var ErrBudgetExhausted = errors.New("m0 capture pool capacity exhausted")

// CapturePool bounds the aggregate reserved payload capacity and the number of
// captures/snapshots managed by this instance. Every in-flight capture reserves
// limit+1 bytes until its Snapshot is closed AND all opened readers are closed.
//
// This is logical payload accounting, NOT a hard Go heap/RSS limit: io.ReadAll,
// source readers, the allocator, and other pools can consume additional memory.
// A production broker must enforce process-wide budgets and owner authorization.
type CapturePool struct {
	mu           sync.Mutex
	maxBytes     int64
	maxSnapshots int
	reserved     int64
	count        int
}

type PoolUsage struct {
	ReservedBytes int64
	Reservations  int
}

func NewCapturePool(maxBytes int64, maxSnapshots int) (*CapturePool, error) {
	if maxBytes < 2 || maxSnapshots < 1 {
		return nil, errors.New("m0 capture pool requires positive snapshot count and at least two bytes")
	}
	return &CapturePool{maxBytes: maxBytes, maxSnapshots: maxSnapshots}, nil
}

func (p *CapturePool) Usage() PoolUsage {
	if p == nil {
		return PoolUsage{}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return PoolUsage{ReservedBytes: p.reserved, Reservations: p.count}
}

// Capture reserves the entire requested limit plus the overflow-detection
// byte BEFORE reading. Admission is non-blocking and fails closed if capacity
// is unavailable. On every capture error the reservation is released.
//
// The returned snapshot must be closed, and every reader opened from it must
// be closed, before the reservation is available for another capture.
func (p *CapturePool) Capture(ctx context.Context, source io.Reader, limit int64) (*Snapshot, error) {
	if p == nil {
		return nil, errors.New("nil m0 capture pool")
	}
	if ctx == nil || source == nil {
		return nil, errors.New("nil context or source")
	}
	if limit < 1 || limit > MaxCaptureBytes {
		return nil, fmt.Errorf("capture limit must be 1..%d", MaxCaptureBytes)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	reservation := limit + 1
	p.mu.Lock()
	if p.count >= p.maxSnapshots || reservation > p.maxBytes-p.reserved {
		p.mu.Unlock()
		return nil, ErrBudgetExhausted
	}
	p.reserved += reservation
	p.count++
	p.mu.Unlock()

	sealed, err := Capture(ctx, source, limit)
	if err != nil {
		p.mu.Lock()
		p.reserved -= reservation
		p.count--
		p.mu.Unlock()
		return nil, err
	}
	return &Snapshot{pool: p, sealed: sealed, reservation: reservation}, nil
}

// Snapshot is a pool-accounted immutable representation. Closing it prevents
// new opens, but already-open readers remain readable until they are closed.
// Close is NOT a live authorization revocation or stream-cancellation fence.
type Snapshot struct {
	pool        *CapturePool
	sealed      *Sealed
	reservation int64
	readers     int
	closed      bool
}

func (s *Snapshot) ETag() string {
	if s == nil || s.pool == nil {
		return ""
	}
	s.pool.mu.Lock()
	defer s.pool.mu.Unlock()
	if s.sealed == nil {
		return ""
	}
	return s.sealed.ETag()
}

func (s *Snapshot) Size() int64 {
	if s == nil || s.pool == nil {
		return 0
	}
	s.pool.mu.Lock()
	defer s.pool.mu.Unlock()
	if s.sealed == nil {
		return 0
	}
	return s.sealed.Size()
}

func (s *Snapshot) OpenRange(expectedETag string, offset, length int64) (io.ReadCloser, error) {
	if s == nil || s.pool == nil {
		return nil, ErrStale
	}
	s.pool.mu.Lock()
	defer s.pool.mu.Unlock()
	if s.closed || s.sealed == nil {
		return nil, ErrStale
	}
	r, err := s.sealed.OpenRange(expectedETag, offset, length)
	if err != nil {
		return nil, err
	}
	s.readers++
	return &poolReader{inner: r, snapshot: s}, nil
}

func (s *Snapshot) retireLocked() {
	if s.closed && s.readers == 0 && s.reservation != 0 {
		s.pool.reserved -= s.reservation
		s.pool.count--
		s.reservation = 0
		s.sealed = nil
	}
}

func (s *Snapshot) Close() error {
	if s == nil || s.pool == nil {
		return nil
	}
	s.pool.mu.Lock()
	defer s.pool.mu.Unlock()
	s.closed = true
	s.retireLocked()
	return nil
}

type poolReader struct {
	inner    io.ReadCloser
	snapshot *Snapshot
	mu       sync.Mutex
	closed   bool
	closeErr error
}

// Read and Close serialize on this reader. Quota cannot be released while a
// Read is still in progress; Close does not interrupt a blocked Read.
func (r *poolReader) Read(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return 0, io.ErrClosedPipe
	}
	return r.inner.Read(p)
}

func (r *poolReader) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return r.closeErr
	}
	r.closed = true
	r.closeErr = r.inner.Close()
	p := r.snapshot.pool
	p.mu.Lock()
	r.snapshot.readers--
	r.snapshot.retireLocked()
	p.mu.Unlock()
	return r.closeErr
}