package mountcontract

import (
	"context"
	"errors"
	"io"
	"net/http"
)

const MaxStagedRangeBytes int64 = 16 << 20
const maxEmptyBodyReads = 100

var ErrIncompleteRangeBody = errors.New("incomplete or oversized range body")

// progressGuard bounds consecutive zero-byte, nil-error reads from an
// untrusted source. It does not time out blocked I/O.
type progressGuard struct {
	reader io.Reader
	empty  int
}

func (g *progressGuard) Read(p []byte) (int, error) {
	n, err := g.reader.Read(p)
	if n > 0 || err != nil {
		g.empty = 0
		return n, err
	}
	g.empty++
	if g.empty >= maxEmptyBodyReads {
		return 0, io.ErrNoProgress
	}
	return 0, nil
}

// StagePinnedRange validates metadata and stages the complete range.
// It returns no partial bytes; authorization and revocation are caller duties.
func StagePinnedRange(pin string, status int, headers http.Header, body io.Reader, start, end, total int64) ([]byte, error) {
	if err := CheckPinnedRangeResponse(pin, status, headers, start, end, total); err != nil {
		return nil, err
	}
	n := end - start + 1
	if n <= 0 || n > MaxStagedRangeBytes || body == nil {
		return nil, ErrIncompleteRangeBody
	}
	buf, err := io.ReadAll(io.LimitReader(&progressGuard{reader: body}, n+1))
	if err != nil {
		if errors.Is(err, ErrReadNotAuthorized) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, ErrIncompleteRangeBody
	}
	if int64(len(buf)) != n {
		return nil, ErrIncompleteRangeBody
	}
	return buf, nil
}
