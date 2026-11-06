package mountcontract

import (
	"errors"
	"io"
	"testing"
)

type shortJournalWriter struct {
	n   int
	err error
}

func (w shortJournalWriter) Write(p []byte) (int, error) { return w.n, w.err }

func TestJournalRecordWriteFaultInjection(t *testing.T) {
	for _, tc := range []struct {
		name string
		n    int
		err  error
		want error
	}{
		{"silent-short", 2, nil, io.ErrShortWrite},
		{"short-with-error", 2, io.ErrClosedPipe, io.ErrClosedPipe},
		{"complete", 5, nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			n, err := writeJournalRecord(shortJournalWriter{tc.n, tc.err}, "hello")
			if n != tc.n || !errors.Is(err, tc.want) || (tc.want == nil && err != nil) {
				t.Fatalf("n=%d err=%v; want n=%d err=%v", n, err, tc.n, tc.want)
			}
		})
	}
}
