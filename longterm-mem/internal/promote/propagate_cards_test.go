package promote

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vaultlayout"
)

// The tests of status propagation for memorycard pages (S1 e of the C22-FU follow-up): a promoted memory that
// memorycard has since superseded or archived has its page's status and related patched, frontmatter only.

// cardWorld promotes the given active memories into a fresh vault through SyncCards and returns the writer, the
// vault and the address of each page by memory id.
func cardWorld(t *testing.T, memories ...memory.CardMemory) (*Writer, *memPrecedence, string, map[int64]string) {
	t.Helper()
	vaultRoot := t.TempDir()
	w := syncCardsWriter(vaultRoot)
	precedence := &memPrecedence{}
	w.Precedence = precedence
	report, err := SyncCards(t.Context(), &fakeCards{memories: memories}, w, "memorycard")
	if err != nil || len(report.Promoted) != len(memories) {
		t.Fatalf("setup run = %+v, %v", report, err)
	}
	addresses := map[int64]string{}
	for i, m := range memories {
		addresses[m.ID] = report.Promoted[i].Page.Address
	}
	precedence.saves = 0
	return w, precedence, vaultRoot, addresses
}

// bodyOf is the page's text after its frontmatter block.
func bodyOf(t *testing.T, page string) string {
	t.Helper()
	block, ok := frontmatterBlock(page)
	if !ok {
		t.Fatalf("page has no frontmatter block:\n%s", page)
	}
	return page[len(block):]
}

// supersededBy is old, retired in favor of successor.
func supersededBy(old, successor memory.CardMemory) memory.CardMemory {
	old.Status, old.SupersededBy, old.SupersededByTitle = "superseded", successor.ID, successor.Title
	return old
}

// A superseded memory's page becomes superseded, its related list names the successor's page in the form
// EmitCardPage renders, and the body is not touched: the patched frontmatter is exactly the one a render of the
// superseded memory would give.
func TestPropagateCards_ASupersededMemorysPageBecomesSupersededAndLinksItsSuccessor(t *testing.T) {
	successor, old := eligibleCard(41, 1), eligibleCard(42, 1)
	w, _, vaultRoot, address := cardWorld(t, successor, old)
	before := readPage(t, vaultRoot, address[42])
	if !strings.Contains(before, "status: mature\n") {
		t.Fatalf("setup page is not current:\n%s", before)
	}
	retired := supersededBy(old, successor)

	report, err := PropagateCards(t.Context(), &fakeCards{memories: []memory.CardMemory{successor, retired}}, w, "memorycard")
	if err != nil {
		t.Fatalf("PropagateCards: %v", err)
	}
	if len(report.Patched) != 1 || report.Patched[0] != address[42] || len(report.Failed) != 0 || len(report.Withheld) != 0 {
		t.Fatalf("report = %+v, want only %s patched", report, address[42])
	}
	after := readPage(t, vaultRoot, address[42])
	want, err := EmitCardPage(retired, address[42], []Link{{Address: address[41], Title: successor.Title}}, utc(w.Clock))
	if err != nil {
		t.Fatal(err)
	}
	if block, _ := frontmatterBlock(after); block != want.Frontmatter {
		t.Errorf("patched frontmatter:\n%s\nwant the render of the superseded memory:\n%s", block, want.Frontmatter)
	}
	if bodyOf(t, after) != bodyOf(t, before) {
		t.Errorf("the body changed:\n%s\nwas:\n%s", bodyOf(t, after), bodyOf(t, before))
	}
	if page := readPage(t, vaultRoot, address[41]); !strings.Contains(page, "status: mature\n") {
		t.Errorf("the successor's page was patched:\n%s", page)
	}
}

// A superseded memory whose successor has no page of its own is still marked superseded, with related left empty.
func TestPropagateCards_ASupersededMemoryWhoseSuccessorHasNoPageStillLandsItsStatus(t *testing.T) {
	old := eligibleCard(42, 1)
	w, _, vaultRoot, address := cardWorld(t, old)
	retired := supersededBy(old, eligibleCard(77, 1))

	report, err := PropagateCards(t.Context(), &fakeCards{memories: []memory.CardMemory{retired}}, w, "memorycard")
	if err != nil || len(report.Patched) != 1 {
		t.Fatalf("report = %+v, %v, want the page patched", report, err)
	}
	page := readPage(t, vaultRoot, address[42])
	if !strings.Contains(page, "status: superseded\n") || !strings.Contains(page, "related: []\n") {
		t.Errorf("page:\n%s", page)
	}
}

// An archived memory's page becomes archived.
func TestPropagateCards_AnArchivedMemorysPageBecomesArchived(t *testing.T) {
	gone := eligibleCard(43, 1)
	w, _, vaultRoot, address := cardWorld(t, gone)
	gone.Status = "archived"

	report, err := PropagateCards(t.Context(), &fakeCards{memories: []memory.CardMemory{gone}}, w, "memorycard")
	if err != nil || len(report.Patched) != 1 || report.Patched[0] != address[43] {
		t.Fatalf("report = %+v, %v, want %s patched", report, err, address[43])
	}
	if page := readPage(t, vaultRoot, address[43]); !strings.Contains(page, "status: archived\n") || !strings.Contains(page, "related: []\n") {
		t.Errorf("page:\n%s", page)
	}
}

// A page edited by hand still gets the status patch, and only that: its body, the human's edit included, stays
// byte-identical, and the precedence entry moves only its frontmatter hash, so the divergence of the body is
// not erased.
func TestPropagateCards_ALocallyEditedPageGetsItsFrontmatterPatchedAndItsBodyUntouched(t *testing.T) {
	gone := eligibleCard(43, 1)
	w, precedence, vaultRoot, address := cardWorld(t, gone)
	path := filepath.Join(vaultRoot, vaultlayout.PageFile(address[43]))
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	edited := string(raw) + "\nA human added this.\n"
	if err := os.WriteFile(path, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	entryBefore, ok := w.Store.Get(address[43])
	if !ok || entryBefore.BodyHash == "" {
		t.Fatalf("setup precedence entry = %+v, %v, want one with a body hash", entryBefore, ok)
	}
	gone.Status = "archived"

	if _, err := PropagateCards(t.Context(), &fakeCards{memories: []memory.CardMemory{gone}}, w, "memorycard"); err != nil {
		t.Fatalf("PropagateCards: %v", err)
	}
	after := readPage(t, vaultRoot, address[43])
	if !strings.Contains(after, "status: archived\n") {
		t.Errorf("status not patched:\n%s", after)
	}
	if bodyOf(t, after) != bodyOf(t, edited) || !strings.Contains(after, "A human added this.") {
		t.Errorf("the edited body changed:\n%s", after)
	}
	entry, _ := w.Store.Get(address[43])
	block, _ := frontmatterBlock(after)
	if entry.BodyHash != entryBefore.BodyHash {
		t.Errorf("body_hash = %q, want it kept as %q", entry.BodyHash, entryBefore.BodyHash)
	}
	if entry.FrontmatterHash != hashText(block) || entry.FrontmatterHash == entryBefore.FrontmatterHash {
		t.Errorf("frontmatter_hash = %q, want the hash of the patched block %q", entry.FrontmatterHash, hashText(block))
	}
	persisted, _ := precedence.persisted.Get(address[43])
	if precedence.saves != 1 || persisted != entry {
		t.Errorf("saves = %d, persisted %+v, want one save of %+v", precedence.saves, persisted, entry)
	}
}

// The precedence store is saved once for a run that patched several pages, and not at all for one that patched none.
func TestPropagateCards_SavesThePrecedenceStoreOnlyWhenItPatchedSomething(t *testing.T) {
	first, second := eligibleCard(43, 1), eligibleCard(44, 1)
	w, precedence, _, _ := cardWorld(t, first, second)

	if _, err := PropagateCards(t.Context(), &fakeCards{memories: []memory.CardMemory{first, second}}, w, "memorycard"); err != nil {
		t.Fatal(err)
	}
	if precedence.saves != 0 {
		t.Fatalf("saves = %d for a run with nothing retired, want 0", precedence.saves)
	}
	first.Status, second.Status = "archived", "archived"
	report, err := PropagateCards(t.Context(), &fakeCards{memories: []memory.CardMemory{first, second}}, w, "memorycard")
	if err != nil || len(report.Patched) != 2 {
		t.Fatalf("report = %+v, %v", report, err)
	}
	if precedence.saves != 1 {
		t.Errorf("saves = %d for two patched pages, want 1", precedence.saves)
	}
}

// A retired memory that has no page of its own is left alone: nothing is written, and it is neither a patch nor a
// failure.
func TestPropagateCards_ARetiredMemoryWithNoPageIsLeftAlone(t *testing.T) {
	w, precedence, vaultRoot, _ := cardWorld(t, eligibleCard(43, 1))
	stranger := eligibleCard(50, 1)
	stranger.Status = "archived"
	before := pageFiles(t, vaultRoot)

	report, err := PropagateCards(t.Context(), &fakeCards{memories: []memory.CardMemory{stranger}}, w, "memorycard")
	if err != nil || len(report.Patched) != 0 || len(report.Failed) != 0 || len(report.Withheld) != 0 {
		t.Fatalf("report = %+v, %v, want nothing", report, err)
	}
	if got := pageFiles(t, vaultRoot); len(got) != len(before) || precedence.saves != 0 {
		t.Errorf("pages %v (was %v), saves %d: a write happened", got, before, precedence.saves)
	}
}

// A retired memory with no readable page while a page cannot be read is withheld, as the Engram pass withholds
// an observation: the unreadable page may be its own.
func TestPropagateCards_AnUnreadablePageWithholdsAMemoryWithNoPage(t *testing.T) {
	vaultRoot := t.TempDir()
	w := syncCardsWriter(vaultRoot)
	pages := filepath.Join(vaultRoot, vaultlayout.PagesDir)
	if err := os.MkdirAll(pages, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(vaultRoot, "nowhere.md"), filepath.Join(pages, "c-000555.md")); err != nil {
		t.Skipf("cannot make a link: %v", err)
	}
	gone := eligibleCard(43, 1)
	gone.Status = "archived"

	report, err := PropagateCards(t.Context(), &fakeCards{memories: []memory.CardMemory{gone}}, w, "memorycard")
	if err != nil {
		t.Fatalf("PropagateCards: %v", err)
	}
	if len(report.Withheld) != 1 || report.Withheld[0] != 43 || len(report.Patched) != 0 || len(report.Failed) != 0 {
		t.Fatalf("report = %+v, want memory 43 withheld", report)
	}
}

// unwritablePage is a page repository that cannot write the page at one address.
type unwritablePage struct {
	PageRepository
	address string
}

func (u unwritablePage) WritePage(address, content string) error {
	if address == u.address {
		return errors.New("disk full")
	}
	return u.PageRepository.WritePage(address, content)
}

// One page that cannot be patched is a failure named by memory, and the run goes on to the next.
func TestPropagateCards_AFailedPatchIsNamedAndTheRunGoesOn(t *testing.T) {
	first, second := eligibleCard(43, 1), eligibleCard(44, 1)
	w, _, _, address := cardWorld(t, first, second)
	w.Pages = unwritablePage{PageRepository: w.Pages, address: address[43]}
	first.Status, second.Status = "archived", "archived"

	report, err := PropagateCards(t.Context(), &fakeCards{memories: []memory.CardMemory{first, second}}, w, "memorycard")
	if err == nil {
		t.Fatal("PropagateCards returned nil with a failed page")
	}
	if len(report.Failed) != 1 || report.Failed[0].ObservationID != 43 || len(report.Patched) != 1 || report.Patched[0] != address[44] {
		t.Fatalf("report = %+v, want memory 43 failed and %s patched", report, address[44])
	}
}

// A memorycard that cannot list its retired memories is the typed listing failure, no patch and no panic.
func TestPropagateCards_AListingFailureIsTheTypedError(t *testing.T) {
	w, _, _, _ := cardWorld(t, eligibleCard(43, 1))
	cards := &fakeCards{allErr: errors.New("store gone")}
	_, err := PropagateCards(t.Context(), cards, w, "memorycard")
	var listed *CardListError
	if !errors.As(err, &listed) {
		t.Fatalf("err = %v, want a CardListError", err)
	}
}

// SyncCardsProject propagates after promoting: a memory superseded in the same run that promoted both it and its
// successor gets the link the promotion could not write (the superseded memory is listed first, so its page was
// created before its successor's), and the run reports the patch, rebuilds and records once.
func TestSyncCardsProject_PropagatesAfterThePromotionPass(t *testing.T) {
	vaultRoot := t.TempDir()
	w := syncCardsWriter(vaultRoot)
	state := &memSyncState{}
	w.SyncState = state
	index := &countingRebuilder{}
	old, successor := eligibleCard(41, 1), eligibleCard(42, 1)
	old = supersededBy(old, successor)

	report, err := SyncCardsProject(t.Context(), &fakeCards{memories: []memory.CardMemory{old, successor}}, w, index, "memorycard")
	if err != nil || report.Failure != NotFailed {
		t.Fatalf("SyncCardsProject = %+v, %v, want a clean run", report, err)
	}
	if len(report.Promoted) != 2 || len(report.Patched) != 1 {
		t.Fatalf("report = %+v, want two promoted and the superseded page patched", report)
	}
	page := readPage(t, vaultRoot, report.Promoted[0].Page.Address)
	if want := "[[" + report.Promoted[1].Page.Address + "|" + successor.Title + "]]"; !strings.Contains(page, want) {
		t.Errorf("the superseded page lacks the link %s:\n%s", want, page)
	}
	if index.calls != 1 || len(state.recorded) != 1 {
		t.Errorf("rebuilds %d, records %d, want 1 of each", index.calls, len(state.recorded))
	}
}

// A page the propagation pass cannot patch is a failed memory of the run: it is in the report, the run is an
// OtherFailure, and the rebuild and the record still happen, as for a memory the promotion pass could not write.
func TestSyncCardsProject_AFailedPatchIsAFailedMemoryAndStillRebuildsAndRecords(t *testing.T) {
	gone := eligibleCard(43, 1)
	w, _, _, address := cardWorld(t, gone)
	w.Pages = unwritablePage{PageRepository: w.Pages, address: address[43]}
	state := &memSyncState{}
	w.SyncState = state
	index := &countingRebuilder{}
	gone.Status = "archived"

	report, err := SyncCardsProject(t.Context(), &fakeCards{memories: []memory.CardMemory{gone}}, w, index, "memorycard")
	if err == nil || report.Failure != OtherFailure {
		t.Fatalf("err = %v, failure = %v, want OtherFailure", err, report.Failure)
	}
	if len(report.Failed) != 1 || report.Failed[0].ObservationID != 43 || len(report.Patched) != 0 {
		t.Errorf("report = %+v, want memory 43 failed", report)
	}
	if index.calls != 1 || len(state.recorded) != 1 {
		t.Errorf("rebuilds %d, records %d, want 1 of each", index.calls, len(state.recorded))
	}
}

// A retired memory withheld by the propagation pass is in the run's report, so the command answers exit 6 for it.
func TestSyncCardsProject_AMemoryWithheldByThePropagationIsInTheReport(t *testing.T) {
	vaultRoot := t.TempDir()
	w := syncCardsWriter(vaultRoot)
	pages := filepath.Join(vaultRoot, vaultlayout.PagesDir)
	if err := os.MkdirAll(pages, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(vaultRoot, "nowhere.md"), filepath.Join(pages, "c-000555.md")); err != nil {
		t.Skipf("cannot make a link: %v", err)
	}
	gone := eligibleCard(43, 1)
	gone.Status = "archived"

	report, err := SyncCardsProject(t.Context(), &fakeCards{memories: []memory.CardMemory{gone}}, w, &countingRebuilder{}, "memorycard")
	if err != nil {
		t.Fatalf("SyncCardsProject: %v", err)
	}
	if len(report.Withheld) != 1 || report.Withheld[0] != 43 {
		t.Errorf("report = %+v, want memory 43 withheld", report)
	}
}

// A memorycard that cannot list its memories for propagation makes the run an unavailable memorycard, but the
// promotion pass already wrote pages, so the index is still rebuilt and the sync recorded (R-031), as Sync does
// when a pass fails.
func TestSyncCardsProject_AnUnlistablePropagationSetIsUnavailableButStillRebuildsAndRecords(t *testing.T) {
	w := syncCardsWriter(t.TempDir())
	state := &memSyncState{}
	w.SyncState = state
	index := &countingRebuilder{}
	cards := &fakeCards{memories: []memory.CardMemory{eligibleCard(11, 1)}, allErr: errors.New("store gone")}

	report, err := SyncCardsProject(t.Context(), cards, w, index, "memorycard")
	if err == nil || report.Failure != MemoryUnavailable {
		t.Fatalf("err = %v, failure = %v, want MemoryUnavailable", err, report.Failure)
	}
	if len(report.Promoted) != 1 || index.calls != 1 || len(state.recorded) != 1 {
		t.Errorf("promoted %d, rebuilds %d, records %d, want 1 of each", len(report.Promoted), index.calls, len(state.recorded))
	}
}

// The walk is change-aware: a page that already says what its memory says is not visited, not counted and not
// written, on the run and on the dry run alike.
func TestPropagateCards_APageThatAlreadySaysWhatItsMemorySaysIsNotVisited(t *testing.T) {
	gone := eligibleCard(43, 1)
	w, precedence, vaultRoot, address := cardWorld(t, gone)
	gone.Status = "archived"
	cards := &fakeCards{memories: []memory.CardMemory{gone}}
	if report, err := PropagateCards(t.Context(), cards, w, "memorycard"); err != nil || len(report.Patched) != 1 {
		t.Fatalf("first run = %+v, %v", report, err)
	}
	before, saves := readPage(t, vaultRoot, address[43]), precedence.saves

	plan, err := PlanCards(t.Context(), cards, w, "memorycard")
	if err != nil || plan.WouldPatch != 0 || len(plan.PatchAddresses) != 0 {
		t.Fatalf("plan = %+v, %v, want nothing to patch", plan, err)
	}
	report, err := PropagateCards(t.Context(), cards, w, "memorycard")
	if err != nil || len(report.Patched) != 0 {
		t.Fatalf("second run = %+v, %v, want nothing patched", report, err)
	}
	if readPage(t, vaultRoot, address[43]) != before || precedence.saves != saves {
		t.Errorf("a page that was already right was written")
	}
}

// A memory that memorycard returned to active brings its page back from superseded or archived to the status and
// related a render of the active memory gives, and nothing else of the page changes.
func TestPropagateCards_ARestoredMemorysPageComesBackFromArchived(t *testing.T) {
	back := eligibleCard(43, 1)
	w, _, vaultRoot, address := cardWorld(t, back)
	gone := back
	gone.Status = "archived"
	if _, err := PropagateCards(t.Context(), &fakeCards{memories: []memory.CardMemory{gone}}, w, "memorycard"); err != nil {
		t.Fatal(err)
	}
	archived := readPage(t, vaultRoot, address[43])
	if !strings.Contains(archived, "status: archived\n") {
		t.Fatalf("setup page is not archived:\n%s", archived)
	}

	report, err := PropagateCards(t.Context(), &fakeCards{memories: []memory.CardMemory{back}}, w, "memorycard")
	if err != nil || len(report.Patched) != 1 || report.Patched[0] != address[43] {
		t.Fatalf("report = %+v, %v, want %s patched", report, err, address[43])
	}
	after := readPage(t, vaultRoot, address[43])
	want, err := EmitCardPage(back, address[43], nil, utc(w.Clock))
	if err != nil {
		t.Fatal(err)
	}
	if block, _ := frontmatterBlock(after); block != want.Frontmatter {
		t.Errorf("restored frontmatter:\n%s\nwant the render of the active memory:\n%s", block, want.Frontmatter)
	}
	if bodyOf(t, after) != bodyOf(t, archived) {
		t.Errorf("the body changed")
	}
}

// An active memory's page whose status a human set by hand (it is neither superseded nor archived) is left alone:
// propagation restores a page from a retirement, it does not police the status of a page memorycard never retired.
func TestPropagateCards_AnActiveMemorysHandSetStatusIsLeftAlone(t *testing.T) {
	m := eligibleCard(43, 1)
	w, precedence, vaultRoot, address := cardWorld(t, m)
	path := filepath.Join(vaultRoot, vaultlayout.PageFile(address[43]))
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(raw), "status: mature\n", "status: seed\n", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	before := readPage(t, vaultRoot, address[43])

	report, err := PropagateCards(t.Context(), &fakeCards{memories: []memory.CardMemory{m}}, w, "memorycard")
	if err != nil || len(report.Patched) != 0 {
		t.Fatalf("report = %+v, %v, want nothing patched", report, err)
	}
	if readPage(t, vaultRoot, address[43]) != before || precedence.saves != 0 {
		t.Errorf("a hand-set status was rewritten")
	}
}

// A precedence store that cannot be persisted names the pass that patched the pages, the card pass here.
func TestPropagateCards_AFailedPrecedenceSaveNamesTheCardPass(t *testing.T) {
	gone := eligibleCard(43, 1)
	w, precedence, _, _ := cardWorld(t, gone)
	precedence.saveErr = errTestSave
	gone.Status = "archived"

	_, err := PropagateCards(t.Context(), &fakeCards{memories: []memory.CardMemory{gone}}, w, "memorycard")
	if err == nil || !strings.Contains(err.Error(), "promote: propagate memorycard: persist precedence") || !errors.Is(err, errTestSave) {
		t.Fatalf("err = %v, want it to name the memorycard pass and wrap the save error", err)
	}
}

// The plan counts the patches the run would make, names the pages, and writes nothing; the run agrees.
func TestPlanCards_CountsTheWouldPatchPagesAndWritesNothing(t *testing.T) {
	first, second, kept := eligibleCard(43, 1), eligibleCard(44, 1), eligibleCard(45, 1)
	w, precedence, vaultRoot, address := cardWorld(t, first, second, kept)
	first.Status, second.Status = "archived", "archived"
	cards := &fakeCards{memories: []memory.CardMemory{first, second, kept}}
	before := map[int64]string{43: readPage(t, vaultRoot, address[43]), 44: readPage(t, vaultRoot, address[44])}

	plan, err := PlanCards(t.Context(), cards, w, "memorycard")
	if err != nil {
		t.Fatalf("PlanCards: %v", err)
	}
	if plan.WouldPatch != 2 || len(plan.PatchAddresses) != 2 || plan.PatchAddresses[0] != address[43] || plan.PatchAddresses[1] != address[44] {
		t.Fatalf("plan = %+v, want the two archived pages named", plan)
	}
	if readPage(t, vaultRoot, address[43]) != before[43] || readPage(t, vaultRoot, address[44]) != before[44] || precedence.saves != 0 {
		t.Errorf("PlanCards wrote")
	}

	report, err := PropagateCards(t.Context(), cards, w, "memorycard")
	if err != nil || len(report.Patched) != plan.WouldPatch {
		t.Errorf("run %+v, %v disagrees with plan %+v", report, err, plan)
	}
}

// A listing of the retired memories that fails is no plan, as the promotion pass's listing failing is none.
func TestPlanCards_AnUnlistableRetiredSetIsAnError(t *testing.T) {
	cards := &fakeCards{allErr: errors.New("store gone")}
	_, err := PlanCards(t.Context(), cards, syncCardsWriter(t.TempDir()), "memorycard")
	var listed *CardListError
	if !errors.As(err, &listed) {
		t.Fatalf("err = %v, want a CardListError", err)
	}
}
