package server

import "testing"

func TestRequestedRangeRequiresDecimalDigits(t *testing.T) {
	for _, requested := range []string{"bytes=+0-4", "bytes=0-+4", "bytes=-+4", "bytes= 0-4", "bytes=0- 4"} {
		t.Run(requested, func(t *testing.T) {
			if validSingleRangeResponse(requested, "bytes 0-4/10", 5) {
				t.Fatalf("accepted malformed requested range %q", requested)
			}
		})
	}
	if !validSingleRangeResponse("bytes=0-4", "bytes 0-4/10", 5) {
		t.Fatal("valid range rejected")
	}
}
