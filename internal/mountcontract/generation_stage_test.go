package mountcontract

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"testing"
)

func TestGenerationPinnedRead(t *testing.T) {
	headers := http.Header{"Etag": {"\"v1\""}, "Content-Range": {"bytes 0-2/3"}, "Content-Length": {"3"}}
	for _, tc := range []struct {
		name          string
		before, after uint64
		fail          bool
	}{
		{"stable", 7, 7, false}, {"revoked", 7, 0, true},
		{"reissued", 7, 8, true}, {"missing", 0, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			grant := func() (uint64, error) {
				calls++
				if calls == 1 {
					return tc.before, nil
				}
				return tc.after, nil
			}
			got, err := StageGenerationPinnedRange(context.Background(), grant, "\"v1\"", 206, headers, bytes.NewReader([]byte("abc")), 0, 2, 3)
			if tc.fail && (!errors.Is(err, ErrReadNotAuthorized) || got != nil) {
				t.Fatalf("expected denial, got %q %v", got, err)
			}
			if !tc.fail && (err != nil || string(got) != "abc" || calls < 4) {
				t.Fatalf("expected complete read, got %q %v checks=%d", got, err, calls)
			}
		})
	}
	got, err := StageGenerationPinnedRange(context.Background(), nil, "\"v1\"", 206, headers, bytes.NewReader([]byte("abc")), 0, 2, 3)
	if !errors.Is(err, ErrReadNotAuthorized) || got != nil {
		t.Fatalf("nil grant: %q %v", got, err)
	}
}
