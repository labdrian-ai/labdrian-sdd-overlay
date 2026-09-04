package ops

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/ops/testdata"
)

// TestStatus: R-010's three scenarios, table-driven (8a.1).
func TestStatus(t *testing.T) {
	cases := []struct {
		name string
		// seedSyncState, when non-empty, is written as the recorded
		// completion timestamp before Status runs; empty means no prior
		// sync ever ran.
		seedSyncState        string
		engramReachable      bool
		vaultProvisioned     bool
		wantLastSync         string
		wantEngramReachable  bool
		wantVaultProvisioned bool
	}{
		{
			name:                 "Healthy status reports all three fields",
			seedSyncState:        "2026-08-31T12:00:00Z",
			engramReachable:      true,
			vaultProvisioned:     true,
			wantLastSync:         "2026-08-31T12:00:00Z",
			wantEngramReachable:  true,
			wantVaultProvisioned: true,
		},
		{
			name:                 "Never-provisioned vault is reported, not an error",
			seedSyncState:        "2026-08-31T12:00:00Z",
			engramReachable:      true,
			vaultProvisioned:     false,
			wantLastSync:         "2026-08-31T12:00:00Z",
			wantEngramReachable:  true,
			wantVaultProvisioned: false,
		},
		{
			name:                 "Never-synced project reports never, not a fabricated timestamp",
			seedSyncState:        "",
			engramReachable:      true,
			vaultProvisioned:     true,
			wantLastSync:         "never",
			wantEngramReachable:  true,
			wantVaultProvisioned: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vaultRoot := t.TempDir()
			if tc.seedSyncState != "" {
				testdata.WriteSyncState(t, vaultRoot, tc.seedSyncState)
			}

			deps := StatusDeps{
				VaultRoot: vaultRoot,
				EngramReachable: func(ctx context.Context) (bool, string) {
					return tc.engramReachable, ""
				},
				VaultProvisioned: func(vaultRoot string) bool {
					return tc.vaultProvisioned
				},
			}

			report, err := Status(context.Background(), deps, "labdrian-sdd-overlay")
			if err != nil {
				t.Fatalf("Status: %v", err)
			}

			if report.EngramReachable != tc.wantEngramReachable {
				t.Errorf("EngramReachable = %t, want %t", report.EngramReachable, tc.wantEngramReachable)
			}
			if report.VaultProvisioned != tc.wantVaultProvisioned {
				t.Errorf("VaultProvisioned = %t, want %t", report.VaultProvisioned, tc.wantVaultProvisioned)
			}
			if report.LastSyncCompletedAt != tc.wantLastSync {
				t.Errorf("LastSyncCompletedAt = %q, want %q", report.LastSyncCompletedAt, tc.wantLastSync)
			}
		})
	}
}

// TestStatus_MalformedSyncStateIsAnErrorNotAFabricatedTimestamp: Status's
// only error path -- a sync-state record that exists but cannot be read or
// parsed -- was exercised by nothing, so a regression that swallowed the
// parse failure and returned "never" (or an empty timestamp) would have
// passed the suite while quietly reporting a project as never-synced when
// its record was in fact corrupt (review finding
// R3-status-error-path-unproved). An unreadable record is a genuine read
// failure, and R-010's whole point is that a timestamp is never invented.
func TestStatus_MalformedSyncStateIsAnErrorNotAFabricatedTimestamp(t *testing.T) {
	vaultRoot := t.TempDir()
	statePath := filepath.Join(vaultRoot, ".vault-meta", "longterm-mem-sync-state.json")
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(statePath), err)
	}
	if err := os.WriteFile(statePath, []byte("{ this is not json"), 0o644); err != nil {
		t.Fatalf("write malformed sync-state record: %v", err)
	}

	deps := StatusDeps{
		VaultRoot:        vaultRoot,
		EngramReachable:  func(ctx context.Context) (bool, string) { return true, "" },
		VaultProvisioned: func(vaultRoot string) bool { return true },
	}

	report, err := Status(context.Background(), deps, "labdrian-sdd-overlay")
	if err == nil {
		t.Fatalf("Status = (%+v, nil), want an error for a record that exists but cannot be parsed", report)
	}
	if !strings.Contains(err.Error(), "longterm-mem-sync-state.json") {
		t.Errorf("error %q does not name the record it failed on", err)
	}
	if report != (StatusReport{}) {
		t.Errorf("report = %+v, want the zero value: no field may carry a value derived from an unreadable record", report)
	}
}
