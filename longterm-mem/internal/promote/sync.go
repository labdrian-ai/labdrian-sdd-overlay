package promote

import (
	"context"
	"fmt"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// Deps are Sync's (and Propagate's) dependencies: Source and Index are ports
// the package owns (a fake or a temp DB behind a store's adapter in tests, and
// a fake so tests never need real vault subprocess scripts to prove R-031's
// wiring), Writer is a concrete production type (a temp vault in tests).
type Deps struct {
	// Source serves the candidate memories of a project and the history
	// propagation reads, whichever store they come from.
	Source Source
	// Writer promotes each eligible, unpromoted-or-revised memory,
	// and (Propagate) owns the precedence sidecar a status-only patch
	// updates.
	Writer *Writer
	// Index rebuilds the vault's index after promotion completes (R-031).
	// A real caller (cmd_sync.go) hands it the vault adapter; none wired,
	// or a nil pointer, is a no-op, letting Sync's promotion-only tests
	// skip the port entirely.
	Index IndexRebuilder
}

// SyncReport summarizes one Sync run: Promoted lists every memory
// Writer.Promote actually wrote (create or update); Skipped counts
// ineligible and already-current memories, neither of which costs a
// write; Bridged counts the memories left alone because they are the same
// piece of memory as one of another store that has a page; Failed names each
// memory the run could not process, so a caller can tell "nothing left to
// promote" from "memory N is broken" instead of inferring it from a single
// aborted error.
type SyncReport struct {
	Promoted []Result
	Skipped  int
	// Bridged counts the memories the run left alone because a memory of another store that is the same piece of
	// memory already has a page (the lineage of the memory names it): they are skipped, never a failure, and nothing
	// is written for them.
	Bridged int
	Failed  []SyncFailure
	// Withheld lists the memories the run left alone because it could not tell whether they already have a
	// page: no readable page carries them and a page could not be read. Nothing was written for them, no
	// address was taken, and a later run decides them once the unreadable pages are repaired or removed. It is
	// not a failure and costs the run nothing else.
	Withheld []int64
	// Unreadable names every page of the vault the run could not read and skipped.
	Unreadable []UnreadablePage
}

// SyncFailure is one memory Sync could not process, kept per
// memory rather than aborting the run (see Sync). ObservationID is the memory's id in its store; it is 0 when the
// id is not a number, and then Err names the real id.
type SyncFailure struct {
	ObservationID int64
	Err           error
}

// withIDError is err, and, when the memory's id is not a number (idErr, from numericID), what says so: a failure
// recorded under ObservationID 0 must still name the memory.
func withIDError(err, idErr error) error {
	if idErr == nil {
		return err
	}
	return fmt.Errorf("%w (%w)", err, idErr)
}

// Sync promotes every eligible, unpromoted-or-revised memory for
// project (R-009), then rebuilds the vault's index and records the sync's
// completion timestamp (R-031). A memory already promoted at its
// current revision is left untouched before anything is written -- neither a
// page write nor a precedence-store write happens for it -- so re-running
// Sync with nothing new to promote is a true no-op, not merely an idempotent
// overwrite (the page's updated: timestamp would otherwise change on every call
// regardless of content). It walks the memories with the one walk of every
// promotion pass (Writer.walk).
//
// One memory that cannot be processed does not abort the run: it is
// recorded in SyncReport.Failed and the walk continues. Candidates
// returns the same set on every run, so aborting mid-loop would let a
// single persistently broken memory wedge the project's sync
// forever -- every later memory never attempted, the index never
// rebuilt, the sync-state record never written, and each retry failing
// identically. Sync still returns a non-nil error when anything failed,
// so a partially successful run can never be mistaken for a clean one.
//
// A page of the vault that cannot be read is not one of those failures. It is
// skipped and named (SyncReport.Unreadable), as the doctor names it, and the
// run goes on: a memory that a readable page carries is promoted as
// ever, and a memory that no readable page carries while a page could
// not be read is left alone (SyncReport.Withheld), because the unreadable page
// may be its own and a new page would be a second one. Nothing is written for
// it and no address is taken; a later run decides it once the page is repaired
// or removed. A pages directory that cannot be listed at all stays what it was,
// the failure of every memory.
func Sync(ctx context.Context, deps Deps, project string) (SyncReport, error) {
	if err := deps.Writer.checkPorts(); err != nil {
		return SyncReport{}, fmt.Errorf("promote: sync: %w", err)
	}
	if err := deps.Writer.checkSyncState(); err != nil {
		return SyncReport{}, fmt.Errorf("promote: sync: %w", err)
	}
	memories, err := deps.Source.Candidates(project)
	if err != nil {
		return SyncReport{}, fmt.Errorf("promote: sync: list observations for %q: %w", project, err)
	}

	walked := deps.Writer.walk(ctx, memories, true)
	report := SyncReport{Unreadable: walked.unreadable, Skipped: walked.skipped, Bridged: walked.bridged, Withheld: walked.withheld}
	for _, outcome := range walked.written {
		// A hand-edited page left alone is in Promoted too, with the page it did not write: the run has always
		// reported it so.
		report.Promoted = append(report.Promoted, outcome.Result)
	}
	for _, failure := range walked.failed {
		report.Failed = append(report.Failed, stateFailure(failure))
	}

	// memory.IsMissing and not "!= nil": a rebuilder that failed to open reaches Deps as an interface holding a
	// nil pointer, which a plain nil comparison takes for a rebuilder and which then panics inside the call
	// (TestSyncWithoutARebuilderSkipsTheRebuild holds both shapes).
	if !memory.IsMissing(deps.Index) {
		if err := deps.Index.RebuildIndex(ctx); err != nil {
			return report, &IndexRebuildError{Err: err}
		}
	}

	if err := deps.Writer.SyncState.RecordSyncCompleted(utc(deps.Writer.Clock)); err != nil {
		return report, fmt.Errorf("promote: sync: %w", err)
	}
	return report, failureError("sync", report.Failed)
}

// stateFailure is the failure of a walk as the passes over observations of Engram have always named it: a page
// state that could not be read says so ("check promoted state: ...").
func stateFailure(failure walkFailure) SyncFailure {
	if failure.deciding {
		failure.Err = fmt.Errorf("check promoted state: %w", failure.Err)
	}
	return failure.SyncFailure
}

// IndexRebuildError is how Sync reports that the vault's index could not be rebuilt after the pages were written:
// a type of its own, so that a caller who needs to tell that failure from the others finds it with errors.As and
// does not read words. Its words are the ones the sync has always said.
type IndexRebuildError struct {
	// Err is the rebuild's own failure.
	Err error
}

func (e *IndexRebuildError) Error() string { return "promote: sync: rebuild index: " + e.Err.Error() }

func (e *IndexRebuildError) Unwrap() error { return e.Err }

// SyncPlan is what a Sync run WOULD do, computed without writing anything.
//
// It exists because a first sync on an established project promotes every
// eligible memory at once -- hundreds of pages -- into a vault whose
// index.md and log.md may be hand-authored, and the run is otherwise
// unannounced. Titles names them rather than only counting them, so the
// operator can recognise what is about to land.
type SyncPlan struct {
	// WouldPromote is how many memories Sync would write a page for.
	WouldPromote int
	// Skipped counts ineligible and already-current memories, and Bridged the ones left alone because a memory of
	// another store that is the same piece of memory has a page.
	Skipped int
	Bridged int
	// Failed names each memory whose promoted state could not even be
	// read. A plan that silently omitted these would under-predict the run.
	Failed []SyncFailure
	// Titles names the memories WouldPromote counts, in the same order.
	Titles []string
	// WouldPatch is how many ALREADY-PROMOTED pages the second pass
	// (Propagate) would rewrite. `sync` runs both passes and reports both
	// counts; a preview that carried only WouldPromote would tell an
	// operator nothing is rewritten immediately before a re-sync rewrites
	// existing pages.
	WouldPatch int
	// PatchAddresses names the pages WouldPatch counts, in the same order.
	PatchAddresses []string
	// Withheld lists the memories either pass would leave alone for want of a page that can be told from
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
// walking the same memories through the same walk and decide the promotion
// pass uses and the same walk the propagation pass uses, so its prediction is
// their own decision rather than a second opinion about it.
func Plan(ctx context.Context, deps Deps, project string) (SyncPlan, error) {
	memories, err := deps.Source.Candidates(project)
	if err != nil {
		return SyncPlan{}, fmt.Errorf("promote: plan: list observations for %q: %w", project, err)
	}

	walked := deps.Writer.walk(ctx, memories, false)
	plan := SyncPlan{
		Unreadable: walked.unreadable, WouldPromote: len(walked.titles), Titles: walked.titles,
		Skipped: walked.skipped, Bridged: walked.bridged, Withheld: walked.withheld,
	}
	for _, failure := range walked.failed {
		plan.Failed = append(plan.Failed, stateFailure(failure))
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
	// A copy: appending to first itself could write into its backing array, which is the caller's.
	merged := append([]int64(nil), first...)
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
	// A copy, as in mergeWithheld: the caller's backing array is not this function's to write into.
	merged := append([]SyncFailure(nil), first...)
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
