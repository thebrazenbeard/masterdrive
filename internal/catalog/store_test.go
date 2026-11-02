package catalog

import (
	"errors"
	"fmt"
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

func TestReloadObservesSeparateProcessWrites(t *testing.T) {
	dir := t.TempDir()
	controller, err := Open(dir)
	if err != nil { t.Fatal(err) }
	if err := controller.Reload(); err != nil { t.Fatalf("empty new catalog should reload: %v", err) }
	cli, err := Open(dir)
	if err != nil { t.Fatal(err) }
	h, err := cli.Upsert("Peer", "https://peer.example")
	if err != nil { t.Fatal(err) }
	if len(controller.List()) != 0 { t.Fatal("controller snapshot changed without reload") }
	if err := controller.Reload(); err != nil { t.Fatal(err) }
	if got, ok := controller.Get(h.ID); !ok || got.Name != "Peer" { t.Fatalf("controller missed CLI registration: %+v, %t", got, ok) }
	if err := cli.Remove(h.ID); err != nil { t.Fatal(err) }
	if err := controller.Reload(); err != nil { t.Fatal(err) }
	if got := controller.List(); len(got) != 0 { t.Fatalf("controller missed CLI removal: %+v", got) }
}

func TestReloadRejectsCorruptDiskAndPreservesLastSnapshot(t *testing.T) {
	dir := t.TempDir()
	writer, err := Open(dir)
	if err != nil { t.Fatal(err) }
	h, err := writer.Upsert("Peer", "https://peer.example")
	if err != nil { t.Fatal(err) }
	controller, err := Open(dir)
	if err != nil { t.Fatal(err) }
	path := filepath.Join(dir, "hosts.json")
	badStates := [][]byte{
		[]byte(`{"version":2,"hosts":[]}`),
		[]byte(`{"version":1,"hosts":[{"id":"forged"}]}`),
		[]byte(`not-json`),
	}
	for _, bad := range badStates {
		if err := os.WriteFile(path, bad, 0600); err != nil { t.Fatal(err) }
		if err := controller.Reload(); err == nil { t.Fatalf("accepted bad state: %s", bad) }
		if got, ok := controller.Get(h.ID); !ok || got.Name != "Peer" { t.Fatalf("corrupt disk clobbered last good snapshot: %+v, %t", got, ok) }
	}
	if err := os.Remove(path); err != nil { t.Fatal(err) }
	if err := controller.Reload(); err == nil { t.Fatal("missing established disk state must fail") }
	if _, ok := controller.Get(h.ID); !ok { t.Fatal("missing file clobbered state") }
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
			_, err := s.Upsert("host", fmt.Sprintf("https://node%d.example", i))
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

// Two CLI processes can both open a catalog before either commits. The
// second must merge the first edit instead of rewriting its stale snapshot.
func TestIndependentWritersDoNotLoseUpdates(t *testing.T) {
	dir := t.TempDir()
	first, err := Open(dir)
	if err != nil { t.Fatal(err) }
	second, err := Open(dir)
	if err != nil { t.Fatal(err) }
	firstHost, err := first.Upsert("First", "https://first.example")
	if err != nil { t.Fatal(err) }
	secondHost, err := second.Upsert("Second", "https://second.example")
	if err != nil { t.Fatal(err) }
	third, err := Open(dir)
	if err != nil { t.Fatal(err) }
	if got := len(third.List()); got != 2 { t.Fatalf("stale writer erased registration: got %d hosts", got) }
	if _, exists := third.Get(firstHost.ID); !exists { t.Fatal("first registration lost") }
	if _, exists := third.Get(secondHost.ID); !exists { t.Fatal("second registration lost") }
	if err := second.Remove(firstHost.ID); err != nil { t.Fatalf("second writer should re-read first registration before removal: %v", err) }
	if err := first.Reload(); err != nil { t.Fatal(err) }
	if _, exists := first.Get(firstHost.ID); exists { t.Fatal("removal not visible to reloader") }
	if _, exists := first.Get(secondHost.ID); !exists { t.Fatal("removal erased another registration") }
}

func TestConcurrentIndependentStoresKeepAllHosts(t *testing.T) {
	dir := t.TempDir()
	const count = 16
	stores := make([]*Store, count)
	for i := range stores {
		var err error
		stores[i], err = Open(dir)
		if err != nil { t.Fatal(err) }
	}
	var wg sync.WaitGroup
	errCh := make(chan error, count)
	for i := range stores {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := stores[i].Upsert(fmt.Sprintf("Host %d", i), fmt.Sprintf("https://host-%d.example", i))
			errCh <- err
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh { if err != nil { t.Fatal(err) } }
	fresh, err := Open(dir)
	if err != nil { t.Fatal(err) }
	if got := len(fresh.List()); got != count { t.Fatalf("lost concurrent cross-store updates: got %d, want %d", got, count) }
}

func TestWriteLockFailsClosedWithoutDestroyingState(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil { t.Fatal(err) }
	h, err := s.Upsert("Safe", "https://safe.example")
	if err != nil { t.Fatal(err) }
	lockPath := filepath.Join(dir, "hosts.json.lock")
	if err := os.WriteFile(lockPath, []byte("simulated active writer"), 0600); err != nil { t.Fatal(err) }
	if _, err := s.Upsert("Blocked", "https://blocked.example"); !errors.Is(err, ErrBusy) {
		t.Fatalf("contended mutation must fail closed with ErrBusy: %v", err)
	}
	if _, exists := s.Get(h.ID); !exists { t.Fatal("contended mutation changed snapshot") }
	if _, err := Open(dir); err != nil { t.Fatalf("active write lock must not prevent snapshot reads: %v", err) }
	if _, err := os.Stat(lockPath); err != nil { t.Fatalf("contended write lock was cleared: %v", err) }
	if err := os.Remove(lockPath); err != nil { t.Fatal(err) }
	reloaded, err := Open(dir)
	if err != nil { t.Fatal(err) }
	if _, exists := reloaded.Get(h.ID); !exists { t.Fatal("contended lock damaged committed state") }
}

func TestMissingEstablishedStateCannotBeRecreatedByStaleWriter(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil { t.Fatal(err) }
	if _, err := s.Upsert("Safe", "https://safe.example"); err != nil { t.Fatal(err) }
	if err := os.Remove(filepath.Join(dir, "hosts.json")); err != nil { t.Fatal(err) }
	if _, err := s.Upsert("Other", "https://other.example"); err == nil { t.Fatal("writer silently reset missing established state") }
	if _, err := os.Stat(filepath.Join(dir, "hosts.json")); !errors.Is(err, os.ErrNotExist) { t.Fatalf("missing state unexpectedly regenerated: %v", err) }
}

// Existence must be decided by the successful file open, not a later Stat.
// A file can disappear immediately after the read, but the caller still
// knows it observed an established catalog and must refuse silent reset.
func TestReadHostsExistenceIsBoundToOpenedSnapshot(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil { t.Fatal(err) }
	if _, err := s.Upsert("Safe", "https://safe.example"); err != nil { t.Fatal(err) }
	path := filepath.Join(dir, "hosts.json")
	hosts, existed, err := readHosts(path, true)
	if err != nil || !existed || len(hosts) != 1 {
		t.Fatalf("valid snapshot must report established state: existed=%v hosts=%d err=%v", existed, len(hosts), err)
	}
	if err := os.Remove(path); err != nil { t.Fatal(err) }
	if !existed { t.Fatal("successful open lost its existence evidence after removal") }
	missing, existedNow, err := readHosts(path, true)
	if err != nil || existedNow || len(missing) != 0 {
		t.Fatalf("fresh catalog should report absent file: existed=%v hosts=%d err=%v", existedNow, len(missing), err)
	}
}
