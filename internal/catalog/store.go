// Package catalog stores the explicitly registered MasterDrive host catalog.
// It does not discover hosts, probe networks, or grant access to remote files.
package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	catalogVersion = 1
	maxCatalogBytes = 2 << 20
	maxHosts = 1000
	writeLockTimeout = 5 * time.Second
	// Native Windows stress testing observed transient CREATE_NEW access
	// denials amid concurrent lock creation/removal. Retry narrowly and briefly;
	// a persistent permission failure must still be returned as permission.
	writeLockWindowsDeniedRetry = 300 * time.Millisecond
)

var (
	ErrNotFound = errors.New("catalog: host not found")
	ErrInvalid = errors.New("catalog: invalid host")
	ErrBusy = errors.New("catalog: another writer holds the catalog lock")
)

// HealthStatus is supplied by a caller; the catalog never measures reachability.
type HealthStatus string

const (
	HealthUnknown HealthStatus = "unknown"
	HealthOnline HealthStatus = "online"
	HealthOffline HealthStatus = "offline"
	HealthDegraded HealthStatus = "degraded"
)

// Host is public catalog metadata only. Credentials must be held by a separate
// secret provider and are deliberately absent from both Host and disk state.
type Host struct {
	ID string `json:"id"`
	Name string `json:"name"`
	BaseURL string `json:"base_url"`
	AddedAt time.Time `json:"added_at"`
	Health HealthStatus `json:"health"`
	LastCheckedAt *time.Time `json:"last_checked_at,omitempty"`
}

type diskDocument struct {
	Version int `json:"version"`
	Hosts []Host `json:"hosts"`
}

// Store protects in-process access and uses a portable exclusive lock file
// around the complete read/modify/write operation. Each mutation re-reads
// disk while holding the lock, so separate CLI processes cannot silently
// overwrite each other's changes. A process crash may leave a stale lock;
// operators must verify that no writer is alive before removing it manually.
// Persistence uses a temporary file and rename, but Go does not guarantee
// atomic rename on non-Unix systems. Windows crash/recovery needs qualification.
type Store struct {
	mu sync.RWMutex
	path string
	hosts map[string]Host
	// True once a state file has been read or written. Losing an established
	// file must fail closed rather than silently resetting all hosts.
	established bool
}

// Open loads a versioned catalog from dir, creating the directory if absent.
// An existing malformed catalog is rejected, never replaced with an empty one.
func Open(dir string) (*Store, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, fmt.Errorf("%w: empty data directory", ErrInvalid)
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("catalog: create directory: %w", err)
	}
	s := &Store{path: filepath.Join(dir, "hosts.json")}
	hosts, existed, err := readHosts(s.path, true)
	if err != nil {
		return nil, err
	}
	s.hosts = hosts
	// Existence is captured by the same open that returned the snapshot.
	// A separate Stat would race with deletion/replacement after read.
	s.established = existed
	return s, nil
}

// Reload atomically replaces the in-memory snapshot with a validated disk
// snapshot. It is intended for a long-lived read-only controller observing
// edits by a separate CLI process. A malformed, missing or oversized file
// leaves the last valid in-memory state untouched. The caller should surface
// that error rather than silently reporting stale contents as current.
//
// Concurrent writers are serialized separately by mutate's exclusive lock.
// Readers may Reload during replacement. Platform read errors (and malformed
// snapshots) must be surfaced, never silently masked with stale in-memory data.
func (s *Store) Reload() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	hosts, _, err := readHosts(s.path, false)
	if errors.Is(err, os.ErrNotExist) && !s.established {
		// A fresh catalog has no file until the first registration.
		return nil
	}
	if err != nil {
		return err
	}
	s.hosts = hosts
	s.established = true
	return nil
}

// readHosts returns existed=true only when it opened a state file. Open uses
// this flag to remember an established catalog without a separate racy Stat.
func readHosts(path string, allowMissing bool) (map[string]Host, bool, error) {
	hosts := make(map[string]Host)
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		if allowMissing {
			return hosts, false, nil
		}
		return nil, false, fmt.Errorf("catalog: missing state file: %w", err)
	}
	if err != nil {
		return nil, false, fmt.Errorf("catalog: open: %w", err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxCatalogBytes+1))
	if err != nil {
		return nil, true, fmt.Errorf("catalog: read: %w", err)
	}
	if len(data) > maxCatalogBytes {
		return nil, true, fmt.Errorf("catalog: file exceeds size limit")
	}
	var doc diskDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, true, fmt.Errorf("catalog: invalid JSON: %w", err)
	}
	if doc.Version != catalogVersion {
		return nil, true, fmt.Errorf("catalog: unsupported version %d", doc.Version)
	}
	if len(doc.Hosts) > maxHosts {
		return nil, true, fmt.Errorf("catalog: too many hosts")
	}
	for _, h := range doc.Hosts {
		canonical, id, err := validateHost(h.Name, h.BaseURL)
		if err != nil || h.BaseURL != canonical || h.ID != id || !validHealth(h.Health) || h.AddedAt.IsZero() || (h.LastCheckedAt != nil && h.LastCheckedAt.IsZero()) {
			return nil, true, fmt.Errorf("catalog: invalid stored host %q", h.ID)
		}
		if _, exists := hosts[id]; exists {
			return nil, true, fmt.Errorf("catalog: duplicate stored host %q", h.ID)
		}
		hosts[id] = h
	}
	return hosts, true, nil
}

// Upsert registers or renames a host chosen explicitly by the caller. The ID
// is stable for the same canonical endpoint; no network operation is performed.
func (s *Store) Upsert(name, baseURL string) (Host, error) {
	canonical, id, err := validateHost(name, baseURL)
	if err != nil {
		return Host{}, err
	}
	var result Host
	err = s.mutate(func(current map[string]Host) (map[string]Host, error) {
		h, exists := current[id]
		if exists {
			h.Name = strings.TrimSpace(name)
		} else {
			if len(current) >= maxHosts {
				return nil, fmt.Errorf("catalog: host limit reached")
			}
			h = Host{ID: id, Name: strings.TrimSpace(name), BaseURL: canonical, AddedAt: time.Now().UTC(), Health: HealthUnknown}
		}
		current[id] = h
		result = h
		return current, nil
	})
	return result, err
}

// Remove forgets a host from this catalog. It does NOT revoke any remote
// credential or delete any remote data; those effects need separate authority.
func (s *Store) Remove(id string) error {
	return s.mutate(func(current map[string]Host) (map[string]Host, error) {
		if _, ok := current[id]; !ok {
			return nil, ErrNotFound
		}
		delete(current, id)
		return current, nil
	})
}

// Get returns the recorded metadata and whether it exists.
func (s *Store) Get(id string) (Host, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	h, ok := s.hosts[id]
	return cloneHost(h), ok
}

// List returns a stable name/ID-sorted snapshot, not live discovery results.
func (s *Store) List() []Host {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return sorted(s.hosts)
}

// SetHealth records a caller-supplied probe result and timestamp. Only the
// component doing a real probe may describe the value as a reachability result.
func (s *Store) SetHealth(id string, status HealthStatus, checkedAt time.Time) error {
	if !validHealth(status) || checkedAt.IsZero() || checkedAt.After(time.Now().Add(time.Minute)) {
		return fmt.Errorf("%w: invalid health metadata", ErrInvalid)
	}
	return s.mutate(func(current map[string]Host) (map[string]Host, error) {
		h, ok := current[id]
		if !ok {
			return nil, ErrNotFound
		}
		t := checkedAt.UTC()
		h.Health = status
		h.LastCheckedAt = &t
		current[id] = h
		return current, nil
	})
}

// mutate serializes all writers across processes and refreshes state before
// applying each change. A failed read or write preserves the in-memory state.
func (s *Store) mutate(change func(map[string]Host) (map[string]Host, error)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return withWriteLock(s.path+".lock", func() error {
		current, _, err := readHosts(s.path, !s.established)
		if err != nil {
			return err
		}
		next, err := change(current)
		if err != nil {
			return err
		}
		if err := s.persist(next); err != nil {
			return err
		}
		s.hosts = next
		s.established = true
		return nil
	})
}

// O_EXCL creation is atomic on supported local filesystems on Windows and
// Unix. Never automatically break a possibly stale lock: doing so could let
// two live writers commit conflicting snapshots.
func withWriteLock(lockPath string, update func() error) error {
	deadline := time.Now().Add(writeLockTimeout)
	var firstWindowsDenied time.Time
	var f *os.File
	for {
		var err error
		f, err = os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err == nil {
			break
		}
		if retryWindowsLockCreateDenied(lockPath, err) {
			if firstWindowsDenied.IsZero() {
				firstWindowsDenied = time.Now()
			}
			if time.Since(firstWindowsDenied) < writeLockWindowsDeniedRetry && time.Now().Before(deadline) {
				time.Sleep(5 * time.Millisecond)
				continue
			}
			// Do not turn a permanent ACL/permissions error into ErrBusy.
			return fmt.Errorf("catalog: create write lock: %w", err)
		}
		if !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("catalog: create write lock: %w", err)
		}
		// An observed existing lock breaks a preceding access-denied streak.
		// A later unlink race gets its own short retry window, while the
		// overall lock acquisition remains bounded by writeLockTimeout.
		firstWindowsDenied = time.Time{}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("%w: %s (verify all writers have exited before manually clearing a stale lock)", ErrBusy, lockPath)
		}
		time.Sleep(25 * time.Millisecond)
	}
	// A PID and timestamp aid manual recovery after a crash; they are not
	// trusted for automatically declaring the lock stale.
	if _, err := fmt.Fprintf(f, "pid=%d acquired=%s\n", os.Getpid(), time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		f.Close()
		os.Remove(lockPath)
		return fmt.Errorf("catalog: record write lock: %w", err)
	}
	updateErr := update()
	closeErr := f.Close()
	removeErr := os.Remove(lockPath)
	return errors.Join(updateErr, closeErr, removeErr)
}

func validHealth(h HealthStatus) bool {
	return h == HealthUnknown || h == HealthOnline || h == HealthOffline || h == HealthDegraded
}

// CanonicalEndpoint checks that a candidate has an origin-only URL, no
// credentials, and TLS for all non-loopback destinations.
func CanonicalEndpoint(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u == nil || u.Opaque != "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawFragment != "" || (u.Path != "" && u.Path != "/") || u.RawPath != "" {
		return "", fmt.Errorf("%w: endpoint must be an origin URL without credentials, path or query", ErrInvalid)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "https" && scheme != "http" {
		return "", fmt.Errorf("%w: unsupported URL scheme", ErrInvalid)
	}
	host := strings.ToLower(u.Hostname())
	if host == "" || strings.Contains(host, "%") || !utf8.ValidString(host) {
		return "", fmt.Errorf("%w: invalid hostname", ErrInvalid)
	}
	ip := net.ParseIP(host)
	if ip == nil && !validDNSHost(host) {
		return "", fmt.Errorf("%w: invalid hostname", ErrInvalid)
	}
	if scheme == "http" && host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return "", fmt.Errorf("%w: remote HTTP requires TLS", ErrInvalid)
	}
	port := u.Port()
	if port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return "", fmt.Errorf("%w: invalid port", ErrInvalid)
		}
		port = strconv.Itoa(n)
	}
	if (scheme == "https" && port == "443") || (scheme == "http" && port == "80") {
		port = ""
	}
	if ip != nil {
		host = ip.String()
	}
	if port != "" {
		host = net.JoinHostPort(host, port)
	} else if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	return scheme + "://" + host, nil
}

func validDNSHost(host string) bool {
	if len(host) > 253 || !isASCII(host) {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, c := range label {
			if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-') {
				return false
			}
		}
	}
	return true
}

func isASCII(s string) bool {
	for _, c := range s { if c > 127 { return false } }
	return true
}

func validateHost(name, endpoint string) (string, string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 128 || !utf8.ValidString(name) || strings.ContainsAny(name, "\x00\r\n") {
		return "", "", fmt.Errorf("%w: invalid display name", ErrInvalid)
	}
	canonical, err := CanonicalEndpoint(endpoint)
	if err != nil { return "", "", err }
	sum := sha256.Sum256([]byte(canonical))
	return canonical, hex.EncodeToString(sum[:16]), nil
}

func cloneHost(h Host) Host {
	if h.LastCheckedAt != nil {
		t := *h.LastCheckedAt
		h.LastCheckedAt = &t
	}
	return h
}

func clone(in map[string]Host) map[string]Host {
	out := make(map[string]Host, len(in))
	for id, h := range in { out[id] = cloneHost(h) }
	return out
}

func sorted(in map[string]Host) []Host {
	out := make([]Host, 0, len(in))
	for _, h := range in { out = append(out, cloneHost(h)) }
	sort.Slice(out, func(i,j int) bool {
		if out[i].Name == out[j].Name { return out[i].ID < out[j].ID }
		return out[i].Name < out[j].Name
	})
	return out
}

func (s *Store) persist(next map[string]Host) error {
	doc := diskDocument{Version: catalogVersion, Hosts: sorted(next)}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil { return fmt.Errorf("catalog: serialize: %w", err) }
	if len(data) > maxCatalogBytes { return fmt.Errorf("catalog: size limit exceeded") }
	dir := filepath.Dir(s.path)
	f, err := os.CreateTemp(dir, ".hosts-*.tmp")
	if err != nil { return fmt.Errorf("catalog: create temporary file: %w", err) }
	tempPath := f.Name()
	defer os.Remove(tempPath)
	if err := f.Chmod(0600); err != nil { f.Close(); return fmt.Errorf("catalog: secure temporary file: %w", err) }
	if _, err := f.Write(data); err != nil { f.Close(); return fmt.Errorf("catalog: write: %w", err) }
	if err := f.Sync(); err != nil { f.Close(); return fmt.Errorf("catalog: sync: %w", err) }
	if err := f.Close(); err != nil { return fmt.Errorf("catalog: close: %w", err) }
	if err := os.Rename(tempPath, s.path); err != nil { return fmt.Errorf("catalog: atomic replace: %w", err) }
	return nil
}
