package mountcontract

import (
	"errors"
	"io"
	"net/http"
)

var ErrReadNotAuthorized = errors.New("read authorization unavailable or revoked")

// StageAuthorizedRange checks authorization both before touching the response
// body and after staging completes. The callback must consult an authoritative,
// current grant and revocation source; a cached boolean is insufficient.
// This is a local read primitive, not a live gateway or mount integration.
func StageAuthorizedRange(authorize func() error, pin string, status int, headers http.Header, body io.Reader, start, end, total int64) ([]byte, error) {
	if authorize == nil || authorize() != nil {
		return nil, ErrReadNotAuthorized
	}
	staged, err := StagePinnedRange(pin, status, headers, body, start, end, total)
	if err != nil {
		return nil, err
	}
	if authorize() != nil {
		return nil, ErrReadNotAuthorized
	}
	return staged, nil
}
