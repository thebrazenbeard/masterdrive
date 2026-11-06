package server

import "testing"

func TestPeerContentRangeRequiresDecimalDigits(t *testing.T) {
	for _, tc := range []struct {
		name, returned string
		want           bool
	}{
		{"valid", "bytes 0-4/10", true},
		{"plus start", "bytes +0-4/10", false},
		{"plus end", "bytes 0-+4/10", false},
		{"plus total", "bytes 0-4/+10", false},
		{"space start", "bytes  0-4/10", false},
		{"space total", "bytes 0-4/ 10", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := validSingleRangeResponse("bytes=0-4", tc.returned, 5); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}
