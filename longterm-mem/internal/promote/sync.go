package promote

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vaultlayout"
)

// syncStateSchema is the sync-state record's schema version.
const syncStateSchema = 1

// Deps are Sync's (and Propagate's) dependencies, mirroring query.Deps's
// function-seam convention: Memory is the port the package owns (a fake or
// a temp DB behind the Engram adapter in tests), Writer is a concrete
// production type (a temp vault in tests), RebuildIndex is a seam so tests
// never need real vault subprocess scripts to prove R-031's wiring.
type Deps struct {
	// Memory serves the candidate observations of a project and the history
	// propagation reads.
	Memory Memory
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
	// Withheld lists the observations the run left alone because it could not tell whether they already have a
	// page: no readable page carries them and a page could not be read. Nothing was written for them, no
	// address was taken, and a later run decides them once the unreadable pages are repaired or removed. It is
	// not a failure and costs the run nothing else.
	Withheld []int64
	// Unreadable names every page of the vault the run could not read and skipped.
	Unreadable []UnreadablePage
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
//
// A page of the vault that cannot be read is not one of those failures. It is
// skipped and named (SyncReport.Unreadable), as the doctor names it, and the
// run goes on: an observation that a readable page carries is promoted as
// ever, and an observation that no readable page carries while a page could
// not be read is left alone (SyncReport.Withheld), because the unreadable page
// may be its own and a new page would be a second one. Nothing is written for
// it and no address is taken; a later run decides it once the page is repaired
// or removed. A pages directory that cannot be listed at all stays what it was,
// the failure of every observation.
func Sync(ctx context.Context, deps Deps, project string) (SyncReport, error) {
	if err := deps.Writer.checkPorts(); err != nil {
		return SyncReport{}, fmt.Errorf("promote: sync: %w", err)
	}
	observations, err := deps.Memory.ListObservations(project)
	if err != nil {
		return SyncReport{}, fmt.Errorf("promote: sync: list observations for %q: %w", project, err)
	}

	report := SyncReport{Unreadable: UnreadablePages(deps.Writer.Pages)}
	for _, obs := range observations {
		decision, err := decidePromotion(deps.Writer.Pages, project, obs)
		if err != nil {
			report.Failed = append(report.Failed, SyncFailure{ObservationID: obs.ID, Err: err})
			continue
		}
		switch decision {
		case promotionNotNeeded:
			report.Skipped++
			continue
		case promotionWithheld:
			report.Withheld = append(report.Withheld, obs.ID)
			continue
		}

		result, err := deps.Writer.Promote(ctx, obs, false)
		if err != nil {
			report.Failed = append(report.Failed, SyncFailure{ObservationID: obs.ID, Err: err})
			continue
		}
		if result.Action.Kind == ActionSkippedUnreadablePages {
			report.Withheld = append(report.Withheld, obs.ID)
			continue
		}
		report.Promoted = append(report.Promoted, result)
	}

	if deps.RebuildIndex != nil {
		if err := deps.RebuildIndex(ctx); err != nil {
			return report, fmt.Errorf("promote: sync: rebuild index: %w", err)
		}
	}

	if err := writeSyncState(deps.Writer.VaultRoot, utc(deps.Writer.Clock)); err != nil {
		return report, fmt.Errorf("promote: sync: %w", err)
	}
	return report, failureError("sync", report.Failed)
}

// promotionDecision is what a run does about one observation.
type promotionDecision int

const (
	// promotionNotNeeded: the observation is ineligible, or its page is already current. Nothing is written.
	promotionNotNeeded promotionDecision = iota
	// promotionNeeded: the run writes a page for the observation.
	promotionNeeded
	// promotionWithheld: no readable page carries the observation and a page could not be read, so it cannot be
	// told whether the observation is promoted. Nothing is written, and it is not a failure.
	promotionWithheld
)

// decidePromotion answers the one question Sync asks of each observation:
// would this run write a page for it? It is extracted so Plan and Sync
// cannot drift apart -- a dry run that predicts from its own copy of this
// walk is a preview that stops describing the run the moment either copy
// changes, and an operator only finds out by running the thing the preview
// existed to let them avoid.
func decidePromotion(pages PageReader, project string, obs memory.Observation) (promotionDecision, error) {
	if !Eligible(obs, false) {
		return promotionNotNeeded, nil
	}
	promoted, ok, err := findPromotedPage(pages, project, int(obs.ID))
	var unknown *unreadablePagesError
	if errors.As(err, &unknown) {
		return promotionWithheld, nil
	}
	if err != nil {
		return promotionNotNeeded, fmt.Errorf("check promoted state: %w", err)
	}
	if ok && promoted.Revision >= obs.RevisionCount {
		return promotionNotNeeded, nil
	}
	return promotionNeeded, nil
}

// SyncPlan is what a Sync run WOULD do, computed without writing anything.
//
// It exists because a first sync on an established project promotes every
// eligible observation at once -- hundreds of pages -- into a vault whose
// index.md and log.md may be hand-authored, and the run is otherwise
// unannounced. Titles names them rather than only counting them, so the
// operator can recognise what is about to land.
type SyncPlan struct {
	// WouldPromote is how many observations Sync would write a page for.
	WouldPromote int
	// Skipped counts ineligible and already-current observations.
	Skipped int
	// Failed names each observation whose promoted state could not even be
	// read. A plan that silently omitted these would under-predict the run.
	Failed []SyncFailure
	// Titles names the observations WouldPromote counts, in the same order.
	Titles []string
	// WouldPatch is how many ALREADY-PROMOTED pages the second pass
	// (Propagate) would rewrite. `sync` runs both passes and reports both
	// counts; a preview that carried only WouldPromote would tell an
	// operator nothing is rewritten immediately before a re-sync rewrites
	// existing pages.
	WouldPatch int
	// PatchAddresses names the pages WouldPatch counts, in the same order.
	PatchAddresses []string
	// Withheld lists the observations either pass would leave alone for want of a page that can be told from
	// the unreadable ones (see SyncReport.Withheld), each once.
	Withheld []int64
	// Unreadable names every page of the vault the run would skip because it cannot be read.
	Unreadable []UnreadablePage
}

// Plan reports what a `sync` run would do for project, writing nothing at
// all: no page, no precedence entry, no patched frontmatter, no index
// rebuild, and no sync-state record.
//
// It covers BOTH passes the command runs -- promotion and propagation --
// walking the same observations through the same decidePromotion and
// decidePatch those passes use, so its prediction is their own decision
// rather than a second opinion about it.
func Plan(ctx context.Context, deps Deps, project string) (SyncPlan, error) {
	observations, err := deps.Memory.ListObservations(project)
	if err != nil {
		return SyncPlan{}, fmt.Errorf("promote: plan: list observations for %q: %w", project, err)
	}

	plan := SyncPlan{Unreadable: UnreadablePages(deps.Writer.Pages)}
	for _, obs := range observations {
		decision, err := decidePromotion(deps.Writer.Pages, project, obs)
		if err != nil {
			plan.Failed = append(plan.Failed, SyncFailure{ObservationID: obs.ID, Err: err})
			continue
		}
		switch decision {
		case promotionNotNeeded:
			plan.Skipped++
		case promotionWithheld:
			plan.Withheld = append(plan.Withheld, obs.ID)
		case promotionNeeded:
			plan.WouldPromote++
			plan.Titles = append(plan.Titles, obs.Title)
		}
	}

	patches, err := patchCandidates(deps, project)
	if err != nil {
		return SyncPlan{}, err
	}
	plan.PatchAddresses = patches.addresses
	plan.WouldPatch = len(patches.addresses)
	plan.Failed = mergeFailures(plan.Failed, patches.failed)
	plan.Withheld = mergeWithheld(plan.Withheld, patches.withheld)
	return plan, nil
}

// mergeWithheld joins the two passes' lists of observations left alone, each observation once, in the order
// first met: both passes ask the same page lookup of the same observation, so one unreadable page withholds an
// observation from both, and it is one observation.
func mergeWithheld(first, second []int64) []int64 {
	seen := make(map[int64]bool, len(first))
	for _, id := range first {
		seen[id] = true
	}
	merged := first
	for _, id := range second {
		if seen[id] {
			continue
		}
		seen[id] = true
		merged = append(merged, id)
	}
	return merged
}

// mergeFailures joins the two passes' failure lists, dropping an entry the
// other pass already reported IDENTICALLY.
//
// Both passes call findPromotedPage on the same observation, so a page
// whose frontmatter cannot be parsed fails both with the same message, and
// one broken page arrived in the list twice: an operator reading "2
// failures" goes looking for a second broken page that does not exist.
//
// Only an exact repeat is dropped. Two DIFFERENT failures for the same
// observation are two real problems -- the promotion pass and the patch
// pass ask different questions of it -- and collapsing those would hide
// one, which is the same lie in the other direction.
func mergeFailures(first, second []SyncFailure) []SyncFailure {
	seen := make(map[string]bool, len(first))
	key := func(f SyncFailure) string {
		return fmt.Sprintf("%d\x00%v", f.ObservationID, f.Err)
	}
	for _, f := range first {
		seen[key(f)] = true
	}
	merged := first
	for _, f := range second {
		if seen[key(f)] {
			continue
		}
		seen[key(f)] = true
		merged = append(merged, f)
	}
	return merged
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

// syncStateRecord is vaultlayout.SyncStateFile's decoded/encoded form.
type syncStateRecord struct {
	Schema              int    `json:"schema"`
	LastSyncCompletedAt string `json:"last_sync_completed_at"`
}

// writeSyncState durably records vaultRoot's sync-state file (R-031, via
// address.go's writeFileAtomic), stamped with the instant the run completed,
// which the caller reads from the Writer's Clock in UTC.
func writeSyncState(vaultRoot string, completedAt time.Time) error {
	record := syncStateRecord{Schema: syncStateSchema, LastSyncCompletedAt: completedAt.Format(time.RFC3339)}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal sync-state record: %w", err)
	}
	full := filepath.Join(vaultRoot, vaultlayout.SyncStateFile)
	return writeFileAtomic(full, append(data, '\n'))
}
