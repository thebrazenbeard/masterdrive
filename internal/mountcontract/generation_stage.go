package mountcontract

import (
	"context"
	"io"
	"net/http"
)

type AuthorizationGeneration func() (uint64, error)

// StageGenerationPinnedRange requires an authoritative nonzero grant generation
// before and after staging. A changed generation invalidates the entire read,
// even if the new grant would otherwise allow access.
// Generation storage, atomicity and durability belong to the authority provider.
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
	return StageAuthorizedRangeContext(ctx, check, pin, status, headers, body, start, end, total)
}
