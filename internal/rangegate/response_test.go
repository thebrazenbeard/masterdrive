package rangegate

import (
 "errors"
 "net/http"
 "testing"
)

func fixture() (http.Header, Pin) {
 h:=make(http.Header)
 h.Set("ETag", "\"v17\"")
 h.Set("Content-Range","bytes 2-5/12")
 h.Set("Content-Length","4")
 return h,Pin{ETag:"\"v17\"",Total:12}
}
func TestExactPartialResponse(t *testing.T) {
 h,p:=fixture()
 if err:=Check(206,h,p,2,5,4);err!=nil {t.Fatal(err)}
}
func TestFailClosed(t *testing.T) {
 cases:=map[string]func(*http.Header,*Pin,*int,*int64,*int64,*int64){
  "full response":func(h *http.Header,p *Pin,s *int,a,b,n *int64){*s=200},
  "wrong etag":func(h *http.Header,p *Pin,s *int,a,b,n *int64){h.Set("ETag","\"v18\"")},
  "weak etag":func(h *http.Header,p *Pin,s *int,a,b,n *int64){p.ETag="W/\"v17\"";h.Set("ETag",p.ETag)},
  "range mismatch":func(h *http.Header,p *Pin,s *int,a,b,n *int64){h.Set("Content-Range","bytes 1-5/12")},
  "total mismatch":func(h *http.Header,p *Pin,s *int,a,b,n *int64){p.Total=13},
  "wrong length":func(h *http.Header,p *Pin,s *int,a,b,n *int64){h.Set("Content-Length","5")},
  "duplicate length":func(h *http.Header,p *Pin,s *int,a,b,n *int64){h.Add("Content-Length","4")},
  "missing length":func(h *http.Header,p *Pin,s *int,a,b,n *int64){h.Del("Content-Length")},
  "noncanonical length":func(h *http.Header,p *Pin,s *int,a,b,n *int64){h.Set("Content-Length","04")},
  "truncated body":func(h *http.Header,p *Pin,s *int,a,b,n *int64){*n=3},
  "extra body":func(h *http.Header,p *Pin,s *int,a,b,n *int64){*n=5},
  "compressed":func(h *http.Header,p *Pin,s *int,a,b,n *int64){h.Set("Content-Encoding","gzip")},
  "multipart":func(h *http.Header,p *Pin,s *int,a,b,n *int64){h.Set("Content-Type","multipart/byteranges; boundary=x")},
  "transfer encoding":func(h *http.Header,p *Pin,s *int,a,b,n *int64){h.Set("Transfer-Encoding","chunked")},
  "invalid range":func(h *http.Header,p *Pin,s *int,a,b,n *int64){*b=12},
  "negative start":func(h *http.Header,p *Pin,s *int,a,b,n *int64){*a=-1},
  "empty total":func(h *http.Header,p *Pin,s *int,a,b,n *int64){p.Total=0},
 }
 for name,mutate:=range cases {
  t.Run(name,func(t *testing.T){
   h,p:=fixture();status:=206;start,end,received:=int64(2),int64(5),int64(4)
   mutate(&h,&p,&status,&start,&end,&received)
   if err:=Check(status,h,p,start,end,received);!errors.Is(err,ErrMismatch){t.Fatalf("expected rejection: %v",err)}
  })
 }
}
