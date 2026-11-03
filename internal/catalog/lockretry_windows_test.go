//go:build windows

package catalog

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestWindowsLockCreateDeniedClassification(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "hosts.json.lock")
	denied := &os.PathError{Op: "open", Path: lockPath, Err: syscall.ERROR_ACCESS_DENIED}
	if !retryWindowsLockCreateDenied(lockPath, denied) {
		t.Fatal("temporary access-denied with absent lock was not eligible for bounded retry")
	}
	if retryWindowsLockCreateDenied(lockPath, os.ErrPermission) {
		t.Fatal("generic permission denial must not be mistaken for Windows deletion contention")
	}
	if retryWindowsLockCreateDenied(lockPath, &os.PathError{Op: "open", Path: lockPath, Err: syscall.Errno(32) /* ERROR_SHARING_VIOLATION */}) {
		t.Fatal("other Windows errors must not be classified as delete contention")
	}
	if err := os.Mkdir(lockPath, 0700); err != nil {
		t.Fatal(err)
	}
	if retryWindowsLockCreateDenied(lockPath, denied) {
		t.Fatal("directory at lock path is not deletion contention")
	}
	if err := os.Remove(lockPath); err != nil {
		t.Fatal(err)
	}
	if retryWindowsLockCreateDenied(filepath.Join(dir, "nonexistent", "hosts.json.lock"), denied) {
		t.Fatal("nonexistent parent is not deletion contention")
	}
	if !errors.Is(denied, os.ErrPermission) {
		t.Fatal("sanity: wrapped Windows access denial must remain a permission error")
	}
}

// A directory placed at the lock pathname is a genuine Windows namespace
// denial, not evidence of a transient concurrent delete or an ACL denial.
func TestWindowsWriteLockDirectoryDenialFailsClosed(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "hosts.json.lock")
	if err := os.Mkdir(lockPath, 0700); err != nil {
		t.Fatal(err)
	}
	called := false
	start := time.Now()
	err := withWriteLock(lockPath, func() error { called = true; return nil })
	if !errors.Is(err, syscall.EISDIR) || errors.Is(err, ErrBusy) {
		t.Fatalf("directory lock must preserve directory error, got %v", err)
	}
	if called {
		t.Fatal("update executed without acquiring exclusive lock")
	}
	if time.Since(start) > time.Second {
		t.Fatal("non-retriable lock namespace denial was delayed")
	}
	if info, statErr := os.Stat(lockPath); statErr != nil || !info.IsDir() {
		t.Fatalf("denied lock path was modified: %v", statErr)
	}
}
