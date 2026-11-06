package mountcontract

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
)

// A shared generation authority must serialize concurrent checks and poison
// all subsequent staged reads after detecting a rollback.
func TestConcurrentStagedReadsFailAfterGenerationRollback(t *testing.T) {
	var mu sync.Mutex
	generation := uint64(12)
	source := func() (uint64, error) { mu.Lock(); defer mu.Unlock(); return generation, nil }
	guard := NewMonotonicGeneration(source)
	headers := http.Header{"Etag": {"\"stable\""}, "Content-Range": {"bytes 0-2/3"}, "Content-Length": {"3"}}
	read := func() ([]byte, error) {
		return StageGenerationPinnedRange(context.Background(), guard.Check, "\"stable\"", 206, headers, bytes.NewReader([]byte("abc")), 0, 2, 3)
	}
	if b, err := read(); err != nil || string(b) != "abc" {
		t.Fatalf("initial read: %q %v", b, err)
	}
	mu.Lock()
	generation = 11
	mu.Unlock()
	if b, err := read(); b != nil || !errors.Is(err, ErrReadNotAuthorized) {
		t.Fatalf("rollback: %q %v", b, err)
	}
	mu.Lock()
	generation = 13
	mu.Unlock()
	const callers = 64
	var wg sync.WaitGroup
	errs := make(chan error, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b, err := read()
			if b != nil || !errors.Is(err, ErrReadNotAuthorized) {
				errs <- errors.New("poisoned authority released bytes or wrong error")
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}
