package promote

import (
	"context"
	"errors"
	"fmt"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// This file is the memorycard dialect of the passes Sync, Plan and SyncProject run for Engram: the same walk
// (Writer.walk) and the same decision over the memories of a Source, worded, counted and ordered as `sync --source
// memorycard` has always reported them (a hand-edited page is counted apart from the promotions, the status
// pass runs before the index is rebuilt, a failure names a memory and not an observation).

// CardSyncReport summarizes one SyncCards run, in the shape of SyncReport: Promoted lists every memory the run
// wrote a page for (create or update), and LocalEdits the ones whose hand-edited page it left alone; Skipped counts
// ineligible and already-current memories, neither of which costs a write; Failed names each memory the run could
// not process (in SyncFailure.ObservationID, which here is the memorycard memory id); Withheld lists the memories
// left alone because a page that could not be read may be theirs; Bridged the memories that are the same piece of
// memory as one of another store that has a page, which are skipped and are not failures.
type CardSyncReport struct {
	SyncReport
	// LocalEdits lists the memories whose newer revision was not written because their page was edited by hand
	// (UpdateInPlace's ActionSkippedLocalEdit); they are not in Promoted, since the run wrote nothing for them.
	LocalEdits []int64
}

// SyncCards promotes the eligible memories of src in project that have no page, or whose page is at an older
// revision (S1 b of the C22-FU follow-up). It decides eligibility with Eligible, the predicate of R-007, reads
// the vault's pages once for the whole run, and decides and writes each memory through the walk and the code an
// explicit promote runs, so that an explicit promote and a sync cannot treat one memory differently.
//
// A memory at the revision its page carries writes nothing: no page and no precedence entry. A memory that is
// bridged to a memory of another store that has a page is skipped and counted, never a failure and never
// written. One memory that cannot be processed does not abort the run: it is recorded in Failed and the walk
// continues, and the run returns a non-nil error when anything failed, as Sync does. A pages directory that cannot
// be listed at all is the failure of every eligible memory.
//
// SyncCards does not propagate status (PropagateCards), rebuild the index, or record the sync: SyncCardsProject
// runs all three.
func SyncCards(ctx context.Context, src Source, w *Writer, project string) (CardSyncReport, error) {
	report, _, err := cardPass(ctx, src, w, project, true)
	return report, err
}

// CardSyncPlan is what a SyncCards run WOULD do, computed without writing anything: the memories it would write
// a page for (a new page or an update in place), named by title in the walk's order, and what the run would
// skip, count as bridged, leave alone or fail on. A page hand-edited since its last promote is only found when
// the run tries to write it, so the plan counts such a memory among WouldPromote.
type CardSyncPlan struct {
	WouldPromote int
	// WouldPatch is how many already-promoted pages PropagateCards would rewrite for a memory that memorycard has
	// superseded or archived, and PatchAddresses names them, in the same order. The prediction reads the pages as
	// they are before the run, so a page the run itself creates is not in it.
	WouldPatch     int
	PatchAddresses []string
	// Titles names the memories WouldPromote counts, in the same order.
	Titles []string
	// Skipped counts ineligible and already-current memories; Bridged the memories a promote would refuse.
	Skipped int
	Bridged int
	// Failed names each memory whose promoted state could not be decided; Withheld the memories left alone for
	// want of a page that can be told from the unreadable ones; Unreadable every page the run would skip.
	Failed     []SyncFailure
	Withheld   []int64
	Unreadable []UnreadablePage
}

// PlanCards reports what SyncCards would do for project and writes nothing: it walks the same memories through
// the same walk and the same decision as SyncCards, stopping where SyncCards would write, so its prediction is
// the run's own decision and not a second opinion about it. A memory it cannot decide is in Failed, and the plan
// is still returned; a run that could not start at all is an error and no plan.
func PlanCards(ctx context.Context, src Source, w *Writer, project string) (CardSyncPlan, error) {
	report, titles, err := cardPass(ctx, src, w, project, false)
	if err != nil && len(report.Failed) == 0 {
		return CardSyncPlan{}, err
	}
	patches, err := cardPatchCandidates(src, w, project)
	if err != nil {
		return CardSyncPlan{}, err
	}
	return CardSyncPlan{
		WouldPromote:   len(titles),
		WouldPatch:     len(patches.addresses),
		PatchAddresses: patches.addresses,
		Titles:         titles,
		Skipped:        report.Skipped,
		Bridged:        report.Bridged,
		Failed:         mergeFailures(report.Failed, patches.failed),
		Withheld:       mergeWithheld(report.Withheld, patches.withheld),
		Unreadable:     report.Unreadable,
	}, nil
}

// cardPass is the memorycard dialect of the walk of SyncCards and PlanCards: it lists the candidates of src,
// walks them, and says what the walk found in the terms of memories. With apply it writes each memory the
// decision says needs a page; without, it names the memory by title and writes nothing, so its report carries no
// Promoted.
func cardPass(ctx context.Context, src Source, w *Writer, project string, apply bool) (CardSyncReport, []string, error) {
	if err := w.checkPorts(); err != nil {
		return CardSyncReport{}, nil, fmt.Errorf("promote: sync memorycard: %w", err)
	}
	memories, err := src.Candidates(project)
	if err != nil {
		return CardSyncReport{}, nil, &CardListError{Project: project, Err: err}
	}
	walked := w.walk(ctx, memories, apply)
	report := CardSyncReport{SyncReport: SyncReport{
		Skipped: walked.skipped, Bridged: walked.bridged, Withheld: walked.withheld, Unreadable: walked.unreadable,
	}}
	for _, outcome := range walked.written {
		if outcome.Result.Action.Kind == ActionSkippedLocalEdit {
			// Nothing was written and the result carries no page: the hand-edited page keeps its address.
			report.LocalEdits = append(report.LocalEdits, outcome.ID)
			continue
		}
		report.Promoted = append(report.Promoted, outcome.Result)
	}
	for _, failure := range walked.failed {
		report.Failed = append(report.Failed, failure.SyncFailure)
	}
	return report, walked.titles, cardFailureError(report.Failed)
}

// CardListError is how SyncCards reports that memorycard could not list the project's memories: a type of its own,
// so that the command that ran it answers the exit code of an unavailable store from what happened and not from
// the words of the error.
type CardListError struct {
	Project string
	Err     error
}

func (e *CardListError) Error() string {
	return fmt.Sprintf("promote: sync memorycard: list memories for %q: %v", e.Project, e.Err)
}

func (e *CardListError) Unwrap() error { return e.Err }

// CardRunFailure is what failed when a memorycard pass (SyncCards, PlanCards) could not run at all, in the terms of
// Failure: a memorycard that cannot be listed (a CardListError, wrapped or not) is MemoryUnavailable, anything else
// OtherFailure. SyncCardsProject and the command's dry run both classify through it, so the two answer the same
// exit code for the same failure.
func CardRunFailure(err error) Failure {
	var listed *CardListError
	if errors.As(err, &listed) {
		return MemoryUnavailable
	}
	return OtherFailure
}

// CardProjectReport is what SyncCardsProject did for one project: SyncCards' report and, when the run failed,
// why, in the terms ProjectSyncReport's Failure uses.
type CardProjectReport struct {
	CardSyncReport
	Patched []string
	Failure Failure
}

// SyncCardsProject is what `longterm-mem sync --source memorycard` does to a project: SyncCards, then
// PropagateCards (R-033), then the rebuild of the vault's index and the record of the sync's completion, the
// steps Sync takes after its own passes (R-031). Like Sync it rebuilds and records even when a memory failed, so
// one broken memory cannot wedge the sync-state forever, and it stops before them only when a pass could not run
// at all.
func SyncCardsProject(ctx context.Context, src Source, w *Writer, index IndexRebuilder, project string) (CardProjectReport, error) {
	if err := w.checkSyncState(); err != nil {
		return CardProjectReport{Failure: OtherFailure}, fmt.Errorf("promote: sync memorycard: %w", err)
	}
	report, err := SyncCards(ctx, src, w, project)
	out := CardProjectReport{CardSyncReport: report}
	switch {
	case CardRunFailure(err) == MemoryUnavailable:
		out.Failure = MemoryUnavailable
		return out, err
	case err != nil && len(report.Failed) == 0:
		out.Failure = OtherFailure
		return out, err
	}
	// Status propagation (S1 e), after the promotion pass, so that a page that pass created before its successor's
	// page existed gets its successor link now. A memory it could not patch is a failed memory like the others.
	patches, patchErr := PropagateCards(ctx, src, w, project)
	out.Patched = patches.Patched
	out.Failed = mergeFailures(out.Failed, patches.Failed)
	out.Withheld = mergeWithheld(out.Withheld, patches.Withheld)
	// Memories that cannot be listed for propagation do not stop the run: the promotion pass has written pages, so
	// the index is rebuilt and the sync recorded as when any pass fails (R-031), and the run then answers
	// MemoryUnavailable.
	unavailable := CardRunFailure(patchErr) == MemoryUnavailable
	if patchErr != nil && !unavailable && len(patches.Failed) == 0 {
		out.Failure = OtherFailure
		return out, patchErr
	}
	if err == nil {
		err = patchErr
	}
	if !memory.IsMissing(index) {
		if rebuildErr := index.RebuildIndex(ctx); rebuildErr != nil {
			out.Failure = IndexRebuildFailed
			return out, &IndexRebuildError{Err: rebuildErr}
		}
	}
	if recordErr := w.SyncState.RecordSyncCompleted(utc(w.Clock)); recordErr != nil {
		out.Failure = OtherFailure
		return out, fmt.Errorf("promote: sync memorycard: %w", recordErr)
	}
	switch {
	case unavailable:
		out.Failure = MemoryUnavailable
	case err != nil:
		out.Failure = OtherFailure
	}
	return out, err
}

// cardFailureError is failureError for memories: it names the first failing memory and its cause, and is nil
// when nothing failed.
func cardFailureError(failed []SyncFailure) error {
	if len(failed) == 0 {
		return nil
	}
	first := failed[0]
	if len(failed) == 1 {
		return fmt.Errorf("promote: sync memorycard: memory %d: %w", first.ObservationID, first.Err)
	}
	return fmt.Errorf("promote: sync memorycard: %d memories failed, first is %d: %w", len(failed), first.ObservationID, first.Err)
}
