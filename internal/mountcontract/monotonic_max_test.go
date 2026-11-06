package mountcontract

import (
	"errors"
	"math"
	"testing"
)

func TestMonotonicGenerationAcceptsMaxUint64Repeatedly(t *testing.T) {
	calls := 0
	guard := NewMonotonicGeneration(func() (uint64, error) {
		calls++
		return math.MaxUint64, nil
	})
	for i := 0; i < 3; i++ {
		got, err := guard.Check()
		if err != nil || got != math.MaxUint64 {
			t.Fatalf("check %d: generation=%d err=%v", i, got, err)
		}
	}
	if calls != 3 {
		t.Fatalf("source called %d times, want 3", calls)
	}
}

func TestMonotonicGenerationPoisonDoesNotCallSourceAgain(t *testing.T) {
	calls := 0
	guard := NewMonotonicGeneration(func() (uint64, error) {
		calls++
		if calls == 1 {
			return 10, nil
		}
		if calls == 2 {
			return 9, nil
		}
		return 11, nil
	})
	if _, err := guard.Check(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err := guard.Check(); !errors.Is(err, ErrReadNotAuthorized) {
			t.Fatalf("check %d: expected denied, got %v", i, err)
		}
	}
	if calls != 2 {
		t.Fatalf("source called after poison: %d", calls)
	}
}
