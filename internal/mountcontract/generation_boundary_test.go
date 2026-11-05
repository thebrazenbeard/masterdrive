package mountcontract

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
)

type generationChangingReader struct {
	reader io.Reader
	change func()
}

func (r *generationChangingReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.change()
	return n, err
}
func TestGenerationChangeDuringBodyReadFailsClosed(t *testing.T) {
	generation := uint64(7)
	grant := func() (uint64, error) { return generation, nil }
	body := &generationChangingReader{reader: bytes.NewReader([]byte("abc")), change: func() { generation = 8 }}
	headers := http.Header{"Etag": {"\"v1\""}, "Content-Range": {"bytes 0-2/3"}, "Content-Length": {"3"}}
	got, err := StageGenerationPinnedRange(context.Background(), grant, "\"v1\"", 206, headers, body, 0, 2, 3)
	if !errors.Is(err, ErrReadNotAuthorized) || got != nil {
		t.Fatalf("changed generation released bytes: %q, %v", got, err)
	}
}
func TestGenerationCheckOnEachBodyReadBoundary(t *testing.T) {
	generation := uint64(7)
	checks := 0
	grant := func() (uint64, error) { checks++; return generation, nil }
	headers := http.Header{"Etag": {"\"v1\""}, "Content-Range": {"bytes 0-2/3"}, "Content-Length": {"3"}}
	got, err := StageGenerationPinnedRange(context.Background(), grant, "\"v1\"", 206, headers, bytes.NewReader([]byte("abc")), 0, 2, 3)
	if err != nil || string(got) != "abc" || checks < 4 {
		t.Fatalf("got %q, %v, checks %d", got, err, checks)
	}
}
