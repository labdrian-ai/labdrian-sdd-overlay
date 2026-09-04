package promote

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
)

// movedFixture seeds one already-promoted page for obs under its current
// project and returns the vault root plus that page, ready for a second
// Promote of the SAME observation under a different project. The allocator
// fixture hands out c-000042, which cannot collide with the seeded
// c-000001.
func movedFixture(t *testing.T, obs engram.Observation, seedAddress string) (string, PrecedenceStore, Page) {
	t.Helper()
	vaultRoot := t.TempDir()
	writeAllocateScript(t, vaultRoot, allocateAddressFixture)
	fixedNow(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))

	store := PrecedenceStore{}
	page := seedPromotedPage(t, vaultRoot, store, obs, seedAddress)
	return vaultRoot, store, page
}

func readPage(t *testing.T, vaultRoot, address string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(vaultRoot, pagePathPrefix, address+".md"))
	if err != nil {
		t.Fatalf("read page %s: %v", address, err)
	}
	return string(data)
}

// TestWriter_Promote_ProjectMoveSupersedesOldPage: an observation moved to
// another Engram project gets a fresh address (its old address encodes the
// project it was promoted under, and R-009's lookup shares that key), and
// the page it left behind is marked superseded with related pointing at the
// new address -- body byte-unchanged, exactly as R-033's own supersession
// patch does it. Without the patch the vault keeps two live pages for one
// engram_id with no signal which is current, which is the duplicate R-008
// forbids.
func TestWriter_Promote_ProjectMoveSupersedesOldPage(t *testing.T) {
	obs := engram.Observation{ID: 321, Type: "decision", Title: "Moved Decision", Content: "Body.", Project: "p-one", RevisionCount: 1}
	vaultRoot, store, oldPage := movedFixture(t, obs, "c-000001")

	moved := obs
	moved.Project = "p-two"
	w := &Writer{VaultRoot: vaultRoot, Store: store}
	result, err := w.Promote(moved, false)
	if err != nil {
		t.Fatalf("Promote after project move: %v", err)
	}
	if result.Action.Kind != ActionCreated {
		t.Fatalf("Action.Kind = %v, want ActionCreated (a moved observation gets a fresh address)", result.Action.Kind)
	}
	if result.Page.Address == oldPage.Address {
		t.Fatalf("moved observation reused the old address %s", oldPage.Address)
	}

	old := readPage(t, vaultRoot, oldPage.Address)
	if !strings.Contains(old, "status: superseded") {
		t.Fatalf("old page was not marked superseded; got:\n%s", old)
	}
	if !strings.Contains(old, "[["+result.Page.Address+"|Moved Decision]]") {
		t.Fatalf("old page's related does not point at the new address %s; got:\n%s", result.Page.Address, old)
	}
	if !strings.HasSuffix(old, oldPage.Body) {
		t.Fatalf("old page's body was rewritten; want it byte-identical, got:\n%s", old)
	}
	if !strings.Contains(old, "project: p-one") {
		t.Fatalf("old page's project was rewritten; the move is history worth keeping. Got:\n%s", old)
	}
}

// TestWriter_Promote_ProjectMoveKeepsOldPageOffTheLocalEditPath: patching
// the old page's frontmatter moves its frontmatter hash, so its precedence
// entry has to move with it. Left stale, the orphan reads as locally edited
// forever -- the permanent wedge R-030's reconciliation exists to end.
func TestWriter_Promote_ProjectMoveKeepsOldPageOffTheLocalEditPath(t *testing.T) {
	obs := engram.Observation{ID: 322, Type: "decision", Title: "Moved Decision", Content: "Body.", Project: "p-one", RevisionCount: 1}
	vaultRoot, store, oldPage := movedFixture(t, obs, "c-000001")

	moved := obs
	moved.Project = "p-two"
	w := &Writer{VaultRoot: vaultRoot, Store: store}
	if _, err := w.Promote(moved, false); err != nil {
		t.Fatalf("Promote after project move: %v", err)
	}

	// Precondition: the patch this test is about actually happened.
	if !strings.Contains(readPage(t, vaultRoot, oldPage.Address), "status: superseded") {
		t.Fatalf("old page was not superseded, so there is no patched hash to check")
	}

	entry, ok := w.Store.Get(oldPage.Address)
	if !ok {
		t.Fatalf("precedence store lost the old page's entry for %s", oldPage.Address)
	}
	if !entry.MatchesPage(readPage(t, vaultRoot, oldPage.Address)) {
		t.Fatalf("old page %s no longer matches its precedence entry after the supersession patch: it now reads as a local edit forever", oldPage.Address)
	}

	// The sidecar on disk, not just the in-memory map, must carry it.
	persisted, err := LoadPrecedenceStore(vaultRoot)
	if err != nil {
		t.Fatalf("LoadPrecedenceStore: %v", err)
	}
	if persisted[oldPage.Address] != entry {
		t.Fatalf("persisted entry %+v for %s differs from the in-memory one %+v", persisted[oldPage.Address], oldPage.Address, entry)
	}
}

// TestWriter_Promote_ProjectMoveIsIdempotent: a second promotion after the
// move must not re-supersede, must not duplicate the related link, and must
// not rewrite the old page at all.
func TestWriter_Promote_ProjectMoveIsIdempotent(t *testing.T) {
	obs := engram.Observation{ID: 323, Type: "decision", Title: "Moved Decision", Content: "Body.", Project: "p-one", RevisionCount: 1}
	vaultRoot, store, oldPage := movedFixture(t, obs, "c-000001")

	moved := obs
	moved.Project = "p-two"
	w := &Writer{VaultRoot: vaultRoot, Store: store}
	if _, err := w.Promote(moved, false); err != nil {
		t.Fatalf("first Promote after project move: %v", err)
	}
	afterFirst := readPage(t, vaultRoot, oldPage.Address)
	entryAfterFirst, _ := w.Store.Get(oldPage.Address)
	oldPath := filepath.Join(vaultRoot, pagePathPrefix, oldPage.Address+".md")
	statBefore, err := os.Stat(oldPath)
	if err != nil {
		t.Fatalf("stat old page: %v", err)
	}

	if _, err := w.Promote(moved, false); err != nil {
		t.Fatalf("second Promote after project move: %v", err)
	}

	afterSecond := readPage(t, vaultRoot, oldPage.Address)
	if afterSecond != afterFirst {
		t.Fatalf("second promotion rewrote the superseded page.\nfirst:\n%s\nsecond:\n%s", afterFirst, afterSecond)
	}
	if strings.Count(afterSecond, "[[") != 1 {
		t.Fatalf("second promotion duplicated the related link; got:\n%s", afterSecond)
	}
	statAfter, err := os.Stat(oldPath)
	if err != nil {
		t.Fatalf("stat old page again: %v", err)
	}
	if !statAfter.ModTime().Equal(statBefore.ModTime()) {
		t.Fatalf("second promotion touched the old page again (mtime moved %v -> %v)", statBefore.ModTime(), statAfter.ModTime())
	}
	if entryAfter, _ := w.Store.Get(oldPage.Address); entryAfter != entryAfterFirst {
		t.Fatalf("second promotion moved the old page's precedence entry %+v -> %+v", entryAfterFirst, entryAfter)
	}
}

// TestWriter_Promote_ProjectMoveLeavesAnExistingSupersessionAlone: an old
// page already superseded -- by R-033's Engram-side propagation, pointing at
// some other successor -- keeps its pointer. Overwriting another successor
// chain silently would destroy real history, and the moved observation stays
// reachable by engram_id regardless.
func TestWriter_Promote_ProjectMoveLeavesAnExistingSupersessionAlone(t *testing.T) {
	obs := engram.Observation{ID: 324, Type: "decision", Title: "Moved Decision", Content: "Body.", Project: "p-one", RevisionCount: 1}
	vaultRoot, store, oldPage := movedFixture(t, obs, "c-000001")

	oldPath := filepath.Join(vaultRoot, pagePathPrefix, oldPage.Address+".md")
	frontmatterHash, _, err := PatchStatusFields(oldPath, "superseded", []string{wikilink("c-000555", "Some Other Successor")})
	if err != nil {
		t.Fatalf("seed an existing supersession: %v", err)
	}
	entry, _ := store.Get(oldPage.Address)
	entry.FrontmatterHash = frontmatterHash
	store.Set(oldPage.Address, entry)
	before := readPage(t, vaultRoot, oldPage.Address)

	moved := obs
	moved.Project = "p-two"
	w := &Writer{VaultRoot: vaultRoot, Store: store}
	if _, err := w.Promote(moved, false); err != nil {
		t.Fatalf("Promote after project move: %v", err)
	}

	after := readPage(t, vaultRoot, oldPage.Address)
	if after != before {
		t.Fatalf("an already-superseded old page was rewritten.\nbefore:\n%s\nafter:\n%s", before, after)
	}
	if !strings.Contains(after, "[[c-000555|Some Other Successor]]") {
		t.Fatalf("the pre-existing successor pointer was clobbered; got:\n%s", after)
	}
}

// TestWriter_Promote_UnmovedObservationStillReusesItsPage is the twin that
// must not regress: an observation promoted again under the SAME project
// still updates its one page in place (R-008) and nothing is superseded.
func TestWriter_Promote_UnmovedObservationStillReusesItsPage(t *testing.T) {
	obs := engram.Observation{ID: 325, Type: "decision", Title: "Stable Decision", Content: "V1.", Project: "p-one", RevisionCount: 1}
	vaultRoot, store, oldPage := movedFixture(t, obs, "c-000001")

	next := obs
	next.RevisionCount = 2
	next.Content = "V2."
	w := &Writer{VaultRoot: vaultRoot, Store: store}
	result, err := w.Promote(next, false)
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if result.Action.Kind != ActionUpdated {
		t.Fatalf("Action.Kind = %v, want ActionUpdated", result.Action.Kind)
	}
	if result.Page.Address != oldPage.Address {
		t.Fatalf("address = %s, want the reused %s", result.Page.Address, oldPage.Address)
	}

	page := readPage(t, vaultRoot, oldPage.Address)
	if strings.Contains(page, "status: superseded") {
		t.Fatalf("an unmoved re-promotion superseded its own page; got:\n%s", page)
	}
	if !strings.Contains(page, "V2.") {
		t.Fatalf("page was not updated in place; got:\n%s", page)
	}

	entries, err := os.ReadDir(filepath.Join(vaultRoot, pagePathPrefix))
	if err != nil {
		t.Fatalf("list memory dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("wiki/memory holds %d pages, want exactly 1", len(entries))
	}
}

// TestWriter_Promote_DistinctObservationsSharingAProjectDoNotCollide is the
// other twin: the match key is engram_id, so two DIFFERENT observations that
// merely share a project must never see each other as a moved page.
func TestWriter_Promote_DistinctObservationsSharingAProjectDoNotCollide(t *testing.T) {
	first := engram.Observation{ID: 326, Type: "decision", Title: "First", Content: "One.", Project: "p-one", RevisionCount: 1}
	vaultRoot, store, firstPage := movedFixture(t, first, "c-000001")

	second := engram.Observation{ID: 327, Type: "decision", Title: "Second", Content: "Two.", Project: "p-one", RevisionCount: 1}
	w := &Writer{VaultRoot: vaultRoot, Store: store}
	result, err := w.Promote(second, false)
	if err != nil {
		t.Fatalf("Promote second observation: %v", err)
	}
	if result.Page.Address == firstPage.Address {
		t.Fatalf("two distinct observations landed on the same address %s", result.Page.Address)
	}

	page := readPage(t, vaultRoot, firstPage.Address)
	if strings.Contains(page, "status: superseded") {
		t.Fatalf("promoting a different observation in the same project superseded an unrelated page; got:\n%s", page)
	}
}
