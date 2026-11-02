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
)

var (
	ErrNotFound = errors.New("catalog: host not found")
	ErrInvalid = errors.New("catalog: invalid host")
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

// Store protects in-process access and uses atomic file replacement for each
// committed edit. Separate processes must not concurrently write the same dir.
type Store struct {
	mu sync.RWMutex
	path string
	hosts map[string]Host
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
	hosts, err := readHosts(s.path, true)
	if err != nil {
		return nil, err
	}
	s.hosts = hosts
	return s, nil
}

// Reload atomically replaces the in-memory snapshot with a validated disk
// snapshot. It is intended for a long-lived read-only controller observing
// edits by a separate CLI process. A malformed, missing or oversized file
// leaves the last valid in-memory state untouched. The caller should surface
// that error rather than silently reporting stale contents as current.
//
// Reload does not make concurrent writers safe: exactly one process should
// own catalog mutations. Readers may safely Reload during atomic replacement.
func (s *Store) Reload() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	hosts, err := readHosts(s.path, false)
	if errors.Is(err, os.ErrNotExist) && len(s.hosts) == 0 {
		// A fresh catalog has no file until the first registration.
		return nil
	}
	if err != nil {
		return err
	}
	s.hosts = hosts
	return nil
}

func readHosts(path string, allowMissing bool) (map[string]Host, error) {
	hosts := make(map[string]Host)
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		if allowMissing {
			return hosts, nil
		}
		return nil, fmt.Errorf("catalog: missing state file: %w", err)
	}
	if err != nil {
		return nil, fmt.Errorf("catalog: open: %w", err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxCatalogBytes+1))
	if err != nil {
		return nil, fmt.Errorf("catalog: read: %w", err)
	}
	if len(data) > maxCatalogBytes {
		return nil, fmt.Errorf("catalog: file exceeds size limit")
	}
	var doc diskDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("catalog: invalid JSON: %w", err)
	}
	if doc.Version != catalogVersion {
		return nil, fmt.Errorf("catalog: unsupported version %d", doc.Version)
	}
	if len(doc.Hosts) > maxHosts {
		return nil, fmt.Errorf("catalog: too many hosts")
	}
	for _, h := range doc.Hosts {
		canonical, id, err := validateHost(h.Name, h.BaseURL)
		if err != nil || h.BaseURL != canonical || h.ID != id || !validHealth(h.Health) || h.AddedAt.IsZero() || (h.LastCheckedAt != nil && h.LastCheckedAt.IsZero()) {
			return nil, fmt.Errorf("catalog: invalid stored host %q", h.ID)
		}
		if _, exists := hosts[id]; exists {
			return nil, fmt.Errorf("catalog: duplicate stored host %q", h.ID)
		}
		hosts[id] = h
	}
	return hosts, nil
}

// Upsert registers or renames a host chosen explicitly by the caller. The ID
// is stable for the same canonical endpoint; no network operation is performed.
func (s *Store) Upsert(name, baseURL string) (Host, error) {
	canonical, id, err := validateHost(name, baseURL)
	if err != nil {
		return Host{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	copy := clone(s.hosts)
	h, exists := copy[id]
	if exists {
		h.Name = strings.TrimSpace(name)
	} else {
		if len(copy) >= maxHosts {
			return Host{}, fmt.Errorf("catalog: host limit reached")
		}
		h = Host{ID: id, Name: strings.TrimSpace(name), BaseURL: canonical, AddedAt: time.Now().UTC(), Health: HealthUnknown}
	}
	copy[id] = h
	if err := s.persist(copy); err != nil {
		return Host{}, err
	}
	s.hosts = copy
	return h, nil
}

// Remove forgets a host from this catalog. It does NOT revoke any remote
// credential or delete any remote data; those effects need separate authority.
func (s *Store) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.hosts[id]; !ok {
		return ErrNotFound
	}
	copy := clone(s.hosts)
	delete(copy, id)
	if err := s.persist(copy); err != nil {
		return err
	}
	s.hosts = copy
	return nil
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
	s.mu.Lock()
	defer s.mu.Unlock()
	h, ok := s.hosts[id]
	if !ok {
		return ErrNotFound
	}
	copy := clone(s.hosts)
	t := checkedAt.UTC()
	h.Health = status
	h.LastCheckedAt = &t
	copy[id] = h
	if err := s.persist(copy); err != nil {
		return err
	}
	s.hosts = copy
	return nil
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
