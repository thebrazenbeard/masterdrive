//go:build linux

package capacity

import (
	"errors"
	"os"
	"syscall"
	"time"
)

func nativeProbe(share string, root *os.Root, at time.Time) (Observation, error) {
	// This file is opened relative to the pinned os.Root, not by resolving the
	// export's original path a second time. A renamed ancestor cannot redirect
	// the measurement to a different volume.
	f, err := root.Open(".")
	if err != nil {
		return Observation{}, err
	}
	defer f.Close()
	var fs syscall.Statfs_t
	if err := syscall.Fstatfs(int(f.Fd()), &fs); err != nil {
		return Observation{}, err
	}
	return linuxObservation(share, fs, at)
}

func linuxObservation(share string, fs syscall.Statfs_t, at time.Time) (Observation, error) {
	// f_frsize is the fundamental fragment size for f_blocks and f_bavail.
	// f_bsize is a preferred I/O size and may be different. Do not substitute.
	if fs.Frsize <= 0 {
		return Observation{}, errors.New("capacity: missing fragment size")
	}
	return FromBlocks(share, fs.Blocks, fs.Bavail, uint64(fs.Frsize), "unprivileged", at)
}
