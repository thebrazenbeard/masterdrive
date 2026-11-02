package versioncontract

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestCaptureOwnsIndependentVersionedBytes(t *testing.T) {
	source := []byte("abcdef")
	sealed, err := Capture(context.Background(), bytes.NewReader(source), 8)
	if err != nil { t.Fatal(err) }
	if sealed.Size() != 6 || !strings.HasPrefix(sealed.ETag(), `"m0-sha256-`) { t.Fatalf("invalid metadata: %d %s", sealed.Size(), sealed.ETag()) }
	source[0] = 'x'
	first, err := sealed.Open(sealed.ETag())
	if err != nil { t.Fatal(err) }
	defer first.Close()
	second, err := sealed.Open(sealed.ETag())
	if err != nil { t.Fatal(err) }
	defer second.Close()
	buf := make([]byte, 2)
	if _, err := first.Read(buf); err != nil || string(buf) != "ab" { t.Fatalf("first reader: %q %v", buf, err) }
	all, err := io.ReadAll(second)
	if err != nil || string(all) != "abcdef" { t.Fatalf("snapshot changed: %q %v", all, err) }
	if _, err := sealed.Open(`"different"`); !errors.Is(err, ErrStale) { t.Fatalf("mismatched version: %v", err) }
	other, err := Capture(context.Background(), strings.NewReader("abcdeg"), 8)
	if err != nil { t.Fatal(err) }
	if other.ETag() == sealed.ETag() { t.Fatal("different sealed bytes shared a version") }
}

func TestCaptureBoundariesAndCancellation(t *testing.T) {
	if _, err := Capture(context.Background(), strings.NewReader("abcd"), 3); !errors.Is(err, ErrTooLarge) { t.Fatalf("oversize: %v", err) }
	if _, err := Capture(context.Background(), strings.NewReader(""), 1); err != nil { t.Fatalf("empty file: %v", err) }
	if _, err := Capture(context.Background(), strings.NewReader("x"), 0); err == nil { t.Fatal("zero limit accepted") }
	if _, err := Capture(context.Background(), strings.NewReader("x"), MaxCaptureBytes+1); err == nil { t.Fatal("global cap ignored") }
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Capture(ctx, strings.NewReader("x"), 2); !errors.Is(err, context.Canceled) { t.Fatalf("cancel: %v", err) }
}

type errReader struct{}
func (errReader) Read([]byte) (int, error) { return 0, errors.New("source failed") }

type partialErrReader struct{}
func (partialErrReader) Read(p []byte) (int, error) {
	copy(p, "incomplete")
	return len("incomplete"), errors.New("source failed after bytes")
}

func TestCaptureDoesNotPublishPartialErrors(t *testing.T) {
	if got, err := Capture(context.Background(), errReader{}, 10); got != nil || err == nil { t.Fatalf("partial snapshot accepted: %v %v", got, err) }
	if got, err := Capture(context.Background(), partialErrReader{}, 16); got != nil || err == nil { t.Fatalf("error after bytes published: %v %v", got, err) }
}
