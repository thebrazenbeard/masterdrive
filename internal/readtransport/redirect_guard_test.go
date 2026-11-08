package readtransport

import (
 "context"
 "errors"
 "io"
 "net/http"
 "strings"
 "testing"
)

type redirectProbe struct { calls int; body *closeProbe }
type closeProbe struct { io.Reader; closed bool }
func (b *closeProbe) Close() error { b.closed = true; return nil }
func (p *redirectProbe) RoundTrip(*http.Request) (*http.Response, error) {
 p.calls++
 p.body = &closeProbe{Reader: strings.NewReader("redirect body")}
 return &http.Response{
  StatusCode: http.StatusFound,
  Header: http.Header{"Location": []string{"https://other.tailnet.ts.net/file"}},
  Body: p.body,
 }, nil
}

func TestAuthorizedClientRejectsRedirectAndClosesBody(t *testing.T) {
 const authority = "node.tailnet.ts.net:443"
 client, err := NewAuthorizedClient(authority)
 if err != nil { t.Fatal(err) }
 guard, ok := client.Transport.(authorizedRoundTripper)
 if !ok { t.Fatal("authorized guard absent") }
 probe := &redirectProbe{}
 guard.base = probe
 client.Transport = guard
 req, err := NewRequest(context.Background(), "https://"+authority+"/share/file", authority)
 if err != nil { t.Fatal(err) }
 resp, err := client.Do(req)
 if !errors.Is(err, ErrEndpoint) {
  if resp != nil && resp.Body != nil { _ = resp.Body.Close() }
  t.Fatalf("redirect not rejected: response=%v err=%v", resp, err)
 }
 if resp != nil { t.Fatalf("redirect response leaked: %v", resp.StatusCode) }
 if probe.calls != 1 || probe.body == nil || !probe.body.closed {
  t.Fatalf("redirect not closed: calls=%d body=%+v", probe.calls, probe.body)
 }
}
