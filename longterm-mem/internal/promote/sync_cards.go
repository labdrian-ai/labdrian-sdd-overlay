package promote

import (
	"context"
	"errors"
	"fmt"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// CardMemories is the port SyncCards lists a project's memories of memorycard through: the memories a promotion
// pass reads, as promote.CardLookup is the port a single explicit promote reads one memory through. A memory it
// serves has the fields ExplicitPromoteCard's lookup fills, SameContentEngramIDs included, because that is what
// tells a memory mirrored by hand from one that is new.
type CardMemories interface {
	ListMemories(project string) ([]memory.CardMemory, error)
	// ListRetiredMemories lists the memories memorycard has superseded or archived: the ones PropagateCards
	// reads. An archived memory is in no other listing, and a page cannot be patched for a memory that is not
	// listed.
	ListRetiredMemories(project string) ([]memory.CardMemory, error)
}

// CardSyncReport summarizes one SyncCards run, in the shape of SyncReport: Promoted lists every memory the run
// wrote a page for (create or update), and LocalEdits the ones whose hand-edited page it left alone; Skipped counts ineligible and already-current memories, neither of which
// costs a write; Failed names each memory the run could not process (in SyncFailure.ObservationID, which here is
// the memorycard memory id); Withheld lists the memories left alone because a page that could not be read may be
// theirs. Bridged adds what only a memory of memorycard can be: the memories that are bridged to an Engram
// observation that has a page, which are skipped and are not failures.
type CardSyncReport struct {
	SyncReport
	// Bridged counts the memories the run left alone because PromoteCard would refuse them with a BridgedError.
	Bridged int
	// LocalEdits lists the memories whose newer revision was not written because their page was edited by hand
	// (UpdateInPlace's ActionSkippedLocalEdit); they are not in Promoted, since the run wrote nothing for them.
	LocalEdits []int64
}

// SyncCards promotes the eligible memories of memorycard in project that have no page, or whose page is at an
// older revision (S1 b of the C22-FU follow-up). It decides eligibility with Eligible, the predicate of R-007,
// reads the vault's pages once for the whole run, and decides and writes each memory through the code PromoteCard
// runs (decideCard and applyCard), so that an explicit promote and a sync cannot treat one memory differently.
//
// A memory at the revision its page carries writes nothing: no page and no precedence entry. A memory that is
// bridged to an Engram observation that has a page is skipped and counted, never a failure and never written.
// One memory that cannot be processed does not abort the run: it is recorded in Failed and the walk continues,
// and the run returns a non-nil error when anything failed, as Sync does. A pages directory that cannot be listed
// at all is the failure of every eligible memory.
//
// SyncCards does not propagate status (PropagateCards), rebuild the index, or record the sync: SyncCardsProject
// runs all three.
func SyncCards(ctx context.Context, cards CardMemories, w *Writer, project string) (CardSyncReport, error) {
	report, _, err := walkCards(ctx, cards, w, project, true)
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
	// Skipped counts ineligible and already-current memories; Bridged the memories PromoteCard would refuse.
	Skipped int
	Bridged int
	// Failed names each memory whose promoted state could not be decided; Withheld the memories left alone for
	// want of a page that can be told from the unreadable ones; Unreadable every page the run would skip.
	Failed     []SyncFailure
	Withheld   []int64
	Unreadable []UnreadablePage
}

// PlanCards reports what SyncCards would do for project and writes nothing: it walks the same memories through
// the same walk and the same decision (decideCard) as SyncCards, stopping where SyncCards would write, so its
// prediction is the run's own decision and not a second opinion about it. A memory it cannot decide is in
// Failed, and the plan is still returned; a run that could not start at all is an error and no plan.
func PlanCards(ctx context.Context, cards CardMemories, w *Writer, project string) (CardSyncPlan, error) {
	report, titles, err := walkCards(ctx, cards, w, project, false)
	if err != nil && len(report.Failed) == 0 {
		return CardSyncPlan{}, err
	}
	patches, err := cardPatchCandidates(cards, w, project)
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

// walkCards is the one walk of SyncCards and PlanCards. With apply it writes each memory the decision says needs a
// page; without, it names the memory by title and writes nothing, so its report carries no Promoted.
func walkCards(ctx context.Context, cards CardMemories, w *Writer, project string, apply bool) (CardSyncReport, []string, error) {
	if err := w.checkPorts(); err != nil {
		return CardSyncReport{}, nil, fmt.Errorf("promote: sync memorycard: %w", err)
	}
	memories, err := cards.ListMemories(project)
	if err != nil {
		return CardSyncReport{}, nil, &CardListError{Project: project, Err: err}
	}

	var report CardSyncReport
	var titles []string
	pages, pagesErr := readCardPages(w.Pages)
	if pagesErr == nil {
		report.Unreadable = pages.unreadable
	}
	for _, m := range memories {
		if !Eligible(memory.Observation{Pinned: m.Pinned, TopicKey: m.TopicKey}, false) {
			report.Skipped++
			continue
		}
		if pagesErr != nil {
			report.Failed = append(report.Failed, SyncFailure{ObservationID: m.ID, Err: pagesErr})
			continue
		}
		scan := pages.scanFor(m)
		decision, err := decideCard(m, scan)
		if err != nil {
			report.Failed = append(report.Failed, SyncFailure{ObservationID: m.ID, Err: err})
			continue
		}
		switch decision {
		case cardBridged:
			report.Bridged++
			continue
		case cardCurrent:
			report.Skipped++
			continue
		case cardWithheld:
			report.Withheld = append(report.Withheld, m.ID)
			continue
		}
		if !apply {
			titles = append(titles, m.Title)
			continue
		}
		result, err := w.applyCard(ctx, m, scan, decision)
		if err != nil {
			report.Failed = append(report.Failed, SyncFailure{ObservationID: m.ID, Err: err})
			continue
		}
		if result.Action.Kind == ActionSkippedLocalEdit {
			// Nothing was written and the result carries no page: the hand-edited page keeps its address.
			report.LocalEdits = append(report.LocalEdits, m.ID)
			continue
		}
		// The memories after this one see the page this one has: a successor link names it.
		pages.noteAddress(m.Project, m.ID, result.Page.Address)
		report.Promoted = append(report.Promoted, result)
	}
	return report, titles, cardFailureError(report.Failed)
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
func SyncCardsProject(ctx context.Context, cards CardMemories, w *Writer, index IndexRebuilder, project string) (CardProjectReport, error) {
	if err := w.checkSyncState(); err != nil {
		return CardProjectReport{Failure: OtherFailure}, fmt.Errorf("promote: sync memorycard: %w", err)
	}
	report, err := SyncCards(ctx, cards, w, project)
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
	patches, patchErr := PropagateCards(ctx, cards, w, project)
	out.Patched = patches.Patched
	out.Failed = append(out.Failed, patches.Failed...)
	out.Withheld = mergeWithheld(out.Withheld, patches.Withheld)
	switch {
	case CardRunFailure(patchErr) == MemoryUnavailable:
		out.Failure = MemoryUnavailable
		return out, patchErr
	case patchErr != nil && len(patches.Failed) == 0:
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
	if err != nil {
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
