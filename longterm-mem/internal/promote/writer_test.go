package promote

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
)

// TestWriter_Promote_CreatesNewPage: task 6.8 REFACTOR, create branch.
// A never-promoted observation gets a brand-new page written under
// wiki/memory/, and the precedence store records its fresh hashes.
func TestWriter_Promote_CreatesNewPage(t *testing.T) {
	vaultRoot := t.TempDir()
	writeAllocateScript(t, vaultRoot, allocateAddressFixture)
	fixedNow(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))

	w := &Writer{VaultRoot: vaultRoot, Store: PrecedenceStore{}}
	obs := engram.Observation{ID: 401, Type: "decision", Title: "Fresh Decision", Content: "Never promoted before.", Project: "labdrian-sdd-overlay", RevisionCount: 1}

	result, err := w.Promote(obs, false)
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if result.Action.Kind != ActionCreated {
		t.Fatalf("Action.Kind = %v, want ActionCreated", result.Action.Kind)
	}

	full := filepath.Join(vaultRoot, result.Page.Path)
	got, err := os.ReadFile(full)
	if err != nil {
		t.Fatalf("read %s: %v", full, err)
	}
	if !strings.Contains(string(got), "Never promoted before.") {
		t.Fatalf("written page missing observation content; got:\n%s", got)
	}

	entry, ok := w.Store.Get(result.Page.Address)
	if !ok {
		t.Fatalf("precedence store has no entry for %s after create", result.Page.Address)
	}
	if entry.BodyHash != hashText(result.Page.Body) || entry.FrontmatterHash != hashText(result.Page.Frontmatter) {
		t.Fatalf("precedence entry %+v does not match the written page's own hashes", entry)
	}
}

// TestWriter_Promote_UpdatesExistingPage: task 6.8 REFACTOR, update
// branch. A re-promoted, not-locally-edited observation goes through
// UpdateInPlace instead of writing a second file.
func TestWriter_Promote_UpdatesExistingPage(t *testing.T) {
	vaultRoot := t.TempDir()
	fixedNow(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))

	obs := engram.Observation{ID: 402, Type: "pattern", Title: "Already Promoted", Content: "V1.", Project: "labdrian-sdd-overlay", RevisionCount: 1}
	first, err := EmitPage(obs, "c-000402", nil)
	if err != nil {
		t.Fatalf("EmitPage (seed v1): %v", err)
	}
	writePromotedPage(t, vaultRoot, first)

	store := PrecedenceStore{}
	seedPrecedence(store, first)

	// findPromotedAddress (address.go) reuses the same page for the same
	// engram_id/project, so Allocate needs no allocator script fixture
	// here -- reuse must never invoke the subprocess.
	w := &Writer{VaultRoot: vaultRoot, Store: store}

	fixedNow(t, time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC))
	obs.RevisionCount = 2
	obs.Content = "V2."

	result, err := w.Promote(obs, false)
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if result.Action.Kind != ActionUpdated {
		t.Fatalf("Action.Kind = %v, want ActionUpdated", result.Action.Kind)
	}
	if result.Page.Address != "c-000402" {
		t.Fatalf("Page.Address = %q, want the reused c-000402", result.Page.Address)
	}

	entries, err := os.ReadDir(filepath.Join(vaultRoot, pagePathPrefix))
	if err != nil {
		t.Fatalf("read %s: %v", pagePathPrefix, err)
	}
	if len(entries) != 1 {
		t.Fatalf("wiki/memory has %d entries, want 1 (update, not a second page)", len(entries))
	}

	got, err := os.ReadFile(filepath.Join(vaultRoot, first.Path))
	if err != nil {
		t.Fatalf("read %s: %v", first.Path, err)
	}
	if !strings.Contains(string(got), "V2.") {
		t.Fatalf("page content not refreshed; got:\n%s", got)
	}
}

// TestWriter_Promote_SkipsLocalEdit: task 6.8 REFACTOR, the R-030
// diagnostic must surface through Promote's single entrypoint, not only
// through UpdateInPlace called directly.
func TestWriter_Promote_SkipsLocalEdit(t *testing.T) {
	vaultRoot := t.TempDir()
	fixedNow(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))

	obs := engram.Observation{ID: 403, Type: "decision", Title: "Edited In Vault", Content: "Original.", Project: "labdrian-sdd-overlay", RevisionCount: 1}
	first, err := EmitPage(obs, "c-000403", nil)
	if err != nil {
		t.Fatalf("EmitPage: %v", err)
	}
	full := writePromotedPage(t, vaultRoot, first)

	store := PrecedenceStore{}
	seedPrecedence(store, first)

	locallyEdited := first.Frontmatter + "Edited directly in the vault.\n"
	if err := os.WriteFile(full, []byte(locallyEdited), 0o644); err != nil {
		t.Fatalf("write local edit: %v", err)
	}

	w := &Writer{VaultRoot: vaultRoot, Store: store}
	obs.RevisionCount = 2

	result, err := w.Promote(obs, false)
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if result.Action.Kind != ActionSkippedLocalEdit {
		t.Fatalf("Action.Kind = %v, want ActionSkippedLocalEdit", result.Action.Kind)
	}
	if result.Action.Diagnostic == nil || !strings.Contains(result.Action.Diagnostic.Detail, "c-000403") {
		t.Fatalf("Action.Diagnostic = %+v, want a diagnostic naming c-000403", result.Action.Diagnostic)
	}

	got, err := os.ReadFile(full)
	if err != nil {
		t.Fatalf("read %s: %v", full, err)
	}
	if string(got) != locallyEdited {
		t.Fatalf("content changed despite a local edit; got:\n%s", got)
	}
}

// TestWriter_Promote_PersistsPrecedenceEntry: the page and its precedence
// entry are the two halves of one promotion, and a gap between them is
// what makes an interrupted run misread its own writes (slice 6b finding
// R4-write-store-persist-crash-window). Writer owns pairing them, so a
// completed Promote must leave the entry durable on disk, not only in
// memory -- a second Writer loading the sidecar afresh sees it.
func TestWriter_Promote_PersistsPrecedenceEntry(t *testing.T) {
	vaultRoot := t.TempDir()
	writeAllocateScript(t, vaultRoot, allocateAddressFixture)
	fixedNow(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))

	w := &Writer{VaultRoot: vaultRoot, Store: PrecedenceStore{}}
	obs := engram.Observation{ID: 404, Type: "decision", Title: "Durable Pairing", Content: "Entry must outlive the process.", Project: "labdrian-sdd-overlay", RevisionCount: 1}

	result, err := w.Promote(obs, false)
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}

	reloaded, err := LoadPrecedenceStore(vaultRoot)
	if err != nil {
		t.Fatalf("LoadPrecedenceStore: %v", err)
	}
	entry, ok := reloaded.Get(result.Page.Address)
	if !ok {
		t.Fatalf("sidecar has no entry for %s after Promote; the page write and its fingerprint did not land together", result.Page.Address)
	}
	if entry.BodyHash != hashText(result.Page.Body) || entry.FrontmatterHash != hashText(result.Page.Frontmatter) {
		t.Fatalf("persisted entry %+v does not match the written page's own hashes", entry)
	}
}

// promoteTwice promotes obs, then promotes a revised copy of it, and
// returns the second Result -- the update branch's fixture.
func promoteTwice(t *testing.T, w *Writer, obs engram.Observation) Result {
	t.Helper()
	if _, err := w.Promote(obs, false); err != nil {
		t.Fatalf("Promote (first): %v", err)
	}
	fixedNow(t, time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC))
	obs.RevisionCount++
	obs.Content = "Revised content."
	result, err := w.Promote(obs, false)
	if err != nil {
		t.Fatalf("Promote (second): %v", err)
	}
	return result
}

// TestWriter_Promote_UpdatePersistsPrecedenceEntry: the update branch must
// persist its refreshed fingerprint too, not only the create branch --
// otherwise deleting Promote's Save on that branch would leave every test
// green while the guarantee it advertises holds for creates alone (review
// finding R3-update-branch-persistence-unproven).
func TestWriter_Promote_UpdatePersistsPrecedenceEntry(t *testing.T) {
	vaultRoot := t.TempDir()
	writeAllocateScript(t, vaultRoot, allocateAddressFixture)
	fixedNow(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))

	w := &Writer{VaultRoot: vaultRoot, Store: PrecedenceStore{}}
	obs := engram.Observation{ID: 405, Type: "decision", Title: "Updated Twice", Content: "First content.", Project: "labdrian-sdd-overlay", RevisionCount: 1}
	result := promoteTwice(t, w, obs)

	if result.Action.Kind != ActionUpdated {
		t.Fatalf("Action.Kind = %v, want ActionUpdated", result.Action.Kind)
	}
	reloaded, err := LoadPrecedenceStore(vaultRoot)
	if err != nil {
		t.Fatalf("LoadPrecedenceStore: %v", err)
	}
	entry, ok := reloaded.Get(result.Page.Address)
	if !ok {
		t.Fatalf("sidecar has no entry for %s after an update", result.Page.Address)
	}
	if entry.BodyHash != hashText(result.Page.Body) {
		t.Fatalf("persisted body hash is stale after an update: entry %+v, want the second revision's hash", entry)
	}
}

// TestWriter_Promote_SkipDoesNotPersist: the branch that writes nothing
// must record nothing either -- persisting after a skip would stamp a
// human's edit as longterm-mem's own work and disable the R-030 guard on
// the next run (review finding R3-update-branch-persistence-unproven).
func TestWriter_Promote_SkipDoesNotPersist(t *testing.T) {
	vaultRoot := t.TempDir()
	writeAllocateScript(t, vaultRoot, allocateAddressFixture)
	fixedNow(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))

	w := &Writer{VaultRoot: vaultRoot, Store: PrecedenceStore{}}
	obs := engram.Observation{ID: 406, Type: "decision", Title: "Edited By Hand", Content: "Original content.", Project: "labdrian-sdd-overlay", RevisionCount: 1}
	first, err := w.Promote(obs, false)
	if err != nil {
		t.Fatalf("Promote (first): %v", err)
	}

	full := filepath.Join(vaultRoot, first.Page.Path)
	locallyEdited := first.Page.Frontmatter + "Edited by a human, never by longterm-mem.\n"
	if err := os.WriteFile(full, []byte(locallyEdited), 0o644); err != nil {
		t.Fatalf("write local edit: %v", err)
	}
	before, err := LoadPrecedenceStore(vaultRoot)
	if err != nil {
		t.Fatalf("LoadPrecedenceStore (before): %v", err)
	}

	fixedNow(t, time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC))
	obs.RevisionCount = 2
	obs.Content = "Revised content."
	second, err := w.Promote(obs, false)
	if err != nil {
		t.Fatalf("Promote (second): %v", err)
	}
	if second.Action.Kind != ActionSkippedLocalEdit {
		t.Fatalf("Action.Kind = %v, want ActionSkippedLocalEdit", second.Action.Kind)
	}

	after, err := LoadPrecedenceStore(vaultRoot)
	if err != nil {
		t.Fatalf("LoadPrecedenceStore (after): %v", err)
	}
	if after[second.Page.Address] != before[second.Page.Address] {
		t.Fatalf("sidecar entry changed on a skip: before %+v, after %+v", before, after)
	}
}

// TestWriter_Promote_CreateRollsBackWhenFingerprintCannotPersist: a create
// that publishes the page but cannot persist its fingerprint leaves a
// page of unknown provenance, which UpdateInPlace then refuses forever --
// so the retry never converges. The page is removed instead, making the
// create branch all-or-nothing (review finding
// R4-create-path-write-save-not-atomic).
func TestWriter_Promote_CreateRollsBackWhenFingerprintCannotPersist(t *testing.T) {
	vaultRoot := t.TempDir()
	writeAllocateScript(t, vaultRoot, allocateAddressFixture)
	fixedNow(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))

	// A directory where the sidecar file belongs: Save's rename cannot
	// replace it, so persistence fails after the page is published.
	blocked := filepath.Join(vaultRoot, precedenceManifestRelPath)
	if err := os.MkdirAll(blocked, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", blocked, err)
	}

	w := &Writer{VaultRoot: vaultRoot, Store: PrecedenceStore{}}
	obs := engram.Observation{ID: 407, Type: "decision", Title: "Unpersistable", Content: "Fingerprint cannot land.", Project: "labdrian-sdd-overlay", RevisionCount: 1}

	if _, err := w.Promote(obs, false); err == nil {
		t.Fatalf("Promote = nil error, want the sidecar persistence failure surfaced")
	}

	orphan := filepath.Join(vaultRoot, pagePathPrefix, "c-000042.md")
	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Fatalf("page %s survived a failed fingerprint write (stat err = %v); it must be rolled back so a retry converges", orphan, err)
	}
}
