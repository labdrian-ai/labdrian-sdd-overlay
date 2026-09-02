package runtime_test

import (
	"testing"

	engineRuntime "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
)

// TestLongtermMemAdapter_StatusMatrix is the substance of 10a.1: the status
// matrix must distinguish supported, EACH of the four named partial
// reasons, and unsupported — never collapsing the four partial reasons into
// one bare "partial". Table-driven over the pure decision function so no
// filesystem fixture can mask a wrong branch.
func TestLongtermMemAdapter_StatusMatrix(t *testing.T) {
	cases := []struct {
		name       string
		state      engineRuntime.LongtermMemComponentState
		wantStatus engineRuntime.CapabilityStatus
		wantReason string
	}{
		{
			name: "supported: binary executable, record present, entry present with matching fingerprint",
			state: engineRuntime.LongtermMemComponentState{
				RootResolvable: true, BinaryPresent: true, RecordPresent: true, EntryPresent: true, FingerprintMatch: true,
			},
			wantStatus: engineRuntime.CapabilitySupported,
			wantReason: "",
		},
		{
			name: "partial: record without entry",
			state: engineRuntime.LongtermMemComponentState{
				RootResolvable: true, BinaryPresent: true, RecordPresent: true, EntryPresent: false,
			},
			wantStatus: engineRuntime.CapabilityPartial,
			wantReason: engineRuntime.LongtermMemReasonRecordWithoutEntry,
		},
		{
			name: "partial: entry without record (unmanaged)",
			state: engineRuntime.LongtermMemComponentState{
				RootResolvable: true, BinaryPresent: true, RecordPresent: false, EntryPresent: true,
			},
			wantStatus: engineRuntime.CapabilityPartial,
			wantReason: engineRuntime.LongtermMemReasonEntryWithoutRecord,
		},
		{
			name: "partial: fingerprint drift",
			state: engineRuntime.LongtermMemComponentState{
				RootResolvable: true, BinaryPresent: true, RecordPresent: true, EntryPresent: true, FingerprintMatch: false,
			},
			wantStatus: engineRuntime.CapabilityPartial,
			wantReason: engineRuntime.LongtermMemReasonFingerprintDrift,
		},
		{
			name: "partial: missing binary",
			state: engineRuntime.LongtermMemComponentState{
				RootResolvable: true, BinaryPresent: false, RecordPresent: true, EntryPresent: true, FingerprintMatch: true,
			},
			wantStatus: engineRuntime.CapabilityPartial,
			wantReason: engineRuntime.LongtermMemReasonMissingBinary,
		},
		{
			name: "unsupported: config root unresolvable",
			state: engineRuntime.LongtermMemComponentState{
				RootResolvable: false,
			},
			wantStatus: engineRuntime.CapabilityUnsupported,
			wantReason: engineRuntime.LongtermMemReasonConfigRootUnresolvable,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, reason := engineRuntime.EvaluateLongtermMemComponentStatus(tc.state)
			if status != tc.wantStatus {
				t.Fatalf("status = %q, want %q", status, tc.wantStatus)
			}
			if reason != tc.wantReason {
				t.Fatalf("reason = %q, want %q — a test that only checked status would have passed while the reason collapsed", reason, tc.wantReason)
			}
		})
	}

	// Distinctness guard: the four partial reasons must be four DIFFERENT
	// strings, so a table-driven test that only asserted CapabilityPartial
	// could never pass while silently collapsing all four into one.
	seen := map[string]bool{}
	for _, tc := range cases {
		if tc.wantStatus != engineRuntime.CapabilityPartial {
			continue
		}
		if seen[tc.wantReason] {
			t.Fatalf("duplicate partial reason %q across cases — the four named reasons must be distinct", tc.wantReason)
		}
		seen[tc.wantReason] = true
	}
	if len(seen) != 4 {
		t.Fatalf("expected exactly 4 distinct named partial reasons in this table, got %d: %v", len(seen), seen)
	}
}
