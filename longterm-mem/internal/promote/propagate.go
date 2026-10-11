package promote

import (
	"context"
	"errors"
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
		// The page is named here for the sake of this pass ("patch <page>: ..."), and again inside the patch for
		// the sake of its own read ("promote: read <page>: ..."): the repeated path is the message the
		// commands have always printed, pinned by TestPropagatePatchesThePagesThroughThePort, and both
		// come from the one PageLocation of the port, so the two cannot name different pages.
		pagePath := deps.Writer.Pages.PageLocation(t.Address)
		frontmatterHash, _, err := PatchStatusFields(deps.Writer.Pages, t.Address, t.Status, t.Related)
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
		entry, _ := deps.Writer.Store.Get(t.Address)
		entry.FrontmatterHash = frontmatterHash
		deps.Writer.Store.Set(t.Address, entry)
		report.Patched = append(report.Patched, t.Address)
		return nil
	})
	if err != nil {
		return PropagateReport{}, err
	}
	report.Failed = outcome.failed
	report.Withheld = outcome.withheld

	if len(report.Patched) > 0 {
		if err := deps.Writer.Precedence.SavePrecedence(deps.Writer.Store); err != nil {
			return report, fmt.Errorf("promote: propagate: persist precedence: %w", err)
		}
	}
	return report, failureError("propagate", report.Failed)
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
// propagation pass would rewrite. It returns the observations it could not
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
	observations, err := deps.Memory.ObservationsIncludingDeleted(project)
	if err != nil {
		return patchOutcome{}, fmt.Errorf("promote: propagate: list observations for %q: %w", project, err)
	}

	bySyncID := make(map[string]memory.Observation, len(observations))
	for _, obs := range observations {
		if obs.SyncID != "" {
			bySyncID[obs.SyncID] = obs
		}
	}

	var outcome patchOutcome
	for _, obs := range observations {
		promoted, ok, err := findPromotedPage(deps.Writer.Pages, project, int(obs.ID))
		var unknown *unreadablePagesError
		if errors.As(err, &unknown) {
			outcome.withheld = append(outcome.withheld, obs.ID)
			continue
		}
		if err != nil {
			outcome.failed = append(outcome.failed, SyncFailure{ObservationID: obs.ID, Err: fmt.Errorf("check promoted state: %w", err)})
			continue
		}
		if !ok {
			continue
		}

		status, related, err := decidePatch(deps, project, obs, bySyncID)
		if err != nil {
			outcome.failed = append(outcome.failed, SyncFailure{ObservationID: obs.ID, Err: err})
			continue
		}
		if status == "" {
			continue
		}

		if err := visit(patchTarget{ObservationID: obs.ID, Address: promoted.Address, Status: status, Related: related}); err != nil {
			outcome.failed = append(outcome.failed, SyncFailure{ObservationID: obs.ID, Err: err})
		}
	}
	return outcome, nil
}

// resolveStatus determines obs's new status and related wikilinks, or
// ("", nil, nil) when obs is untouched (R-033 scenario 3: neither
// soft-deleted nor the older side of a supersedes edge). A supersedes
// edge only produces a patch for the OLDER side (D11): the newer
// observation is the survivor and is left alone -- its own turn through
// this loop finds no edge naming it as the older side, so it is never
// patched.
func resolveStatus(deps Deps, project string, obs memory.Observation, bySyncID map[string]memory.Observation) (status string, related []string, err error) {
	edges, err := deps.Memory.RelatedEdges(obs.ID)
	if err != nil {
		return "", nil, err
	}
	for _, edge := range edges {
		if edge.Relation != "supersedes" {
			continue
		}
		otherSyncID := edge.SourceSyncID
		if obs.SyncID != "" && otherSyncID == obs.SyncID {
			otherSyncID = edge.TargetSyncID
		}
		other, ok := bySyncID[otherSyncID]
		if !ok || other.CreatedAt <= obs.CreatedAt {
			// obs is the newer side (or the other side is unrecognized):
			// nothing to patch for obs on this edge.
			continue
		}
		successor, ok, err := findPromotedPage(deps.Writer.Pages, project, int(other.ID))
		var unknown *unreadablePagesError
		if err != nil && !errors.As(err, &unknown) {
			return "", nil, err
		}
		if !ok {
			// The successor has no page yet. It used to be left untouched
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
			return "superseded", nil, nil
		}
		return "superseded", []string{wikilink(successor.Address, other.Title)}, nil
	}

	if obs.DeletedAt != "" {
		return "archived", nil, nil
	}
	return "", nil, nil
}

// decidePatch answers the one question Propagate asks of each observation
// that already has a page: does this run rewrite it, and to what?
//
// It is extracted for the same reason decidePromotion is. `sync` runs two
// passes and reports two counts, so a preview that predicts only the first
// tells an operator "nothing will be rewritten" immediately before a
// re-sync rewrites existing pages. Plan calls this; so does Propagate; and
// neither holds a second opinion about what the other will do.
func decidePatch(deps Deps, project string, obs memory.Observation, bySyncID map[string]memory.Observation) (string, []string, error) {
	status, related, err := resolveStatus(deps, project, obs, bySyncID)
	if err != nil {
		return "", nil, fmt.Errorf("resolve status: %w", err)
	}
	return status, related, nil
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
