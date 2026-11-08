// Package rangegate validates metadata for one pinned read-only HTTP range.
// It does not authenticate the peer, prove snapshot immutability, or mount data.
package rangegate

import (
 "errors"
 "fmt"
 "net/http"
 "strconv"
 "strings"
)

var ErrMismatch = errors.New("rangegate: response does not match pinned range")

// Pin is trusted local state, not metadata copied from the response.
// The caller must establish identity, grant, and immutable source provenance.
type Pin struct {
 ETag string
 Total int64
}

// Check rejects a response that cannot be interpreted as the exact requested
// representation slice. The caller must still enforce the byte count while
// streaming, reject redirects, and recheck authorization on every request.
func Check(status int, header http.Header, pin Pin, start, end, received int64) error {
 if pin.Total <= 0 || start < 0 || end < start || end >= pin.Total ||
  received != end-start+1 || status != http.StatusPartialContent ||
  !strongETag(pin.ETag) || header.Get("ETag") != pin.ETag ||
  header.Get("Content-Encoding") != "" ||
  header.Get("Transfer-Encoding") != "" ||
  header.Get("Content-Range") != fmt.Sprintf("bytes %d-%d/%d",start,end,pin.Total) {
  return ErrMismatch
 }
 if ct := strings.ToLower(header.Get("Content-Type")); strings.HasPrefix(strings.TrimSpace(ct),"multipart/") {
  return ErrMismatch
 }
 values, present := header["Content-Length"]
 if !present || len(values) != 1 { return ErrMismatch }
 n,err := strconv.ParseInt(values[0],10,64)
 if err != nil || n != received || values[0] != strconv.FormatInt(n,10) { return ErrMismatch }
 return nil
}

func strongETag(s string) bool {
 if len(s) < 3 || s[0] != '"' || s[len(s)-1] != '"' { return false }
 for _,c := range s[1:len(s)-1] {
  if c < 0x21 || c > 0x7e || c == '"' || c == '\\' { return false }
 }
 return true
}
