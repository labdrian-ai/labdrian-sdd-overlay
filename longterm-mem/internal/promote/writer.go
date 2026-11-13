package promote

import (
	"context"
	"errors"
	"fmt"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vaultlayout"
)

// Writer is longterm-mem's single promotion entrypoint (6.8 REFACTOR):
// it owns eligibility, address allocation, and dispatching to a fresh
// write (create) or UpdateInPlace (update) so sync (slice 7) and the
// explicit promote command (slice 8b) share one code path instead of
// each re-implementing the create/update decision.
type Writer struct {
	// Pages is where the pages of the vault are read and written: the
	// scan for an observation's page, the check for the page about to be
	// created, the replacement of a page, and the patches that supersede
	// the pages an observation left behind. A Writer without one is refused
	// by Promote and SyncCards before they write anything, and by PropagateCards
	// before it patches anything.
	Pages PageRepository
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
	// Precedence persists Store: the Writer saves through it after every
	// promotion that wrote a page. Store and Precedence are one pairing,
	// the store a repository holds and the repository it is saved
	// through, and UsePrecedence sets both from one repository so the
	// composition root cannot wire a store loaded from one vault to the
	// repository of another. A Writer without a repository is refused by
	// Promote, SyncCards and PropagateCards before they write anything.
	Precedence PrecedenceRepository
	// Addresses hands out the address of each new page. It is asked only
	// when a promotion needs a fresh address: re-promoting a page that
	// already exists reuses its own and never reaches it. A promotion that
	// needs one with none wired is refused before anything is written.
	Addresses AddressAllocator
	// AddressMap is where each new page is entered in the vault's address map
	// once it has an address. Like Addresses it is asked only when a promotion
	// needs a fresh address, and a promotion that needs it with none wired is
	// refused before the allocator is asked, so no address is spent on it.
	AddressMap AddressMapRecorder
	// Index is the vault's master catalog and Log its append-only promotion
	// log: every promotion that wrote a page registers it in both, after
	// the precedence sidecar is durable. A Writer without either is refused
	// by Promote and SyncCards before they write anything.
	Index IndexRepository
	Log   LogRepository
	// SyncState records when a sync completed. Only SyncCards asks for it, and a
	// SyncCards that finds none is refused before it does anything.
	SyncState SyncStateRecorder
	// Clock dates the pages, the promotion log and the sync-state record.
	// The composition root wires a real one; a Writer without one is
	// refused by Promote and SyncCards before they touch the vault. PropagateCards
	// patches pages in place and never reads the time, so it asks for none.
	Clock Clock
}

// UsePrecedence wires the Writer to repo: it loads the precedence store from
// repo and keeps repo as the repository the store is saved through, so the
// two handles on the store are paired by construction. A repository that is
// missing, or cannot be loaded, leaves the Writer as it was; the load
// failure reaches the caller as it came.
func (w *Writer) UsePrecedence(repo PrecedenceRepository) error {
	if memory.IsMissing(repo) {
		return errNoPrecedenceRepository
	}
	store, err := repo.LoadPrecedence()
	if err != nil {
		return err
	}
	w.Store = store
	w.Precedence = repo
	return nil
}

// checkPorts reports the first port the Writer needs and was not handed: the
// clock, then the ports a patch needs, then the catalog and the log a promotion
// registers its page in.
func (w *Writer) checkPorts() error {
	if memory.IsMissing(w.Clock) {
		return errNoClock
	}
	if err := w.checkPatchPorts(); err != nil {
		return err
	}
	if memory.IsMissing(w.Index) {
		return errNoIndexRepository
	}
	if memory.IsMissing(w.Log) {
		return errNoLogRepository
	}
	return nil
}

// checkSyncState reports whether the Writer was handed the recorder of the
// sync state, which only SyncCards asks for.
func (w *Writer) checkSyncState() error {
	if memory.IsMissing(w.SyncState) {
		return errNoSyncStateRecorder
	}
	return nil
}

// checkPrecedence reports whether the Writer was handed the repository its
// precedence store is persisted through.
func (w *Writer) checkPrecedence() error {
	if memory.IsMissing(w.Precedence) {
		return errNoPrecedenceRepository
	}
	return nil
}

// checkPages reports whether the Writer was handed the repository its pages
// are read and written through.
func (w *Writer) checkPages() error {
	if memory.IsMissing(w.Pages) {
		return errNoPageRepository
	}
	return nil
}

// checkPatchPorts reports the first port PropagateCards needs and was not handed:
// the repository the precedence store is persisted through, and the pages it
// patches.
func (w *Writer) checkPatchPorts() error {
	if err := w.checkPrecedence(); err != nil {
		return err
	}
	return w.checkPages()
}

// Result reports what Promote did for one memory.
type Result struct {
	Page   Page
	Action Action
}

// Promote renders m as a vault page and writes it, either as a brand
// new page (ActionCreated) or, when the observation was already
// promoted, through UpdateInPlace (ActionUpdated, or ActionSkippedLocalEdit
// per R-030). explicit is forwarded to Eligible, matching the explicit
// promote surface's override semantics (R-007); an ineligible memory is left
// untouched and reports a Result with no page and ActionNone (the zero
// Result) and no error, since ineligibility is a normal skip a scanning
// caller (sync) must not treat as a failure. Every promotion that actually
// wrote a page persists the precedence sidecar; a create persists it BEFORE
// publishing the page, since a published page with no recorded provenance is
// one UpdateInPlace would refuse from then on, while a recorded fingerprint
// with no page is simply a create the next run finishes.
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
// consistency check) and writes nothing, by design. SyncCards does not reach it
// either: an observation already promoted at its current revision is
// skipped before Writer.Promote is ever called (R-009's true-no-op gate),
// and a page whose registration failed is exactly that observation. The
// one path that does repair it is an explicit promote of that observation
// (ExplicitPromoteCard), which re-enters here, takes the update branch, and
// registers on every write it does not skip. So: doctor names it, and an
// explicit promote fixes it.
func (w *Writer) Promote(ctx context.Context, m memory.Memory, explicit bool) (Result, error) {
	if err := w.checkPorts(); err != nil {
		return Result{}, err
	}
	if !Eligible(m, explicit) {
		return Result{}, nil
	}
	style, err := styleOf(m.Identity.Source)
	if err != nil {
		return Result{}, err
	}
	index, err := readPages(w.Pages)
	if err != nil {
		return Result{}, err
	}
	scan := index.scanFor(m)
	chosen, err := decide(m, scan, style, style.rewriteCurrent)
	if err != nil {
		return Result{}, err
	}
	return w.apply(ctx, m, style, scan, chosen)
}

// apply does what chosen says about m, over the scan it was decided from: the one write path of every memory,
// whichever store it comes from. The explicit promote and every pass of a sync reach it through here, so they
// cannot treat one memory differently.
func (w *Writer) apply(ctx context.Context, m memory.Memory, style pageStyle, scan pageScan, chosen decision) (Result, error) {
	switch chosen {
	case decideBridged:
		return Result{}, &BridgedError{Memory: m.Identity, Peer: scan.bridgedAs, Address: scan.bridged.address(), SameContent: scan.mirrored}
	case decideCurrent:
		address := scan.own.address()
		return Result{Page: Page{Address: address, Path: vaultlayout.PageFile(address)}, Action: Action{Kind: ActionAlreadyPromoted}}, nil
	case decideWithheld:
		// A page that could not be read may be this memory's own. Taking an address and writing a page would make it
		// a second one, so nothing is taken and nothing is written: the memory is reported as left alone, with the
		// pages that kept it from being decided.
		return Result{Action: unreadablePagesAction(style.noun, m.Identity.ID, scan.unreadable)}, nil
	}

	// The pages the memory left behind in another project were collected before anything is written, but the
	// timing is not what makes it correct: they are the pages that carry its identity under a project other than
	// its own, so the page this run is about to create or update can never be among them.
	var address string
	if scan.own != nil {
		address = scan.own.address()
	} else {
		var err error
		if address, err = takeAddress(ctx, w.Addresses, w.AddressMap, utc(w.Clock)); err != nil {
			return Result{}, err
		}
	}
	var related []Link
	if style.linksSuccessor {
		related = scan.successorLinks(m)
	}
	page, err := style.emit(m, address, related, utc(w.Clock))
	if err != nil {
		return Result{}, err
	}

	existingPath := w.Pages.PageLocation(address)
	exists, err := w.Pages.PageExists(address)
	if err != nil {
		return Result{}, fmt.Errorf("promote: stat %s: %w", existingPath, err)
	}
	if exists {
		action, err := UpdateInPlace(w.Pages, w.Store, page)
		if err != nil {
			return Result{}, err
		}
		// A skip wrote nothing, so there is no new fingerprint to pair and
		// nothing to register.
		if action.Kind != ActionSkippedLocalEdit {
			if err := w.Precedence.SavePrecedence(w.Store); err != nil {
				return Result{}, err
			}
			if err := w.register(page.Address, m.Title); err != nil {
				return Result{}, err
			}
		}
		// The successor already exists on disk (this branch was chosen by
		// its presence), so pointing at it is safe even when the update
		// itself was skipped: superseding the pages the memory left
		// behind is independent of whether its current page needed new
		// content.
		if err := w.supersedeMoved(scan.moved, page.Address, m.Title); err != nil {
			return Result{}, err
		}
		return Result{Page: page, Action: action}, nil
	}

	// Fingerprint first, page second. The two writes cannot be made atomic
	// with respect to each other, so the only real choice is which of the
	// two orphan states a killed process may leave behind -- and the two
	// are not equally recoverable:
	//
	//   page without entry (the old order) is unrecoverable. The page's own
	//   address is reused, PageExists finds it, and UpdateInPlace
	//   refuses it as unknown provenance -- which, being a skip, also
	//   suppresses the Save and the registration that would have repaired
	//   it. Every later run repeats that exact skip: a fixed point.
	//
	//   entry without page (this order) is self-healing. PageExists finds no
	//   page, so the next run takes this same create branch and finishes
	//   the job, catalog and log included.
	//
	// The Save-failure rollback below only covers a Save that RETURNS an
	// error; a killed process returns nothing for any compensating removal
	// to react to, which is exactly why the ordering has to carry the
	// guarantee rather than the cleanup.
	if err := w.publishNew(address, page, existingPath); err != nil {
		return Result{}, err
	}
	if err := w.register(address, m.Title); err != nil {
		return Result{}, err
	}
	if err := w.supersedeMoved(scan.moved, address, m.Title); err != nil {
		return Result{}, err
	}
	return Result{Page: page, Action: Action{Kind: ActionCreated}}, nil
}

// publishNew records page's fingerprint and then publishes the page, in that order and with the rollback the
// ordering comment of Promote explains. It is the create path every new page takes, whichever store the page
// is promoted from; existingPath only names the page in a message.
func (w *Writer) publishNew(address string, page Page, existingPath string) error {
	w.Store.Set(address, entryFor(page))
	if err := w.Precedence.SavePrecedence(w.Store); err != nil {
		// Nothing has been published yet, so there is no page to withdraw:
		// drop the in-memory entry and report the failure.
		delete(w.Store, address)
		return fmt.Errorf("promote: persist precedence for %s: %w (no page published)", address, err)
	}
	if err := w.Pages.WritePage(address, page.Frontmatter+page.Body); err != nil {
		// The fingerprint is durable but no page carries it. A retry would
		// converge regardless (the create branch is chosen by the page's
		// own absence), but an entry claiming provenance over a file that
		// does not exist is still a lie the sidecar should not tell.
		//
		// allocateAddress's own two writes are deliberately NOT withdrawn with
		// it. The address number the allocator advanced belongs to a
		// counter this package can only call forward, so it is burned
		// whatever happens here; and the .raw/.manifest.json address_map
		// row allocateAddress wrote is left alone because a row without a page is
		// inert -- doctor's address-map rule walks PAGES looking for their
		// rows, never rows looking for their pages -- while rewriting that
		// wiki-ingest-owned file to delete it is a real write that can
		// itself fail. The residue of a failed create is therefore one
		// skipped address number and one dangling manifest row pointing at
		// a path no later run reuses (the retry allocates a fresh
		// address), and neither wedges anything.
		delete(w.Store, address)
		if saveErr := w.Precedence.SavePrecedence(w.Store); saveErr != nil {
			return fmt.Errorf("promote: write page %s: %w (and withdrawing its precedence entry failed: %v)", existingPath, err, saveErr)
		}
		return err
	}
	return nil
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
// heals the same way -- NOT automatically. SyncCards gates before Writer.Promote
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
// produces more than one orphan, so this follows the shape of PropagateCards: record
// the failure, carry on, and persist whatever was patched before
// reporting. Returning early would leave an orphan superseded on disk with
// a stale sidecar entry, and because a superseded page is skipped from
// then on, nothing would ever re-record its hash: it would read as locally
// edited forever, the exact permanent wedge R-030's reconciliation exists
// to end.
//
// LOCAL-EDIT PRECEDENCE (R-030). The patch is unconditional, exactly as
// the one of PropagateCards is, and for the same reason: it rewrites two frontmatter
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
		path := w.Pages.PageLocation(old.Address)
		raw, err := w.Pages.ReadPage(old.Address)
		if err != nil {
			failures = append(failures, fmt.Errorf("promote: read superseded page %s: %w", path, err))
			continue
		}
		block, ok := frontmatterBlock(raw)
		if !ok {
			failures = append(failures, fmt.Errorf("promote: %s has no parseable frontmatter block", path))
			continue
		}
		if parseFrontmatterFields(block)[statusField] == "superseded" {
			continue
		}

		frontmatterHash, _, err := PatchStatusFields(w.Pages, old.Address, "superseded", []string{wikilink(successorAddress, successorTitle)})
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
		if err := w.Precedence.SavePrecedence(w.Store); err != nil {
			failures = append(failures, fmt.Errorf("promote: persist precedence after superseding a moved page: %w", err))
		}
	}
	return errors.Join(failures...)
}

// register records addr/title's promotion in the vault's master catalog
// and append-only promotion log (R-029, task 7.10): the two writes
// register.go's RegisterIndex/RegisterLog perform, the log stamped with the
// writer's clock like every other date it writes.
func (w *Writer) register(addr, title string) error {
	if err := RegisterIndex(w.Index, addr, title); err != nil {
		return err
	}
	return RegisterLog(w.Log, addr, title, utc(w.Clock))
}
