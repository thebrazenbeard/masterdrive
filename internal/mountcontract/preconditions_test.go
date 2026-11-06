package mountcontract

import (
	"errors"
	"net/http"
	"testing"
)

func TestReadVersion(t *testing.T) {
	cases := []struct {
		name, pin string
		values    []string
		want      error
	}{
		{"match", "\"v1\"", []string{"\"v1\""}, nil},
		{"changed", "\"v1\"", []string{"\"v2\""}, ErrVersionChanged},
		{"weak-pin", "W/\"v1\"", []string{"\"v1\""}, ErrUnsafeVersion},
		{"weak-peer", "\"v1\"", []string{"W/\"v1\""}, ErrUnsafeVersion},
		{"missing", "\"v1\"", nil, ErrAmbiguousVersion},
		{"duplicate", "\"v1\"", []string{"\"v1\"", "\"v1\""}, ErrAmbiguousVersion},
		{"coalesced", "\"v1\"", []string{"\"v1\", \"v1\""}, ErrAmbiguousVersion},
		{"empty", "\"\"", []string{"\"\""}, ErrUnsafeVersion},
		{"control", "\"v1\"", []string{"\"v1\n\""}, ErrUnsafeVersion},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := make(http.Header)
			for _, v := range tc.values {
				h.Add("ETag", v)
			}
			got := CheckReadVersion(tc.pin, h)
			if !errors.Is(got, tc.want) || (tc.want == nil && got != nil) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}
