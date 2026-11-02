package capacity

import (
	"os"
	"testing"
	"time"
)

func TestProbePinnedRootInvalidInputDoesNotProbe(t *testing.T) {
	at := time.Unix(5, 0)
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	for _, tc := range []struct {
		share string
		root *os.Root
		at time.Time
	}{
		{"", root, at},
		{"docs", nil, at},
		{"docs", root, time.Time{}},
	} {
		if _, err := ProbePinnedRoot(tc.share, tc.root, tc.at); err == nil {
			t.Fatalf("accepted invalid input: %+v", tc)
		}
	}
}
