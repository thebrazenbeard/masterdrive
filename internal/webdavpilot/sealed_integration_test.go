package webdavpilot_test

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/thebrazenbeard/masterdrive/internal/versioncontract"
	"github.com/thebrazenbeard/masterdrive/internal/webdavpilot"
)

// This is deliberately a test-only integration seam. It accepts a single
// explicitly authorized in-memory fixture; no controller, listener, mount,
// filesystem source, or production identity binding is involved.
const (
	seamHost  = "fedcba9876543210fedcba9876543210"
	seamShare = "fixture"
	seamFile  = "sample.txt"
	seamRoot  = "/dav/" + seamHost + "/" + seamShare
)

var seamModifiedAt = time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)

type sealedFixtureBackend struct {
	sealed        *versioncontract.Sealed
	revoked       atomic.Bool
	opens         atomic.Int32
	childrenCount int
}

func (b *sealedFixtureBackend) check(ctx context.Context, k webdavpilot.Key) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if b.revoked.Load() {
		return webdavpilot.ErrUnauthorized
	}
	if k.HostID != seamHost || k.ShareID != seamShare {
		return webdavpilot.ErrNotFound
	}
	return nil
}

func (b *sealedFixtureBackend) fileMeta(name string) webdavpilot.Metadata {
	return webdavpilot.Metadata{
		Name: name, Size: b.sealed.Size(), StrongETag: b.sealed.ETag(), ModifiedAt: seamModifiedAt,
	}
}

func (b *sealedFixtureBackend) Stat(ctx context.Context, k webdavpilot.Key) (webdavpilot.Metadata, error) {
	if err := b.check(ctx, k); err != nil {
		return webdavpilot.Metadata{}, err
	}
	switch k.Relative {
	case "":
		return webdavpilot.Metadata{Name: seamShare, Directory: true, ModifiedAt: seamModifiedAt}, nil
	case seamFile:
		return b.fileMeta(seamFile), nil
	default:
		return webdavpilot.Metadata{}, webdavpilot.ErrNotFound
	}
}

func (b *sealedFixtureBackend) Children(ctx context.Context, k webdavpilot.Key, limit int) ([]webdavpilot.Metadata, bool, error) {
	if err := b.check(ctx, k); err != nil {
		return nil, false, err
	}
	if k.Relative != "" {
		return nil, false, webdavpilot.ErrNotFound
	}
	if limit < 1 || limit > 200 {
		return nil, false, errors.New("fixture children budget invalid")
	}
	count := b.childrenCount
	if count > limit {
		count = limit
	}
	children := make([]webdavpilot.Metadata, 0, count)
	for i := 0; i < count; i++ {
		name := seamFile
		if i != 0 {
			name = fmt.Sprintf("extra%03d.txt", i)
		}
		children = append(children, b.fileMeta(name))
	}
	return children, b.childrenCount <= limit, nil
}

func (b *sealedFixtureBackend) OpenSnapshot(ctx context.Context, k webdavpilot.Key, expected string) (webdavpilot.ReadSeekCloser, error) {
	if err := b.check(ctx, k); err != nil {
		return nil, err
	}
	if k.Relative != seamFile {
		return nil, webdavpilot.ErrNotFound
	}
	b.opens.Add(1)
	reader, err := b.sealed.Open(expected)
	if errors.Is(err, versioncontract.ErrStale) {
		return nil, webdavpilot.ErrStale
	}
	return reader, err
}

func seamRequest(t *testing.T, h http.Handler, method, relative string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, "http://127.0.0.1:8777"+seamRoot+relative, nil)
	for name, value := range headers {
		r.Header.Set(name, value)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func seamSetup(t *testing.T, data []byte) (*webdavpilot.Handler, *sealedFixtureBackend, *atomic.Bool) {
	t.Helper()
	sealed, err := versioncontract.Capture(context.Background(), bytes.NewReader(data), versioncontract.MaxCaptureBytes)
	if err != nil {
		t.Fatal(err)
	}
	b := &sealedFixtureBackend{sealed: sealed, childrenCount: 1}
	allowed := &atomic.Bool{}
	allowed.Store(true)
	h, err := webdavpilot.New(b, func(_ *http.Request, k webdavpilot.Key) bool {
		return allowed.Load() && k.HostID == seamHost && k.ShareID == seamShare
	})
	if err != nil {
		t.Fatal(err)
	}
	return h, b, allowed
}

func TestSealedBytesRemainIdenticalAcrossIndependentWebDAVRanges(t *testing.T) {
	source := []byte("The snapshot fixture is immutable after capture.")
	expected := string(bytes.Clone(source))
	h, backend, _ := seamSetup(t, source)
	source[0] = 'X' // A real filesystem capture would still need native snapshot coherence.
	etag := backend.sealed.ETag()
	if !strings.HasPrefix(etag, `"m0-sha256-`) {
		t.Fatalf("not a strong content hash: %q", etag)
	}

	get := seamRequest(t, h, http.MethodGet, "/"+seamFile, nil)
	if get.Code != http.StatusOK || get.Body.String() != expected || get.Header().Get("ETag") != etag {
		t.Fatalf("full GET status=%d body=%q etag=%q", get.Code, get.Body.String(), get.Header().Get("ETag"))
	}
	head := seamRequest(t, h, http.MethodHead, "/"+seamFile, nil)
	if head.Code != http.StatusOK || head.Body.Len() != 0 || head.Header().Get("Content-Length") != fmt.Sprint(len(expected)) || head.Header().Get("ETag") != etag {
		t.Fatalf("HEAD status=%d bytes=%d headers=%v", head.Code, head.Body.Len(), head.Header())
	}

	// Each request independently obtains a new seekable reader, not a shared
	// mutable cursor. Together the ranges must reconstruct the exact GET bytes.
	mid := len(expected) / 2
	var joined strings.Builder
	for _, bounds := range [][2]int{{0, mid - 1}, {mid, len(expected) - 1}} {
		start, end := bounds[0], bounds[1]
		r := seamRequest(t, h, http.MethodGet, "/"+seamFile, map[string]string{
			"Range": fmt.Sprintf("bytes=%d-%d", start, end), "If-Range": etag,
		})
		if r.Code != http.StatusPartialContent || r.Header().Get("ETag") != etag ||
			r.Header().Get("Content-Range") != fmt.Sprintf("bytes %d-%d/%d", start, end, len(expected)) ||
			r.Body.String() != expected[start:end+1] {
			t.Fatalf("range %d-%d status=%d body=%q headers=%v", start, end, r.Code, r.Body.String(), r.Header())
		}
		joined.WriteString(r.Body.String())
	}
	if joined.String() != expected || backend.opens.Load() != 4 { // GET + HEAD + two ranges
		t.Fatalf("range reconstruction=%q opens=%d", joined.String(), backend.opens.Load())
	}

	stale := seamRequest(t, h, http.MethodGet, "/"+seamFile, map[string]string{
		"Range": "bytes=0-2", "If-Range": `"other-version"`,
	})
	if stale.Code != http.StatusOK || stale.Body.String() != expected || stale.Header().Get("ETag") != etag {
		t.Fatalf("stale If-Range must serve full current representation: status=%d body=%q", stale.Code, stale.Body.String())
	}
	stale = seamRequest(t, h, http.MethodGet, "/"+seamFile, map[string]string{"If-Match": `"other-version"`})
	if stale.Code != http.StatusPreconditionFailed || stale.Body.String() == expected {
		t.Fatalf("stale If-Match status=%d body=%q", stale.Code, stale.Body.String())
	}
}

func TestSealedVersionVisibleInBoundedPropfindAndRevocation(t *testing.T) {
	h, backend, allowed := seamSetup(t, []byte("authorized fixture"))
	for _, depth := range []string{"0", "1"} {
		w := seamRequest(t, h, "PROPFIND", "/", map[string]string{"Depth": depth})
		if w.Code != 207 {
			t.Fatalf("PROPFIND depth=%s status=%d: %s", depth, w.Code, w.Body.String())
		}
		var result struct {
			XMLName   xml.Name
			Responses []struct {
				Href     string `xml:"href"`
				PropStat struct {
					Prop struct {
						ETag string `xml:"getetag"`
					} `xml:"prop"`
				} `xml:"propstat"`
			} `xml:"response"`
		}
		if err := xml.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		wantResponses := 1
		if depth == "1" {
			wantResponses = 2
		}
		if result.XMLName.Space != "DAV:" || result.XMLName.Local != "multistatus" || len(result.Responses) != wantResponses {
			t.Fatalf("invalid multistatus depth=%s: %+v", depth, result)
		}
		if depth == "1" && (result.Responses[1].Href != seamRoot+"/"+seamFile || result.Responses[1].PropStat.Prop.ETag != backend.sealed.ETag()) {
			t.Fatalf("wrong versioned child metadata: %+v", result.Responses[1])
		}
	}

	backend.childrenCount = 201
	tooMany := seamRequest(t, h, "PROPFIND", "/", map[string]string{"Depth": "1"})
	if tooMany.Code != http.StatusServiceUnavailable || strings.Contains(tooMany.Body.String(), "multistatus") {
		t.Fatalf("unbounded children accepted status=%d body=%q", tooMany.Code, tooMany.Body.String())
	}
	backend.childrenCount = 1
	allowed.Store(false)
	for _, method := range []string{http.MethodGet, http.MethodHead, "PROPFIND"} {
		w := seamRequest(t, h, method, "/"+seamFile, map[string]string{"Depth": "0"})
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("method %s bypassed revoked authorization: %d", method, w.Code)
		}
	}
	allowed.Store(true)
	backend.revoked.Store(true)
	w := seamRequest(t, h, http.MethodGet, "/"+seamFile, nil)
	if w.Code != http.StatusForbidden || backend.opens.Load() != 0 {
		t.Fatalf("backend revocation failed: status=%d opens=%d", w.Code, backend.opens.Load())
	}
	for _, method := range []string{http.MethodPut, http.MethodDelete, "PROPPATCH", "MKCOL"} {
		w = seamRequest(t, h, method, "/"+seamFile, nil)
		if w.Code != http.StatusMethodNotAllowed {
			t.Fatalf("write method %s accepted: %d", method, w.Code)
		}
	}
}

func TestSeamCaptureRejectsOversizeWithoutPublishingBytes(t *testing.T) {
	data := bytes.Repeat([]byte{'x'}, int(versioncontract.MaxCaptureBytes)+1)
	sealed, err := versioncontract.Capture(context.Background(), bytes.NewReader(data), versioncontract.MaxCaptureBytes)
	if !errors.Is(err, versioncontract.ErrTooLarge) || sealed != nil {
		t.Fatalf("oversize source published: sealed=%v err=%v", sealed, err)
	}
	// A successful fixture capture permits independent readers but never writes.
	sealed, err = versioncontract.Capture(context.Background(), bytes.NewReader(data[:4]), 4)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := sealed.Open(sealed.ETag())
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	got, err := io.ReadAll(reader)
	if err != nil || string(got) != "xxxx" {
		t.Fatalf("sealed reader=%q err=%v", got, err)
	}
}
