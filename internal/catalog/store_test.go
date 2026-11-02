package catalog

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPersistentExplicitRegistration(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil { t.Fatal(err) }
	h, err := s.Upsert("Lappy", "HTTPS://Storage.EXAMPLE:443/")
	if err != nil { t.Fatal(err) }
	if h.BaseURL != "https://storage.example" || h.ID == "" || h.Health != HealthUnknown { t.Fatalf("unexpected host: %+v", h) }
	second, err := s.Upsert("Main laptop", "https://storage.example")
	if err != nil { t.Fatal(err) }
	if second.ID != h.ID || second.AddedAt != h.AddedAt || second.Name != "Main laptop" { t.Fatalf("upsert did not preserve identity: %+v", second) }
	if hosts := s.List(); len(hosts) != 1 { t.Fatalf("want one host, got %d", len(hosts)) }
	reopened, err := Open(dir)
	if err != nil { t.Fatal(err) }
	got, ok := reopened.Get(h.ID)
	if !ok || got.ID != second.ID || got.Name != second.Name || got.BaseURL != second.BaseURL || !got.AddedAt.Equal(second.AddedAt) || got.Health != second.Health { t.Fatalf("roundtrip mismatch: %+v, %t", got, ok) }
	if err := reopened.Remove(h.ID); err != nil { t.Fatal(err) }
	if err := reopened.Remove(h.ID); !errors.Is(err, ErrNotFound) { t.Fatalf("want ErrNotFound, got %v", err) }
	reopenedAgain, err := Open(dir)
	if err != nil { t.Fatal(err) }
	if len(reopenedAgain.List()) != 0 { t.Fatal("removed host survived reopen") }
}

func TestCanonicalEndpointRejectsInsecureAndCredentialedURLs(t *testing.T) {
	valid := map[string]string{
		"http://localhost:8080/": "http://localhost:8080",
		"http://127.0.0.1:80": "http://127.0.0.1",
		"http://[::1]:9090": "http://[::1]:9090",
		"https://Example.NET:443/": "https://example.net",
		"https://[2001:db8::1]:8443": "https://[2001:db8::1]:8443",
	}
	for raw, want := range valid {
		got, err := CanonicalEndpoint(raw)
		if err != nil || got != want { t.Errorf("CanonicalEndpoint(%q) = %q, %v; want %q", raw, got, err, want) }
	}
	invalid := []string{
		"", "http://storage.example", "http://192.168.1.2:8080",
		"https://user:password@storage.example", "https://storage.example/api",
		"https://storage.example/?token=secret", "https://storage.example/#fragment",
		"file:///tmp/files", "https://bad..host", "https://example.com:70000",
		"https://example.com:0", "https://example.com:bad", "https://[fe80::1%25eth0]",
	}
	for _, raw := range invalid {
		if _, err := CanonicalEndpoint(raw); err == nil { t.Errorf("unexpectedly accepted %q", raw) }
	}
}

func TestExplicitHealthMetadataOnly(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil { t.Fatal(err) }
	h, err := s.Upsert("Node", "https://node.example")
	if err != nil { t.Fatal(err) }
	if h.LastCheckedAt != nil { t.Fatal("registration fabricated a probe") }
	if err := s.SetHealth(h.ID, HealthOnline, time.Time{}); err == nil { t.Fatal("accepted zero timestamp") }
	if err := s.SetHealth(h.ID, "pretend", time.Now()); err == nil { t.Fatal("accepted unknown status") }
	when := time.Now().Add(-time.Second).UTC().Truncate(time.Second)
	if err := s.SetHealth(h.ID, HealthOffline, when); err != nil { t.Fatal(err) }
	got, _ := s.Get(h.ID)
	if got.Health != HealthOffline || got.LastCheckedAt == nil || !got.LastCheckedAt.Equal(when) { t.Fatalf("health mismatch %+v", got) }
	if err := s.SetHealth("missing", HealthOnline, when); !errors.Is(err, ErrNotFound) { t.Fatalf("want ErrNotFound, got %v", err) }
}

func TestRefusesCorruptStateWithoutErasingIt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hosts.json")
	bad := []byte(`{"version":1,"hosts":[{"id":"forged","name":"host","base_url":"https://x.example","added_at":"2026-01-01T00:00:00Z","health":"unknown"}]}`)
	if err := os.WriteFile(path, bad, 0600); err != nil { t.Fatal(err) }
	if _, err := Open(dir); err == nil { t.Fatal("accepted forged ID") }
	got, err := os.ReadFile(path)
	if err != nil { t.Fatal(err) }
	if string(got) != string(bad) { t.Fatal("corrupt file was silently changed") }
}

func TestCatalogHasNoCredentialField(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil { t.Fatal(err) }
	if _, err := s.Upsert("secret", "https://user:highly-secret@example.com"); err == nil { t.Fatal("credentialed endpoint accepted") }
	if _, err := s.Upsert("safe", "https://example.com"); err != nil { t.Fatal(err) }
	data, err := os.ReadFile(filepath.Join(dir, "hosts.json"))
	if err != nil { t.Fatal(err) }
	if strings.Contains(string(data), "highly-secret") || strings.Contains(string(data), "token") || strings.Contains(string(data), "password") {
		t.Fatal("credential-like data leaked into registry")
	}
}

func TestConcurrentUpdatesKeepEveryHost(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil { t.Fatal(err) }
	const count = 32
	var wg sync.WaitGroup
	errCh := make(chan error, count)
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := s.Upsert("host", "https://node"+string(rune('a'+i))+".example")
			errCh <- err
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh { if err != nil { t.Fatal(err) } }
	if got := len(s.List()); got != count { t.Fatalf("lost concurrent registration: %d != %d", got, count) }
}

func TestSnapshotCannotMutateStore(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil { t.Fatal(err) }
	h, err := s.Upsert("one", "https://one.example")
	if err != nil { t.Fatal(err) }
	when := time.Now().UTC()
	if err := s.SetHealth(h.ID, HealthOnline, when); err != nil { t.Fatal(err) }
	items := s.List()
	items[0].Name = "modified"
	*items[0].LastCheckedAt = time.Time{}
	got, _ := s.Get(h.ID)
	if got.Name != "one" || got.LastCheckedAt.IsZero() { t.Fatal("snapshot mutated internal state") }
}
