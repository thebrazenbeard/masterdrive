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

func TestOpenRangeIsVersionPinnedAndExact(t *testing.T) {
	source := []byte("0123456789")
	s, err := Capture(context.Background(), bytes.NewReader(source), 10)
	if err != nil {
		t.Fatal(err)
	}
	source[3] = 'x'
	for _, tc := range []struct {
		off, length int64
		want        string
	}{
		{0, 4, "0123"}, {3, 4, "3456"}, {9, 1, "9"}, {10, 0, ""},
	} {
		r, err := s.OpenRange(s.ETag(), tc.off, tc.length)
		if err != nil {
			t.Fatalf("%+v: %v", tc, err)
		}
		b, err := io.ReadAll(r)
		if err != nil {
			t.Fatal(err)
		}
		if err := r.Close(); err != nil {
			t.Fatal(err)
		}
		if string(b) != tc.want {
			t.Fatalf("%+v: got %q", tc, b)
		}
	}
	for _, tc := range []struct{ off, length int64 }{
		{-1, 1}, {0, -1}, {11, 0}, {9, 2}, {1, int64(^uint64(0) >> 1)},
	} {
		r, err := s.OpenRange(s.ETag(), tc.off, tc.length)
		if r != nil || !errors.Is(err, ErrInvalidRange) {
			t.Fatalf("%+v: reader=%v err=%v", tc, r, err)
		}
	}
	if r, err := s.OpenRange("\"wrong\"", -1, 1); r != nil || !errors.Is(err, ErrStale) {
		t.Fatalf("stale version leaked range validity: %v", err)
	}
	if r, err := (*Sealed)(nil).OpenRange("anything", 0, 0); r != nil || !errors.Is(err, ErrStale) {
		t.Fatalf("nil version: %v", err)
	}
	empty, err := Capture(context.Background(), strings.NewReader(""), 1)
	if err != nil {
		t.Fatal(err)
	}
	r, err := empty.OpenRange(empty.ETag(), 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	b, err := io.ReadAll(r)
	if err != nil || len(b) != 0 {
		t.Fatalf("empty version: %v %q", err, b)
	}
}

func TestOpenRangeConcurrentIndependentReaders(t *testing.T) {
	s, err := Capture(context.Background(), strings.NewReader(strings.Repeat("abcd", 2048)), 8192)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				r, err := s.OpenRange(s.ETag(), 400, 400)
				if err != nil {
					t.Error(err)
					return
				}
				b, err := io.ReadAll(r)
				r.Close()
				if err != nil || string(b) != strings.Repeat("abcd", 100) {
					t.Errorf("concurrent range: %v %q", err, b)
					return
				}
			}
		}()
	}
	wg.Wait()
}

type neverProgress struct{}

func (neverProgress) Read([]byte) (int, error) { return 0, nil }

type brieflyIdle struct{ calls int }

func (r *brieflyIdle) Read(p []byte) (int, error) {
	r.calls++
	if r.calls < 4 {
		return 0, nil
	}
	return strings.NewReader("ok").Read(p)
}

func TestCaptureRejectsNonProgressingReader(t *testing.T) {
	got, err := Capture(context.Background(), neverProgress{}, 16)
	if got != nil || !errors.Is(err, io.ErrNoProgress) {
		t.Fatalf("reader=%v err=%v", got, err)
	}
}

func TestCapturePermitsBriefEmptyReads(t *testing.T) {
	// A reader may briefly return (0,nil); this must not be rejected early.
	r := &brieflyIdle{}
	got, err := Capture(context.Background(), io.LimitReader(r, 2), 16)
	if err != nil || got == nil {
		t.Fatalf("briefly idle: %v", err)
	}
}
