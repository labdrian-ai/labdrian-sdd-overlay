package promote

import (
	"fmt"
)

// PropagateReport summarizes one PropagateCards run: Patched lists the address
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

// patchPage rewrites the status and related lines of the page t names and records the patch in the precedence
// store: the one patcher of the propagation pass (PropagateCards). The caller persists the store once for its run (savePatched).
func (w *Writer) patchPage(t patchTarget) error {
	// The page is named here for the sake of this pass ("patch <page>: ..."), and again inside the patch for
	// the sake of its own read ("promote: read <page>: ..."): the repeated path is the message the
	// commands have always printed, pinned by TestPropagateCardsPatchesThePagesThroughThePort, and both
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

// patchPlan is what the propagation pass would do: the pages it would rewrite, and what the walk found out
// besides.
type patchPlan struct {
	patchOutcome
	addresses []string
}
