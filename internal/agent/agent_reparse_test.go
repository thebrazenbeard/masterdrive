package agent

import (
	"errors"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// windowsTestJunction creates a genuine NTFS directory junction using the
// Windows PowerShell 5.1 filesystem provider. Both ends are always paths made
// by the same t.TempDir test. Environment variables carry the paths so test
// directory names never become executable PowerShell source.
//
// The explicit non-recursive cleanup runs before t.TempDir's cleanup. If a
// junction could not be removed, fail the test rather than deliberately walk
// through it with a recursive shell deletion command.
func windowsTestJunction(t *testing.T, junction, target string) {
	t.Helper()
	if runtime.GOOS != "windows" {
		t.Fatal("junction helper must only run on Windows")
	}
	if !filepath.IsAbs(junction) || !filepath.IsAbs(target) {
		t.Fatal("junction fixture paths must be absolute")
	}
	t.Cleanup(func() {
		if err := os.Remove(junction); err != nil && !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("remove disposable junction without recursion: %v", err)
		}
	})
	const create = `$ErrorActionPreference = 'Stop'; New-Item -ItemType Junction -Path ([System.Management.Automation.WildcardPattern]::Escape($env:MASTERDRIVE_TEST_JUNCTION)) -Target $env:MASTERDRIVE_TEST_TARGET | Out-Null`
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", create)
	cmd.Env = append(os.Environ(), "MASTERDRIVE_TEST_JUNCTION="+junction, "MASTERDRIVE_TEST_TARGET="+target)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Windows junction fixture creation failed (native junction qualification unavailable): %v: %s", err, strings.TrimSpace(string(output)))
	}
	if _, err := os.Lstat(junction); err != nil {
		t.Fatalf("Windows junction fixture is missing: %v", err)
	}
}

// This is a boundary test, not a unit test of path-string normalization. An
// authenticated client must not read a sibling directory after an untrusted
// filesystem actor puts an NTFS junction inside an explicitly exported root.
func TestWindowsJunctionOutsideExportIsRejected(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("native NTFS junction test requires Windows")
	}
	parent := t.TempDir()
	export := filepath.Join(parent, "export")
	outside := filepath.Join(parent, "outside")
	for _, dir := range []string{export, outside} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	const sentinel = "outside-junction-secret-do-not-disclose"
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte(sentinel), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(export, "approved"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(export, "approved", "ok.txt"), []byte("inside"), 0o600); err != nil {
		t.Fatal(err)
	}
	windowsTestJunction(t, filepath.Join(export, "bridge"), outside)
	h := testHandler(t, []Export{{ID: "docs", Name: "Approved docs", Path: export}})
	for _, endpoint := range []string{
		"/v1/list?share=docs&path=bridge",
		"/v1/download?share=docs&path=bridge%2Fsecret.txt",
	} {
		w := request(h, http.MethodGet, endpoint, testToken)
		if w.Code >= 200 && w.Code < 300 {
			t.Errorf("escaped through Windows junction at %q: HTTP %d", endpoint, w.Code)
		}
		if strings.Contains(w.Body.String(), sentinel) {
			t.Errorf("junction escaped share and disclosed sentinel at %q", endpoint)
		}
	}
	allowed := request(h, http.MethodGet, "/v1/download?share=docs&path=approved%2Fok.txt", testToken)
	if allowed.Code != http.StatusOK || allowed.Body.String() != "inside" {
		t.Fatalf("legitimate nested export was rejected: HTTP %d, body %q", allowed.Code, allowed.Body.String())
	}
}

// Registration does not make a child path immutable. os.Root must reject an
// escape junction inserted after the agent already opened its export root.
func TestWindowsJunctionAddedAfterRegistrationIsRejected(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("native NTFS junction test requires Windows")
	}
	parent := t.TempDir()
	export := filepath.Join(parent, "export")
	outside := filepath.Join(parent, "outside")
	for _, dir := range []string{export, outside} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	const sentinel = "post-registration-junction-secret"
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte(sentinel), 0o600); err != nil {
		t.Fatal(err)
	}
	h := testHandler(t, []Export{{ID: "docs", Name: "Approved docs", Path: export}})
	windowsTestJunction(t, filepath.Join(export, "new-bridge"), outside)
	for _, endpoint := range []string{
		"/v1/download?share=docs&path=new-bridge%2Fsecret.txt",
		"/v1/list?share=docs&path=new-bridge",
	} {
		w := request(h, http.MethodGet, endpoint, testToken)
		if w.Code >= 200 && w.Code < 300 {
			t.Errorf("post-registration junction escaped export: %q -> HTTP %d", endpoint, w.Code)
		}
		if strings.Contains(w.Body.String(), sentinel) {
			t.Errorf("post-registration junction exposed secret at %q", endpoint)
		}
	}
}

// filepath.Localize and os.Root must not turn DOS device names into special
// file/device access. This tests the actual HTTP entry point as well.
func TestWindowsReservedDevicePathsFailClosed(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("reserved-device path rules are Windows-specific")
	}
	h := testHandler(t, []Export{{ID: "docs", Name: "Approved docs", Path: t.TempDir()}})
	for _, name := range []string{"NUL", "CON", "PRN", "AUX", "COM1", "LPT1"} {
		t.Run(name, func(t *testing.T) {
			if localized, err := localPath(name); err == nil {
				t.Errorf("accepted reserved Windows device name %q as %q", name, localized)
			}
			target := "/v1/download?share=docs&path=" + url.QueryEscape(name)
			w := request(h, http.MethodGet, target, testToken)
			if w.Code >= 200 && w.Code < 300 {
				t.Errorf("reserved Windows device name %q returned HTTP %d", name, w.Code)
			}
		})
	}
}
