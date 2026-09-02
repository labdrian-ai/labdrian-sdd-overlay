package promote

import (
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
	// whose fingerprint cannot be persisted withdraws its page, so that
	// branch is all-or-nothing; an update narrows the gap to the two
	// renames, which UpdateInPlace's content-identity reconciliation
	// then closes on the next run. An interrupted run therefore leaves
	// N consistent pages rather than N pages of lost provenance.
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
// sidecar before returning; a create that cannot persist withdraws its
// page and reports the failure, since a published page with no recorded
// provenance is one UpdateInPlace would refuse from then on.
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
// page; a later sync or `doctor` run can repair the missing entry.
func (w *Writer) Promote(obs engram.Observation, explicit bool) (Result, error) {
	if !Eligible(obs, explicit) {
		return Result{}, nil
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
		return Result{Page: page, Action: action}, nil
	case !os.IsNotExist(err):
		return Result{}, fmt.Errorf("promote: stat %s: %w", existingPath, err)
	}

	if err := writeFileAtomic(existingPath, []byte(page.Frontmatter+page.Body)); err != nil {
		return Result{}, err
	}
	w.Store.Set(address, PrecedenceEntry{
		BodyHash:        hashText(page.Body),
		FrontmatterHash: hashText(page.Frontmatter),
	})
	if err := w.Store.Save(w.VaultRoot); err != nil {
		// The page is published but nothing records that longterm-mem
		// wrote it, and UpdateInPlace refuses pages of unknown
		// provenance -- so leaving it would strand this observation
		// until an operator repaired the sidecar by hand. Withdraw the
		// page instead: nothing was promoted, and a retry starts clean.
		delete(w.Store, address)
		if rmErr := os.Remove(existingPath); rmErr != nil {
			return Result{}, fmt.Errorf("promote: persist precedence for %s: %w (and rolling back %s failed: %v)", address, err, existingPath, rmErr)
		}
		return Result{}, fmt.Errorf("promote: persist precedence for %s: %w (page withdrawn)", address, err)
	}
	if err := w.register(address, obs.Title); err != nil {
		return Result{}, err
	}
	return Result{Page: page, Action: Action{Kind: ActionCreated}}, nil
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
