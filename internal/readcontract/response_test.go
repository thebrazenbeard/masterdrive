package readcontract

import (
	"errors"
	"net/http"
	"testing"
)

func fixture() (ExpectedRange, *http.Response) {
	w := ExpectedRange{StrongETag: "\"sha256-abc\"", Size: 100, Offset: 10, Length: 20}
	r := &http.Response{
		StatusCode:    http.StatusPartialContent,
		ContentLength: 20,
		Header:        make(http.Header),
	}
	r.Header.Set("ETag", w.StrongETag)
	r.Header.Set("Content-Range", "bytes 10-29/100")
	r.Header.Set("Content-Length", "20")
	return w, r
}

func TestValidatePartialAcceptsExactPinnedRange(t *testing.T) {
	w, r := fixture()
	if err := ValidatePartial(w, r); err != nil {
		t.Fatal(err)
	}
}

func TestValidatePartialFailsClosed(t *testing.T) {
	cases := []struct {
		name   string
		change func(*ExpectedRange, *http.Response)
	}{
		{"nil response", func(_ *ExpectedRange, r *http.Response) { *r = http.Response{} }},
		{"full body", func(_ *ExpectedRange, r *http.Response) { r.StatusCode = http.StatusOK }},
		{"range unsatisfiable", func(_ *ExpectedRange, r *http.Response) { r.StatusCode = http.StatusRequestedRangeNotSatisfiable }},
		{"missing validator", func(_ *ExpectedRange, r *http.Response) { r.Header.Del("ETag") }},
		{"weak validator", func(_ *ExpectedRange, r *http.Response) { r.Header.Set("ETag", "W/\"sha256-abc\"") }},
		{"mismatched validator", func(_ *ExpectedRange, r *http.Response) { r.Header.Set("ETag", "\"other\"") }},
		{"duplicate validator", func(_ *ExpectedRange, r *http.Response) { r.Header.Add("ETag", "\"other\"") }},
		{"missing range", func(_ *ExpectedRange, r *http.Response) { r.Header.Del("Content-Range") }},
		{"wrong start", func(_ *ExpectedRange, r *http.Response) { r.Header.Set("Content-Range", "bytes 11-30/100") }},
		{"wrong total", func(_ *ExpectedRange, r *http.Response) { r.Header.Set("Content-Range", "bytes 10-29/101") }},
		{"unknown total", func(_ *ExpectedRange, r *http.Response) { r.Header.Set("Content-Range", "bytes 10-29/*") }},
		{"duplicate range", func(_ *ExpectedRange, r *http.Response) { r.Header.Add("Content-Range", "bytes 10-29/100") }},
		{"wrong body length", func(_ *ExpectedRange, r *http.Response) { r.ContentLength = 19 }},
		{"wrong header length", func(_ *ExpectedRange, r *http.Response) { r.Header.Set("Content-Length", "19") }},
		{"missing header length", func(_ *ExpectedRange, r *http.Response) { r.Header.Del("Content-Length") }},
		{"duplicate length", func(_ *ExpectedRange, r *http.Response) { r.Header.Add("Content-Length", "20") }},
		{"compressed", func(_ *ExpectedRange, r *http.Response) { r.Header.Set("Content-Encoding", "gzip") }},
		{"automatically decompressed", func(_ *ExpectedRange, r *http.Response) { r.Uncompressed = true }},
		{"chunked", func(_ *ExpectedRange, r *http.Response) { r.TransferEncoding = []string{"chunked"} }},
		{"weak requested validator", func(w *ExpectedRange, _ *http.Response) { w.StrongETag = "W/\"sha256-abc\"" }},
		{"unquoted requested validator", func(w *ExpectedRange, _ *http.Response) { w.StrongETag = "sha256-abc" }},
		{"invalid requested validator", func(w *ExpectedRange, _ *http.Response) { w.StrongETag = "\"bad\\n\"" }},
		{"zero source size", func(w *ExpectedRange, _ *http.Response) { w.Size = 0 }},
		{"negative offset", func(w *ExpectedRange, _ *http.Response) { w.Offset = -1 }},
		{"past end", func(w *ExpectedRange, _ *http.Response) { w.Offset = 100 }},
		{"negative length", func(w *ExpectedRange, _ *http.Response) { w.Length = -1 }},
		{"range exceeds source", func(w *ExpectedRange, _ *http.Response) { w.Length = 91 }},
		{"overflow attempt", func(w *ExpectedRange, _ *http.Response) {
			w.Size = 1<<63 - 1
			w.Offset = 1<<63 - 2
			w.Length = 1<<63 - 1
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w, r := fixture()
			tc.change(&w, r)
			if err := ValidatePartial(w, r); !errors.Is(err, ErrUnsafePartial) {
				t.Fatalf("expected unsafe response, got %v", err)
			}
		})
	}
}

func TestValidatePartialNilResponse(t *testing.T) {
	w, _ := fixture()
	if err := ValidatePartial(w, nil); !errors.Is(err, ErrUnsafePartial) {
		t.Fatalf("nil response: %v", err)
	}
}

func TestValidatePartialBoundarySingleLastByte(t *testing.T) {
	w, r := fixture()
	w.Offset, w.Length = 99, 1
	r.ContentLength = 1
	r.Header.Set("Content-Range", "bytes 99-99/100")
	r.Header.Set("Content-Length", "1")
	if err := ValidatePartial(w, r); err != nil {
		t.Fatal(err)
	}
}
