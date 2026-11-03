//go:build !windows

package catalog

func retryWindowsLockCreateDenied(string, error) bool { return false }
