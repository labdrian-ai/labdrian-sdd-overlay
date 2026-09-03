package promote

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
)

// writePromotedPage writes page's rendered content to vaultRoot/page.Path,
// returning the on-disk path -- simulating a page an earlier promotion
// (EmitPage + a plain file write) already put on disk.
func writePromotedPage(t *testing.T, vaultRoot string, page Page) string {
	t.Helper()
	full := filepath.Join(vaultRoot, page.Path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
	}
	if err := os.WriteFile(full, []byte(page.Frontmatter+page.Body), 0o644); err != nil {
		t.Fatalf("write %s: %v", full, err)
	}
	return full
}

// seedPrecedence records page's own content hashes in store, simulating
// that longterm-mem itself was the last writer of page (the baseline
// UpdateInPlace's local-edit detection compares against).
func seedPrecedence(store PrecedenceStore, page Page) {
	store.Set(page.Address, PrecedenceEntry{
		BodyHash:        hashText(page.Body),
		FrontmatterHash: hashText(page.Frontmatter),
	})
}

// TestUpdate_UnmodifiedPageUpdatesInPlace: R-008 scenario 1 (task 6.3).
// Observation X was previously promoted to page V and has not been
// locally modified since; X's revision count increases and promotion
// runs again; V's content and updated timestamp refresh, and no second
// page for X is created.
func TestUpdate_UnmodifiedPageUpdatesInPlace(t *testing.T) {
	vaultRoot := t.TempDir()
	fixedNow(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))

	obs := engram.Observation{ID: 301, Type: "decision", Title: "Widget Decision", Content: "V1 content.", Project: "labdrian-sdd-overlay", RevisionCount: 1}
	first, err := EmitPage(obs, "c-000301", nil)
	if err != nil {
		t.Fatalf("EmitPage (v1): %v", err)
	}
	existingPath := writePromotedPage(t, vaultRoot, first)

	store, err := LoadPrecedenceStore(vaultRoot)
	if err != nil {
		t.Fatalf("LoadPrecedenceStore: %v", err)
	}
	seedPrecedence(store, first)

	fixedNow(t, time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC))
	obs.RevisionCount = 2
	obs.Content = "V2 content."
	second, err := EmitPage(obs, "c-000301", nil)
	if err != nil {
		t.Fatalf("EmitPage (v2): %v", err)
	}

	action, err := UpdateInPlace(store, second, existingPath)
	if err != nil {
		t.Fatalf("UpdateInPlace: %v", err)
	}
	if action.Kind != ActionUpdated {
		t.Fatalf("action.Kind = %v, want ActionUpdated", action.Kind)
	}

	got, err := os.ReadFile(existingPath)
	if err != nil {
		t.Fatalf("read %s: %v", existingPath, err)
	}
	if !strings.Contains(string(got), "V2 content.") {
		t.Fatalf("content not refreshed; got:\n%s", got)
	}
	if !strings.Contains(string(got), "updated: 2026-08-15") {
		t.Fatalf("updated timestamp not refreshed; got:\n%s", got)
	}
	if !strings.Contains(string(got), "engram_revision: 2") {
		t.Fatalf("engram_revision not refreshed; got:\n%s", got)
	}

	entries, err := os.ReadDir(filepath.Join(vaultRoot, pagePathPrefix))
	if err != nil {
		t.Fatalf("read %s: %v", pagePathPrefix, err)
	}
	if len(entries) != 1 {
		t.Fatalf("wiki/memory has %d entries, want 1 (no second page created)", len(entries))
	}
}

// TestUpdate_RetitleKeepsSameFile: R-008 scenario 2 (task 6.4). X's
// Engram title changed since its last promotion but its id is unchanged;
// the same on-disk page is updated with the new title, no new file is
// created, and no old file is orphaned.
func TestUpdate_RetitleKeepsSameFile(t *testing.T) {
	vaultRoot := t.TempDir()
	fixedNow(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))

	obs := engram.Observation{ID: 302, Type: "pattern", Title: "Original Title", Content: "Body.", Project: "labdrian-sdd-overlay", RevisionCount: 1}
	first, err := EmitPage(obs, "c-000302", nil)
	if err != nil {
		t.Fatalf("EmitPage (original): %v", err)
	}
	existingPath := writePromotedPage(t, vaultRoot, first)

	store, err := LoadPrecedenceStore(vaultRoot)
	if err != nil {
		t.Fatalf("LoadPrecedenceStore: %v", err)
	}
	seedPrecedence(store, first)

	obs.Title = "Renamed Title"
	obs.RevisionCount = 2
	second, err := EmitPage(obs, "c-000302", nil)
	if err != nil {
		t.Fatalf("EmitPage (retitled): %v", err)
	}
	if second.Path != first.Path {
		t.Fatalf("Path changed across a retitle: %q -> %q", first.Path, second.Path)
	}

	action, err := UpdateInPlace(store, second, existingPath)
	if err != nil {
		t.Fatalf("UpdateInPlace: %v", err)
	}
	if action.Kind != ActionUpdated {
		t.Fatalf("action.Kind = %v, want ActionUpdated", action.Kind)
	}

	got, err := os.ReadFile(existingPath)
	if err != nil {
		t.Fatalf("read %s: %v", existingPath, err)
	}
	if !strings.Contains(string(got), `title: "Renamed Title"`) {
		t.Fatalf("title not updated; got:\n%s", got)
	}

	entries, err := os.ReadDir(filepath.Join(vaultRoot, pagePathPrefix))
	if err != nil {
		t.Fatalf("read %s: %v", pagePathPrefix, err)
	}
	if len(entries) != 1 {
		t.Fatalf("wiki/memory has %d entries, want 1 (no orphaned old file)", len(entries))
	}
	if entries[0].Name() != "c-000302.md" {
		t.Fatalf("wiki/memory entry = %q, want c-000302.md (same file, not a new one)", entries[0].Name())
	}
}

// TestUpdate_LocallyEditedPageSkippedWithDiagnostic: R-030 scenario 1
// (task 6.5). A promoted page's on-disk content diverges from what
// longterm-mem itself last wrote (a human or agent edited it directly);
// the content update is skipped and a diagnostic names the page.
func TestUpdate_LocallyEditedPageSkippedWithDiagnostic(t *testing.T) {
	vaultRoot := t.TempDir()
	fixedNow(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))

	obs := engram.Observation{ID: 303, Type: "decision", Title: "Locally Edited", Content: "Original body.", Project: "labdrian-sdd-overlay", RevisionCount: 1}
	first, err := EmitPage(obs, "c-000303", nil)
	if err != nil {
		t.Fatalf("EmitPage: %v", err)
	}
	existingPath := writePromotedPage(t, vaultRoot, first)

	store, err := LoadPrecedenceStore(vaultRoot)
	if err != nil {
		t.Fatalf("LoadPrecedenceStore: %v", err)
	}
	seedPrecedence(store, first)

	// A human/agent edits the page directly in the vault, after
	// longterm-mem last wrote it -- the precedence store above still
	// records the pre-edit hash.
	locallyEdited := first.Frontmatter + "Human-edited body, never written by longterm-mem.\n"
	if err := os.WriteFile(existingPath, []byte(locallyEdited), 0o644); err != nil {
		t.Fatalf("write local edit: %v", err)
	}

	obs.RevisionCount = 2
	second, err := EmitPage(obs, "c-000303", nil)
	if err != nil {
		t.Fatalf("EmitPage (re-promotion): %v", err)
	}

	action, err := UpdateInPlace(store, second, existingPath)
	if err != nil {
		t.Fatalf("UpdateInPlace: %v", err)
	}
	if action.Kind != ActionSkippedLocalEdit {
		t.Fatalf("action.Kind = %v, want ActionSkippedLocalEdit", action.Kind)
	}
	if action.Diagnostic == nil {
		t.Fatalf("action.Diagnostic = nil, want a diagnostic naming the skipped page")
	}
	if !strings.Contains(action.Diagnostic.Detail, "c-000303") {
		t.Fatalf("diagnostic %q does not name the skipped page c-000303", action.Diagnostic.Detail)
	}

	got, err := os.ReadFile(existingPath)
	if err != nil {
		t.Fatalf("read %s: %v", existingPath, err)
	}
	if string(got) != locallyEdited {
		t.Fatalf("content was modified despite a local edit; got:\n%s\nwant (unchanged local edit):\n%s", got, locallyEdited)
	}
}

// TestUpdate_UnmodifiedPageUpdatesNormally: R-030 scenario 2 (task 6.6).
// A promoted page that has not been locally modified since longterm-mem
// last wrote it: sync re-promotes it and the normal update-in-place
// behavior applies, with no skip.
func TestUpdate_UnmodifiedPageUpdatesNormally(t *testing.T) {
	vaultRoot := t.TempDir()
	fixedNow(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))

	obs := engram.Observation{ID: 304, Type: "decision", Title: "Not Locally Edited", Content: "Same content, no local edit.", Project: "labdrian-sdd-overlay", RevisionCount: 1}
	page, err := EmitPage(obs, "c-000304", nil)
	if err != nil {
		t.Fatalf("EmitPage: %v", err)
	}
	existingPath := writePromotedPage(t, vaultRoot, page)

	store, err := LoadPrecedenceStore(vaultRoot)
	if err != nil {
		t.Fatalf("LoadPrecedenceStore: %v", err)
	}
	seedPrecedence(store, page)

	// Re-promotion re-renders byte-identical content (nothing changed
	// upstream in Engram): still must not be misread as a local edit.
	action, err := UpdateInPlace(store, page, existingPath)
	if err != nil {
		t.Fatalf("UpdateInPlace: %v", err)
	}
	if action.Kind != ActionUpdated {
		t.Fatalf("action.Kind = %v, want ActionUpdated (no skip)", action.Kind)
	}
	if action.Diagnostic != nil {
		t.Fatalf("action.Diagnostic = %+v, want nil for a normal update", action.Diagnostic)
	}
}

// TestUpdate_UnknownProvenancePageIsNotOverwritten: a page exists at
// existingPath but the precedence store holds no entry for its address,
// so longterm-mem has no record of ever having written it (a lost,
// truncated or hand-deleted sidecar; a page promoted before tracking
// existed; a file a human created at that path). Provenance is unknown,
// so the write fails closed exactly like a local edit rather than
// destroying content nobody can prove longterm-mem authored (review
// findings R3-missing-entry-overwrites,
// R4-fail-open-missing-precedence-entry).
func TestUpdate_UnknownProvenancePageIsNotOverwritten(t *testing.T) {
	vaultRoot := t.TempDir()
	fixedNow(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))

	obs := engram.Observation{ID: 305, Type: "decision", Title: "Untracked On Disk", Content: "Body nobody tracked.", Project: "labdrian-sdd-overlay", RevisionCount: 1}
	existing, err := EmitPage(obs, "c-000305", nil)
	if err != nil {
		t.Fatalf("EmitPage: %v", err)
	}
	existingPath := writePromotedPage(t, vaultRoot, existing)
	onDisk := existing.Frontmatter + existing.Body

	// Deliberately NOT seeded: the store has no entry for this address.
	store := PrecedenceStore{}

	obs.RevisionCount = 2
	obs.Content = "Freshly rendered body."
	incoming, err := EmitPage(obs, "c-000305", nil)
	if err != nil {
		t.Fatalf("EmitPage (re-promotion): %v", err)
	}

	action, err := UpdateInPlace(store, incoming, existingPath)
	if err != nil {
		t.Fatalf("UpdateInPlace: %v", err)
	}
	if action.Kind != ActionSkippedLocalEdit {
		t.Fatalf("action.Kind = %v, want ActionSkippedLocalEdit for a page of unknown provenance", action.Kind)
	}
	if action.Diagnostic == nil || !strings.Contains(action.Diagnostic.Detail, "c-000305") {
		t.Fatalf("action.Diagnostic = %+v, want a diagnostic naming c-000305", action.Diagnostic)
	}

	got, err := os.ReadFile(existingPath)
	if err != nil {
		t.Fatalf("read %s: %v", existingPath, err)
	}
	if string(got) != onDisk {
		t.Fatalf("untracked page was overwritten; got:\n%s\nwant (unchanged):\n%s", got, onDisk)
	}
}

// TestUpdate_InterruptedPriorWriteReconciles: the page write and the
// precedence entry are two separate durable steps, so an interruption
// between them leaves the page holding new content while the store still
// holds the pre-update hashes. That page is not a local edit and must not
// be skipped forever: when the on-disk content is byte-identical to what
// this update would write, the entry is re-derived from disk and the
// update reported as applied (review finding
// R4-write-store-persist-crash-window).
func TestUpdate_InterruptedPriorWriteReconciles(t *testing.T) {
	vaultRoot := t.TempDir()
	fixedNow(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))

	obs := engram.Observation{ID: 306, Type: "decision", Title: "Interrupted Write", Content: "V1 body.", Project: "labdrian-sdd-overlay", RevisionCount: 1}
	first, err := EmitPage(obs, "c-000306", nil)
	if err != nil {
		t.Fatalf("EmitPage: %v", err)
	}
	existingPath := writePromotedPage(t, vaultRoot, first)

	store := PrecedenceStore{}
	seedPrecedence(store, first)

	fixedNow(t, time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC))
	obs.RevisionCount = 2
	obs.Content = "V2 body."
	second, err := EmitPage(obs, "c-000306", nil)
	if err != nil {
		t.Fatalf("EmitPage (re-promotion): %v", err)
	}

	// The previous run wrote V2 to disk and then died before its caller
	// persisted the store, so the entry still fingerprints V1.
	if err := os.WriteFile(existingPath, []byte(second.Frontmatter+second.Body), 0o644); err != nil {
		t.Fatalf("simulate interrupted write: %v", err)
	}

	action, err := UpdateInPlace(store, second, existingPath)
	if err != nil {
		t.Fatalf("UpdateInPlace: %v", err)
	}
	if action.Kind != ActionUpdated {
		t.Fatalf("action.Kind = %v, want ActionUpdated (an interrupted own write is not a local edit)", action.Kind)
	}
	entry, ok := store.Get(second.Address)
	if !ok {
		t.Fatalf("store has no entry for %s after reconciliation", second.Address)
	}
	if entry.BodyHash != hashText(second.Body) || entry.FrontmatterHash != hashText(second.Frontmatter) {
		t.Fatalf("entry = %+v, want the re-derived hashes of the on-disk content", entry)
	}
}

// TestUpdate_UnrecordedOwnWriteIsAdopted: the create path's own crash
// window (the page landed, the sidecar Save never did) leaves a page with
// NO store entry at all, so the !tracked branch refused it before the
// content-identity reconciliation below could ever look at it -- a
// permanent fixed point, because a skip also suppresses the sidecar Save
// and the catalog/log repair that would have healed it.
//
// The page's own bytes settle it: when they are what this promotion would
// itself write, the page is our interrupted write and is adopted rather
// than refused forever. The comparison must ignore created:/updated:,
// which EmitPage stamps from the wall clock -- otherwise adoption would
// work on a same-day retry and stop working the next day, which is worse
// than not adopting at all.
func TestUpdate_UnrecordedOwnWriteIsAdopted(t *testing.T) {
	vaultRoot := t.TempDir()
	fixedNow(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))

	obs := engram.Observation{ID: 307, Type: "decision", Title: "Unrecorded Own Write", Content: "Body.", Project: "labdrian-sdd-overlay", RevisionCount: 1}
	onDisk, err := EmitPage(obs, "c-000307", nil)
	if err != nil {
		t.Fatalf("EmitPage (interrupted create): %v", err)
	}
	existingPath := writePromotedPage(t, vaultRoot, onDisk)

	// Deliberately NOT seeded: the create's sidecar Save never ran.
	store := PrecedenceStore{}

	// A later day, so the re-render differs from disk in exactly the two
	// wall-clock stamps and nothing else.
	fixedNow(t, time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC))
	incoming, err := EmitPage(obs, "c-000307", nil)
	if err != nil {
		t.Fatalf("EmitPage (retry): %v", err)
	}
	if incoming.Frontmatter+incoming.Body == onDisk.Frontmatter+onDisk.Body {
		t.Fatalf("fixture is not exercising the timestamp hazard: the two renders are byte-identical")
	}

	action, err := UpdateInPlace(store, incoming, existingPath)
	if err != nil {
		t.Fatalf("UpdateInPlace: %v", err)
	}
	if action.Kind != ActionUpdated {
		t.Fatalf("action.Kind = %v, want ActionUpdated: an untracked page whose content is our own render (ignoring created:/updated:) is an interrupted write, not a page of unknown provenance", action.Kind)
	}
	entry, ok := store.Get(incoming.Address)
	if !ok {
		t.Fatalf("store has no entry for %s after adoption", incoming.Address)
	}
	if entry.BodyHash != hashText(incoming.Body) || entry.FrontmatterHash != hashText(incoming.Frontmatter) {
		t.Fatalf("entry = %+v, want the adopted page's own hashes", entry)
	}
	got, err := os.ReadFile(existingPath)
	if err != nil {
		t.Fatalf("read %s: %v", existingPath, err)
	}
	if string(got) != incoming.Frontmatter+incoming.Body {
		t.Fatalf("adopted page was not refreshed to the freshly rendered content; got:\n%s", got)
	}
}

// TestUpdate_UntrackedPageDifferingOnlyInANonVolatileFieldIsNotAdopted:
// adoption normalizes created: and updated: away, and NOTHING else. A page
// a human changed in any other frontmatter field -- here status:, the field
// R-033's own patcher writes -- is still a page of content longterm-mem
// cannot prove it wrote, and must still be refused. This pins the
// normalization's scope: widening it to "some frontmatter differences are
// fine" would silently reopen the hole
// TestUpdate_UnknownProvenancePageIsNotOverwritten guards.
func TestUpdate_UntrackedPageDifferingOnlyInANonVolatileFieldIsNotAdopted(t *testing.T) {
	vaultRoot := t.TempDir()
	fixedNow(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))

	obs := engram.Observation{ID: 308, Type: "decision", Title: "Hand Edited Status", Content: "Body.", Project: "labdrian-sdd-overlay", RevisionCount: 1}
	page, err := EmitPage(obs, "c-000308", nil)
	if err != nil {
		t.Fatalf("EmitPage: %v", err)
	}
	existingPath := writePromotedPage(t, vaultRoot, page)

	edited := strings.Replace(page.Frontmatter, "status: developing", "status: mature", 1)
	if edited == page.Frontmatter {
		t.Fatalf("fixture did not change status:; frontmatter was:\n%s", page.Frontmatter)
	}
	if err := os.WriteFile(existingPath, []byte(edited+page.Body), 0o644); err != nil {
		t.Fatalf("write hand edit: %v", err)
	}

	fixedNow(t, time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC))
	incoming, err := EmitPage(obs, "c-000308", nil)
	if err != nil {
		t.Fatalf("EmitPage (retry): %v", err)
	}

	action, err := UpdateInPlace(PrecedenceStore{}, incoming, existingPath)
	if err != nil {
		t.Fatalf("UpdateInPlace: %v", err)
	}
	if action.Kind != ActionSkippedLocalEdit {
		t.Fatalf("action.Kind = %v, want ActionSkippedLocalEdit: only created:/updated: are volatile, every other divergence is still unknown provenance", action.Kind)
	}
	got, err := os.ReadFile(existingPath)
	if err != nil {
		t.Fatalf("read %s: %v", existingPath, err)
	}
	if string(got) != edited+page.Body {
		t.Fatalf("hand-edited page was overwritten; got:\n%s", got)
	}
}

// TestActionKind_String: task 8b.11 infrastructure. cmd_promote.go's CLI
// output and the MCP promote tool's PromoteOut.Action both render an
// ActionKind through this one method instead of separately mapping its
// int encoding, so the two surfaces cannot render two different names for
// the same outcome.
func TestActionKind_String(t *testing.T) {
	tests := []struct {
		kind ActionKind
		want string
	}{
		{ActionCreated, "created"},
		{ActionUpdated, "updated"},
		{ActionSkippedLocalEdit, "skipped_local_edit"},
		{ActionKind(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.kind.String(); got != tt.want {
			t.Errorf("ActionKind(%d).String() = %q, want %q", tt.kind, got, tt.want)
		}
	}
}
