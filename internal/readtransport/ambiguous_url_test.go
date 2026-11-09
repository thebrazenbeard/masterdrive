package readtransport

import (
 "context"
 "errors"
 "net/http"
 "testing"
)

func TestRejectAmbiguousURLRepresentations(t *testing.T) {
 const authority = "mesh.example.test"
 for _, raw := range []string{
  "https://mesh.example.test/file?",
  "https://mesh.example.test/a%2Fb",
  "https://mesh.example.test/a%2fb",
 } {
  t.Run(raw, func(t *testing.T) {
   if _, err := NewRequest(context.Background(), raw, authority); !errors.Is(err, ErrEndpoint) {
    t.Fatalf("NewRequest(%q) error = %v, want ErrEndpoint", raw, err)
   }
  })
 }
}

func TestAuthorizedTransportRejectsAmbiguousURLBeforeNetwork(t *testing.T) {
 client, err := NewAuthorizedClient("mesh.example.test")
 if err != nil { t.Fatal(err) }
 for _, raw := range []string{"https://mesh.example.test/file?", "https://mesh.example.test/a%2Fb"} {
  req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, raw, nil)
  if err != nil { t.Fatal(err) }
  if _, err := client.Do(req); !errors.Is(err, ErrEndpoint) {
   t.Fatalf("Do(%q) error = %v, want ErrEndpoint", raw, err)
  }
 }
}
