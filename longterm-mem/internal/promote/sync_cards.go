package promote

import (
	"context"
	"fmt"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// CardMemories is the port SyncCards lists a project's memories of memorycard through: the memories a promotion
// pass reads, as promote.CardLookup is the port a single explicit promote reads one memory through. A memory it
// serves has the fields ExplicitPromoteCard's lookup fills, SameContentEngramIDs included, because that is what
// tells a memory mirrored by hand from one that is new.
type CardMemories interface {
	ListMemories(project string) ([]memory.CardMemory, error)
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
// SyncCards does not rebuild the index, record the sync, or propagate status: the command that runs it does.
func SyncCards(ctx context.Context, cards CardMemories, w *Writer, project string) (CardSyncReport, error) {
	if err := w.checkPorts(); err != nil {
		return CardSyncReport{}, fmt.Errorf("promote: sync memorycard: %w", err)
	}
	memories, err := cards.ListMemories(project)
	if err != nil {
		return CardSyncReport{}, fmt.Errorf("promote: sync memorycard: list memories for %q: %w", project, err)
	}

	var report CardSyncReport
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
	return report, cardFailureError(report.Failed)
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
