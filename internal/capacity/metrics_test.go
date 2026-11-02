package capacity

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestFromBlocksUsesExactDecimalStrings(t *testing.T) {
	at := time.Date(2026, 11, 2, 1, 0, 0, 0, time.FixedZone("UTC+1", 3600))
	got, err := FromBlocks("docs", 9007199254740992, 9007199254740991, 1, "unprivileged", at)
	if err != nil { t.Fatal(err) }
	if got.TotalReportedBytes != "9007199254740992" || got.AvailableReportedBytes != "9007199254740991" {
		t.Fatalf("lost large-integer fidelity: %+v", got)
	}
	if got.MeasuredAt == nil || got.MeasuredAt.Location() != time.UTC || got.Status != "measured" || !got.ReadOnlyExport {
		t.Fatalf("wrong observation provenance: %+v", got)
	}
	data, err := json.Marshal(got)
	if err != nil { t.Fatal(err) }
	if !strings.Contains(string(data), `"total_reported_bytes":"9007199254740992"`) {
		t.Fatalf("JSON may not use lossy numbers: %s", data)
	}
}

func TestFromBlocksRejectsOverflowAndInvalidSemantics(t *testing.T) {
	at := time.Now()
	cases := []struct { total, available, unit uint64; semantics string }{
		{^uint64(0), 1, 4096, "unprivileged"},
		{1, ^uint64(0), 1, "unprivileged"},
		{10, 5, 0, "caller_available"},
		{10, 5, 4096, "pretend_share_quota"},
	}
	for _, tc := range cases {
		if _, err := FromBlocks("docs", tc.total, tc.available, tc.unit, tc.semantics, at); !errors.Is(err, ErrInvalidMeasurement) {
			t.Fatalf("accepted invalid sample %+v: %v", tc, err)
		}
	}
}

func TestUnknownOmitsNumbersAndDoesNotInventFreeSpace(t *testing.T) {
	got, err := Unknown("docs", "unavailable", "probe_failed")
	if err != nil { t.Fatal(err) }
	data, err := json.Marshal(got)
	if err != nil { t.Fatal(err) }
	if strings.Contains(string(data), "bytes") || got.Status != "unavailable" || !got.ReadOnlyExport {
		t.Fatalf("unavailable snapshot claimed measured capacity: %s", data)
	}
	if _, err := Unknown("docs", "measured", "probe_failed"); err == nil {
		t.Fatal("accepted missing numeric sample as measured")
	}
}
