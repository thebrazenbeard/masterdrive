package versioncontract

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
)

func mustPool(t *testing.T, bytes int64, count int) *CapturePool {
	t.Helper()
	p, err := NewCapturePool(bytes, count)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCapturePoolInvalidConfigurationAndInputs(t *testing.T) {
	for _, tc := range []struct {
		bytes int64
		count int
	}{{0, 1}, {1, 1}, {10, 0}, {-1, 2}} {
		if _, err := NewCapturePool(tc.bytes, tc.count); err == nil {
			t.Fatalf("accepted invalid configuration %+v", tc)
		}
	}
	p := mustPool(t, 10, 1)
	if _, err := p.Capture(nil, strings.NewReader("x"), 1); err == nil {
		t.Fatal("nil context accepted")
	}
	if _, err := p.Capture(context.Background(), nil, 1); err == nil {
		t.Fatal("nil source accepted")
	}
	if _, err := p.Capture(context.Background(), strings.NewReader("x"), 0); err == nil {
		t.Fatal("zero limit accepted")
	}
	if _, err := p.Capture(context.Background(), strings.NewReader("x"), MaxCaptureBytes+1); err == nil {
		t.Fatal("oversize limit accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := p.Capture(ctx, strings.NewReader("x"), 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled context: %v", err)
	}
	if got := p.Usage(); got != (PoolUsage{}) {
		t.Fatalf("invalid inputs reserved resources: %+v", got)
	}
	if _, err := (*CapturePool)(nil).Capture(context.Background(), strings.NewReader("x"), 1); err == nil {
		t.Fatal("nil pool accepted capture")
	}
}

func TestCapturePoolReservesBeforeReadAndReleasesOnError(t *testing.T) {
	p := mustPool(t, 5, 1)
	_, err := p.Capture(context.Background(), strings.NewReader("123456"), 4)
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("expected oversize error: %v", err)
	}
	if got := p.Usage(); got != (PoolUsage{}) {
		t.Fatalf("failed capture leaked reservation: %+v", got)
	}
	if _, err := p.Capture(context.Background(), strings.NewReader(""), 5); !errors.Is(err, ErrBudgetExhausted) {
		t.Fatalf("unreserved overflow byte was accepted: %v", err)
	}
	s, err := p.Capture(context.Background(), strings.NewReader("abc"), 4)
	if err != nil {
		t.Fatal(err)
	}
	if got := p.Usage(); got.ReservedBytes != 5 || got.Reservations != 1 {
		t.Fatalf("incorrect reservation: %+v", got)
	}
	if s.Size() != 3 || s.ETag() == "" {
		t.Fatalf("invalid captured metadata: %d %s", s.Size(), s.ETag())
	}
	if _, err := p.Capture(context.Background(), strings.NewReader("x"), 1); !errors.Is(err, ErrBudgetExhausted) {
		t.Fatalf("concurrent admission was not rejected: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if got := p.Usage(); got != (PoolUsage{}) {
		t.Fatalf("close failed to release reservation: %+v", got)
	}
	if _, err := s.OpenRange(s.ETag(), 0, 0); !errors.Is(err, ErrStale) {
		t.Fatalf("closed snapshot reopened: %v", err)
	}
}

func TestCapturePoolReaderLeaseRetainsBudgetAfterSnapshotClose(t *testing.T) {
	p := mustPool(t, 5, 1)
	s, err := p.Capture(context.Background(), strings.NewReader("abcd"), 4)
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.OpenRange(s.ETag(), 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.OpenRange("wrong", 0, 1); !errors.Is(err, ErrStale) {
		t.Fatalf("stale validator accepted: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.OpenRange(s.ETag(), 0, 1); !errors.Is(err, ErrStale) {
		t.Fatalf("new read after close: %v", err)
	}
	if _, err := p.Capture(context.Background(), strings.NewReader("x"), 1); !errors.Is(err, ErrBudgetExhausted) {
		t.Fatalf("reader lease allowed premature budget reuse: %v", err)
	}
	got, err := io.ReadAll(r)
	if err != nil || string(got) != "bc" {
		t.Fatalf("in-flight reader lost immutable bytes: %q %v", got, err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if usage := p.Usage(); usage != (PoolUsage{}) {
		t.Fatalf("reader close leaked reservation: %+v", usage)
	}
	next, err := p.Capture(context.Background(), strings.NewReader("x"), 1)
	if err != nil {
		t.Fatalf("capacity not restored: %v", err)
	}
	defer next.Close()
}

func TestCapturePoolConcurrentOpenAndClose(t *testing.T) {
	p := mustPool(t, 8193, 1)
	s, err := p.Capture(context.Background(), strings.NewReader(strings.Repeat("abcd", 2048)), 8192)
	if err != nil {
		t.Fatal(err)
	}
	tag := s.ETag()
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 30; j++ {
				r, err := s.OpenRange(tag, 0, 4)
				if errors.Is(err, ErrStale) {
					return
				}
				if err != nil {
					t.Errorf("open: %v", err)
					return
				}
				b, err := io.ReadAll(r)
				if err != nil || string(b) != "abcd" {
					t.Errorf("read: %q %v", b, err)
				}
				if err := r.Close(); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	if usage := p.Usage(); usage != (PoolUsage{}) {
		t.Fatalf("concurrent close leaked budget: %+v", usage)
	}
}
