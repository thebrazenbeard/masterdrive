package boundedread

import (
 "crypto/sha256"
 "errors"
 "strings"
 "testing"
)

func TestReadExactSHA256(t *testing.T) {
 expected := sha256.Sum256([]byte("abcd"))
 got, err := ReadExactSHA256(strings.NewReader("abcd"), 4, 4, expected)
 if err != nil || string(got) != "abcd" { t.Fatalf("valid: %q, %v", got, err) }
 for _, tc := range []struct{name, body string; size,limit int64; digest [32]byte; want error}{
  {"tampered-same-length","abce",4,4,expected,ErrDigest},
  {"wrong-trusted-digest","abcd",4,4,sha256.Sum256([]byte("other")),ErrDigest},
  {"truncated","abc",4,4,expected,ErrRangeBody},
  {"oversized","abcde",4,4,expected,ErrRangeBody},
  {"over-limit","abcd",4,3,expected,ErrLimit},
 } {
  t.Run(tc.name,func(t *testing.T){
   bytes, err := ReadExactSHA256(strings.NewReader(tc.body),tc.size,tc.limit,tc.digest)
   if bytes != nil || !errors.Is(err,tc.want) { t.Fatalf("got %q, %v; want %v",bytes,err,tc.want) }
  })
 }
}
