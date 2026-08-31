package promote

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
)

// syncStateRelPath is the vault-relative sync-state record R-031 requires
// (Anchors: "<vault>/.vault-meta/longterm-mem-sync-state.json", schema 1).
const syncStateRelPath = ".vault-meta/longterm-mem-sync-state.json"

// syncStateSchema is the sync-state record's schema version.
const syncStateSchema = 1

// Deps are Sync's (and Propagate's) dependencies, mirroring query.Deps's
// function-seam convention: Engram/Writer are concrete production types
// (a temp DB and a temp vault in tests), RebuildIndex is a seam so tests
// never need real vault subprocess scripts to prove R-031's wiring.
type Deps struct {
	// Engram lists candidate observations for project.
	Engram *engram.Store
	// Writer promotes each eligible, unpromoted-or-revised observation,
	// and (Propagate) owns the precedence sidecar a status-only patch
	// updates.
	Writer *Writer
	// RebuildIndex rebuilds the vault's index after promotion completes
	// (R-031). A real caller (cmd_sync.go) wraps vault.Rebuild; a nil
	// RebuildIndex is treated as a no-op, letting Sync's promotion-only
	// tests skip the seam entirely.
	RebuildIndex func(ctx context.Context) error
}

// SyncReport summarizes one Sync run: Promoted lists every observation
// Writer.Promote actually wrote (create or update); Skipped counts
// ineligible and already-current observations, neither of which costs a
// write; Failed names each observation the run could not process, so a
// caller can tell "nothing left to promote" from "observation N is
// broken" instead of inferring it from a single aborted error.
type SyncReport struct {
	Promoted []Result
	Skipped  int
	Failed   []SyncFailure
}

// SyncFailure is one observation Sync could not process, kept per
// observation rather than aborting the run (see Sync).
type SyncFailure struct {
	ObservationID int64
	Err           error
}

// Sync promotes every eligible, unpromoted-or-revised observation for
// project (R-009), then rebuilds the vault's index and records the sync's
// completion timestamp (R-031). An observation already promoted at its
// current revision is left untouched before Writer.Promote is ever
// called -- neither a page write nor a precedence-store write happens for
// it -- so re-running Sync with nothing new to promote is a true no-op,
// not merely an idempotent overwrite (EmitPage's updated: timestamp would
// otherwise change on every call regardless of content).
//
// One observation that cannot be processed does not abort the run: it is
// recorded in SyncReport.Failed and the walk continues. ListObservations
// returns the same set on every run, so aborting mid-loop would let a
// single persistently broken observation wedge the project's sync
// forever -- every later observation never attempted, the index never
// rebuilt, the sync-state record never written, and each retry failing
// identically. Sync still returns a non-nil error when anything failed,
// so a partially successful run can never be mistaken for a clean one.
func Sync(ctx context.Context, deps Deps, project string) (SyncReport, error) {
	observations, err := deps.Engram.ListObservations(project)
	if err != nil {
		return SyncReport{}, fmt.Errorf("promote: sync: list observations for %q: %w", project, err)
	}

	var report SyncReport
	for _, obs := range observations {
		if !Eligible(obs, false) {
			report.Skipped++
			continue
		}

		promoted, ok, err := findPromotedPage(deps.Writer.VaultRoot, project, int(obs.ID))
		if err != nil {
			report.Failed = append(report.Failed, SyncFailure{ObservationID: obs.ID, Err: fmt.Errorf("check promoted state: %w", err)})
			continue
		}
		if ok && promoted.Revision >= obs.RevisionCount {
			report.Skipped++
			continue
		}

		result, err := deps.Writer.Promote(obs, false)
		if err != nil {
			report.Failed = append(report.Failed, SyncFailure{ObservationID: obs.ID, Err: err})
			continue
		}
		report.Promoted = append(report.Promoted, result)
	}

	if deps.RebuildIndex != nil {
		if err := deps.RebuildIndex(ctx); err != nil {
			return report, fmt.Errorf("promote: sync: rebuild index: %w", err)
		}
	}

	if err := writeSyncState(deps.Writer.VaultRoot); err != nil {
		return report, fmt.Errorf("promote: sync: %w", err)
	}
	return report, failureError("sync", report.Failed)
}

// failureError summarizes op's per-observation failures as one error,
// naming the first failing observation and its cause so a caller that
// only logs err still learns which observation to look at. It is nil when
// nothing failed. Shared by Sync and Propagate, which both step over a
// broken observation rather than letting it wedge the whole run.
func failureError(op string, failed []SyncFailure) error {
	if len(failed) == 0 {
		return nil
	}
	first := failed[0]
	if len(failed) == 1 {
		return fmt.Errorf("promote: %s: observation %d: %w", op, first.ObservationID, first.Err)
	}
	return fmt.Errorf("promote: %s: %d observations failed, first is %d: %w", op, len(failed), first.ObservationID, first.Err)
}

// syncStateRecord is syncStateRelPath's decoded/encoded form.
type syncStateRecord struct {
	Schema              int    `json:"schema"`
	LastSyncCompletedAt string `json:"last_sync_completed_at"`
}

// writeSyncState atomically records vaultRoot's sync-state file (R-031,
// tmp+fsync+rename via address.go's writeFileAtomic), stamped with
// nowFunc (page.go) so tests can assert a deterministic completion
// timestamp via fixedNow.
func writeSyncState(vaultRoot string) error {
	record := syncStateRecord{Schema: syncStateSchema, LastSyncCompletedAt: nowFunc().Format(time.RFC3339)}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal sync-state record: %w", err)
	}
	full := filepath.Join(vaultRoot, syncStateRelPath)
	return writeFileAtomic(full, append(data, '\n'))
}
