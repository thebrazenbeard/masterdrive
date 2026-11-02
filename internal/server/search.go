package server

import (
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	pathpkg "path"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/thebrazenbeard/masterdrive/internal/catalog"
)

const (
	maxSearchTargets = 8
	maxSearchConcurrency = 3
	maxSearchMatches = 50
	maxSearchOffset = 10000
	maxSearchWindow = 256
	searchTimeout = 20 * time.Second
)

// Federated search is an explicit, bounded fanout over selected share roots.
// It never discovers devices, crawls descendants, indexes disks, or writes.
type searchTarget struct { hostID, shareID string }

type searchEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Type string `json:"type"`
	Size int64 `json:"size"`
	ModifiedAt time.Time `json:"modified_at"`
}

type searchSource struct {
	HostID string `json:"host_id"`
	ShareID string `json:"share_id"`
	Entries []searchEntry `json:"entries"`
	Scanned int `json:"scanned"`
	NextOffset *int `json:"next_offset"`
	Truncated bool `json:"truncated"`
	Error string `json:"error,omitempty"`
}

func serveFederatedSearch(w http.ResponseWriter, r *http.Request, store *catalog.Store, tokens map[string]string, client *http.Client) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) < 2 || len(q) > 128 || !utf8.ValidString(q) || strings.ContainsAny(q, "\x00\r\n") {
		fail(w, http.StatusBadRequest, "invalid_search", "query must contain 2-128 UTF-8 bytes")
		return
	}
	path := r.URL.Query().Get("path")
	if path != "" && (!fs.ValidPath(path) || strings.ContainsAny(path, "\\\x00")) {
		fail(w, http.StatusBadRequest, "invalid_path", "search folder must be a portable relative path")
		return
	}
	limit, offset, ok := parseSearchWindow(r)
	if !ok {
		fail(w, http.StatusBadRequest, "invalid_window", "search limit or offset is outside the bounded range")
		return
	}
	rawTargets := r.URL.Query()["target"]
	if len(rawTargets) == 0 || len(rawTargets) > maxSearchTargets || (offset != 0 && len(rawTargets) != 1) {
		fail(w, http.StatusBadRequest, "invalid_targets", "choose 1-8 explicit shares; offset paging requires one target")
		return
	}
	targets := make([]searchTarget, 0, len(rawTargets))
	seen := make(map[string]bool, len(rawTargets))
	for _, raw := range rawTargets {
		hostID, shareID, hasShare := strings.Cut(raw, ":")
		if !hasShare || !validSearchID(hostID, 32, 32, true) || !validSearchID(shareID, 1, 64, false) || seen[raw] {
			fail(w, http.StatusBadRequest, "invalid_targets", "each unique target must be host-id:share-id")
			return
		}
		seen[raw] = true
		targets = append(targets, searchTarget{hostID, shareID})
	}

	ctx, cancel := context.WithTimeout(r.Context(), searchTimeout)
	defer cancel()
	results := make([]searchSource, len(targets))
	semaphore := make(chan struct{}, maxSearchConcurrency)
	var wg sync.WaitGroup
	for i, target := range targets {
		results[i] = searchSource{HostID: target.hostID, ShareID: target.shareID, Entries: []searchEntry{}}
		wg.Add(1)
		go func(i int, target searchTarget) {
			defer wg.Done()
			select {
			case semaphore <- struct{}{}:
				defer func(){ <-semaphore }()
			case <-ctx.Done():
				results[i].Error = "timeout"
				return
			}
			searchOneSource(ctx, &results[i], target, q, path, limit, offset, store, tokens, client)
		}(i, target)
	}
	wg.Wait()
	complete := true
	for _, source := range results {
		if source.Error != "" || source.NextOffset != nil || source.Truncated { complete = false }
	}
	respond(w, http.StatusOK, struct {
		Query string `json:"query"`
		Path string `json:"path"`
		Scope string `json:"scope"`
		Complete bool `json:"complete"`
		Sources []searchSource `json:"sources"`
	}{Query:q, Path:path, Scope:"immediate_children", Complete:complete, Sources:results})
}

func searchOneSource(ctx context.Context, result *searchSource, target searchTarget, q, path string, limit, offset int, store *catalog.Store, tokens map[string]string, client *http.Client) {
	host, found := store.Get(target.hostID)
	if !found { result.Error = "unknown_host"; return }
	secret := tokens[host.ID]
	if secret == "" { result.Error = "peer_token_missing"; return }
	endpoint, err := url.Parse(host.BaseURL)
	if err != nil || endpoint.User != nil { result.Error = "bad_peer"; return }
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/v1/search"
	params := url.Values{"share": {target.shareID}, "path": {path}, "q": {q}, "limit": {strconv.Itoa(limit)}, "offset": {strconv.Itoa(offset)}}
	endpoint.RawQuery = params.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil { result.Error = "bad_peer"; return }
	request.Header.Set("Authorization", "Bearer "+secret)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Accept-Encoding", "identity")
	response, err := client.Do(request)
	if err != nil { result.Error = "peer_unavailable"; return }
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK { result.Error = "peer_error"; return }
	if encoding := response.Header.Get("Content-Encoding"); encoding != "" && !strings.EqualFold(encoding, "identity") {
		result.Error = "unexpected_encoding"; return
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxPeerJSON+1))
	if err != nil || len(data) > maxPeerJSON { result.Error = "invalid_peer_response"; return }
	var payload struct {
		Share string `json:"share"`
		Path string `json:"path"`
		Query string `json:"query"`
		Scope string `json:"scope"`
		Entries []searchEntry `json:"entries"`
		Scanned int `json:"scanned"`
		NextOffset *int `json:"next_offset"`
		Truncated bool `json:"truncated"`
	}
	if json.Unmarshal(data, &payload) != nil || payload.Share != target.shareID || payload.Path != path ||
		payload.Query != q || payload.Scope != "immediate_children" || len(payload.Entries) > limit ||
		payload.Scanned < 0 || payload.Scanned > maxSearchWindow ||
		len(payload.Entries) > payload.Scanned ||
		(payload.NextOffset != nil && (payload.Scanned == 0 || *payload.NextOffset != offset+payload.Scanned || *payload.NextOffset > maxSearchOffset)) {
		result.Error = "invalid_peer_response"; return
	}
	for _, entry := range payload.Entries {
		if entry.Name == "" || entry.Name == "." || entry.Name == ".." || len(entry.Name) > 1024 ||
			strings.ContainsAny(entry.Name, "/\\\x00") || len(entry.Path) > 2048 ||
			!fs.ValidPath(entry.Path) || strings.ContainsAny(entry.Path, "\\\x00") ||
			entry.Path != pathpkg.Join(path, entry.Name) ||
			entry.Type != "file" && entry.Type != "directory" && entry.Type != "symlink" && entry.Type != "other" {
			result.Error = "invalid_peer_response"; return
		}
	}
	result.Entries = payload.Entries
	result.Scanned = payload.Scanned
	result.NextOffset = payload.NextOffset
	result.Truncated = payload.Truncated
}

func parseSearchWindow(r *http.Request) (limit, offset int, ok bool) {
	limit, offset = 25, 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		var err error
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > maxSearchMatches { return 0, 0, false }
	}
	if raw := r.URL.Query().Get("offset"); raw != "" {
		var err error
		offset, err = strconv.Atoi(raw)
		if err != nil || offset < 0 || offset > maxSearchOffset { return 0, 0, false }
	}
	return limit, offset, true
}

func validSearchID(id string, minLen, maxLen int, hexOnly bool) bool {
	if len(id) < minLen || len(id) > maxLen { return false }
	for _, c := range id {
		if hexOnly {
			if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') { return false }
		} else if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}
