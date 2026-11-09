package readtransport

import (
 "context"
 "errors"
 "net/http"
 "testing"
)

type nilBodyTransport struct { calls int }
func (p *nilBodyTransport) RoundTrip(*http.Request) (*http.Response, error) {
 p.calls++
 return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header)}, nil
}

func TestAuthorizedTransportRejectsSuccessWithoutBody(t *testing.T) {
 probe := &nilBodyTransport{}
 guard := authorizedRoundTripper{authority:"mesh.example.test", base:probe}
 req, err := NewRequest(context.Background(), "https://mesh.example.test/file", "mesh.example.test")
 if err != nil { t.Fatal(err) }
 resp, err := guard.RoundTrip(req)
 if !errors.Is(err, ErrEndpoint) || resp != nil {
  t.Fatalf("missing body: response=%v error=%v, want nil ErrEndpoint", resp, err)
 }
 if probe.calls != 1 { t.Fatalf("transport calls=%d, want 1", probe.calls) }
}
