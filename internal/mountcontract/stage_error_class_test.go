package mountcontract

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
)

type errorBody struct{ err error }

func (b errorBody) Read([]byte) (int, error) { return 0, b.err }

func TestStagePinnedRangePreservesSecurityErrors(t *testing.T) {
	h := http.Header{"Etag": []string{"\"v1\""}, "Content-Range": []string{"bytes 0-0/1"}, "Content-Length": []string{"1"}}
	for _, want := range []error{ErrReadNotAuthorized, context.Canceled, context.DeadlineExceeded} {
		data, err := StagePinnedRange("\"v1\"", http.StatusPartialContent, h, errorBody{want}, 0, 0, 1)
		if data != nil || !errors.Is(err, want) {
			t.Fatalf("want %v, got %v, bytes %v", want, err, data)
		}
	}
	data, err := StagePinnedRange("\"v1\"", http.StatusPartialContent, h, errorBody{io.ErrUnexpectedEOF}, 0, 0, 1)
	if data != nil || !errors.Is(err, ErrIncompleteRangeBody) {
		t.Fatalf("unexpected body error: %v", err)
	}
}
