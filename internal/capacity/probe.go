package capacity

import (
	"errors"
	"os"
	"time"
)

// ProbePinnedRoot reads the backing filesystem of an already-authorized,
// already-open export root. It performs no directory traversal, is not an
// HTTP endpoint, and never grants allocation or write permissions. The caller
// must retain ownership of root for the entire call and serialize Close.
//
// Unknown probes disclose neither the host path nor the raw OS error.
func ProbePinnedRoot(share string, root *os.Root, at time.Time) (Observation, error) {
	if share == "" || root == nil || at.IsZero() {
		return Observation{}, ErrInvalidMeasurement
	}
	obs, err := nativeProbe(share, root, at)
	if err == nil {
		return obs, nil
	}
	if errors.Is(err, errUnsupportedNativeProbe) {
		return Unknown(share, "unsupported", "platform_not_supported")
	}
	return Unknown(share, "unavailable", "probe_failed")
}

var errUnsupportedNativeProbe = errors.New("capacity: native probe unsupported")
