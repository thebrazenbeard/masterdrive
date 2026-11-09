package readtransport

import (
 "context"
 "errors"
 "io"
 "net/http"
 "strings"
 "testing"
)

type statusProbeTransport struct {
 status int
 body *statusProbeBody
 calls int
}
func (p *statusProbeTransport) RoundTrip(*http.Request) (*http.Response, error) {
 p.calls++
 p.body = &statusProbeBody{Reader: strings.NewReader("test")}
 return &http.Response{StatusCode:p.status, Body:p.body, Header:make(http.Header)}, nil
}
type statusProbeBody struct {
 io.Reader
 closed bool
}
func (b *statusProbeBody) Close() error { b.closed=true; return nil }

func TestAuthorizedTransportRejectsNonRepresentationStatuses(t *testing.T) {
 for _, status := range []int{100, 101, 103, 201, 204, 301, 302, 304, 307, 308, 400, 401, 403, 404, 416, 429, 500, 503} {
  t.Run(http.StatusText(status), func(t *testing.T) {
   probe := &statusProbeTransport{status:status}
   guard := authorizedRoundTripper{authority:"mesh.example.test",base:probe}
   req, err := NewRequest(context.Background(),"https://mesh.example.test/file","mesh.example.test")
   if err != nil { t.Fatal(err) }
   resp, err := guard.RoundTrip(req)
   if !errors.Is(err,ErrEndpoint) || resp != nil { t.Fatalf("status %d: resp=%v err=%v; want nil, ErrEndpoint",status,resp,err) }
   if probe.calls != 1 || probe.body == nil || !probe.body.closed { t.Fatalf("status %d: transport calls=%d, body=%+v",status,probe.calls,probe.body) }
  })
 }
}
func TestAuthorizedTransportAcceptsOnlyRepresentationStatuses(t *testing.T) {
 for _, status := range []int{http.StatusOK,http.StatusPartialContent} {
  probe := &statusProbeTransport{status:status}
  guard := authorizedRoundTripper{authority:"mesh.example.test",base:probe}
  req, err := NewRequest(context.Background(),"https://mesh.example.test/file","mesh.example.test")
  if err != nil { t.Fatal(err) }
  resp, err := guard.RoundTrip(req)
  if err != nil || resp == nil || resp.StatusCode != status { t.Fatalf("status %d: resp=%v err=%v",status,resp,err) }
  if probe.body.closed { t.Fatal("valid response body closed prematurely") }
  _ = resp.Body.Close()
 }
}
