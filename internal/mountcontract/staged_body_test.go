package mountcontract

import (
	"bytes"
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestStagePinnedRange(t *testing.T) {
	h := http.Header{}
	h.Set("ETag", "`v1`")
	h.Set("Content-Range", "bytes 2-4/10")
	h.Set("Content-Length", "3")
	// A real HTTP ETag requires quotation marks.
	h.Set("ETag", "\"v1\"")
	for _, tc := range []struct {
		name, body string
		wantOK     bool
	}{
		{"exact", "abc", true},
		{"short", "ab", false},
		{"empty", "", false},
		{"long", "abcd", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := StagePinnedRange("\"v1\"", 206, h, strings.NewReader(tc.body), 2, 4, 10)
			if tc.wantOK {
				if err != nil || !bytes.Equal(got, []byte("abc")) {
					t.Fatalf("got %q err %v", got, err)
				}
			} else if !errors.Is(err, ErrIncompleteRangeBody) || got != nil {
				t.Fatalf("unsafe release %q err %v", got, err)
			}
		})
	}
	h.Set("ETag", "\"changed\"")
	if got, err := StagePinnedRange("\"v1\"", 206, h, strings.NewReader("abc"), 2, 4, 10); err == nil || got != nil {
		t.Fatal("accepted changed version")
	}
}
