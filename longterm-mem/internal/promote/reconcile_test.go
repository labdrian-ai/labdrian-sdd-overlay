package promote

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
)

// wedgedLegacyPage builds the exact state doctor's precedence-sidecar check
// names and promotion permanently refuses: a sidecar entry recording NO
// promoted revision (a vault promoted before the store recorded one) whose
// page has ALSO diverged from that entry's fingerprints.
//
// It returns the vault root, the page's address, and the observation the
// page was rendered from, so a caller can drive a LATER revision through
// UpdateInPlace afterwards.
func wedgedLegacyPage(t *testing.T, id int64, address string) (vaultRoot string, obs engram.Observation) {
	t.Helper()
	vaultRoot = t.TempDir()
	fixedNow(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))

	obs = engram.Observation{ID: id, Type: "decision", Title: "Wedged Legacy", Content: "V1 body.", Project: "labdrian-sdd-overlay", RevisionCount: 1}
	first, err := EmitPage(obs, address, nil)
	if err != nil {
		t.Fatalf("EmitPage (v1): %v", err)
	}
	existingPath := writePromotedPage(t, vaultRoot, first)

	// The legacy entry: content fingerprints, no recorded revision.
	legacy := entryFor(first)
	legacy.PromotedRevision = 0
	store := PrecedenceStore{address: legacy}
	if err := store.Save(vaultRoot); err != nil {
		t.Fatalf("seed legacy sidecar: %v", err)
	}

	// The divergence: revision 2 landed on disk and the sidecar never
	// caught up, so the entry fingerprints a render that is no longer
	// there and carries no revision to attribute the new one to.
	fixedNow(t, time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC))
	obs.RevisionCount = 2
	obs.Content = "V2 body."
	diverged, err := EmitPage(obs, address, nil)
	if err != nil {
		t.Fatalf("EmitPage (v2): %v", err)
	}
	if err := os.WriteFile(existingPath, []byte(diverged.Frontmatter+diverged.Body), 0o644); err != nil {
		t.Fatalf("simulate the diverged page: %v", err)
	}
	return vaultRoot, obs
}

// TestReconcile_AdoptsAWedgedLegacyPageAndLeavesTheWedgedState is the
// property the command exists for, and it is NOT "one call succeeded".
//
// Promotion refuses a legacy entry whose page has diverged because there is
// genuinely no evidence separating the writer's own unrecorded write from a
// human's edit, and that refusal is a skip, so it suppresses the very store
// write that would repair the entry: the page is refused identically on
// every later run, forever. A human naming ONE address is the consent the
// automatic path lacks.
//
// So the assertion is that the page LEFT that state: after reconcile, a
// SUBSEQUENT revision must drive through UpdateInPlace as an ordinary
// update, exactly like any normally-recorded page.
func TestReconcile_AdoptsAWedgedLegacyPageAndLeavesTheWedgedState(t *testing.T) {
	const address = "c-000601"
	vaultRoot, obs := wedgedLegacyPage(t, 601, address)

	// Precondition: the page really is wedged today. Without this the test
	// could pass against a vault that was never stuck.
	fixedNow(t, time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC))
	obs.RevisionCount = 3
	obs.Content = "V3 body."
	next, err := EmitPage(obs, address, nil)
	if err != nil {
		t.Fatalf("EmitPage (v3): %v", err)
	}
	existingPath := filepath.Join(vaultRoot, next.Path)

	before, err := LoadPrecedenceStore(vaultRoot)
	if err != nil {
		t.Fatalf("LoadPrecedenceStore (before): %v", err)
	}
	action, err := UpdateInPlace(before, next, existingPath)
	if err != nil {
		t.Fatalf("UpdateInPlace (before reconcile): %v", err)
	}
	if action.Kind != ActionSkippedLocalEdit {
		t.Fatalf("fixture is not wedged: UpdateInPlace answered %v before reconcile, want ActionSkippedLocalEdit", action.Kind)
	}

	outcome, err := Reconcile(vaultRoot, address)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if !outcome.Adopted {
		t.Fatalf("Reconcile outcome = %+v, want the page adopted", outcome)
	}
	if outcome.PromotedRevision != 2 {
		t.Fatalf("Reconcile recorded revision %d, want 2 -- the revision the page on disk actually carries", outcome.PromotedRevision)
	}

	// The twin of "the call succeeded": the page must now behave like any
	// normally-recorded one.
	after, err := LoadPrecedenceStore(vaultRoot)
	if err != nil {
		t.Fatalf("LoadPrecedenceStore (after): %v", err)
	}
	action, err = UpdateInPlace(after, next, existingPath)
	if err != nil {
		t.Fatalf("UpdateInPlace (after reconcile): %v", err)
	}
	if action.Kind != ActionUpdated {
		t.Fatalf("UpdateInPlace answered %v after reconcile, want ActionUpdated -- the page never left the wedged state", action.Kind)
	}
	got, err := os.ReadFile(existingPath)
	if err != nil {
		t.Fatalf("read %s: %v", existingPath, err)
	}
	if string(got) != next.Frontmatter+next.Body {
		t.Fatalf("page was not refreshed to the later revision after reconcile; got:\n%s", got)
	}
}

// TestReconcile_AdoptsAPageWithNoEntryAtAll: the other state the
// precedence-sidecar diagnostic names -- a page longterm-mem published
// without recording that it did, whose bytes have since moved on, so the
// byte-identity adoption cannot settle it either.
func TestReconcile_AdoptsAPageWithNoEntryAtAll(t *testing.T) {
	const address = "c-000602"
	vaultRoot := t.TempDir()
	fixedNow(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))

	obs := engram.Observation{ID: 602, Type: "decision", Title: "Untracked", Content: "Body.", Project: "labdrian-sdd-overlay", RevisionCount: 4}
	page, err := EmitPage(obs, address, nil)
	if err != nil {
		t.Fatalf("EmitPage: %v", err)
	}
	writePromotedPage(t, vaultRoot, page)

	outcome, err := Reconcile(vaultRoot, address)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if !outcome.Adopted || outcome.PromotedRevision != 4 {
		t.Fatalf("Reconcile outcome = %+v, want the page adopted at revision 4", outcome)
	}

	store, err := LoadPrecedenceStore(vaultRoot)
	if err != nil {
		t.Fatalf("LoadPrecedenceStore: %v", err)
	}
	entry, ok := store.Get(address)
	if !ok {
		t.Fatalf("sidecar has no entry for %s after reconcile", address)
	}
	if !entry.MatchesPage(page.Frontmatter + page.Body) {
		t.Fatalf("entry %+v does not fingerprint the page on disk", entry)
	}
}

// TestReconcile_RefusesAPageThatIsNotWedged covers the two states reconcile
// must NOT silently re-adopt, and they are deliberately answered
// differently.
func TestReconcile_RefusesAPageThatIsNotWedged(t *testing.T) {
	t.Run("an entry that already matches its page is a clear no-op", func(t *testing.T) {
		const address = "c-000603"
		vaultRoot := t.TempDir()
		fixedNow(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))

		obs := engram.Observation{ID: 603, Type: "decision", Title: "Healthy", Content: "Body.", Project: "labdrian-sdd-overlay", RevisionCount: 2}
		page, err := EmitPage(obs, address, nil)
		if err != nil {
			t.Fatalf("EmitPage: %v", err)
		}
		writePromotedPage(t, vaultRoot, page)
		store := PrecedenceStore{}
		seedPrecedence(store, page)
		if err := store.Save(vaultRoot); err != nil {
			t.Fatalf("seed sidecar: %v", err)
		}

		outcome, err := Reconcile(vaultRoot, address)
		if err != nil {
			t.Fatalf("Reconcile on a healthy page: %v -- re-running reconcile after the vault is repaired must not fail", err)
		}
		if outcome.Adopted {
			t.Fatalf("Reconcile outcome = %+v, want no adoption: the entry already describes the page", outcome)
		}
	})

	t.Run("an ordinary local edit is refused, not adopted", func(t *testing.T) {
		const address = "c-000604"
		vaultRoot := t.TempDir()
		fixedNow(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))

		obs := engram.Observation{ID: 604, Type: "decision", Title: "Edited By Hand", Content: "Body.", Project: "labdrian-sdd-overlay", RevisionCount: 2}
		page, err := EmitPage(obs, address, nil)
		if err != nil {
			t.Fatalf("EmitPage: %v", err)
		}
		existingPath := writePromotedPage(t, vaultRoot, page)
		store := PrecedenceStore{}
		seedPrecedence(store, page)
		if err := store.Save(vaultRoot); err != nil {
			t.Fatalf("seed sidecar: %v", err)
		}
		// A human edits the body. The entry still records a positive
		// revision, so promotion refuses this page BY DESIGN and keeps
		// refusing it -- that is the human's edit being preserved, not a
		// wedge, and reconcile must not launder it into an adoption.
		if err := os.WriteFile(existingPath, []byte(page.Frontmatter+page.Body+"\nA human wrote this.\n"), 0o644); err != nil {
			t.Fatalf("simulate a local edit: %v", err)
		}

		if _, err := Reconcile(vaultRoot, address); !errors.Is(err, ErrLocalEditPreserved) {
			t.Fatalf("Reconcile on a locally edited page returned %v, want ErrLocalEditPreserved", err)
		}
	})
}

// TestReconcile_UnknownAddressFailsCleanly: an address with no page is a
// typo or a stale doctor report, and it must be a clean, distinguishable
// failure rather than an adoption of nothing.
func TestReconcile_UnknownAddressFailsCleanly(t *testing.T) {
	vaultRoot := t.TempDir()
	if _, err := Reconcile(vaultRoot, "c-999999"); !errors.Is(err, ErrPageNotFound) {
		t.Fatalf("Reconcile on an unknown address returned %v, want ErrPageNotFound", err)
	}
}

// TestReconcile_RefusesAPageWithNoUsableRevision: adopting a page whose own
// engram_revision cannot be read would record another entry with no usable
// revision -- recreating the exact wedge this command exists to end, while
// reporting success.
func TestReconcile_RefusesAPageWithNoUsableRevision(t *testing.T) {
	const address = "c-000605"
	vaultRoot := t.TempDir()
	fixedNow(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))

	obs := engram.Observation{ID: 605, Type: "decision", Title: "No Revision", Content: "Body.", Project: "labdrian-sdd-overlay", RevisionCount: 2}
	page, err := EmitPage(obs, address, nil)
	if err != nil {
		t.Fatalf("EmitPage: %v", err)
	}
	existingPath := writePromotedPage(t, vaultRoot, page)
	stripped := blankFrontmatterValues(page.Frontmatter, engramRevisionField)
	if err := os.WriteFile(existingPath, []byte(stripped+page.Body), 0o644); err != nil {
		t.Fatalf("strip engram_revision: %v", err)
	}

	if _, err := Reconcile(vaultRoot, address); err == nil {
		t.Fatalf("Reconcile adopted a page whose engram_revision cannot be read, recreating the wedge it exists to end")
	}
}
