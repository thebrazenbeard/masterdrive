//go:build linux

package capacity

import (
	"math"
	"os"
	"syscall"
	"testing"
	"time"
)

func TestLinuxObservationUsesFragmentSizeAndUnprivilegedBlocks(t *testing.T) {
	at := time.Unix(500, 0)
	fs := syscall.Statfs_t{Blocks: 100, Bavail: 30, Bsize: 65536, Frsize: 4096}
	obs, err := linuxObservation("export", fs, at)
	if err != nil {
		t.Fatal(err)
	}
	if obs.Status != "measured" || obs.TotalReportedBytes != "409600" ||
		obs.AvailableReportedBytes != "122880" || obs.AvailabilitySemantics != "unprivileged" ||
		!obs.ReadOnlyExport || obs.Scope != "backing_filesystem" {
		t.Fatalf("wrong capacity semantics: %+v", obs)
	}
}

func TestLinuxObservationRejectsAmbiguityAndOverflow(t *testing.T) {
	for _, fs := range []syscall.Statfs_t{
		{Blocks: 100, Bavail: 20, Frsize: 0},
		{Blocks: 100, Bavail: 20, Frsize: -1},
		{Blocks: 100, Bavail: 101, Frsize: 512},
		{Blocks: math.MaxUint64, Bavail: 5, Frsize: 4096},
		{Blocks: 0, Bavail: 0, Frsize: 4096},
	} {
		if _, err := linuxObservation("export", fs, time.Now()); err == nil {
			t.Fatalf("accepted invalid statfs: %+v", fs)
		}
	}
}

func TestLinuxProbePinnedTemporaryDirectory(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	obs, err := ProbePinnedRoot("temporary", root, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if obs.Status != "measured" || obs.AvailabilitySemantics != "unprivileged" {
		t.Fatalf("local filesystem not measured: %+v", obs)
	}
}
