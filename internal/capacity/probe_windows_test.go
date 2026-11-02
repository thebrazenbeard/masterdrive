//go:build windows

package capacity

import (
	"os"
	"testing"
	"time"
)

func TestWindowsVolumeRootStrictlyFromGUID(t *testing.T) {
	const guid = "01234567-89ab-cdef-0123-456789abcdef"
	valid := `\\?\Volume{` + guid + `}\safe\directory`
	got, err := volumeRootFromFinalPath(valid)
	if err != nil || got != `\\?\Volume{`+guid+`}\` {
		t.Fatalf("valid pinned GUID path: %q err=%v", got, err)
	}
	for _, value := range []string{
		`\\?\UNC\server\share\folder`,
		`\\?\C:\folder`,
		`C:\folder`,
		`\\?\Volume{not-a-guid}\folder`,
		`\\?\Volume{` + guid + `}anything`,
		`\\?\Volume{` + guid,
	} {
		if _, err := volumeRootFromFinalPath(value); err == nil {
			t.Errorf("accepted untrusted/non-local volume path %q", value)
		}
	}
}

func TestWindowsProbeDoesNotFabricateValues(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	obs, err := ProbePinnedRoot("temporary", root, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if obs.Status == "measured" {
		if obs.AvailabilitySemantics != "caller_available" || obs.TotalReportedBytes == "" || obs.AvailableReportedBytes == "" {
			t.Fatalf("malformed Windows measurement: %+v", obs)
		}
	} else if obs.Status != "unavailable" || obs.TotalReportedBytes != "" || obs.AvailableReportedBytes != "" {
		t.Fatalf("Windows probe fabricated fallback: %+v", obs)
	}
}
