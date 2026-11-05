package mountcontract

import (
	"errors"
	"sync/atomic"
	"testing"
)

func TestMonotonicGenerationSourcePanicPoisonsGuard(t *testing.T) {
	var calls atomic.Int32
	gate := NewMonotonicGeneration(func() (uint64, error) {
		calls.Add(1)
		panic("authority unavailable")
	})
	for i := 0; i < 3; i++ {
		n, err := gate.Check()
		if n != 0 || !errors.Is(err, ErrReadNotAuthorized) {
			t.Fatalf("check %d: generation=%d error=%v", i, n, err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("poisoned authority called %d times", calls.Load())
	}
}
