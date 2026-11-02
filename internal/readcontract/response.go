// Package readcontract validates source-issued HTTP range metadata for a future
// read-only mount adapter. It is not an authorization check or a mount.
package readcontract

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

var ErrUnsafePartial = errors.New("read contract: unsafe partial response")

// ExpectedRange is an immutable version and exact byte window obtained from
// an independently authorized, trusted source. Caller-supplied fields are NOT
// evidence of authorization or of a coherent filesystem snapshot.
type ExpectedRange struct {
	StrongETag string
	Size       int64
	Offset     int64
	Length     int64
}

// ValidatePartial checks HTTP response metadata before a consumer accepts any
// body bytes. A caller must additionally enforce body length, cancellation,
// stream revocation, and authorization; this function does not read the body.
func ValidatePartial(w ExpectedRange, resp *http.Response) error {
	if !strongETag(w.StrongETag) || w.Size <= 0 || w.Offset < 0 ||
		w.Length <= 0 || w.Offset >= w.Size || w.Length > w.Size-w.Offset {
		return ErrUnsafePartial
	}
	if resp == nil || resp.StatusCode != http.StatusPartialContent ||
		resp.Header == nil || resp.Uncompressed || len(resp.TransferEncoding) != 0 {
		return ErrUnsafePartial
	}
	if len(resp.Header.Values("ETag")) != 1 ||
		resp.Header.Get("ETag") != w.StrongETag {
		return ErrUnsafePartial
	}
	if len(resp.Header.Values("Content-Encoding")) != 0 {
		return ErrUnsafePartial
	}
	expectedRange := fmt.Sprintf("bytes %d-%d/%d",
		w.Offset, w.Offset+w.Length-1, w.Size)
	if len(resp.Header.Values("Content-Range")) != 1 ||
		resp.Header.Get("Content-Range") != expectedRange {
		return ErrUnsafePartial
	}
	expectedLength := strconv.FormatInt(w.Length, 10)
	if len(resp.Header.Values("Content-Length")) != 1 ||
		strings.TrimSpace(resp.Header.Get("Content-Length")) != expectedLength ||
		resp.ContentLength != w.Length {
		return ErrUnsafePartial
	}
	return nil
}

func strongETag(tag string) bool {
	if len(tag) < 3 || tag[0] != '"' || tag[len(tag)-1] != '"' {
		return false
	}
	for i := 1; i < len(tag)-1; i++ {
		c := tag[i]
		if c < 0x21 || c > 0x7e || c == '"' {
			return false
		}
	}
	return true
}
