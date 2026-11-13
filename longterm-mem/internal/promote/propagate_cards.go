package promote

import (
	"context"
	"fmt"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// PropagateCards is the propagation pass of `sync --source memorycard` (S1 e of the C22-FU follow-up, the memorycard
// counterpart of Propagate and R-033): every page that is behind its memory, because memorycard has superseded or
// archived the memory since the page was written, or restored it to active since, gets its status and related
// lines patched, and nothing else of the page. A page that already says what its memory says is left alone and not
// counted. The body, and every
// other frontmatter line, stay as they were, even on a page a human has edited by hand, and the patch is recorded
// in the precedence store the way Propagate records it: only the frontmatter hash moves, so the body's divergence
// is not erased (Writer.patchPage is the one patcher, shared with Propagate).
//
// memorycard's supersede and archive change a memory's status and add no version, so the revision a page carries
// stays current and the promotion pass (SyncCards) has nothing to do for them: the two passes are independent, and
// the order of SyncCardsProject (promotion, then this pass) is only what lets a page the promotion pass created
// before its successor's page existed get its successor link on the same run.
//
// A retired memory that has no page of its own is left alone, and one with no readable page while a page could
// not be read is withheld, because the unreadable page may be its own (as Propagate withholds an observation). One
// page that cannot be patched does not abort the run: it is recorded in Failed and the walk goes on, and the run
// returns a non-nil error when anything failed. A memorycard that cannot list its memories is a CardListError.
func PropagateCards(ctx context.Context, src Source, w *Writer, project string) (PropagateReport, error) {
	if err := w.checkPatchPorts(); err != nil {
		return PropagateReport{}, fmt.Errorf("promote: propagate memorycard: %w", err)
	}
	var report PropagateReport
	outcome, err := eachCardPatchTarget(src, w, project, func(t patchTarget) error {
		if err := w.patchPage(t); err != nil {
			return err
		}
		report.Patched = append(report.Patched, t.Address)
		return nil
	})
	if err != nil {
		return PropagateReport{}, err
	}
	report.Failed = outcome.failed
	report.Withheld = outcome.withheld

	if err := w.savePatched("propagate memorycard", report.Patched); err != nil {
		return report, err
	}
	return report, cardFailureError(report.Failed)
}

// cardPatchCandidates reports the pages PropagateCards would rewrite, through the same eachCardPatchTarget walk
// PropagateCards itself runs, and writes nothing: the dry run's prediction is the run's own walk.
func cardPatchCandidates(src Source, w *Writer, project string) (patchPlan, error) {
	var plan patchPlan
	outcome, err := eachCardPatchTarget(src, w, project, func(t patchTarget) error {
		plan.addresses = append(plan.addresses, t.Address)
		return nil
	})
	if err != nil {
		return patchPlan{}, err
	}
	plan.patchOutcome = outcome
	return plan, nil
}

// eachCardPatchTarget walks the memories of project once, reading the vault's pages once, and calls visit for every
// page of one of them that is behind its memory: the page that carries the memory's identity, and whose status
// or related differs from what the memory says (patchedFrontmatter of its block is not its block). A page that
// already says it is not visited, so a run and a dry run count only real patches. A memory that its store has not
// retired leaves its page alone unless the page still says superseded or archived, which is a memory restored to
// active: a status a human set by hand on a page its store never retired is not propagation's to police. It
// returns what it could not decide or left alone, and an error only when the walk could not start. It is the ONE
// walk of PropagateCards and cardPatchCandidates, as eachPatchTarget is of Propagate and Plan.
func eachCardPatchTarget(src Source, w *Writer, project string, visit func(patchTarget) error) (patchOutcome, error) {
	memories, err := src.Memories(project)
	if err != nil {
		return patchOutcome{}, &CardListError{Project: project, Err: err}
	}
	var outcome patchOutcome
	if len(memories) == 0 {
		return outcome, nil
	}
	index, err := readPages(w.Pages)
	if err != nil {
		// As in the promotion pass: a pages directory that cannot be listed at all is the failure of every memory
		// that could have been retired; the promotion pass names the eligible ones, and the others have no page to patch.
		for _, m := range memories {
			if isRetired(m) {
				id, _ := numericID(m)
				outcome.failed = append(outcome.failed, SyncFailure{ObservationID: id, Err: err})
			}
		}
		return outcome, nil
	}
	for _, m := range memories {
		id, idErr := numericID(m)
		scan := index.scanFor(m)
		if scan.own == nil {
			// An unreadable page may be a retired memory's own. An active memory with no page is the promotion
			// pass's business (it may be ineligible), never withheld here.
			if isRetired(m) && len(scan.unreadable) > 0 {
				if idErr != nil {
					outcome.failed = append(outcome.failed, SyncFailure{ObservationID: id, Err: idErr})
					continue
				}
				outcome.withheld = append(outcome.withheld, id)
			}
			continue
		}
		if idErr != nil {
			// The memory has a page and no id to name it by in a failure or a patch: it is failed in its own words.
			outcome.failed = append(outcome.failed, SyncFailure{ObservationID: id, Err: idErr})
			continue
		}
		if m.Status == memory.StatusUnknown {
			// The store could not say what became of the memory, so there is no status to patch into its page: a
			// page that says retired is not brought back on a guess, and one that says current is not retired.
			outcome.failed = append(outcome.failed, SyncFailure{ObservationID: id, Err: fmt.Errorf("resolve status: %w", m.StatusCause)})
			continue
		}
		block := scan.own.Block
		if !isRetired(m) && !pageIsRetired(block) {
			continue
		}
		status, related := cardStatus(m), relatedWikilinks(scan.successorLinks(m))
		if patchedFrontmatter(block, status, related) == block {
			continue
		}
		target := patchTarget{ObservationID: id, Address: scan.own.address(), Status: status, Related: related}
		if err := visit(target); err != nil {
			outcome.failed = append(outcome.failed, SyncFailure{ObservationID: id, Err: err})
		}
	}
	return outcome, nil
}

// isRetired reports whether the store of m has superseded or archived it.
func isRetired(m memory.Memory) bool {
	return m.Status == memory.StatusSuperseded || m.Status == memory.StatusArchived
}

// pageIsRetired reports whether the frontmatter block of a page says superseded or archived.
func pageIsRetired(block string) bool {
	status := strings.TrimSpace(parseFrontmatterFields(block)[statusField])
	return status == "superseded" || status == "archived"
}
