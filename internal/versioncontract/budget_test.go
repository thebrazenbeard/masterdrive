package versioncontract

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
)

func TestBudgetRetainsQuotaUntilAllReadersClose(t *testing.T) {
	b, err := NewBudget(10, 1)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := b.Capture(context.Background(), strings.NewReader("abcd"), 6)
	if err != nil {
		t.Fatal(err)
	}
	if used, objects := b.Usage(); used != 4 || objects != 1 {
		t.Fatalf("usage after shrink = (%d,%d), want (4,1)", used, objects)
	}
	if _, err := b.Capture(context.Background(), strings.NewReader("e"), 1); !errors.Is(err, ErrBudgetExhausted) {
		t.Fatalf("second object incorrectly admitted: %v", err)
	}
	etag, size, err := lease.Metadata()
	if err != nil || size != 4 || !strings.HasPrefix(etag, `"m0-sha256-`) {
		t.Fatalf("metadata = (%q,%d,%v)", etag, size, err)
	}
	if _, err := lease.Open(`"wrong"`); !errors.Is(err, ErrStale) {
		t.Fatalf("stale version admitted: %v", err)
	}
	reader, err := lease.Open(etag)
	if err != nil {
		t.Fatal(err)
	}
	lease.Release()
	lease.Release() // idempotent and must not free a live reader's quota.
	if used, objects := b.Usage(); used != 4 || objects != 1 {
		t.Fatalf("quota prematurely released = (%d,%d)", used, objects)
	}
	if _, err := lease.Open(etag); !errors.Is(err, ErrReleased) {
		t.Fatalf("open after release: %v", err)
	}
	if got, err := io.ReadAll(reader); err != nil || string(got) != "abcd" {
		t.Fatalf("in-flight reader lost fixed bytes: %q %v", got, err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.Read(make([]byte, 1)); !errors.Is(err, ErrReleased) {
		t.Fatalf("closed reader remained readable: %v", err)
	}
	if _, err := reader.Seek(0, io.SeekStart); !errors.Is(err, ErrReleased) {
		t.Fatalf("closed reader remained seekable: %v", err)
	}
	if used, objects := b.Usage(); used != 0 || objects != 0 {
		t.Fatalf("quota leaked after final close = (%d,%d)", used, objects)
	}
	if _, _, err := lease.Metadata(); !errors.Is(err, ErrReleased) {
		t.Fatalf("metadata after release: %v", err)
	}
	next, err := b.Capture(context.Background(), strings.NewReader("ok"), 2)
	if err != nil {
		t.Fatalf("reclaim did not admit next object: %v", err)
	}
	next.Release()
}

func TestLeaseValueCopyCannotReclaimSameReservationTwice(t *testing.T) {
	b, err := NewBudget(8, 1)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := b.Capture(context.Background(), strings.NewReader("abcd"), 4)
	if err != nil {
		t.Fatal(err)
	}
	copyOfLease := *lease
	tag, _, err := copyOfLease.Metadata()
	if err != nil {
		t.Fatal(err)
	}
	reader, err := copyOfLease.Open(tag)
	if err != nil {
		t.Fatal(err)
	}
	lease.Release()
	copyOfLease.Release()
	if used, objects := b.Usage(); used != 4 || objects != 1 {
		t.Fatalf("duplicate release reclaimed active reader: (%d,%d)", used, objects)
	}
	if _, err := copyOfLease.Open(tag); !errors.Is(err, ErrReleased) {
		t.Fatalf("copied handle reopened released lease: %v", err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	if used, objects := b.Usage(); used != 0 || objects != 0 {
		t.Fatalf("copied handle double-released quota: (%d,%d)", used, objects)
	}
	next, err := b.Capture(context.Background(), strings.NewReader("new"), 3)
	if err != nil {
		t.Fatalf("budget failed after copied handle closure: %v", err)
	}
	next.Release()
}

func TestBudgetValueCopySharesReservations(t *testing.T) {
	b, err := NewBudget(8, 2)
	if err != nil {
		t.Fatal(err)
	}
	first, err := b.Capture(context.Background(), strings.NewReader("abcd"), 4)
	if err != nil {
		t.Fatal(err)
	}
	copyOfBudget := *b
	if used, objects := copyOfBudget.Usage(); used != 4 || objects != 1 {
		t.Fatalf("copied budget lost original accounting: (%d,%d)", used, objects)
	}
	if _, err := copyOfBudget.Capture(context.Background(), strings.NewReader("xyz"), 4); !errors.Is(err, ErrBudgetExhausted) {
		t.Fatalf("copied budget overcommitted retained bytes: %v", err)
	}
	first.Release()
	second, err := copyOfBudget.Capture(context.Background(), strings.NewReader("xyz"), 4)
	if err != nil {
		t.Fatalf("recovered copied budget did not admit capture: %v", err)
	}
	second.Release()
	if used, objects := b.Usage(); used != 0 || objects != 0 {
		t.Fatalf("copied budget leaked accounting: (%d,%d)", used, objects)
	}
}

// stalledReader permits inspection of an in-progress reservation. Capture
// cannot cancel an arbitrary blocked source Read; this test deliberately
// releases the reader rather than claiming it can.
type stalledReader struct {
	entered chan struct{}
	resume  chan struct{}
	once    sync.Once
}

func (r *stalledReader) Read(p []byte) (int, error) {
	r.once.Do(func() { close(r.entered) })
	<-r.resume
	n := copy(p, "hi")
	return n, io.EOF
}

func TestBudgetReservesBeforeReadingAndShrinksOnSuccess(t *testing.T) {
	b, err := NewBudget(9, 2)
	if err != nil {
		t.Fatal(err)
	}
	r := &stalledReader{entered: make(chan struct{}), resume: make(chan struct{})}
	done := make(chan struct {
		lease *Lease
		err   error
	}, 1)
	go func() {
		lease, err := b.Capture(context.Background(), r, 8)
		done <- struct {
			lease *Lease
			err   error
		}{lease, err}
	}()
	<-r.entered
	if used, objects := b.Usage(); used != 9 || objects != 1 {
		t.Fatalf("in-progress reservation = (%d,%d)", used, objects)
	}
	if _, err := b.Capture(context.Background(), strings.NewReader("x"), 1); !errors.Is(err, ErrBudgetExhausted) {
		t.Fatalf("overcommitted during source Read: %v", err)
	}
	close(r.resume)
	result := <-done
	if result.err != nil {
		t.Fatal(result.err)
	}
	if used, objects := b.Usage(); used != 2 || objects != 1 {
		t.Fatalf("successful capture did not shrink reservation: (%d,%d)", used, objects)
	}
	result.lease.Release()
	if used, objects := b.Usage(); used != 0 || objects != 0 {
		t.Fatalf("quota leak after asynchronous capture: (%d,%d)", used, objects)
	}
}

func TestBudgetReturnsReservationOnFailure(t *testing.T) {
	b, err := NewBudget(7, 1)
	if err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := b.Capture(canceled, strings.NewReader("a"), 3); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation = %v", err)
	}
	if _, err := b.Capture(context.Background(), bytes.NewReader([]byte("1234567")), 6); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("oversize = %v", err)
	}
	broken := errors.New("source failed")
	if _, err := b.Capture(context.Background(), failingReader{broken}, 6); !errors.Is(err, broken) {
		t.Fatalf("source failure = %v", err)
	}
	if _, err := b.Capture(context.Background(), noProgressReader{}, 6); !errors.Is(err, ErrNoProgress) {
		t.Fatalf("pathological no-progress source = %v", err)
	}
	if used, objects := b.Usage(); used != 0 || objects != 0 {
		t.Fatalf("reservation leaked on failure = (%d,%d)", used, objects)
	}
}

type failingReader struct{ err error }

func (r failingReader) Read([]byte) (int, error) { return 0, r.err }

type noProgressReader struct{}

func (noProgressReader) Read([]byte) (int, error) { return 0, nil }

func TestBudgetValidationAndConcurrentReleases(t *testing.T) {
	var neverAcquired Lease
	neverAcquired.Release()
	neverAcquired.Release()
	var noLease *Lease
	noLease.Release()
	if _, _, err := neverAcquired.Metadata(); !errors.Is(err, ErrReleased) {
		t.Fatalf("zero-value lease exposed metadata: %v", err)
	}
	for _, tc := range []struct {
		bytes   int64
		objects int
	}{{0, 1}, {1, 1}, {1, 0}, {-1, 1}, {1, -1}} {
		if _, err := NewBudget(tc.bytes, tc.objects); err == nil {
			t.Fatalf("accepted invalid budget %+v", tc)
		}
	}
	var absent *Budget
	if _, err := absent.Capture(context.Background(), strings.NewReader("x"), 1); err == nil {
		t.Fatal("nil budget admitted source")
	}
	b, err := NewBudget(16, 2)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.Capture(context.Background(), strings.NewReader("x"), 17); err == nil {
		t.Fatal("accepted per-object limit greater than global cap")
	}
	lease, err := b.Capture(context.Background(), strings.NewReader("valid"), 8)
	if err != nil {
		t.Fatal(err)
	}
	etag, _, err := lease.Metadata()
	if err != nil {
		t.Fatal(err)
	}
	readers := make([]io.ReadSeekCloser, 8)
	for i := range readers {
		readers[i], err = lease.Open(etag)
		if err != nil {
			t.Fatal(err)
		}
	}
	lease.Release()
	var wg sync.WaitGroup
	for _, reader := range readers {
		wg.Add(1)
		go func(r io.ReadSeekCloser) {
			defer wg.Done()
			_, _ = io.ReadAll(r)
			_ = r.Close()
			_ = r.Close()
		}(reader)
	}
	wg.Wait()
	if used, objects := b.Usage(); used != 0 || objects != 0 {
		t.Fatalf("concurrent close leaked quota = (%d,%d)", used, objects)
	}
}
