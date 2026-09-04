// Package ops implements longterm-mem's operability surface: a health-check
// Status and a diagnostic Doctor (R-010, R-011). Both are read-only -- they
// inspect Engram and the vault but never write to either -- so an operator
// can run either at any time without risking vault state.
package ops

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// syncStateRelPath mirrors promote's own (unexported) syncStateRelPath
// constant: the same vault-relative sync-state contract path
// (.vault-meta/longterm-mem-sync-state.json, D6), read here independently
// since Status is a read-only consumer with no promote package dependency
// of its own -- lint.go's checkAddressMap follows the same
// hardcode-the-contract-path convention for .raw/.manifest.json rather
// than importing the writer that produces it.
const syncStateRelPath = ".vault-meta/longterm-mem-sync-state.json"

// neverSynced is Report.LastSyncCompletedAt's value when no sync-state
// record exists yet. Status must never fabricate a timestamp (R-010).
const neverSynced = "never"

// StatusDeps are Status's dependencies (function-seam convention matching
// query.Deps/promote.Deps): EngramReachable and VaultProvisioned are seams
// so a test can prove Status's own composition logic without a real
// Engram DB connection or a fully provisioned vault fixture.
type StatusDeps struct {
	// EngramReachable reports whether Engram is reachable and, when it is
	// not, why. Required.
	EngramReachable func(ctx context.Context) (bool, string)
	// VaultProvisioned reports whether VaultRoot has been fully indexed.
	// Production wires vault.Provisioned. Required.
	VaultProvisioned func(vaultRoot string) bool
	// VaultRoot is the resolved vault path Status inspects (provisioning
	// state and the sync-state record).
	VaultRoot string
}

// StatusReport is Status's R-010 output: Engram reachability, the vault's
// provisioning state, and the last recorded sync completion.
type StatusReport struct {
	Project             string `json:"project"`
	EngramReachable     bool   `json:"engram_reachable"`
	EngramDetail        string `json:"engram_detail,omitempty"`
	VaultProvisioned    bool   `json:"vault_provisioned"`
	LastSyncCompletedAt string `json:"last_sync_completed_at"`
}

// Status reports Engram reachability, project's vault provisioning state,
// and the last successful sync completion time (R-010). A never-provisioned
// vault and a never-synced project are reported states, not errors --
// Status only returns a non-nil error for a genuine read failure (e.g. a
// malformed sync-state record), never for an unhealthy-but-readable field.
func Status(ctx context.Context, deps StatusDeps, project string) (StatusReport, error) {
	report := StatusReport{Project: project}

	report.EngramReachable, report.EngramDetail = deps.EngramReachable(ctx)
	report.VaultProvisioned = deps.VaultProvisioned(deps.VaultRoot)

	lastSync, err := readLastSyncCompletedAt(deps.VaultRoot)
	if err != nil {
		return StatusReport{}, err
	}
	report.LastSyncCompletedAt = lastSync

	return report, nil
}

// readLastSyncCompletedAt reads vaultRoot's sync-state record and returns
// its last_sync_completed_at field, or neverSynced when the record does
// not exist yet -- a project that has never synced must never be reported
// with a fabricated or stale timestamp (R-010).
func readLastSyncCompletedAt(vaultRoot string) (string, error) {
	full := filepath.Join(vaultRoot, syncStateRelPath)
	data, err := os.ReadFile(full)
	if err != nil {
		if os.IsNotExist(err) {
			return neverSynced, nil
		}
		return "", fmt.Errorf("ops: read %s: %w", full, err)
	}

	var record struct {
		LastSyncCompletedAt string `json:"last_sync_completed_at"`
	}
	if err := json.Unmarshal(data, &record); err != nil {
		return "", fmt.Errorf("ops: parse %s: %w", full, err)
	}
	if record.LastSyncCompletedAt == "" {
		return neverSynced, nil
	}
	return record.LastSyncCompletedAt, nil
}
