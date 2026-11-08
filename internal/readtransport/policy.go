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
  u.RawQuery != "" || !strings.EqualFold(u.Scheme, "https") ||
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
