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

	// findPromotedPage (address.go) reuses the same page for the same
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
// whose fingerprint cannot be persisted must publish no page at all --
// a page of unknown provenance is one UpdateInPlace refuses, so a retry
// would never converge (review finding
// R4-create-path-write-save-not-atomic).
//
// Reworked for the reversed create order: the fingerprint is now written
// first, so a failing Save never reaches the page write and the guarantee
// holds structurally instead of by compensating removal. The assertion is
// correspondingly stronger than "the page file is gone": the promoted-pages
// DIRECTORY must never have been created either. Under the old order
// writeFileAtomic's own MkdirAll left wiki/memory behind after the page was
// withdrawn, so this assertion is what pins the ordering rather than the
// cleanup.
func TestWriter_Promote_CreateRollsBackWhenFingerprintCannotPersist(t *testing.T) {
	vaultRoot := t.TempDir()
	writeAllocateScript(t, vaultRoot, allocateAddressFixture)
	fixedNow(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))

	// A directory where the sidecar file belongs: Save's rename cannot
	// replace it, so persistence fails.
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
		t.Fatalf("page %s survived a failed fingerprint write (stat err = %v); it must never be published so a retry converges", orphan, err)
	}
	memoryDir := filepath.Join(vaultRoot, pagePathPrefix)
	if _, err := os.Stat(memoryDir); !os.IsNotExist(err) {
		t.Fatalf("%s exists (stat err = %v) after a create whose fingerprint could not persist; the page write must not have been attempted at all, since a killed process between the two writes returns no error for a compensating removal to react to", memoryDir, err)
	}
}

// TestWriter_Promote_CreatePersistsPrecedenceBeforeThePage: the create
// path's fatal window was between the page write and the sidecar Save. A
// killed process there returns no error, so the Save-failure rollback above
// never runs, and what survives is a published page whose provenance
// nothing records -- which UpdateInPlace then refuses forever, with sync
// never even reaching it and no repair command anywhere in the module.
// Reversing the two writes closes the window: the fingerprint is durable
// BEFORE the page exists, so the worst an interrupted create can leave is
// an entry with no page, and os.Stat then takes the next run straight back
// down the create branch (pinned by
// TestWriter_Promote_CreateResumesAfterAnUnpairedFingerprint).
//
// Ordering is observable from outside the package by failing the page
// write and looking for the sidecar: under the old order the Save never ran
// at all, so no sidecar file existed. The same run pins the rollback half
// -- an entry must not outlive a page that never landed.
func TestWriter_Promote_CreatePersistsPrecedenceBeforeThePage(t *testing.T) {
	vaultRoot := t.TempDir()
	writeAllocateScript(t, vaultRoot, allocateAddressFixture)
	fixedNow(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))

	// A dangling symlink where wiki/memory belongs: os.Stat of the page
	// path still reports "does not exist" (so Promote takes the create
	// branch) while writeFileAtomic's own MkdirAll fails with EEXIST --
	// deterministically and regardless of the uid the test runs as, the
	// same discipline internal/ops's unreadable-page fixture uses.
	memoryDir := filepath.Join(vaultRoot, pagePathPrefix)
	if err := os.MkdirAll(filepath.Dir(memoryDir), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(memoryDir), err)
	}
	if err := os.Symlink(filepath.Join(vaultRoot, "wiki", "nonexistent-target"), memoryDir); err != nil {
		t.Fatalf("create dangling symlink: %v", err)
	}

	w := &Writer{VaultRoot: vaultRoot, Store: PrecedenceStore{}}
	obs := engram.Observation{ID: 408, Type: "decision", Title: "Unwritable Page", Content: "The page write cannot land.", Project: "labdrian-sdd-overlay", RevisionCount: 1}

	if _, err := w.Promote(obs, false); err == nil {
		t.Fatalf("Promote = nil error, want the page write failure surfaced")
	}

	sidecar := filepath.Join(vaultRoot, precedenceManifestRelPath)
	if _, err := os.Stat(sidecar); err != nil {
		t.Fatalf("sidecar %s was never written (stat err = %v); the fingerprint must be persisted BEFORE the page, so an interrupted create can never publish a page of unrecorded provenance", sidecar, err)
	}

	reloaded, err := LoadPrecedenceStore(vaultRoot)
	if err != nil {
		t.Fatalf("LoadPrecedenceStore: %v", err)
	}
	if _, ok := reloaded.Get("c-000042"); ok {
		t.Fatalf("sidecar still records c-000042 after a create whose page never landed; the entry must be withdrawn (full store: %+v)", reloaded)
	}
	if _, ok := w.Store.Get("c-000042"); ok {
		t.Fatalf("in-memory store still records c-000042 after a create whose page never landed (full store: %+v)", w.Store)
	}
}

// TestWriter_Promote_CreateResumesAfterAnUnpairedFingerprint: the state the
// reversed create order leaves behind when a process is killed between its
// two writes -- a durable precedence entry with no page on disk. That state
// must converge on the next run rather than becoming a fixed point of its
// own: os.Stat finds no page, so Promote takes the create branch again and
// republishes it, together with the catalog and log entries the killed run
// never reached.
func TestWriter_Promote_CreateResumesAfterAnUnpairedFingerprint(t *testing.T) {
	vaultRoot := t.TempDir()
	writeAllocateScript(t, vaultRoot, allocateAddressFixture)
	fixedNow(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))

	w := &Writer{VaultRoot: vaultRoot, Store: PrecedenceStore{}}
	obs := engram.Observation{ID: 409, Type: "decision", Title: "Entry Without A Page", Content: "Body.", Project: "labdrian-sdd-overlay", RevisionCount: 1}
	first, err := w.Promote(obs, false)
	if err != nil {
		t.Fatalf("Promote (first): %v", err)
	}

	// Simulate the kill: the fingerprint is durable, the page never landed,
	// and neither did the catalog/log entries that follow it.
	pagePath := filepath.Join(vaultRoot, first.Page.Path)
	if err := os.Remove(pagePath); err != nil {
		t.Fatalf("simulate interrupted create: %v", err)
	}
	for _, rel := range []string{indexMdRelPath, logMdRelPath} {
		if err := os.Remove(filepath.Join(vaultRoot, rel)); err != nil {
			t.Fatalf("simulate interrupted create (%s): %v", rel, err)
		}
	}

	fixedNow(t, time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC))
	second, err := w.Promote(obs, false)
	if err != nil {
		t.Fatalf("Promote (resume): %v", err)
	}
	if second.Action.Kind != ActionCreated {
		t.Fatalf("Action.Kind = %v, want ActionCreated: an entry with no page is a create the next run finishes, not a refusal", second.Action.Kind)
	}
	if _, err := os.Stat(pagePath); err != nil {
		t.Fatalf("page %s was not republished (stat err = %v); an unpaired fingerprint must not wedge the create branch", pagePath, err)
	}
	for _, rel := range []string{indexMdRelPath, logMdRelPath} {
		data, err := os.ReadFile(filepath.Join(vaultRoot, rel))
		if err != nil {
			t.Fatalf("read %s after the resumed create: %v", rel, err)
		}
		if !strings.Contains(string(data), second.Page.Address) {
			t.Fatalf("%s does not record %s after the resumed create; got:\n%s", rel, second.Page.Address, data)
		}
	}
}

// TestWriter_Promote_AdoptsAnUnrecordedOwnPageAndRepairsRegistration: the
// field state the reversed create order prevents but cannot retroactively
// fix -- a vault already wedged by the old order, where the page and its
// address map survived a kill but the precedence sidecar, the catalog and
// the log did not. Before this, every subsequent run reported
// skipped_local_edit and repaired nothing, forever. The page's own content
// proves it is ours, so it is adopted back into the precedence store and
// its registration repaired.
func TestWriter_Promote_AdoptsAnUnrecordedOwnPageAndRepairsRegistration(t *testing.T) {
	vaultRoot := t.TempDir()
	writeAllocateScript(t, vaultRoot, allocateAddressFixture)
	fixedNow(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))

	w := &Writer{VaultRoot: vaultRoot, Store: PrecedenceStore{}}
	obs := engram.Observation{ID: 410, Type: "decision", Title: "Wedged In The Field", Content: "Body.", Project: "labdrian-sdd-overlay", RevisionCount: 1}
	first, err := w.Promote(obs, false)
	if err != nil {
		t.Fatalf("Promote (first): %v", err)
	}

	// The old order's crash window: the page and .raw/.manifest.json
	// survived; the sidecar, the catalog and the log never landed.
	for _, rel := range []string{precedenceManifestRelPath, indexMdRelPath, logMdRelPath} {
		if err := os.Remove(filepath.Join(vaultRoot, rel)); err != nil {
			t.Fatalf("simulate the pre-fix crash window (%s): %v", rel, err)
		}
	}

	// A later day, so the freshly rendered page differs from the one on
	// disk in exactly the two volatile stamps EmitPage takes from the wall
	// clock -- the second-order hazard that makes a byte-equality
	// reconciliation succeed only on a same-day retry.
	fixedNow(t, time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC))
	wedged := &Writer{VaultRoot: vaultRoot, Store: PrecedenceStore{}}
	second, err := wedged.Promote(obs, false)
	if err != nil {
		t.Fatalf("Promote (wedged retry): %v", err)
	}
	if second.Action.Kind == ActionSkippedLocalEdit {
		t.Fatalf("Action.Kind = ActionSkippedLocalEdit: a page whose content is byte-identical to our own render (ignoring created:/updated:) is our interrupted write, not a human's edit, and refusing it is a permanent fixed point")
	}
	if second.Action.Kind != ActionUpdated {
		t.Fatalf("Action.Kind = %v, want ActionUpdated", second.Action.Kind)
	}

	reloaded, err := LoadPrecedenceStore(vaultRoot)
	if err != nil {
		t.Fatalf("LoadPrecedenceStore: %v", err)
	}
	entry, ok := reloaded.Get(first.Page.Address)
	if !ok {
		t.Fatalf("sidecar still has no entry for %s: adoption must repair the provenance the crash lost", first.Page.Address)
	}
	if entry.BodyHash != hashText(second.Page.Body) || entry.FrontmatterHash != hashText(second.Page.Frontmatter) {
		t.Fatalf("adopted entry %+v does not fingerprint the page now on disk", entry)
	}
	for _, rel := range []string{indexMdRelPath, logMdRelPath} {
		data, err := os.ReadFile(filepath.Join(vaultRoot, rel))
		if err != nil {
			t.Fatalf("read %s after adoption: %v", rel, err)
		}
		if !strings.Contains(string(data), first.Page.Address) {
			t.Fatalf("%s does not record %s after adoption; the registration the crash lost must be repaired too, got:\n%s", rel, first.Page.Address, data)
		}
	}
}

// TestWriter_Promote_CreateRegistersIndexAndLog: R-029, task 7.10. A
// promotion that actually writes a brand-new page must also register it
// in the vault's master catalog (wiki/index.md) and record the promotion
// event in the append-only log (wiki/log.md) -- RegisterIndex/RegisterLog
// (register.go, Slice 5) had zero production callers until this task.
func TestWriter_Promote_CreateRegistersIndexAndLog(t *testing.T) {
	vaultRoot := t.TempDir()
	writeAllocateScript(t, vaultRoot, allocateAddressFixture)
	fixedNow(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))

	w := &Writer{VaultRoot: vaultRoot, Store: PrecedenceStore{}}
	obs := engram.Observation{ID: 501, Type: "decision", Title: "Catalog Me", Content: "Body.", Project: "labdrian-sdd-overlay", RevisionCount: 1}

	result, err := w.Promote(obs, false)
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}

	indexData, err := os.ReadFile(filepath.Join(vaultRoot, "wiki", "index.md"))
	if err != nil {
		t.Fatalf("read wiki/index.md: %v", err)
	}
	if !strings.Contains(string(indexData), "[["+result.Page.Address+"|Catalog Me]]") {
		t.Fatalf("wiki/index.md does not list the promoted page; got:\n%s", indexData)
	}

	logData, err := os.ReadFile(filepath.Join(vaultRoot, "wiki", "log.md"))
	if err != nil {
		t.Fatalf("read wiki/log.md: %v", err)
	}
	if !strings.Contains(string(logData), "## [2026-08-01] promote | Catalog Me") {
		t.Fatalf("wiki/log.md does not record the promotion event; got:\n%s", logData)
	}
}

// TestWriter_Promote_UpdateRegistersIndexAndLog: R-029, task 7.10. The
// update branch (a re-promoted, not-locally-edited observation) must
// register too -- a second, distinct log.md entry for the fresh
// promotion event, and the SAME index.md entry updated in place rather
// than duplicated (RegisterIndex's own idempotent-replace contract).
func TestWriter_Promote_UpdateRegistersIndexAndLog(t *testing.T) {
	vaultRoot := t.TempDir()
	writeAllocateScript(t, vaultRoot, allocateAddressFixture)
	fixedNow(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))

	w := &Writer{VaultRoot: vaultRoot, Store: PrecedenceStore{}}
	obs := engram.Observation{ID: 502, Type: "decision", Title: "Updated Twice", Content: "First content.", Project: "labdrian-sdd-overlay", RevisionCount: 1}
	result := promoteTwice(t, w, obs)
	if result.Action.Kind != ActionUpdated {
		t.Fatalf("Action.Kind = %v, want ActionUpdated", result.Action.Kind)
	}

	logData, err := os.ReadFile(filepath.Join(vaultRoot, "wiki", "log.md"))
	if err != nil {
		t.Fatalf("read wiki/log.md: %v", err)
	}
	content := string(logData)
	if strings.Count(content, "## [") != 2 {
		t.Fatalf("wiki/log.md must record both the create and the update as separate entries; got:\n%s", content)
	}
	if !strings.Contains(content, "## [2026-08-15] promote | Updated Twice") {
		t.Fatalf("wiki/log.md missing the update's own entry; got:\n%s", content)
	}

	indexData, err := os.ReadFile(filepath.Join(vaultRoot, "wiki", "index.md"))
	if err != nil {
		t.Fatalf("read wiki/index.md: %v", err)
	}
	if strings.Count(string(indexData), result.Page.Address) != 1 {
		t.Fatalf("wiki/index.md must register the reused address exactly once (idempotent replace, not a duplicate entry); got:\n%s", indexData)
	}
}

// TestWriter_Promote_SkipDoesNotRegister: R-029, task 7.10. The branch
// that writes nothing (ActionSkippedLocalEdit) must register nothing
// either -- registering a page longterm-mem never actually wrote would
// misrepresent the catalog/log as reflecting the skipped promotion.
func TestWriter_Promote_SkipDoesNotRegister(t *testing.T) {
	vaultRoot := t.TempDir()
	writeAllocateScript(t, vaultRoot, allocateAddressFixture)
	fixedNow(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))

	w := &Writer{VaultRoot: vaultRoot, Store: PrecedenceStore{}}
	obs := engram.Observation{ID: 503, Type: "decision", Title: "Edited By Hand", Content: "Original content.", Project: "labdrian-sdd-overlay", RevisionCount: 1}
	first, err := w.Promote(obs, false)
	if err != nil {
		t.Fatalf("Promote (first): %v", err)
	}

	full := filepath.Join(vaultRoot, first.Page.Path)
	locallyEdited := first.Page.Frontmatter + "Edited by a human, never by longterm-mem.\n"
	if err := os.WriteFile(full, []byte(locallyEdited), 0o644); err != nil {
		t.Fatalf("write local edit: %v", err)
	}

	indexBefore, err := os.ReadFile(filepath.Join(vaultRoot, "wiki", "index.md"))
	if err != nil {
		t.Fatalf("read wiki/index.md (before): %v", err)
	}
	logBefore, err := os.ReadFile(filepath.Join(vaultRoot, "wiki", "log.md"))
	if err != nil {
		t.Fatalf("read wiki/log.md (before): %v", err)
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

	indexAfter, err := os.ReadFile(filepath.Join(vaultRoot, "wiki", "index.md"))
	if err != nil {
		t.Fatalf("read wiki/index.md (after): %v", err)
	}
	if string(indexBefore) != string(indexAfter) {
		t.Fatalf("wiki/index.md changed on a skip; before:\n%s\nafter:\n%s", indexBefore, indexAfter)
	}

	logAfter, err := os.ReadFile(filepath.Join(vaultRoot, "wiki", "log.md"))
	if err != nil {
		t.Fatalf("read wiki/log.md (after): %v", err)
	}
	if string(logBefore) != string(logAfter) {
		t.Fatalf("wiki/log.md changed on a skip; before:\n%s\nafter:\n%s", logBefore, logAfter)
	}
}

// TestWriter_Promote_IneligibleDoesNotRegister: R-029, task 7.10. An
// ineligible observation writes nothing (R-007) and must register
// nothing either -- proving the zero-Result early return never reaches
// registration.
func TestWriter_Promote_IneligibleDoesNotRegister(t *testing.T) {
	vaultRoot := t.TempDir()
	w := &Writer{VaultRoot: vaultRoot, Store: PrecedenceStore{}}
	obs := engram.Observation{ID: 504, Type: "note", Title: "Not Eligible", Content: "Body.", Project: "labdrian-sdd-overlay", RevisionCount: 1}

	result, err := w.Promote(obs, false)
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if result.Page.Address != "" {
		t.Fatalf("Page.Address = %q, want the zero Result R-007 promises for an ineligible observation", result.Page.Address)
	}

	if _, err := os.Stat(filepath.Join(vaultRoot, "wiki", "index.md")); !os.IsNotExist(err) {
		t.Fatalf("wiki/index.md exists after an ineligible observation was promoted (stat err = %v)", err)
	}
	if _, err := os.Stat(filepath.Join(vaultRoot, "wiki", "log.md")); !os.IsNotExist(err) {
		t.Fatalf("wiki/log.md exists after an ineligible observation was promoted (stat err = %v)", err)
	}
}
