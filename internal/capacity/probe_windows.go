//go:build windows

package capacity

import (
	"errors"
	"os"
	"regexp"
	"runtime"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const volumeNameGUID = 0x1 // GetFinalPathNameByHandleW VOLUME_NAME_GUID

var windowsVolumeGUID = regexp.MustCompile(`^[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}$`)
var getFinalPathNameByHandle = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetFinalPathNameByHandleW")

func nativeProbe(share string, root *os.Root, at time.Time) (Observation, error) {
	// GetDiskFreeSpaceExW is a path API: querying the configured export path
	// would follow a replaced junction/symlink. Resolve an immutable open root
	// handle to a volume GUID instead, then query that volume root.
	f, err := root.Open(".")
	if err != nil {
		return Observation{}, err
	}
	defer f.Close()
	buf := make([]uint16, 4096)
	n, _, callErr := getFinalPathNameByHandle.Call(f.Fd(), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), volumeNameGUID)
	runtime.KeepAlive(buf)
	if n == 0 || n >= uintptr(len(buf)) {
		_ = callErr // Never disclose the native error or absolute path.
		return Observation{}, errors.New("capacity: no stable local volume GUID")
	}
	volumeRoot, err := volumeRootFromFinalPath(windows.UTF16ToString(buf[:int(n)]))
	if err != nil {
		return Observation{}, err
	}
	p, err := windows.UTF16PtrFromString(volumeRoot)
	if err != nil {
		return Observation{}, err
	}
	var callerAvailable, callerTotal, volumeFree uint64
	if err := windows.GetDiskFreeSpaceEx(p, &callerAvailable, &callerTotal, &volumeFree); err != nil {
		return Observation{}, err
	}
	// Ignore volumeFree; only callerAvailable represents what this identity
	// could allocate. This is NOT MasterDrive's writable quota.
	return FromBlocks(share, callerTotal, callerAvailable, 1, "caller_available", at)
}

func volumeRootFromFinalPath(value string) (string, error) {
	// UNC/network shares have no volume GUID and deliberately return unknown.
	const prefix = `\\?\Volume{`
	if !strings.HasPrefix(value, prefix) {
		return "", errors.New("capacity: non-local or unrecognized volume")
	}
	rest := value[len(prefix):]
	end := strings.Index(rest, `}\`)
	if end == -1 || !windowsVolumeGUID.MatchString(rest[:end]) {
		return "", errors.New("capacity: invalid volume GUID")
	}
	return prefix + rest[:end] + `}\`, nil
}
