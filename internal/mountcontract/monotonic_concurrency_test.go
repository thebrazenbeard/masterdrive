package mountcontract

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

func TestMonotonicGenerationConcurrentChecksSerializeSource(t *testing.T) {
	var active, peak atomic.Int32
	var generation atomic.Uint64
	guard := NewMonotonicGeneration(func() (uint64, error) {
		if active.Add(1) != 1 {
			peak.Store(2)
		}
		defer active.Add(-1)
		return generation.Add(1), nil
	})
	const workers = 64
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := guard.Check(); errs <- err }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent Check: %v", err)
		}
	}
	if peak.Load() != 0 {
		t.Fatal("generation source called concurrently")
	}
	if generation.Load() != workers {
		t.Fatalf("got %d calls, want %d", generation.Load(), workers)
	}
}

func TestMonotonicGenerationConcurrentRollbackPoison(t *testing.T) {
	var generation atomic.Uint64
	generation.Store(100)
	guard := NewMonotonicGeneration(func() (uint64, error) { return generation.Load(), nil })
	if _, err := guard.Check(); err != nil {
		t.Fatal(err)
	}
	generation.Store(99)
	if _, err := guard.Check(); !errors.Is(err, ErrReadNotAuthorized) {
		t.Fatalf("rollback: %v", err)
	}
	generation.Store(101)
	const workers = 32
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if v, err := guard.Check(); v != 0 || !errors.Is(err, ErrReadNotAuthorized) {
				t.Errorf("poisoned Check returned %d, %v", v, err)
			}
		}()
	}
	wg.Wait()
}
