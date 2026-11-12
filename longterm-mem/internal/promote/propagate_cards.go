package promote

import (
	"context"
	"fmt"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// PropagateCards is the propagation pass of `sync --source memorycard` (S1 e of the C22-FU follow-up, the memorycard
// counterpart of Propagate and R-033): every memory that memorycard has superseded or archived since its page was
// written gets that page's status and related lines patched, and nothing else of the page. The body, and every
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
// returns a non-nil error when anything failed. A memorycard that cannot list its retired memories is a
// CardListError.
func PropagateCards(ctx context.Context, cards CardMemories, w *Writer, project string) (PropagateReport, error) {
	if err := w.checkPatchPorts(); err != nil {
		return PropagateReport{}, fmt.Errorf("promote: propagate memorycard: %w", err)
	}
	var report PropagateReport
	outcome, err := eachCardPatchTarget(cards, w, project, func(t patchTarget) error {
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

	if err := w.savePatched(len(report.Patched)); err != nil {
		return report, err
	}
	return report, cardFailureError(report.Failed)
}

// cardPatchCandidates reports the pages PropagateCards would rewrite, through the same eachCardPatchTarget walk
// PropagateCards itself runs, and writes nothing: the dry run's prediction is the run's own walk.
func cardPatchCandidates(cards CardMemories, w *Writer, project string) (patchPlan, error) {
	var plan patchPlan
	outcome, err := eachCardPatchTarget(cards, w, project, func(t patchTarget) error {
		plan.addresses = append(plan.addresses, t.Address)
		return nil
	})
	if err != nil {
		return patchPlan{}, err
	}
	plan.patchOutcome = outcome
	return plan, nil
}

// eachCardPatchTarget walks the retired memories of project once, reading the vault's pages once, and calls visit
// for every page of one of them: the page that carries the memory's memorycard_id. It returns what it could not
// decide or left alone, and an error only when the walk could not start. It is the ONE walk of PropagateCards and
// cardPatchCandidates, as eachPatchTarget is of Propagate and Plan.
func eachCardPatchTarget(cards CardMemories, w *Writer, project string, visit func(patchTarget) error) (patchOutcome, error) {
	memories, err := cards.ListRetiredMemories(project)
	if err != nil {
		return patchOutcome{}, &CardListError{Project: project, Err: err}
	}
	var outcome patchOutcome
	if len(memories) == 0 {
		return outcome, nil
	}
	pages, err := readCardPages(w.Pages)
	if err != nil {
		// As in the promotion pass: a pages directory that cannot be listed at all is the failure of every memory.
		for _, m := range memories {
			outcome.failed = append(outcome.failed, SyncFailure{ObservationID: m.ID, Err: err})
		}
		return outcome, nil
	}
	for _, m := range memories {
		scan := pages.scanFor(m)
		if scan.own == nil {
			if len(scan.unreadable) > 0 {
				outcome.withheld = append(outcome.withheld, m.ID)
			}
			continue
		}
		target := patchTarget{
			ObservationID: m.ID,
			Address:       scan.own.address(),
			Status:        m.Status,
			Related:       relatedWikilinks(cardRelated(m, scan.byCardID)),
		}
		if err := visit(target); err != nil {
			outcome.failed = append(outcome.failed, SyncFailure{ObservationID: m.ID, Err: err})
		}
	}
	return outcome, nil
}

// cardRelated is the related list of m's page: the successor's page when m has a successor that has one, named in
// the form EmitCardPage renders, and nothing otherwise. byCardID maps the memorycard ids of m's project to the
// addresses of their pages.
func cardRelated(m memory.CardMemory, byCardID map[int64]string) []Link {
	if m.SupersededBy > 0 {
		if successor, ok := byCardID[m.SupersededBy]; ok {
			return []Link{{Address: successor, Title: m.SupersededByTitle}}
		}
	}
	return nil
}
