// Package readtransport defines conservative HTTP client-side preconditions for
// future read-only mesh transfers. It does not establish device identity or grants.
package readtransport

import (
 "context"
 "crypto/tls"
 "errors"
 "net"
 "net/http"
 "net/url"
 "strings"
 "time"
)

var ErrEndpoint = errors.New("readtransport: unauthorized HTTPS endpoint")

// NewRequest creates a GET request only for an exact, previously authorized
// HTTPS authority (host and optional port). Do not derive authority from the peer.
// Redirects must remain disabled on the client. This does not verify grants.
func NewRequest(ctx context.Context, rawURL, authorizedAuthority string) (*http.Request, error) {
 if ctx == nil || authorizedAuthority == "" { return nil, ErrEndpoint }
 u, err := url.Parse(rawURL)
 if err != nil || u == nil || u.Opaque != "" || u.User != nil || u.Fragment != "" ||
  u.RawQuery != "" || u.ForceQuery || u.RawPath != "" || u.RawFragment != "" || !strings.EqualFold(u.Scheme, "https") ||
  u.Host == "" || u.Host != authorizedAuthority || u.Hostname() == "" ||
  strings.HasSuffix(u.Hostname(), ".") || u.Path == "" {
  return nil, ErrEndpoint
 }
 if strings.ContainsAny(u.Host, "@\\") { return nil, ErrEndpoint }
 req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
 if err != nil { return nil, ErrEndpoint }
 return req, nil
}

// NewClient is a dedicated, cookie-free, redirect-denying client with finite
// connection, header, and total-request deadlines. It deliberately ignores
// environment proxy variables to avoid sending private mesh traffic to a proxy.
// Caller must close response bodies and independently validate TLS peer identity,
// source grants, response metadata, and exact bytes before exposing content.
func NewClient() *http.Client {
 return &http.Client{
  Timeout: 30 * time.Second,
  CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
  Transport: &http.Transport{
   Proxy: nil,
   DialContext: (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 15 * time.Second}).DialContext,
   TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
   TLSHandshakeTimeout: 5 * time.Second,
   ResponseHeaderTimeout: 10 * time.Second,
   ExpectContinueTimeout: time.Second,
   DisableCompression: true,
   DisableKeepAlives: true,
   MaxResponseHeaderBytes: 32 << 10,
  },
 }
}


// authorizedRoundTripper enforces destination admission even when callers
// construct requests without NewRequest or mutate a request before Do.
type authorizedRoundTripper struct {
 authority string
 base http.RoundTripper
}

func (t authorizedRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
 if req == nil || req.URL == nil || req.Method != http.MethodGet ||
  req.Host != req.URL.Host || req.URL.Host != t.authority ||
  req.Body != nil || req.GetBody != nil {
  return nil, ErrEndpoint
 }
 if _, err := NewRequest(req.Context(), req.URL.String(), t.authority); err != nil {
  return nil, ErrEndpoint
 }
 // A redirect response is never a usable read representation. Reject it at
 // the transport boundary as well as disabling automatic redirect following.
 resp, err := t.base.RoundTrip(req)
 if err != nil { return resp, err }
 if resp == nil { return nil, ErrEndpoint }
 if resp.StatusCode >= 300 && resp.StatusCode < 400 {
  if resp.Body != nil { _ = resp.Body.Close() }
  return nil, ErrEndpoint
 }
 return resp, nil
}

// NewAuthorizedClient binds the HTTPS destination at the transport boundary.
// This prevents bypassing NewRequest through client.Do or client.Get.
// Authority is an operator-pinned exact host[:port], not peer-provided data.
// It does not establish source grants, Tailscale identity, or immutable bytes.
func NewAuthorizedClient(authority string) (*http.Client, error) {
 if authority == "" { return nil, ErrEndpoint }
 probe, err := NewRequest(context.Background(), "https://"+authority+"/", authority)
 if err != nil || probe.URL.Host != authority { return nil, ErrEndpoint }
 c := NewClient()
 c.Transport = authorizedRoundTripper{authority: authority, base: c.Transport}
 return c, nil
}
