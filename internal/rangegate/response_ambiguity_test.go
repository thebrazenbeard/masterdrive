package rangegate

import (
 "net/http"
 "testing"
)

func TestAmbiguousResponseHeadersFailClosed(t *testing.T) {
 pin := Pin{ETag: "\"fixed\"", Total: 8}
 base := func() http.Header {
  h := make(http.Header)
  h.Set("ETag", pin.ETag)
  h.Set("Content-Range", "bytes 0-2/8")
  h.Set("Content-Length", "3")
  return h
 }
 cases := []struct {
  name string
  mutate func(http.Header)
 }{
  {"duplicate-etag",func(h http.Header){ h.Add("ETag",pin.ETag) }},
  {"case-variant-etag",func(h http.Header){ h["etag"]=[]string{pin.ETag} }},
  {"duplicate-range",func(h http.Header){ h.Add("Content-Range","bytes 0-2/8") }},
  {"case-variant-range",func(h http.Header){ h["content-range"]=[]string{"bytes 0-2/8"} }},
  {"duplicate-length",func(h http.Header){ h.Add("Content-Length","3") }},
  {"case-variant-length",func(h http.Header){ h["content-length"]=[]string{"3"} }},
  {"empty-encoding",func(h http.Header){ h["Content-Encoding"]=[]string{""} }},
  {"empty-transfer-encoding",func(h http.Header){ h["Transfer-Encoding"]=[]string{""} }},
  {"duplicate-content-type",func(h http.Header){ h["Content-Type"]=[]string{"application/octet-stream","application/octet-stream"} }},
  {"noncanonical-encoding",func(h http.Header){ h["content-encoding"]=[]string{"gzip"} }},
  {"noncanonical-transfer-encoding",func(h http.Header){ h["transfer-encoding"]=[]string{"chunked"} }},
 }
 if err:=Check(http.StatusPartialContent,base(),pin,0,2,3);err!=nil {
  t.Fatalf("valid control rejected: %v",err)
 }
 for _,tc:=range cases {
  t.Run(tc.name,func(t *testing.T){
   h:=base()
   tc.mutate(h)
   if err:=Check(http.StatusPartialContent,h,pin,0,2,3);err==nil {
    t.Fatal("ambiguous response accepted")
   }
  })
 }
}
