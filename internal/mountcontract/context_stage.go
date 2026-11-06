package mountcontract

import (
	"context"
	"errors"
	"io"
	"net/http"
)

var ErrReadCanceled = errors.New("range read canceled")

// contextReader checks cancellation between reads. It cannot interrupt a
// blocked underlying Read; callers must also configure transport deadlines.
type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := r.reader.Read(p)
	if canceled := r.ctx.Err(); canceled != nil {
		return 0, canceled
	}
	return n, err
}

// StageAuthorizedRangeContext stages bytes without releasing them when
// cancellation or authorization revocation is observed at a read boundary.
// Authorization remains external and must be authoritative.
func StageAuthorizedRangeContext(ctx context.Context, authorize func() error, pin string, status int, headers http.Header, body io.Reader, start, end, total int64) ([]byte, error) {
	if ctx == nil || ctx.Err() != nil {
		return nil, ErrReadCanceled
	}
	if authorize == nil || authorize() != nil {
		return nil, ErrReadNotAuthorized
	}
	if body == nil {
		return nil, ErrIncompleteRangeBody
	}
	staged, err := StagePinnedRange(pin, status, headers, contextReader{ctx: ctx, reader: body}, start, end, total)
	if ctx.Err() != nil {
		return nil, ErrReadCanceled
	}
	if err != nil {
		return nil, err
	}
	if authorize() != nil {
		return nil, ErrReadNotAuthorized
	}
	if ctx.Err() != nil {
		return nil, ErrReadCanceled
	}
	return staged, nil
}
