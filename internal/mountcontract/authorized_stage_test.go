package mountcontract

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"testing"
)

type revokingReader struct {
	src    io.Reader
	revoke func()
}

func (r *revokingReader) Read(p []byte) (int, error) {
	n, err := r.src.Read(p)
	if n > 0 {
		r.revoke()
	}
	return n, err
}
func TestStageAuthorizedRangeRevocation(t *testing.T) {
	h := http.Header{}
	h.Set("ETag", "\""+"v1"+"\"")
	h.Set("Content-Range", "bytes 0-2/3")
	h.Set("Content-Length", "3")
	granted := true
	checks := 0
	authorize := func() error {
		checks++
		if !granted {
			return errors.New("revoked")
		}
		return nil
	}
	body := &revokingReader{src: bytes.NewReader([]byte("abc")), revoke: func() { granted = false }}
	got, err := StageAuthorizedRange(authorize, "\"v1\"", 206, h, body, 0, 2, 3)
	if !errors.Is(err, ErrReadNotAuthorized) || got != nil || checks != 2 {
		t.Fatalf("revoked read released: %q %v checks=%d", got, err, checks)
	}
	granted = false
	checks = 0
	body2 := &countReader{src: bytes.NewReader([]byte("abc"))}
	got, err = StageAuthorizedRange(authorize, "\"v1\"", 206, h, body2, 0, 2, 3)
	if !errors.Is(err, ErrReadNotAuthorized) || got != nil || checks != 1 || body2.calls != 0 {
		t.Fatalf("pre-revoked read touched body: %q %v checks=%d calls=%d", got, err, checks, body2.calls)
	}
	got, err = StageAuthorizedRange(nil, "\"v1\"", 206, h, body2, 0, 2, 3)
	if !errors.Is(err, ErrReadNotAuthorized) || got != nil {
		t.Fatalf("nil authority accepted")
	}
	granted = true
	checks = 0
	got, err = StageAuthorizedRange(authorize, "\"v1\"", 206, h, bytes.NewReader([]byte("abc")), 0, 2, 3)
	if err != nil || !bytes.Equal(got, []byte("abc")) || checks != 2 {
		t.Fatalf("valid read failed: %q %v checks=%d", got, err, checks)
	}
}

type countReader struct {
	src   io.Reader
	calls int
}

func (r *countReader) Read(p []byte) (int, error) { r.calls++; return r.src.Read(p) }
