package mountcontract

import (
	"errors"
	"net/http"
	"testing"
)

func TestPinnedRangeResponse(t *testing.T) {
	good := func() http.Header {
		h := http.Header{}
		h.Set("ETag", `"v1"`)
		h.Set("Content-Range", "bytes 2-4/10")
		h.Set("Content-Length", "3")
		return h
	}
	if err := CheckPinnedRangeResponse(`"v1"`, 206, good(), 2, 4, 10); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*http.Header)
		status int
	}{
		{"status", func(*http.Header) {}, 200},
		{"changed-etag", func(h *http.Header) { h.Set("ETag", `"v2"`) }, 206},
		{"range", func(h *http.Header) { h.Set("Content-Range", "bytes 2-5/10") }, 206},
		{"length", func(h *http.Header) { h.Set("Content-Length", "4") }, 206},
		{"duplicate-range", func(h *http.Header) { h.Add("Content-Range", "bytes 2-4/10") }, 206},
		{"duplicate-length", func(h *http.Header) { h.Add("Content-Length", "3") }, 206},
		{"encoding", func(h *http.Header) { h.Set("Content-Encoding", "gzip") }, 206},
		{"transfer", func(h *http.Header) { h.Set("Transfer-Encoding", "chunked") }, 206},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := good()
			tc.mutate(&h)
			if err := CheckPinnedRangeResponse(`"v1"`, tc.status, h, 2, 4, 10); err == nil {
				t.Fatal("accepted unsafe range")
			}
		})
	}
	for _, tc := range []struct{ start, end, total int64 }{{-1, 4, 10}, {2, 1, 10}, {2, 10, 10}, {2, 4, 0}} {
		if !errors.Is(CheckPinnedRangeResponse(`"v1"`, 206, good(), tc.start, tc.end, tc.total), ErrUnsafeRange) {
			t.Fatal(tc)
		}
	}
}
