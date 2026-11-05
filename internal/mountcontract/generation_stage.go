package mountcontract

import (
	"context"
	"io"
	"net/http"
)

type AuthorizationGeneration func() (uint64, error)

// generationReader refuses bytes if the authority changes while a body is
// being staged. The final authorization check still applies before release.
type generationReader struct {
	reader io.Reader
	check  func() error
}

func (r generationReader) Read(p []byte) (int, error) {
	if err := r.check(); err != nil {
		return 0, err
	}
	n, err := r.reader.Read(p)
	if checkErr := r.check(); checkErr != nil {
		return 0, checkErr
	}
	return n, err
}

// StageGenerationPinnedRange rejects a missing or changed grant generation
// before, during, and after staging. This is cooperative sampling, not an
// atomic revocation lease; authoritative durable state remains external.
func StageGenerationPinnedRange(ctx context.Context, grant AuthorizationGeneration, pin string, status int, headers http.Header, body io.Reader, start, end, total int64) ([]byte, error) {
	if grant == nil {
		return nil, ErrReadNotAuthorized
	}
	var initial uint64
	check := func() error {
		generation, err := grant()
		if err != nil || generation == 0 || (initial != 0 && generation != initial) {
			return ErrReadNotAuthorized
		}
		initial = generation
		return nil
	}
	if err := check(); err != nil {
		return nil, err
	}
	if body == nil {
		return nil, ErrIncompleteRangeBody
	}
	return StageAuthorizedRangeContext(ctx, check, pin, status, headers, generationReader{reader: body, check: check}, start, end, total)
}
