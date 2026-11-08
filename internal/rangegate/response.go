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
  !strongETag(pin.ETag) {
  return ErrMismatch
 }
 etag, ok := singleHeader(header, "ETag")
 if !ok || etag != pin.ETag { return ErrMismatch }
 cr, ok := singleHeader(header, "Content-Range")
 if !ok || cr != fmt.Sprintf("bytes %d-%d/%d",start,end,pin.Total) { return ErrMismatch }
 length, ok := singleHeader(header, "Content-Length")
 if !ok { return ErrMismatch }
 n,err := strconv.ParseInt(length,10,64)
 if err != nil || n != received || length != strconv.FormatInt(n,10) { return ErrMismatch }
 // Reject even empty-valued or duplicated encoding/transfer metadata.
 if hasHeader(header,"Content-Encoding") || hasHeader(header,"Transfer-Encoding") { return ErrMismatch }
 if ct, present := singleHeader(header,"Content-Type"); !present {
  if hasHeader(header,"Content-Type") { return ErrMismatch }
 } else if strings.HasPrefix(strings.ToLower(strings.TrimSpace(ct)),"multipart/") {
  return ErrMismatch
 }
 return nil
}

// Use EqualFold because http.Header is a map and callers can construct
// noncanonical keys. Duplicate fields (including differently cased keys)
// are ambiguous and must not be silently reduced to the first value.
func singleHeader(h http.Header, name string) (string, bool) {
 var value string
 count := 0
 for key, values := range h {
  if strings.EqualFold(key,name) {
   count += len(values)
   if len(values) == 1 { value = values[0] }
  }
 }
 return value, count == 1
}

func hasHeader(h http.Header, name string) bool {
 for key := range h {
  if strings.EqualFold(key,name) { return true }
 }
 return false
}

func strongETag(s string) bool {
 if len(s) < 3 || s[0] != '"' || s[len(s)-1] != '"' { return false }
 for _,c := range s[1:len(s)-1] {
  if c < 0x21 || c > 0x7e || c == '"' || c == '\\' { return false }
 }
 return true
}
