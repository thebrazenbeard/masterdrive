package mountcontract

import (
	"errors"
	"io"
	"net/http"
)

const MaxStagedRangeBytes int64 = 16 << 20

var ErrIncompleteRangeBody = errors.New("incomplete or oversized range body")

// StagePinnedRange checks metadata and stages exactly the requested bytes.
// It never returns partial bytes. Callers must authenticate and authorize the
// peer before calling and recheck revocation before exposing the returned bytes.
func StagePinnedRange(pin string, status int, headers http.Header, body io.Reader, start, end, total int64) ([]byte, error) {
	if err := CheckPinnedRangeResponse(pin, status, headers, start, end, total); err != nil {
		return nil, err
	}
	n := end - start + 1
	if n <= 0 || n > MaxStagedRangeBytes || body == nil {
		return nil, ErrIncompleteRangeBody
	}
	// Read one extra byte to detect a lying or overlong peer.
	buf, err := io.ReadAll(io.LimitReader(body, n+1))
	if err != nil || int64(len(buf)) != n {
		return nil, ErrIncompleteRangeBody
	}
	return buf, nil
}
