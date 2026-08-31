package promote

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
)

// PropagateReport summarizes one Propagate run: Patched lists the address
// of every promoted page whose status/related frontmatter was rewritten,
// and Failed names each observation the run could not process.
type PropagateReport struct {
	Patched []string
	Failed  []SyncFailure
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
	observations, err := deps.Engram.ObservationsIncludingDeleted(project)
	if err != nil {
		return PropagateReport{}, fmt.Errorf("promote: propagate: list observations for %q: %w", project, err)
	}

	bySyncID := make(map[string]engram.Observation, len(observations))
	for _, obs := range observations {
		if obs.SyncID != "" {
			bySyncID[obs.SyncID] = obs
		}
	}

	var report PropagateReport
	for _, obs := range observations {
		promoted, ok, err := findPromotedPage(deps.Writer.VaultRoot, project, int(obs.ID))
		if err != nil {
			report.Failed = append(report.Failed, SyncFailure{ObservationID: obs.ID, Err: fmt.Errorf("check promoted state: %w", err)})
			continue
		}
		if !ok {
			continue
		}

		status, related, err := resolveStatus(deps, project, obs, bySyncID)
		if err != nil {
			report.Failed = append(report.Failed, SyncFailure{ObservationID: obs.ID, Err: fmt.Errorf("resolve status: %w", err)})
			continue
		}
		if status == "" {
			continue
		}

		pagePath := filepath.Join(deps.Writer.VaultRoot, pagePathPrefix, promoted.Address+".md")
		frontmatterHash, _, err := PatchStatusFields(pagePath, status, related)
		if err != nil {
			report.Failed = append(report.Failed, SyncFailure{ObservationID: obs.ID, Err: fmt.Errorf("patch %s: %w", pagePath, err)})
			continue
		}
		// Only the frontmatter hash moves: longterm-mem wrote that block,
		// and nothing else. Recording the on-disk body as our own last
		// write would stamp a human's edit as ours and erase the
		// divergence R-030 depends on, so a later Sync would overwrite
		// that edit in silence. An absent entry keeps its zero BodyHash,
		// which UpdateInPlace reads as diverged and refuses -- the safe
		// side for a page whose body nobody can prove we wrote.
		entry, _ := deps.Writer.Store.Get(promoted.Address)
		entry.FrontmatterHash = frontmatterHash
		deps.Writer.Store.Set(promoted.Address, entry)
		report.Patched = append(report.Patched, promoted.Address)
	}

	if len(report.Patched) > 0 {
		if err := deps.Writer.Store.Save(deps.Writer.VaultRoot); err != nil {
			return report, fmt.Errorf("promote: propagate: persist precedence: %w", err)
		}
	}
	return report, failureError("propagate", report.Failed)
}

// resolveStatus determines obs's new status and related wikilinks, or
// ("", nil, nil) when obs is untouched (R-033 scenario 3: neither
// soft-deleted nor the older side of a supersedes edge). A supersedes
// edge only produces a patch for the OLDER side (D11): the newer
// observation is the survivor and is left alone -- its own turn through
// this loop finds no edge naming it as the older side, so it is never
// patched.
func resolveStatus(deps Deps, project string, obs engram.Observation, bySyncID map[string]engram.Observation) (status string, related []string, err error) {
	edges, err := deps.Engram.RelatedEdges(obs.ID)
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
		successor, ok, err := findPromotedPage(deps.Writer.VaultRoot, project, int(other.ID))
		if err != nil {
			return "", nil, err
		}
		if !ok {
			// The successor is not itself promoted yet -- there is no
			// page to link to, so obs is left untouched for now.
			continue
		}
		return "superseded", []string{wikilink(successor.Address, other.Title)}, nil
	}

	if obs.DeletedAt != "" {
		return "archived", nil, nil
	}
	return "", nil, nil
}
