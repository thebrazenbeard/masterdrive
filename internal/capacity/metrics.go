// Package capacity contains conservative, platform-neutral capacity response
// construction. It does not inspect any disk, enumerate shares, or grant a
// write entitlement. Native platform probes must be separately qualified.
package capacity

import (
	"errors"
	"math/bits"
	"strconv"
	"time"
)

var ErrInvalidMeasurement = errors.New("capacity: invalid filesystem observation")

// Observation represents an underlying filesystem reading, never a
// MasterDrive share quota or an additive pool allocation.
type Observation struct {
	Share                 string `json:"share"`
	Status                string `json:"status"`
	Scope                 string `json:"scope"`
	TotalReportedBytes    string `json:"total_reported_bytes,omitempty"`
	AvailableReportedBytes string `json:"available_reported_bytes,omitempty"`
	AvailabilitySemantics string `json:"availability_semantics,omitempty"`
	MeasuredAt            *time.Time `json:"measured_at,omitempty"`
	ReadOnlyExport        bool `json:"read_only_export"`
	Reason                string `json:"reason,omitempty"`
}

// Unknown builds a fail-closed response without fabricating zero capacity.
func Unknown(share, status, reason string) (Observation, error) {
	if share == "" || (status != "unavailable" && status != "unsupported") ||
		(reason != "probe_failed" && reason != "platform_not_supported") {
		return Observation{}, ErrInvalidMeasurement
	}
	return Observation{
		Share: share, Status: status, Scope: "backing_filesystem",
		ReadOnlyExport: true, Reason: reason,
	}, nil
}

// FromBlocks converts native fragment counts, rejecting overflow and
// impossible values. The caller owns both the share authorization and probe.
// Semantics must be "unprivileged" (Linux) or "caller_available" (Windows).
func FromBlocks(share string, totalBlocks, availableBlocks, bytesPerBlock uint64, semantics string, at time.Time) (Observation, error) {
	if share == "" || totalBlocks == 0 || bytesPerBlock == 0 || availableBlocks > totalBlocks || at.IsZero() ||
		(semantics != "unprivileged" && semantics != "caller_available") {
		return Observation{}, ErrInvalidMeasurement
	}
	th, total := bits.Mul64(totalBlocks, bytesPerBlock)
	ah, available := bits.Mul64(availableBlocks, bytesPerBlock)
	if th != 0 || ah != 0 {
		return Observation{}, ErrInvalidMeasurement
	}
	stamp := at.UTC()
	return Observation{
		Share: share, Status: "measured", Scope: "backing_filesystem",
		TotalReportedBytes: strconv.FormatUint(total, 10),
		AvailableReportedBytes: strconv.FormatUint(available, 10),
		AvailabilitySemantics: semantics,
		MeasuredAt: &stamp, ReadOnlyExport: true,
	}, nil
}
