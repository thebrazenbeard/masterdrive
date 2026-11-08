package readtransport

import (
 "context"
 "errors"
 "net/http"
 "net/http/httptest"
 "testing"
 "time"
)

func TestNewRequest(t *testing.T) {
 const authority = "node.tailnet.ts.net:443"
 valid := "https://" + authority + "/share/file"
 req, err := NewRequest(context.Background(), valid, authority)
 if err != nil || req.Method != http.MethodGet || req.URL.Host != authority { t.Fatalf("valid request: %v", err) }
 cases := []struct{name, url, authority string}{
  {"http", "http://"+authority+"/file", authority},
  {"other host", "https://other.tailnet.ts.net/file", authority},
  {"prefix host", "https://"+authority+".evil/file", authority},
  {"userinfo", "https://user@"+authority+"/file", authority},
  {"fragment", valid+"#secret", authority},
  {"query", valid+"?token=secret", authority},
  {"no path", "https://"+authority, authority},
  {"no authority", valid, ""},
  {"trailing dot", "https://node.tailnet.ts.net.:443/file", "node.tailnet.ts.net.:443"},
  {"malformed", "://", authority},
  {"empty", "", authority},
 }
 for _, tc := range cases { t.Run(tc.name, func(t *testing.T) {
  if _, err := NewRequest(context.Background(), tc.url, tc.authority); !errors.Is(err, ErrEndpoint) { t.Fatalf("expected ErrEndpoint, got %v", err) }
 }) }
 if _, err := NewRequest(nil, valid, authority); !errors.Is(err, ErrEndpoint) { t.Fatalf("nil context: %v", err) }
}

func TestClientDoesNotFollowRedirect(t *testing.T) {
 destinationReached := false
 destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { destinationReached = true; w.WriteHeader(http.StatusOK) }))
 defer destination.Close()
 origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, destination.URL, http.StatusFound) }))
 defer origin.Close()
 client := NewClient()
 response, err := client.Get(origin.URL)
 if err != nil { t.Fatal(err) }
 defer response.Body.Close()
 if response.StatusCode != http.StatusFound || destinationReached { t.Fatalf("redirect followed: status=%d reached=%v", response.StatusCode, destinationReached) }
}

func TestClientSecurityDefaults(t *testing.T) {
 c := NewClient()
 if c.Timeout <= 0 || c.Timeout > time.Minute || c.Jar != nil || c.CheckRedirect == nil { t.Fatal("unsafe client defaults") }
 tr, ok := c.Transport.(*http.Transport)
 if !ok || tr.Proxy != nil || !tr.DisableCompression || !tr.DisableKeepAlives || tr.ResponseHeaderTimeout <= 0 ||
  tr.TLSHandshakeTimeout <= 0 || tr.MaxResponseHeaderBytes <= 0 || tr.TLSClientConfig == nil { t.Fatal("unsafe transport defaults") }
}
