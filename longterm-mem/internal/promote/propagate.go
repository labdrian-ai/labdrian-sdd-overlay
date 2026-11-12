package promote

import (
	"context"
	"fmt"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// PropagateReport summarizes one Propagate run: Patched lists the address
// of every promoted page whose status/related frontmatter was rewritten,
// and Failed names each observation the run could not process.
type PropagateReport struct {
	Patched []string
	Failed  []SyncFailure
	// Withheld lists the observations the pass left alone because no readable page carries them and a page
	// could not be read: the page of one of them may be the unreadable one, and a status cannot be patched
	// into a page that cannot be found. Not a failure; a later run decides them.
	Withheld []int64
}

// Propagate patches every already-promoted page whose observation was
// soft-deleted or superseded in Engram since it was last promoted (R-033):
// status becomes superseded (with related pointing at the successor's
// page, D11's created_at-newer-wins rule, independent of the edge's
// stated direction) or archived (soft-deleted with no accepted supersedes
// edge to a successor). Only the status: and related: frontmatter lines
// change -- the body, and every other frontmatter field, are left exactly
// as they were, even on a page a human has since edited by hand. The
// patch is unconditional: unlike Writer.Promote/UpdateInPlace, Propagate
// never fails closed on unknown or diverged precedence, because Engram's
// canonical soft-delete/supersession state must always land (D11 "canon
// wins" for a status-only patch). The patch is recorded in the precedence
// sidecar afterward, but only its frontmatter_hash: that block is what
// longterm-mem just wrote, so a later sync will not misread this patch as
// a human edit. body_hash is deliberately left exactly as it was --
// rewriting it to whatever body is on disk would stamp a human's edit as
// longterm-mem's own last write and erase the divergence R-030 depends
// on, letting the next sync overwrite that edit in silence.
func Propagate(ctx context.Context, deps Deps, project string) (PropagateReport, error) {
	var report PropagateReport
	if err := deps.Writer.checkPatchPorts(); err != nil {
		return PropagateReport{}, err
	}
	outcome, err := eachPatchTarget(deps, project, func(t patchTarget) error {
		if err := deps.Writer.patchPage(t); err != nil {
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

	if err := deps.Writer.savePatched("propagate", report.Patched); err != nil {
		return report, err
	}
	return report, failureError("propagate", report.Failed)
}

// patchPage rewrites the status and related lines of the page t names and records the patch in the precedence
// store: the one patcher of the propagation passes, the Engram pass (Propagate) and the memorycard pass
// (PropagateCards) alike. The caller persists the store once for its run (savePatched).
func (w *Writer) patchPage(t patchTarget) error {
	// The page is named here for the sake of this pass ("patch <page>: ..."), and again inside the patch for
	// the sake of its own read ("promote: read <page>: ..."): the repeated path is the message the
	// commands have always printed, pinned by TestPropagatePatchesThePagesThroughThePort, and both
	// come from the one PageLocation of the port, so the two cannot name different pages.
	pagePath := w.Pages.PageLocation(t.Address)
	frontmatterHash, _, err := PatchStatusFields(w.Pages, t.Address, t.Status, t.Related)
	if err != nil {
		return fmt.Errorf("patch %s: %w", pagePath, err)
	}
	// Only the frontmatter hash moves: longterm-mem wrote that block,
	// and nothing else. Recording the on-disk body as our own last
	// write would stamp a human's edit as ours and erase the
	// divergence R-030 depends on, so a later Sync would overwrite
	// that edit in silence. An absent entry keeps its zero BodyHash,
	// which UpdateInPlace reads as diverged and refuses -- the safe
	// side for a page whose body nobody can prove we wrote.
	entry, _ := w.Store.Get(t.Address)
	entry.FrontmatterHash = frontmatterHash
	w.Store.Set(t.Address, entry)
	return nil
}

// savePatched persists the precedence store once for a run of pass that patched pages, and not at all for one that
// patched none. pass names the run in the error: "propagate" for Engram pages, "propagate memorycard" for the
// pages of memorycard.
func (w *Writer) savePatched(pass string, patched []string) error {
	if len(patched) == 0 {
		return nil
	}
	if err := w.Precedence.SavePrecedence(w.Store); err != nil {
		return fmt.Errorf("promote: %s: persist precedence: %w", pass, err)
	}
	return nil
}

// patchTarget is one already-promoted page the propagation pass would
// rewrite, with the status it would be rewritten to.
type patchTarget struct {
	ObservationID int64
	Address       string
	Status        string
	Related       []string
}

// patchOutcome is what a walk of the propagation pass found beyond the pages it visited: the observations it
// could not decide, and those it left alone because no readable page carries them while a page could not be
// read (the page of one of them may be that one, and a status cannot be patched into a page that cannot be
// found).
type patchOutcome struct {
	failed   []SyncFailure
	withheld []int64
}

// eachPatchTarget walks project once and calls visit for every page the
// propagation pass would rewrite. It returns the memories it could not
// decide or left alone, and an error only when the walk itself could not
// start.
//
// This is the ONE walk. Propagate visits to patch; Plan visits to count.
// The first attempt at the preview gave it its own copy of this loop and
// shared only the decision inside it -- which is the same defect the
// preview exists to prevent, moved one level out: the two copies agreed on
// the day they were written and nothing structural kept them agreeing. A
// filter added here is now added for both, because there is only one here.
func eachPatchTarget(deps Deps, project string, visit func(patchTarget) error) (patchOutcome, error) {
	memories, err := engramView{deps.Source}.Memories(project)
	if err != nil {
		return patchOutcome{}, fmt.Errorf("promote: propagate: list observations for %q: %w", project, err)
	}

	var outcome patchOutcome
	index, indexErr := readPages(deps.Writer.Pages)
	for _, m := range memories {
		id, _ := numericID(m)
		style, err := styleOf(m.Identity.Source)
		if err != nil {
			outcome.failed = append(outcome.failed, SyncFailure{ObservationID: id, Err: err})
			continue
		}
		if indexErr != nil {
			outcome.failed = append(outcome.failed, SyncFailure{ObservationID: id, Err: fmt.Errorf("check promoted state: %w", indexErr)})
			continue
		}
		scan := index.scanFor(m)
		if scan.own == nil {
			// A page that could not be read may be this memory's own, and a status cannot be patched into a page
			// that cannot be found.
			if len(scan.unreadable) > 0 {
				outcome.withheld = append(outcome.withheld, id)
			}
			continue
		}
		promoted, err := scan.own.resolve(style.requireAddress)
		if err != nil {
			outcome.failed = append(outcome.failed, SyncFailure{ObservationID: id, Err: fmt.Errorf("check promoted state: %w", err)})
			continue
		}

		status, related, err := decidePatch(index, style, m)
		if err != nil {
			outcome.failed = append(outcome.failed, SyncFailure{ObservationID: id, Err: err})
			continue
		}
		if status == "" {
			continue
		}

		if err := visit(patchTarget{ObservationID: id, Address: promoted.Address, Status: status, Related: related}); err != nil {
			outcome.failed = append(outcome.failed, SyncFailure{ObservationID: id, Err: err})
		}
	}
	return outcome, nil
}

// decidePatch answers the one question Propagate asks of each memory that
// already has a page: does this run rewrite it, and to what? The status is the
// memory's own (the adapter of its store maps what the store keeps to it, R-033
// scenario 3): an active memory is untouched, a superseded one is
// superseded, with related pointing at the successor's page (D11's
// created_at-newer-wins rule is the adapter's, independent of the edge's
// stated direction), and an archived one is archived. It returns ("", nil, nil)
// when the memory is untouched.
//
// It is extracted for the same reason the promotion decision is. `sync` runs two
// passes and reports two counts, so a preview that predicts only the first
// tells an operator "nothing will be rewritten" immediately before a
// re-sync rewrites existing pages. Plan calls this; so does Propagate; and
// neither holds a second opinion about what the other will do.
func decidePatch(index *pageIndex, style pageStyle, m memory.Memory) (string, []string, error) {
	switch m.Status {
	case memory.StatusUnknown:
		// The store could not say what became of the memory, so there is no status to patch into its page.
		return "", nil, fmt.Errorf("resolve status: %w", m.StatusCause)
	case memory.StatusSuperseded:
		// The successor has no page yet: it used to be left untouched
		// here, on the reasoning that there was nothing to link to --
		// which confuses being unable to DECORATE the fact with being
		// unable to RECORD it. The link is a convenience; the status
		// is the warning, and it is the whole point. Dropped, the page
		// goes on reading as current, and a memory that reads as
		// current is one an agent acts on and reintroduces: exactly
		// the failure this machinery exists to prevent. The ledger's
		// truth does not depend on promotion order, so the status
		// lands with related left empty, and a later run adds the link
		// once the successor has a page of its own.
		successor := index.ownPage(m.Project, m.Successor)
		if successor == nil {
			return "superseded", nil, nil
		}
		page, err := successor.resolve(style.requireAddress)
		if err != nil {
			return "", nil, fmt.Errorf("resolve status: %w", err)
		}
		return "superseded", []string{wikilink(page.Address, m.SuccessorTitle)}, nil
	case memory.StatusArchived:
		return "archived", nil, nil
	}
	return "", nil, nil
}

// patchCandidates reports the addresses the propagation pass would
// rewrite, using the same eachPatchTarget walk Propagate itself runs.
func patchCandidates(deps Deps, project string) (patchPlan, error) {
	var plan patchPlan
	outcome, err := eachPatchTarget(deps, project, func(t patchTarget) error {
		plan.addresses = append(plan.addresses, t.Address)
		return nil
	})
	if err != nil {
		return patchPlan{}, err
	}
	plan.patchOutcome = outcome
	return plan, nil
}

// patchPlan is what the propagation pass would do: the pages it would rewrite, and what the walk found out
// besides.
type patchPlan struct {
	patchOutcome
	addresses []string
}
