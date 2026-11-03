package versioncontract

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
)

var (
	ErrBudgetExhausted = errors.New("m0 capture budget exhausted")
	ErrReleased        = errors.New("m0 capture lease released")
	ErrNoProgress      = errors.New("m0 source reader made no progress")
)

// Budget is a source-only admission fence for retained sealed representations.
// It bounds aggregate logical representation bytes and the number of captures,
// including work still reading its source. It does not measure Go heap overhead,
// qualify filesystem snapshot coherence, or create a host-wide service.
type Budget struct {
	*budgetState
}

// Copies of the exported Budget handle share the same accounting state.
// That prevents admission bypass when a caller passes Budget by value.
type budgetState struct {
	mu         sync.Mutex
	maxBytes   int64
	maxObjects int
	usedBytes  int64
	objects    int
}

func NewBudget(maxBytes int64, maxObjects int) (*Budget, error) {
	if maxBytes < 2 || maxObjects <= 0 {
		return nil, fmt.Errorf("budget requires at least two bytes and a positive object count")
	}
	return &Budget{budgetState: &budgetState{maxBytes: maxBytes, maxObjects: maxObjects}}, nil
}

// Usage returns current reservations, including captures in progress and
// released leases whose readers are still open.
func (b *Budget) Usage() (bytes int64, objects int) {
	if b == nil || b.budgetState == nil {
		return 0, 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.usedBytes, b.objects
}

func (b *Budget) reserve(limit int64) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.objects >= b.maxObjects || limit > b.maxBytes-b.usedBytes {
		return ErrBudgetExhausted
	}
	b.usedBytes += limit
	b.objects++
	return nil
}

func (b *Budget) shrink(reserved, actual int64) {
	b.mu.Lock()
	b.usedBytes -= reserved - actual
	b.mu.Unlock()
}

func (b *Budget) returnQuota(reserved int64) {
	b.mu.Lock()
	b.usedBytes -= reserved
	b.objects--
	b.mu.Unlock()
}

// Capture pre-reserves the requested per-object limit plus one probe byte
// before reading source, matching the underlying Capture's maximum read.
// A successful capture shrinks its reservation to the exact retained size.
// Call Release to retire the lease; capacity is returned only after all
// independently opened readers are closed. Source.Read may still block despite
// ctx cancellation, as in the underlying Capture primitive.
func (b *Budget) Capture(ctx context.Context, source io.Reader, limit int64) (*Lease, error) {
	if b == nil || b.budgetState == nil {
		return nil, errors.New("nil budget")
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
	if err := b.reserve(reservation); err != nil {
		return nil, err
	}
	sealed, err := Capture(ctx, &progressReader{reader: source}, limit)
	if err != nil {
		b.returnQuota(reservation)
		return nil, err
	}
	actual := sealed.Size()
	b.shrink(reservation, actual)
	return &Lease{leaseState: &leaseState{budget: b, sealed: sealed, reserved: actual}}, nil
}

// progressReader stops a pathological stream of (0, nil) reads. It cannot
// detect or interrupt a single blocking Read; callers own source cancellation.
type progressReader struct {
	reader io.Reader
	empty  int
}

func (r *progressReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n == 0 && err == nil && len(p) != 0 {
		r.empty++
		if r.empty >= 16 {
			return 0, ErrNoProgress
		}
	} else if n > 0 {
		r.empty = 0
	}
	return n, err
}

// Lease owns one immutable representation until Release and all open readers
// finish. Release prevents new opens, but does not destroy bytes already being
// read; an actual revocation mechanism must cancel transport work separately.
type Lease struct {
	*leaseState
}

// The accounting state is shared by all value copies of Lease. A caller can
// copy the exported handle; copied handles must never return the same quota
// twice or reopen a representation after any copy has released it.
type leaseState struct {
	mu        sync.Mutex
	budget    *Budget
	sealed    *Sealed
	reserved  int64
	readers   int
	released  bool
	reclaimed bool
}

func (l *Lease) Metadata() (etag string, size int64, err error) {
	if l == nil || l.leaseState == nil {
		return "", 0, ErrReleased
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.released || l.sealed == nil {
		return "", 0, ErrReleased
	}
	return l.sealed.ETag(), l.sealed.Size(), nil
}

func (l *Lease) Open(expectedETag string) (io.ReadSeekCloser, error) {
	if l == nil || l.leaseState == nil {
		return nil, ErrReleased
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.released || l.sealed == nil {
		return nil, ErrReleased
	}
	r, err := l.sealed.Open(expectedETag)
	if err != nil {
		return nil, err
	}
	l.readers++
	return &leaseReader{inner: r, lease: l}, nil
}

func (l *Lease) Release() {
	if l == nil || l.leaseState == nil {
		return
	}
	l.mu.Lock()
	if l.released {
		l.mu.Unlock()
		return
	}
	l.released = true
	returnNow := l.readers == 0 && !l.reclaimed
	if returnNow {
		l.reclaimed = true
		l.sealed = nil
	}
	reserved := l.reserved
	l.mu.Unlock()
	if returnNow {
		l.budget.returnQuota(reserved)
	}
}

func (l *Lease) readerClosed() {
	l.mu.Lock()
	l.readers--
	returnNow := l.released && l.readers == 0 && !l.reclaimed
	if returnNow {
		l.reclaimed = true
		l.sealed = nil
	}
	reserved := l.reserved
	l.mu.Unlock()
	if returnNow {
		l.budget.returnQuota(reserved)
	}
}

type leaseReader struct {
	mu     sync.Mutex
	inner  io.ReadSeekCloser
	lease  *Lease
	closed bool
}

func (r *leaseReader) Read(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return 0, ErrReleased
	}
	return r.inner.Read(p)
}

func (r *leaseReader) Seek(offset int64, whence int) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return 0, ErrReleased
	}
	return r.inner.Seek(offset, whence)
}

func (r *leaseReader) Close() error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true
	inner := r.inner
	r.inner = nil
	r.mu.Unlock()
	err := inner.Close()
	r.lease.readerClosed()
	return err
}
