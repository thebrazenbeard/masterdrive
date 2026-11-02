package readcontract

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"
)

func boundedFixture(data []byte) (ExpectedRange, *http.Response) {
	w, r := fixture()
	w.Length = int64(len(data))
	r.ContentLength = w.Length
	r.Header.Set("Content-Length", "20")
	r.Body = io.NopCloser(bytes.NewReader(data))
	return w, r
}

func readDeadline() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 3*time.Second)
}

func TestReadBoundedPartialExact(t *testing.T) {
	data := []byte("01234567890123456789")
	w, r := boundedFixture(data)
	ctx, cancel := readDeadline()
	defer cancel()
	got, err := ReadBoundedPartial(ctx, w, r, 20, make(chan struct{}))
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestReadBoundedPartialRejectsUnsafeBodies(t *testing.T) {
	cases := []struct {
		name   string
		data   []byte
		limit  int64
		change func(*http.Response)
	}{
		{"short", []byte("0123456789"), 20, nil},
		{"extra", []byte("01234567890123456789X"), 20, nil},
		{"quota", []byte("01234567890123456789"), 19, nil},
		{"wrong headers", []byte("01234567890123456789"), 20,
			func(r *http.Response) { r.Header.Set("ETag", "\"stale\"") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w, r := boundedFixture(tc.data)
			w.Length = 20
			r.ContentLength = 20
			if tc.change != nil {
				tc.change(r)
			}
			ctx, cancel := readDeadline()
			defer cancel()
			got, err := ReadBoundedPartial(ctx, w, r, tc.limit, make(chan struct{}))
			want := ErrUnsafeBody
			if tc.name == "wrong headers" {
				want = ErrUnsafePartial
			}
			if !errors.Is(err, want) || got != nil {
				t.Fatalf("expected %v and no bytes, got %q %v", want, got, err)
			}
		})
	}
}

func TestReadBoundedPartialPreconditions(t *testing.T) {
	data := []byte("01234567890123456789")
	tests := []struct {
		name    string
		ctx     context.Context
		revoked <-chan struct{}
		max     int64
	}{
		{"nil context", nil, make(chan struct{}), 20},
		{"missing deadline", context.Background(), make(chan struct{}), 20},
		{"missing revocation", context.Background(), nil, 20},
		{"zero budget", context.Background(), make(chan struct{}), 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w, r := boundedFixture(data)
			got, err := ReadBoundedPartial(tc.ctx, w, r, tc.max, tc.revoked)
			if !errors.Is(err, ErrUnsafeBody) || got != nil {
				t.Fatalf("got %q, %v", got, err)
			}
		})
	}
}

func TestReadBoundedPartialAlreadyRevoked(t *testing.T) {
	w, r := boundedFixture([]byte("01234567890123456789"))
	revoked := make(chan struct{})
	close(revoked)
	ctx, cancel := readDeadline()
	defer cancel()
	got, err := ReadBoundedPartial(ctx, w, r, 20, revoked)
	if !errors.Is(err, ErrRevoked) || got != nil {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestReadBoundedPartialRevocationInterruptsPipe(t *testing.T) {
	w, r := boundedFixture([]byte("01234567890123456789"))
	reader, writer := io.Pipe()
	defer writer.Close()
	r.Body = reader
	revoked := make(chan struct{})
	ctx, cancel := readDeadline()
	defer cancel()
	result := make(chan error, 1)
	go func() {
		got, err := ReadBoundedPartial(ctx, w, r, 20, revoked)
		if got != nil {
			result <- errors.New("returned partial data")
			return
		}
		result <- err
	}()
	if _, err := writer.Write([]byte("first")); err != nil {
		t.Fatal(err)
	}
	close(revoked)
	select {
	case err := <-result:
		if !errors.Is(err, ErrRevoked) {
			t.Fatalf("expected revocation, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("revocation did not interrupt pipe read")
	}
}

func TestReadBoundedPartialCancellationInterruptsPipe(t *testing.T) {
	w, r := boundedFixture([]byte("01234567890123456789"))
	reader, writer := io.Pipe()
	defer writer.Close()
	r.Body = reader
	ctx, cancel := readDeadline()
	revoked := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		got, err := ReadBoundedPartial(ctx, w, r, 20, revoked)
		if got != nil {
			result <- errors.New("returned partial data")
			return
		}
		result <- err
	}()
	if _, err := writer.Write([]byte("first")); err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected cancellation, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancellation did not interrupt pipe read")
	}
}

type emptyBody struct {
	mu     sync.Mutex
	closed bool
}

func (b *emptyBody) Read(_ []byte) (int, error) { return 0, nil }
func (b *emptyBody) Close() error {
	b.mu.Lock()
	b.closed = true
	b.mu.Unlock()
	return nil
}

func TestReadBoundedPartialNoProgressAndClose(t *testing.T) {
	w, r := boundedFixture([]byte("01234567890123456789"))
	body := &emptyBody{}
	r.Body = body
	ctx, cancel := readDeadline()
	defer cancel()
	got, err := ReadBoundedPartial(ctx, w, r, 20, make(chan struct{}))
	if !errors.Is(err, ErrUnsafeBody) || got != nil {
		t.Fatalf("got %q, %v", got, err)
	}
	body.mu.Lock()
	closed := body.closed
	body.mu.Unlock()
	if !closed {
		t.Fatal("body not closed")
	}
}

func TestReadBoundedPartialRejectsOversizeWindow(t *testing.T) {
	w, r := fixture()
	w.Length = MaxBufferedPartial + 1
	w.Size = w.Length + w.Offset
	r.Body = io.NopCloser(bytes.NewReader(nil))
	ctx, cancel := readDeadline()
	defer cancel()
	got, err := ReadBoundedPartial(ctx, w, r, MaxBufferedPartial+1, make(chan struct{}))
	if !errors.Is(err, ErrUnsafeBody) || got != nil {
		t.Fatalf("got %q, %v", got, err)
	}
}
func TestReadBoundedPartialRevocationAtEOFProbe(t *testing.T) {
	w, r := boundedFixture([]byte("01234567890123456789"))
	reader, writer := io.Pipe()
	defer writer.Close()
	r.Body = reader
	revoked := make(chan struct{})
	ctx, cancel := readDeadline()
	defer cancel()
	result := make(chan error, 1)
	go func() {
		got, err := ReadBoundedPartial(ctx, w, r, 20, revoked)
		if got != nil {
			result <- errors.New("returned data despite revocation")
			return
		}
		result <- err
	}()
	if _, err := writer.Write([]byte("01234567890123456789")); err != nil {
		t.Fatal(err)
	}
	close(revoked)
	select {
	case err := <-result:
		if !errors.Is(err, ErrRevoked) {
			t.Fatalf("got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("revocation did not interrupt EOF probe")
	}
}

func TestReadBoundedPartialDeadline(t *testing.T) {
	w, r := boundedFixture([]byte("01234567890123456789"))
	reader, writer := io.Pipe()
	defer writer.Close()
	r.Body = reader
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	got, err := ReadBoundedPartial(ctx, w, r, 20, make(chan struct{}))
	if !errors.Is(err, context.DeadlineExceeded) || got != nil {
		t.Fatalf("got %q, %v", got, err)
	}
}
