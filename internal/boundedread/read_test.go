package boundedread

import (
 "bytes"
 "errors"
 "io"
 "strings"
 "testing"
)

type brokenReader struct{}
func (brokenReader) Read([]byte) (int,error) { return 0, errors.New("broken") }

func TestReadExact(t *testing.T) {
 cases := []struct{name string; data string; expected,limit int64; want string; err error}{
  {"exact","abcd",4,4,"abcd",nil},
  {"short","abc",4,4,"",ErrRangeBody},
  {"long","abcde",4,4,"",ErrRangeBody},
  {"overlimit","abcd",4,3,"",ErrLimit},
  {"zeroexpected","",0,4,"",ErrLimit},
  {"zerolimit","abcd",4,0,"",ErrLimit},
  {"negative","abcd",-1,4,"",ErrLimit},
 }
 for _,tc:=range cases {t.Run(tc.name,func(t *testing.T){
  got,err:=ReadExact(strings.NewReader(tc.data),tc.expected,tc.limit)
  if tc.err==nil {if err!=nil || string(got)!=tc.want {t.Fatalf("got %q, %v",got,err)}} else if !errors.Is(err,tc.err) || got!=nil {t.Fatalf("got %q, %v",got,err)}
 })}
}
func TestNilAndBroken(t *testing.T) {
 if b,e:=ReadExact(nil,1,1); b!=nil || !errors.Is(e,ErrLimit) {t.Fatalf("nil reader: %v",e)}
 if b,e:=ReadExact(brokenReader{},1,1); b!=nil || !errors.Is(e,ErrRangeBody) {t.Fatalf("broken reader: %v",e)}
}
func TestNoReadAboveLimit(t *testing.T) {
 reader:=bytes.NewReader([]byte("secret"))
 if b,e:=ReadExact(reader,6,5); b!=nil || !errors.Is(e,ErrLimit) {t.Fatalf("limit: %v",e)}
 if n:=reader.Len(); n!=6 {t.Fatalf("read despite limit: %d",n)}
}
func TestErrorAfterExactBytes(t *testing.T) {
 r:=io.MultiReader(strings.NewReader("abcd"),brokenReader{})
 if b,e:=ReadExact(r,4,4); b!=nil || !errors.Is(e,ErrRangeBody) {t.Fatalf("post-body read: %v",e)}
}
