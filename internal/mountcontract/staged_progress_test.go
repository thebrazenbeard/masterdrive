package mountcontract

import (
	"bytes"
	"errors"
	"net/http"
	"testing"
)

type stalledRangeReader struct{ calls int }

func (r *stalledRangeReader) Read(p []byte) (int, error) { r.calls++; return 0, nil }

type delayedRangeReader struct {
	calls int
	data  *bytes.Reader
}

func (r *delayedRangeReader) Read(p []byte) (int, error) {
	r.calls++
	if r.calls < maxEmptyBodyReads {
		return 0, nil
	}
	return r.data.Read(p)
}
func TestStagedRangeNonProgressFailsClosed(t *testing.T) {
	h := http.Header{}
	h.Set("ETag", "\"v1\"")
	h.Set("Content-Range", "bytes 0-2/3")
	h.Set("Content-Length", "3")
	stalled := &stalledRangeReader{}
	got, err := StagePinnedRange("\"v1\"", 206, h, stalled, 0, 2, 3)
	if !errors.Is(err, ErrIncompleteRangeBody) || got != nil || stalled.calls != maxEmptyBodyReads {
		t.Fatalf("non-progressing reader: bytes=%q err=%v calls=%d", got, err, stalled.calls)
	}
	delayed := &delayedRangeReader{data: bytes.NewReader([]byte("abc"))}
	got, err = StagePinnedRange("\"v1\"", 206, h, delayed, 0, 2, 3)
	if err != nil || !bytes.Equal(got, []byte("abc")) {
		t.Fatalf("progress after empty reads: %q %v", got, err)
	}
}
