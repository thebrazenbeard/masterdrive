//go:build !linux && !windows

package capacity

import (
	"os"
	"time"
)

func nativeProbe(_ string, _ *os.Root, _ time.Time) (Observation, error) {
	return Observation{}, errUnsupportedNativeProbe
}
