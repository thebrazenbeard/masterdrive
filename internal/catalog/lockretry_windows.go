//go:build windows

package catalog

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

// retryWindowsLockCreateDenied is deliberately narrower than os.ErrPermission:
// Windows CREATE_NEW can transiently return ERROR_ACCESS_DENIED amid concurrent
// lock-file creation/deletion. The exact kernel cause is not established.
// Only retry if the lock path is
// missing or is a regular file in an existing directory. The caller has a
// separate short deadline and returns the original access-denied error if it
// persists; exclusive creation remains the only successful lock acquisition.
func retryWindowsLockCreateDenied(lockPath string, err error) bool {
	var errno syscall.Errno
	if !errors.As(err, &errno) || errno != syscall.ERROR_ACCESS_DENIED {
		return false
	}
	parent, err := os.Stat(filepath.Dir(lockPath))
	if err != nil || !parent.IsDir() {
		return false
	}
	entry, err := os.Lstat(lockPath)
	if errors.Is(err, os.ErrNotExist) {
		return true
	}
	return err == nil && entry.Mode().IsRegular()
}
