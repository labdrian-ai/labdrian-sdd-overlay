package promote

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
)

// TestReconcile_AdoptsARevisionZeroPageAndLeavesTheWedgedState covers the
// population the command exists for and used to decline.
//
// Eligibility (eligible.go) admits a pinned or eligible-typed observation
// whose revision_count is 0, and EmitPage copies that 0 into the page. So a
// revision-0 page is an ORDINARY promoted page, not corruption -- and once
// its entry diverges it is wedged exactly like any other legacy entry:
// doctor's precedence-sidecar check (PromotedRevision <= 0 && !MatchesPage)
// names it as the state nothing in the promotion path can repair.
//
// Refusing to adopt it left the one advertised remedy declining the very
// population it was written for. The refusal's premise -- that recording a 0
// recreates the wedge -- is false: the wedge needs an entry that no longer
// MATCHES its page, and an adopted entry fingerprints the bytes on disk, so
// UpdateInPlace's divergence branch is not reached at all on the next
// promotion.
//
// The assertion is therefore the same one the positive-revision case makes:
// the page LEFT the wedged state, proven by driving a later revision through
// UpdateInPlace.
func TestReconcile_AdoptsARevisionZeroPageAndLeavesTheWedgedState(t *testing.T) {
	const address = "c-000606"
	const project = "labdrian-sdd-overlay"
	vaultRoot := t.TempDir()
	fixedNow(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))

	// A pinned observation Engram has never revised: eligible, revision 0.
	obs := engram.Observation{ID: 606, Type: "decision", Title: "Never Revised", Content: "V1 body.", Project: project, RevisionCount: 0, Pinned: true}
	first, err := EmitPage(obs, address, nil)
	if err != nil {
		t.Fatalf("EmitPage (v1): %v", err)
	}
	existingPath := writePromotedPage(t, vaultRoot, first)

	store := PrecedenceStore{}
	seedPrecedence(store, first)
	if err := store.Save(vaultRoot); err != nil {
		t.Fatalf("seed sidecar: %v", err)
	}

	// The divergence: a later write landed on disk and the sidecar never
	// caught up, so the entry fingerprints bytes that are no longer there.
	fixedNow(t, time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC))
	obs.Content = "V1 body, republished."
	diverged, err := EmitPage(obs, address, nil)
	if err != nil {
		t.Fatalf("EmitPage (diverged): %v", err)
	}
	if err := os.WriteFile(existingPath, []byte(diverged.Frontmatter+diverged.Body), 0o644); err != nil {
		t.Fatalf("simulate the diverged page: %v", err)
	}

	// Precondition: the page really is wedged today.
	fixedNow(t, time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC))
	obs.RevisionCount = 1
	obs.Content = "V2 body."
	next, err := EmitPage(obs, address, nil)
	if err != nil {
		t.Fatalf("EmitPage (v2): %v", err)
	}
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

	outcome, err := Reconcile(vaultRoot, project, address)
	if err != nil {
		t.Fatalf("Reconcile on a revision-0 page: %v -- revision 0 is an ordinary promoted state, and refusing it declines the population this command exists for", err)
	}
	if !outcome.Adopted || outcome.PromotedRevision != 0 {
		t.Fatalf("Reconcile outcome = %+v, want the page adopted at revision 0", outcome)
	}

	after, err := LoadPrecedenceStore(vaultRoot)
	if err != nil {
		t.Fatalf("LoadPrecedenceStore (after): %v", err)
	}
	action, err = UpdateInPlace(after, next, existingPath)
	if err != nil {
		t.Fatalf("UpdateInPlace (after reconcile): %v", err)
	}
	if action.Kind != ActionUpdated {
		t.Fatalf("UpdateInPlace answered %v after reconcile, want ActionUpdated -- the revision-0 page never left the wedged state", action.Kind)
	}
	got, err := os.ReadFile(existingPath)
	if err != nil {
		t.Fatalf("read %s: %v", existingPath, err)
	}
	if string(got) != next.Frontmatter+next.Body {
		t.Fatalf("page was not refreshed to the later revision after reconcile; got:\n%s", got)
	}
}

// TestReconcile_RefusesAnAddressOutsideThePagesDirectory: the address is
// operator-supplied and used to be joined straight into a path, so
// `reconcile ../../secret` read a file OUTSIDE wiki/memory, adopted it, and
// wrote a sidecar entry keyed "../../secret" that no promotion can ever
// match -- permanent pollution of a shared file, from one typo or paste.
func TestReconcile_RefusesAnAddressOutsideThePagesDirectory(t *testing.T) {
	const project = "labdrian-sdd-overlay"
	vaultRoot := t.TempDir()
	fixedNow(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))

	obs := engram.Observation{ID: 607, Type: "decision", Title: "Outside", Content: "Body.", Project: project, RevisionCount: 2}
	page, err := EmitPage(obs, "c-000607", nil)
	if err != nil {
		t.Fatalf("EmitPage: %v", err)
	}
	// A page-shaped file that is NOT a promoted page.
	if err := os.WriteFile(filepath.Join(vaultRoot, "secret.md"), []byte(page.Frontmatter+page.Body), 0o644); err != nil {
		t.Fatalf("write the outside file: %v", err)
	}

	for _, address := range []string{"../../secret", "../secret", "nested/c-000607", "c-000607.md", ""} {
		if _, err := Reconcile(vaultRoot, project, address); err == nil {
			t.Fatalf("Reconcile adopted address %q, which is not a promoted-page address", address)
		}
	}

	store, err := LoadPrecedenceStore(vaultRoot)
	if err != nil {
		t.Fatalf("LoadPrecedenceStore: %v", err)
	}
	if len(store) != 0 {
		t.Fatalf("a refused traversal still wrote %d sidecar entr(ies): %+v", len(store), store)
	}
}

// TestReconcile_RefusesAPageWhoseFrontmatterDisagrees: the file found at
// wiki/memory/<address>.md must actually BE the promoted page for that
// address and that project. A file whose own frontmatter names another
// address (a copy, a rename, a hand-made file) would be adopted under a key
// describing different content, and a page belonging to another project
// would be adopted by whoever named it first.
func TestReconcile_RefusesAPageWhoseFrontmatterDisagrees(t *testing.T) {
	const project = "labdrian-sdd-overlay"
	fixedNow(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))

	t.Run("the page names a different address", func(t *testing.T) {
		const address = "c-000608"
		vaultRoot := t.TempDir()
		page, err := EmitPage(engram.Observation{ID: 608, Type: "decision", Title: "Copied", Content: "Body.", Project: project, RevisionCount: 2}, "c-000999", nil)
		if err != nil {
			t.Fatalf("EmitPage: %v", err)
		}
		writePromotedPageAt(t, vaultRoot, address, page)

		if _, err := Reconcile(vaultRoot, project, address); err == nil {
			t.Fatalf("Reconcile adopted a file whose own frontmatter names a different address")
		}
	})

	t.Run("the page belongs to another project", func(t *testing.T) {
		const address = "c-000609"
		vaultRoot := t.TempDir()
		page, err := EmitPage(engram.Observation{ID: 609, Type: "decision", Title: "Other Project", Content: "Body.", Project: "someone-elses-project", RevisionCount: 2}, address, nil)
		if err != nil {
			t.Fatalf("EmitPage: %v", err)
		}
		writePromotedPageAt(t, vaultRoot, address, page)

		if _, err := Reconcile(vaultRoot, project, address); err == nil {
			t.Fatalf("Reconcile adopted a page belonging to another project")
		}
	})
}

// writePromotedPageAt writes page's bytes at wiki/memory/<address>.md,
// deliberately allowing address to differ from the address the page itself
// carries -- which is the mismatch the tests above need.
func writePromotedPageAt(t *testing.T, vaultRoot, address string, page Page) string {
	t.Helper()
	full := filepath.Join(vaultRoot, pagePathPrefix, address+".md")
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
	}
	if err := os.WriteFile(full, []byte(page.Frontmatter+page.Body), 0o644); err != nil {
		t.Fatalf("write %s: %v", full, err)
	}
	return full
}
