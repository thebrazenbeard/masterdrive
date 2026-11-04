package mountcontract

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

var ErrUnsafeRange = errors.New("unsafe range response")

// CheckPinnedRangeResponse validates metadata before any range bytes are exposed.
// The caller must separately authenticate the peer, authorize access, and verify
// that the body contains exactly end-start+1 bytes before releasing those bytes.
func CheckPinnedRangeResponse(pin string, status int, h http.Header, start, end, total int64) error {
	if start < 0 || end < start || total <= end || end == int64(^uint64(0)>>1) {
		return ErrUnsafeRange
	}
	if status != http.StatusPartialContent {
		return ErrUnsafeRange
	}
	if err := CheckReadVersion(pin, h); err != nil {
		return err
	}
	ranges := h.Values("Content-Range")
	if len(ranges) != 1 {
		return ErrUnsafeRange
	}
	expected := fmt.Sprintf("bytes %d-%d/%d", start, end, total)
	if ranges[0] != expected {
		return ErrUnsafeRange
	}
	lengths := h.Values("Content-Length")
	if len(lengths) != 1 || strings.TrimSpace(lengths[0]) != lengths[0] {
		return ErrUnsafeRange
	}
	length, err := strconv.ParseInt(lengths[0], 10, 64)
	if err != nil || length != end-start+1 {
		return ErrUnsafeRange
	}
	if len(h.Values("Content-Encoding")) != 0 {
		return ErrUnsafeRange
	}
	if len(h.Values("Transfer-Encoding")) != 0 {
		return ErrUnsafeRange
	}
	return nil
}
