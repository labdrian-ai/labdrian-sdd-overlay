package promote

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
)

// Writer is longterm-mem's single promotion entrypoint (6.8 REFACTOR):
// it owns eligibility, address allocation, and dispatching to a fresh
// write (create) or UpdateInPlace (update) so sync (slice 7) and the
// explicit promote command (slice 8b) share one code path instead of
// each re-implementing the create/update decision.
type Writer struct {
	// VaultRoot is the vault checkout Promote allocates addresses in and
	// writes pages under.
	VaultRoot string
	// Store is the precedence store Promote consults and updates
	// (R-030). UpdateInPlace leaves persistence to its caller, and
	// Writer is that caller: it saves the sidecar as part of every
	// promotion that wrote a page, and of none that did not. A create
	// persists the fingerprint before publishing the page, so the only
	// state an interruption can leave is an entry with no page -- which
	// the create branch itself finishes on the next run; an update
	// narrows the gap to the two renames, which UpdateInPlace's
	// content-identity reconciliation then closes on the next run. An
	// interrupted run therefore leaves N consistent pages rather than N
	// pages of lost provenance.
	Store PrecedenceStore
}

// Result reports what Promote did for one observation.
type Result struct {
	Page   Page
	Action Action
}

// Promote renders obs as a vault page and writes it, either as a brand
// new page (ActionCreated) or, when the observation was already
// promoted, through UpdateInPlace (ActionUpdated, or ActionSkippedLocalEdit
// per R-030). explicit is forwarded to Eligible, matching the explicit
// promote surface's override semantics (R-007); an ineligible obs is left
// untouched and reports a zero Result with no error, since ineligibility
// is a normal skip a scanning caller (sync) must not treat as a failure.
// Every promotion that actually wrote a page persists the precedence
// sidecar; a create persists it BEFORE publishing the page, since a
// published page with no recorded provenance is one UpdateInPlace would
// refuse from then on, while a recorded fingerprint with no page is simply
// a create the next run finishes.
//
// Every promotion that actually wrote a page also registers it in the
// vault's master catalog and append-only promotion log (R-029, task
// 7.10), after the precedence sidecar is durable; a skip (ActionSkipped
// LocalEdit) or an ineligible observation registers nothing, since
// neither wrote a page to register. Unlike a failed Store.Save (which
// withdraws the page, because a published page of unprovable provenance
// is one UpdateInPlace refuses forever), a registration failure is NOT
// in that category: the page itself is still valid and its provenance is
// already durable in the precedence sidecar, so only its catalog/log
// entry is missing. Promote surfaces the error without withdrawing the
// page.
//
// That repair is not automatic, and this comment used to claim otherwise.
// `doctor` REPORTS the page as unregistered (its wiki-registration-
// consistency check) and writes nothing, by design. Sync does not reach it
// either: an observation already promoted at its current revision is
// skipped before Writer.Promote is ever called (R-009's true-no-op gate),
// and a page whose registration failed is exactly that observation. The
// one path that does repair it is an explicit promote of that observation
// (ExplicitPromote), which re-enters here, takes the update branch, and
// registers on every write it does not skip. So: doctor names it, and an
// explicit promote fixes it.
func (w *Writer) Promote(obs engram.Observation, explicit bool) (Result, error) {
	if !Eligible(obs, explicit) {
		return Result{}, nil
	}

	// Collected before anything is written, but the timing is not what
	// makes it correct: findMovedPages filters on project != obs.Project,
	// so the page this run is about to create or update can never appear
	// in it, whenever the scan runs.
	moved, err := findMovedPages(w.VaultRoot, obs.Project, int(obs.ID))
	if err != nil {
		return Result{}, err
	}

	address, err := Allocate(w.VaultRoot, obs.Project, int(obs.ID))
	if err != nil {
		return Result{}, err
	}

	page, err := EmitPage(obs, address, nil)
	if err != nil {
		return Result{}, err
	}

	existingPath := filepath.Join(w.VaultRoot, page.Path)
	switch _, err := os.Stat(existingPath); {
	case err == nil:
		action, err := UpdateInPlace(w.Store, page, existingPath)
		if err != nil {
			return Result{}, err
		}
		// A skip wrote nothing, so there is no new fingerprint to pair and
		// nothing to register.
		if action.Kind != ActionSkippedLocalEdit {
			if err := w.Store.Save(w.VaultRoot); err != nil {
				return Result{}, err
			}
			if err := w.register(page.Address, obs.Title); err != nil {
				return Result{}, err
			}
		}
		// The successor already exists on disk (this branch was chosen by
		// its presence), so pointing at it is safe even when the update
		// itself was skipped: superseding the pages the observation left
		// behind is independent of whether its current page needed new
		// content.
		if err := w.supersedeMoved(moved, page.Address, obs.Title); err != nil {
			return Result{}, err
		}
		return Result{Page: page, Action: action}, nil
	case !os.IsNotExist(err):
		return Result{}, fmt.Errorf("promote: stat %s: %w", existingPath, err)
	}

	// Fingerprint first, page second. The two writes cannot be made atomic
	// with respect to each other, so the only real choice is which of the
	// two orphan states a killed process may leave behind -- and the two
	// are not equally recoverable:
	//
	//   page without entry (the old order) is unrecoverable. Allocate
	//   reuses the page's own address, os.Stat finds it, and UpdateInPlace
	//   refuses it as unknown provenance -- which, being a skip, also
	//   suppresses the Save and the registration that would have repaired
	//   it. Every later run repeats that exact skip: a fixed point.
	//
	//   entry without page (this order) is self-healing. os.Stat finds no
	//   page, so the next run takes this same create branch and finishes
	//   the job, catalog and log included.
	//
	// The Save-failure rollback below only covers a Save that RETURNS an
	// error; a killed process returns nothing for any compensating removal
	// to react to, which is exactly why the ordering has to carry the
	// guarantee rather than the cleanup.
	w.Store.Set(address, entryFor(page))
	if err := w.Store.Save(w.VaultRoot); err != nil {
		// Nothing has been published yet, so there is no page to withdraw:
		// drop the in-memory entry and report the failure.
		delete(w.Store, address)
		return Result{}, fmt.Errorf("promote: persist precedence for %s: %w (no page published)", address, err)
	}
	if err := writeFileAtomic(existingPath, []byte(page.Frontmatter+page.Body)); err != nil {
		// The fingerprint is durable but no page carries it. A retry would
		// converge regardless (the create branch is chosen by the page's
		// own absence), but an entry claiming provenance over a file that
		// does not exist is still a lie the sidecar should not tell.
		//
		// Allocate's own two writes are deliberately NOT withdrawn with
		// it. The address number allocate-address.sh advanced belongs to a
		// vault script this package can only call forward, so it is burned
		// whatever happens here; and the .raw/.manifest.json address_map
		// row Allocate wrote is left alone because a row without a page is
		// inert -- doctor's address-map rule walks PAGES looking for their
		// rows, never rows looking for their pages -- while rewriting that
		// wiki-ingest-owned file to delete it is a real write that can
		// itself fail. The residue of a failed create is therefore one
		// skipped address number and one dangling manifest row pointing at
		// a path no later run reuses (the retry allocates a fresh
		// address), and neither wedges anything.
		delete(w.Store, address)
		if saveErr := w.Store.Save(w.VaultRoot); saveErr != nil {
			return Result{}, fmt.Errorf("promote: write page %s: %w (and withdrawing its precedence entry failed: %v)", existingPath, err, saveErr)
		}
		return Result{}, err
	}
	if err := w.register(address, obs.Title); err != nil {
		return Result{}, err
	}
	if err := w.supersedeMoved(moved, address, obs.Title); err != nil {
		return Result{}, err
	}
	return Result{Page: page, Action: Action{Kind: ActionCreated}}, nil
}

// supersedeMoved marks every page the observation left behind in another
// project as superseded, with related pointing at the page it now lives on
// (successorAddress). It reuses R-033's idiom exactly -- status
// "superseded" plus a related wikilink to the successor, patched through
// PatchStatusFields, with the precedence sidecar updated afterward -- so
// there is one supersession mechanism in this package, not two.
//
// ORDERING. This runs only AFTER the successor page is on disk, and the
// choice is the lesser of two evils rather than a self-healing one. A
// superseded page pointing at a page that does not exist is a dangling
// wikilink the vault's own lint rule flags and no later run repairs,
// because the pointer already looks done. An interruption in the other
// order leaves the successor page with the orphan still live beside it,
// which is the same shape as a failed registration twelve lines up, and it
// heals the same way -- NOT automatically. Sync gates before Writer.Promote
// is ever called (R-009's true-no-op gate), and after an interrupted move
// the successor already exists at the observation's current revision, so
// sync skips it and never reaches here. Until the observation's revision
// advances, the one path that repairs it is an explicit promote of that
// observation, which re-enters Promote, takes the update branch, and
// supersedes the orphan from there. Two live pages that a later run can
// still reconcile beat a dangling pointer no run repairs at all.
//
// AN ALREADY-SUPERSEDED PAGE IS NEVER TOUCHED. The rule's load-bearing
// payoff is that it refuses to clobber a successor pointer somebody else
// set -- R-033's Engram-side propagation may already have superseded this
// page toward a different successor, which is real history; silently
// overwriting another successor chain would be its own defect, and the
// moved observation stays reachable by engram_id regardless. It is NOT
// what makes a repeat promotion a no-op: PatchStatusFields already writes
// nothing when the patched block is byte-identical (frontmatter.go's
// `if newBlock != fmBlock`), so a second run would leave the page,
// its mtime and its precedence entry alone even without this check.
//
// A FAILING PAGE DOES NOT DISCARD THE ONES ALREADY PATCHED. A double move
// produces more than one orphan, so this follows Propagate's shape: record
// the failure, carry on, and persist whatever was patched before
// reporting. Returning early would leave an orphan superseded on disk with
// a stale sidecar entry, and because a superseded page is skipped from
// then on, nothing would ever re-record its hash: it would read as locally
// edited forever, the exact permanent wedge R-030's reconciliation exists
// to end.
//
// LOCAL-EDIT PRECEDENCE (R-030). The patch is unconditional, exactly as
// Propagate's is, and for the same reason: it rewrites two frontmatter
// lines and never re-bodies the page, so it destroys no human edit -- a
// human's prose, their added frontmatter keys, everything else survives
// byte for byte. Skipping a locally edited orphan would instead leave a
// second live page for one engram_id with nothing marking it stale, which
// is the very duplicate R-008 forbids. Only the frontmatter hash is
// re-recorded, never the body hash: stamping the on-disk body as our own
// last write would erase the divergence R-030 depends on and let the next
// sync overwrite that edit in silence.
func (w *Writer) supersedeMoved(moved []promotedPage, successorAddress, successorTitle string) error {
	patched := false
	var failures []error
	for _, old := range moved {
		path := filepath.Join(w.VaultRoot, pagePathPrefix, old.Address+".md")
		raw, err := os.ReadFile(path)
		if err != nil {
			failures = append(failures, fmt.Errorf("promote: read superseded page %s: %w", path, err))
			continue
		}
		block, ok := frontmatterBlock(string(raw))
		if !ok {
			failures = append(failures, fmt.Errorf("promote: %s has no parseable frontmatter block", path))
			continue
		}
		if parseFrontmatterFields(block)[statusField] == "superseded" {
			continue
		}

		frontmatterHash, _, err := PatchStatusFields(path, "superseded", []string{wikilink(successorAddress, successorTitle)})
		if err != nil {
			failures = append(failures, fmt.Errorf("promote: supersede %s: %w", path, err))
			continue
		}
		entry, _ := w.Store.Get(old.Address)
		entry.FrontmatterHash = frontmatterHash
		w.Store.Set(old.Address, entry)
		patched = true
	}
	if patched {
		if err := w.Store.Save(w.VaultRoot); err != nil {
			failures = append(failures, fmt.Errorf("promote: persist precedence after superseding a moved page: %w", err))
		}
	}
	return errors.Join(failures...)
}

// register records addr/title's promotion in the vault's master catalog
// and append-only promotion log (R-029, task 7.10): the two writes
// register.go's RegisterIndex/RegisterLog perform, using nowFunc() for
// RegisterLog's timestamp so tests can pin it with fixedNow, matching
// EmitPage's own convention.
func (w *Writer) register(addr, title string) error {
	if err := RegisterIndex(filepath.Join(w.VaultRoot, indexMdRelPath), addr, title); err != nil {
		return err
	}
	return RegisterLog(filepath.Join(w.VaultRoot, logMdRelPath), addr, title, nowFunc())
}
