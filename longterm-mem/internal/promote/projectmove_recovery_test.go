package promote

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
)

// writeRawPage writes a hand-authored wiki/memory/ page verbatim, so a test
// can seed promotion state EmitPage would never produce: a page with no
// address line, or one whose address field disagrees with its filename.
func writeRawPage(t *testing.T, vaultRoot, file, raw string) {
	t.Helper()
	dir := filepath.Join(vaultRoot, pagePathPrefix)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, file), []byte(raw), 0o644); err != nil {
		t.Fatalf("write raw page %s: %v", file, err)
	}
}

// TestWriter_Promote_CorruptedPageInAnotherProjectDoesNotBlockThisOne: the
// moved-page lookup matches on engram_id alone, so a corrupted page under
// SOME OTHER project became visible to this project's promotion for the
// first time. It must not fail it. findPromotedPage is also Sync's R-009
// gate and Propagate's lookup, so one unrepairable page under project A
// would otherwise wedge project B's sync and supersession propagation
// forever -- no path ever writes an address into that page.
func TestWriter_Promote_CorruptedPageInAnotherProjectDoesNotBlockThisOne(t *testing.T) {
	vaultRoot := t.TempDir()
	writeAllocateScript(t, vaultRoot, allocateAddressFixture)
	fixedNow(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))
	writeRawPage(t, vaultRoot, "c-000001.md", "---\nengram_id: 905\nproject: p-one\n---\n\nBody.\n")

	obs := engram.Observation{ID: 905, Type: "decision", Title: "Live Decision", Content: "Body.", Project: "p-two", RevisionCount: 1}
	w := &Writer{VaultRoot: vaultRoot, Store: PrecedenceStore{}}
	result, err := w.Promote(obs, false)
	if err != nil {
		t.Fatalf("Promote under p-two was blocked by a corrupted page under p-one: %v", err)
	}
	if result.Action.Kind != ActionCreated {
		t.Fatalf("Action.Kind = %v, want ActionCreated", result.Action.Kind)
	}
}

// TestFindPromotedPage_CorruptionIsScopedToItsOwnProject: the same rule at
// the lookup Sync and Propagate share. A page under another project whose
// engram_revision cannot be parsed is not this project's problem; the same
// corruption under this project still fails closed.
func TestFindPromotedPage_CorruptionIsScopedToItsOwnProject(t *testing.T) {
	vaultRoot := t.TempDir()
	writeRawPage(t, vaultRoot, "c-000001.md", "---\nengram_id: 906\nproject: p-one\naddress: c-000001\nengram_revision: not-a-number\n---\n\nBody.\n")

	if _, ok, err := findPromotedPage(vaultRoot, "p-two", 906); err != nil {
		t.Fatalf("findPromotedPage(p-two) = %v, want no error: a corrupted page under p-one is not p-two's problem", err)
	} else if ok {
		t.Fatalf("findPromotedPage(p-two) ok = true, want false: the page belongs to p-one")
	}

	if _, _, err := findPromotedPage(vaultRoot, "p-one", 906); err == nil {
		t.Fatalf("findPromotedPage(p-one) = nil error, want the corruption to still fail closed for the project that owns the page")
	}
}

// TestWriter_Promote_ProjectMoveSupersedesFromTheUpdateBranch: the moved
// observation's successor page ALREADY exists, so Promote takes the update
// branch. The orphan must still be superseded there -- that branch is the
// only path that finishes a move whose supersession was interrupted.
func TestWriter_Promote_ProjectMoveSupersedesFromTheUpdateBranch(t *testing.T) {
	obs := engram.Observation{ID: 328, Type: "decision", Title: "Moved Decision", Content: "Body.", Project: "p-one", RevisionCount: 1}
	vaultRoot, store, oldPage := movedFixture(t, obs, "c-000001")

	successorObs := obs
	successorObs.Project = "p-two"
	successorPage := seedPromotedPage(t, vaultRoot, store, successorObs, "c-000042")

	revised := successorObs
	revised.RevisionCount = 2
	revised.Content = "Body v2."
	w := &Writer{VaultRoot: vaultRoot, Store: store}
	result, err := w.Promote(revised, false)
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if result.Action.Kind != ActionUpdated {
		t.Fatalf("Action.Kind = %v, want ActionUpdated (the successor page already existed)", result.Action.Kind)
	}
	if result.Page.Address != successorPage.Address {
		t.Fatalf("address = %s, want the existing successor %s", result.Page.Address, successorPage.Address)
	}

	old := readPage(t, vaultRoot, oldPage.Address)
	if !strings.Contains(old, "status: superseded") {
		t.Fatalf("the update branch left the orphan live; got:\n%s", old)
	}
	if !strings.Contains(old, "[["+successorPage.Address+"|Moved Decision]]") {
		t.Fatalf("the orphan does not point at the successor %s; got:\n%s", successorPage.Address, old)
	}
	entry, ok := w.Store.Get(oldPage.Address)
	if !ok || !entry.MatchesPage(old) {
		t.Fatalf("the orphan's precedence entry %+v was not re-recorded after the patch: it now reads as locally edited forever", entry)
	}
}

// TestWriter_Promote_ProjectMoveSupersedesEvenWhenTheSuccessorUpdateIsSkipped:
// the same branch when the successor page itself is locally edited. Its
// update is skipped (R-030), but superseding the orphan is independent of
// whether the successor needed new content -- skipping it would leave two
// live pages for one engram_id, the duplicate R-008 forbids.
func TestWriter_Promote_ProjectMoveSupersedesEvenWhenTheSuccessorUpdateIsSkipped(t *testing.T) {
	obs := engram.Observation{ID: 329, Type: "decision", Title: "Moved Decision", Content: "Body.", Project: "p-one", RevisionCount: 1}
	vaultRoot, store, oldPage := movedFixture(t, obs, "c-000001")

	successorObs := obs
	successorObs.Project = "p-two"
	successorPage := seedPromotedPage(t, vaultRoot, store, successorObs, "c-000042")
	// A human edits the successor page's body: its recorded body hash no
	// longer covers what is on disk, so UpdateInPlace refuses to rewrite it.
	successorPath := filepath.Join(vaultRoot, pagePathPrefix, successorPage.Address+".md")
	if err := os.WriteFile(successorPath, []byte(successorPage.Frontmatter+successorPage.Body+"\nA human wrote this.\n"), 0o644); err != nil {
		t.Fatalf("seed a local edit on the successor: %v", err)
	}

	revised := successorObs
	revised.RevisionCount = 2
	revised.Content = "Body v2."
	w := &Writer{VaultRoot: vaultRoot, Store: store}
	result, err := w.Promote(revised, false)
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if result.Action.Kind != ActionSkippedLocalEdit {
		t.Fatalf("Action.Kind = %v, want ActionSkippedLocalEdit", result.Action.Kind)
	}

	old := readPage(t, vaultRoot, oldPage.Address)
	if !strings.Contains(old, "status: superseded") {
		t.Fatalf("a skipped successor update also skipped superseding the orphan; got:\n%s", old)
	}
	if !strings.Contains(old, "[["+successorPage.Address+"|Moved Decision]]") {
		t.Fatalf("the orphan does not point at the successor %s; got:\n%s", successorPage.Address, old)
	}
	entry, ok := w.Store.Get(oldPage.Address)
	if !ok || !entry.MatchesPage(old) {
		t.Fatalf("the orphan's precedence entry %+v was not re-recorded after the patch", entry)
	}
}

// TestWriter_Promote_ProjectMoveKeepsPatchedHashesWhenALaterPageFails: a
// double move produces more than one orphan. Propagate records a per-page
// failure and still saves what it patched; supersession must do the same.
// Returning on the second orphan's failure discards the first's
// already-written frontmatter hash, and because that page is superseded on
// disk nothing ever re-records it: it reads as locally edited forever --
// the permanent wedge this whole rule exists to prevent.
func TestWriter_Promote_ProjectMoveKeepsPatchedHashesWhenALaterPageFails(t *testing.T) {
	obs := engram.Observation{ID: 330, Type: "decision", Title: "Twice Moved", Content: "Body.", Project: "p-one", RevisionCount: 1}
	vaultRoot, store, firstOrphan := movedFixture(t, obs, "c-000001")
	// The second orphan's address field disagrees with its filename, so the
	// scan finds it but the supersession patch cannot open it.
	writeRawPage(t, vaultRoot, "c-000002.md", "---\nengram_id: 330\nproject: p-three\naddress: c-999999\n---\n\nBody.\n")

	moved := obs
	moved.Project = "p-two"
	w := &Writer{VaultRoot: vaultRoot, Store: store}
	if _, err := w.Promote(moved, false); err == nil {
		t.Fatalf("Promote = nil error, want the unreachable second orphan reported")
	} else if !strings.Contains(err.Error(), "c-999999") {
		t.Fatalf("error %q does not name the orphan that failed", err)
	}

	old := readPage(t, vaultRoot, firstOrphan.Address)
	if !strings.Contains(old, "status: superseded") {
		t.Fatalf("the first orphan was not superseded; got:\n%s", old)
	}
	persisted, err := LoadPrecedenceStore(vaultRoot)
	if err != nil {
		t.Fatalf("LoadPrecedenceStore: %v", err)
	}
	entry, ok := persisted.Get(firstOrphan.Address)
	if !ok || !entry.MatchesPage(old) {
		t.Fatalf("the first orphan's patched hash was discarded by the later failure: persisted entry %+v no longer matches its page", entry)
	}
}

// TestWriter_Promote_ProjectMoveKeepsPatchedHashesWhenALaterPageIsUnparseable
// pins the SECOND of supersedeMoved's per-page failure branches.
//
// Three branches accumulate-and-continue rather than returning: an
// unreadable file, an unparseable frontmatter block, and a failing patch.
// The sibling test above reaches only the first, because its second orphan
// names an address with no file at all. Reverting the unparseable branch to
// an early return therefore left the whole module green -- so the fix for
// the discard-what-was-patched defect was pinned on one third of its own
// surface.
//
// The third branch, a failing PatchStatusFields, is NOT covered and is not
// worth a hollow test: PatchStatusFields' own read and parse failures are
// shadowed by the two checks above it, so the only way to reach it is a
// write failure, which this suite cannot provoke portably -- writeFileAtomic
// renames a temp file into a directory the test must keep writable to seed
// the fixture at all.
func TestWriter_Promote_ProjectMoveKeepsPatchedHashesWhenALaterPageIsUnparseable(t *testing.T) {
	obs := engram.Observation{ID: 331, Type: "decision", Title: "Twice Moved", Content: "Body.", Project: "p-one", RevisionCount: 1}
	vaultRoot, store, firstOrphan := movedFixture(t, obs, "c-000001")
	// The second orphan is found by the scan (it carries the engram_id and a
	// foreign project) and its address names a file that EXISTS -- but that
	// file has no frontmatter block, so the patch reaches the parse branch
	// rather than the read branch.
	writeRawPage(t, vaultRoot, "c-000002.md", "---\nengram_id: 331\nproject: p-three\naddress: c-000003\n---\n\nBody.\n")
	writeRawPage(t, vaultRoot, "c-000003.md", "Just prose. No frontmatter delimiters at all.\n")

	moved := obs
	moved.Project = "p-two"
	w := &Writer{VaultRoot: vaultRoot, Store: store}
	if _, err := w.Promote(moved, false); err == nil {
		t.Fatalf("Promote = nil error, want the unparseable second orphan reported")
	} else if !strings.Contains(err.Error(), "c-000003") {
		t.Fatalf("error %q does not name the orphan that failed to parse", err)
	}

	old := readPage(t, vaultRoot, firstOrphan.Address)
	if !strings.Contains(old, "status: superseded") {
		t.Fatalf("the first orphan was not superseded; got:\n%s", old)
	}
	persisted, err := LoadPrecedenceStore(vaultRoot)
	if err != nil {
		t.Fatalf("LoadPrecedenceStore: %v", err)
	}
	entry, ok := persisted.Get(firstOrphan.Address)
	if !ok || !entry.MatchesPage(old) {
		t.Fatalf("an unparseable later page discarded the first orphan's patched hash: persisted entry %+v no longer matches its page", entry)
	}
}
