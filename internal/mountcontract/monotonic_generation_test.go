package mountcontract

import (
	"errors"
	"sync"
	"testing"
)

func TestMonotonicGenerationRejectsRollbackPermanently(t *testing.T) {
	var mu sync.Mutex
	current := uint64(10)
	gate := NewMonotonicGeneration(func() (uint64, error) { mu.Lock(); defer mu.Unlock(); return current, nil })
	if n, e := gate.Check(); e != nil || n != 10 {
		t.Fatalf("initial: %d %v", n, e)
	}
	mu.Lock()
	current = 11
	mu.Unlock()
	if n, e := gate.Check(); e != nil || n != 11 {
		t.Fatalf("advance: %d %v", n, e)
	}
	mu.Lock()
	current = 10
	mu.Unlock()
	if n, e := gate.Check(); n != 0 || !errors.Is(e, ErrReadNotAuthorized) {
		t.Fatalf("rollback: %d %v", n, e)
	}
	mu.Lock()
	current = 12
	mu.Unlock()
	if n, e := gate.Check(); n != 0 || !errors.Is(e, ErrReadNotAuthorized) {
		t.Fatalf("rollback recovery: %d %v", n, e)
	}
}
func TestMonotonicGenerationMissingSource(t *testing.T) {
	if _, e := NewMonotonicGeneration(nil).Check(); !errors.Is(e, ErrReadNotAuthorized) {
		t.Fatal(e)
	}
	var nilGate *MonotonicGeneration
	if _, e := nilGate.Check(); !errors.Is(e, ErrReadNotAuthorized) {
		t.Fatal(e)
	}
}
