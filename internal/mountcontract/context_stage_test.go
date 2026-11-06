package mountcontract

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"testing"
)

type cancelAfterRead struct {
	ctx    context.CancelFunc
	reader *bytes.Reader
}

func (r *cancelAfterRead) Read(p []byte) (int, error) { n, e := r.reader.Read(p); r.ctx(); return n, e }

func TestContextStageRejectsCancellationAndRevocation(t *testing.T) {
	headers := http.Header{"Etag": []string{"\"v1\""}, "Content-Range": []string{"bytes 0-2/3"}, "Content-Length": []string{"3"}}
	good := func() error { return nil }
	ctx, cancel := context.WithCancel(context.Background())
	data, err := StageAuthorizedRangeContext(ctx, good, "\"v1\"", 206, headers, &cancelAfterRead{cancel, bytes.NewReader([]byte("abc"))}, 0, 2, 3)
	if !errors.Is(err, ErrReadCanceled) || data != nil {
		t.Fatalf("canceled read released bytes: %v %q", err, data)
	}
	ctx2, cancel2 := context.WithCancel(context.Background())
	cancel2()
	data, err = StageAuthorizedRangeContext(ctx2, good, "\"v1\"", 206, headers, bytes.NewReader([]byte("abc")), 0, 2, 3)
	if !errors.Is(err, ErrReadCanceled) || data != nil {
		t.Fatalf("pre-canceled read: %v %q", err, data)
	}
	data, err = StageAuthorizedRangeContext(context.Background(), func() error { return errors.New("revoked") }, "\"v1\"", 206, headers, bytes.NewReader([]byte("abc")), 0, 2, 3)
	if !errors.Is(err, ErrReadNotAuthorized) || data != nil {
		t.Fatalf("revoked read: %v %q", err, data)
	}
	data, err = StageAuthorizedRangeContext(context.Background(), good, "\"v1\"", 206, headers, bytes.NewReader([]byte("abc")), 0, 2, 3)
	if err != nil || string(data) != "abc" {
		t.Fatalf("authorized read: %v %q", err, data)
	}
}
