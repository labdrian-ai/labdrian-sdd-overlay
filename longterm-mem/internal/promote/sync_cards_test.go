package promote

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// fakeCards is a CardMemories that serves a fixed list and counts its calls.
type fakeCards struct {
	memories []memory.CardMemory
	err      error
	listed   []string
}

func (f *fakeCards) ListMemories(project string) ([]memory.CardMemory, error) {
	f.listed = append(f.listed, project)
	return f.memories, f.err
}

// countingPages counts how often a run lists the vault's pages.
type countingPages struct {
	PageRepository
	lists int
}

func (c *countingPages) ListPages() ([]string, error) {
	c.lists++
	return c.PageRepository.ListPages()
}

func syncCardsWriter(vaultRoot string) *Writer {
	w := cardWriter(vaultRoot, PrecedenceStore{})
	w.Addresses = &sequentialAddresses{}
	return w
}

// eligibleCard is a memory of memorycard that R-007 selects: it is pinned.
func eligibleCard(id int64, revision int) memory.CardMemory {
	return memory.CardMemory{ID: id, Project: "memorycard", Type: "decision", Title: fmt.Sprintf("Card %d", id), Body: fmt.Sprintf("Body %d.", id), Pinned: true, Revision: revision}
}

// An eligible memory that has no page is promoted, with the Writer's create path.
func TestSyncCards_PromotesAnEligibleMemoryWithNoPage(t *testing.T) {
	vaultRoot := t.TempDir()
	w := syncCardsWriter(vaultRoot)
	cards := &fakeCards{memories: []memory.CardMemory{eligibleCard(11, 1)}}

	report, err := SyncCards(t.Context(), cards, w, "memorycard")
	if err != nil {
		t.Fatalf("SyncCards: %v", err)
	}
	if len(report.Promoted) != 1 || report.Promoted[0].Action.Kind != ActionCreated || report.Skipped != 0 || report.Bridged != 0 || len(report.Failed) != 0 {
		t.Fatalf("report = %+v, want one created page and nothing else", report)
	}
	if got := pageFiles(t, vaultRoot); len(got) != 1 {
		t.Fatalf("pages = %v, want one", got)
	}
	if len(cards.listed) != 1 || cards.listed[0] != "memorycard" {
		t.Errorf("the port was asked for %v, want the project once", cards.listed)
	}
}

// A memory at the revision its page carries is a no-op: no page write and no precedence write.
func TestSyncCards_AMemoryAtItsPromotedRevisionWritesNothing(t *testing.T) {
	vaultRoot := t.TempDir()
	w := syncCardsWriter(vaultRoot)
	cards := &fakeCards{memories: []memory.CardMemory{eligibleCard(11, 2)}}
	first, err := SyncCards(t.Context(), cards, w, "memorycard")
	if err != nil || len(first.Promoted) != 1 {
		t.Fatalf("first run = %+v, %v", first, err)
	}
	address := first.Promoted[0].Page.Address
	before := readPage(t, vaultRoot, address)
	precedence := &memPrecedence{}
	w.Precedence = precedence

	again, err := SyncCards(t.Context(), cards, w, "memorycard")
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if len(again.Promoted) != 0 || again.Skipped != 1 {
		t.Fatalf("second report = %+v, want nothing promoted and the memory skipped", again)
	}
	if readPage(t, vaultRoot, address) != before {
		t.Errorf("the page was written")
	}
	if precedence.saves != 0 {
		t.Errorf("the precedence store was saved %d times, want 0", precedence.saves)
	}
	if got := pageFiles(t, vaultRoot); len(got) != 1 {
		t.Errorf("pages = %v, want one", got)
	}
}

// A memory at a newer revision than its page updates that page in place, never a second one.
func TestSyncCards_ANewerRevisionUpdatesItsPageInPlace(t *testing.T) {
	vaultRoot := t.TempDir()
	w := syncCardsWriter(vaultRoot)
	first, err := SyncCards(t.Context(), &fakeCards{memories: []memory.CardMemory{eligibleCard(11, 1)}}, w, "memorycard")
	if err != nil || len(first.Promoted) != 1 {
		t.Fatalf("first run = %+v, %v", first, err)
	}
	address := first.Promoted[0].Page.Address

	newer := eligibleCard(11, 2)
	newer.Body = "A new version."
	report, err := SyncCards(t.Context(), &fakeCards{memories: []memory.CardMemory{newer}}, w, "memorycard")
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if len(report.Promoted) != 1 || report.Promoted[0].Action.Kind != ActionUpdated || report.Promoted[0].Page.Address != address {
		t.Fatalf("report = %+v, want one update of %s", report, address)
	}
	if got := pageFiles(t, vaultRoot); len(got) != 1 {
		t.Fatalf("pages = %v, want one", got)
	}
	if page := readPage(t, vaultRoot, address); !strings.Contains(page, "memorycard_revision: 2\n") || !strings.Contains(page, "A new version.") {
		t.Errorf("page not brought to revision 2:\n%s", page)
	}
}

// A memory that R-007 does not select (not pinned, no curated topic_key) is skipped and counted, and no page exists for it.
func TestSyncCards_AnIneligibleMemoryIsSkipped(t *testing.T) {
	vaultRoot := t.TempDir()
	w := syncCardsWriter(vaultRoot)
	note := eligibleCard(12, 1)
	note.Pinned, note.TopicKey = false, "sdd/some-change/state"
	cards := &fakeCards{memories: []memory.CardMemory{note, eligibleCard(13, 1)}}

	report, err := SyncCards(t.Context(), cards, w, "memorycard")
	if err != nil {
		t.Fatalf("SyncCards: %v", err)
	}
	if len(report.Promoted) != 1 || report.Skipped != 1 || report.Bridged != 0 || len(report.Failed) != 0 {
		t.Fatalf("report = %+v, want memory 13 promoted and memory 12 skipped", report)
	}
	if got := pageFiles(t, vaultRoot); len(got) != 1 {
		t.Fatalf("pages = %v, want one", got)
	}
}

// engramPageIn writes the page of Engram observation id into the vault and records it in store, as a promoted
// Engram page is, and returns its path.
func engramPageIn(t *testing.T, vaultRoot string, store PrecedenceStore, id int64, address string) string {
	t.Helper()
	obs := memory.Observation{ID: id, Type: "pattern", Title: "Old Engram Page", Content: "Engram.", Project: "memorycard", RevisionCount: 2, TopicKey: "x/y"}
	page, err := EmitPage(obs, address, nil, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	seedPrecedence(store, page)
	return writePromotedPage(t, vaultRoot, page)
}

// A bridged memory, by import_map or by the same content, is skipped and counted. It is no failure, it writes
// nothing, the run goes on to the memory after it, and the Engram page stays as it was.
func TestSyncCards_ABridgedMemoryIsSkippedAndCountedNeverFailed(t *testing.T) {
	vaultRoot := t.TempDir()
	w := syncCardsWriter(vaultRoot)
	full := engramPageIn(t, vaultRoot, w.Store, 402, "c-000402")
	before, _ := os.ReadFile(full)

	byBridge := eligibleCard(21, 1)
	byBridge.EngramID = 402
	byContent := eligibleCard(22, 1)
	byContent.SameContentEngramIDs = []int64{401, 402}
	cards := &fakeCards{memories: []memory.CardMemory{byBridge, byContent, eligibleCard(23, 1)}}

	report, err := SyncCards(t.Context(), cards, w, "memorycard")
	if err != nil {
		t.Fatalf("SyncCards returned %v, want nil: a bridged memory is not a failure", err)
	}
	if report.Bridged != 2 || len(report.Failed) != 0 || len(report.Promoted) != 1 || report.Promoted[0].Action.Kind != ActionCreated {
		t.Fatalf("report = %+v, want 2 bridged, no failure and memory 23 created", report)
	}
	if got := pageFiles(t, vaultRoot); len(got) != 2 {
		t.Fatalf("pages = %v, want the Engram page and memory 23's page only", got)
	}
	if after, _ := os.ReadFile(full); string(after) != string(before) {
		t.Errorf("the Engram page changed")
	}
}

// A failure that is not a refusal stays a failure: the run names the memory and goes on, as Sync does.
func TestSyncCards_AFailureIsNamedAndTheRunGoesOn(t *testing.T) {
	vaultRoot := t.TempDir()
	w := syncCardsWriter(vaultRoot)
	first, err := SyncCards(t.Context(), &fakeCards{memories: []memory.CardMemory{eligibleCard(11, 1)}}, w, "memorycard")
	if err != nil || len(first.Promoted) != 1 {
		t.Fatalf("first run = %+v, %v", first, err)
	}
	// A page of memory 11 whose revision cannot be read: the memory is a failure, memory 14 is still promoted.
	path := filepath.Join(vaultRoot, "wiki", "memory", first.Promoted[0].Page.Address+".md")
	raw, _ := os.ReadFile(path)
	broken := []byte(strings.Replace(string(raw), "memorycard_revision: 1", "memorycard_revision: one", 1))
	if err := os.WriteFile(path, broken, 0o644); err != nil {
		t.Fatal(err)
	}

	report, err := SyncCards(t.Context(), &fakeCards{memories: []memory.CardMemory{eligibleCard(11, 2), eligibleCard(14, 1)}}, w, "memorycard")
	if err == nil {
		t.Fatal("SyncCards returned nil with a failed memory")
	}
	if len(report.Failed) != 1 || report.Failed[0].ObservationID != 11 || len(report.Promoted) != 1 || report.Bridged != 0 {
		t.Fatalf("report = %+v, want memory 11 failed and memory 14 promoted", report)
	}
	var bridged *BridgedError
	if errors.As(err, &bridged) {
		t.Errorf("err = %v, must not be a bridged refusal", err)
	}
}

// A run reads the vault's pages once, however many memories it decides, not once per memory.
func TestSyncCards_ReadsThePagesOncePerRun(t *testing.T) {
	vaultRoot := t.TempDir()
	w := syncCardsWriter(vaultRoot)
	if _, err := SyncCards(t.Context(), &fakeCards{memories: []memory.CardMemory{eligibleCard(11, 1)}}, w, "memorycard"); err != nil {
		t.Fatal(err)
	}
	counting := &countingPages{PageRepository: pagesIn(vaultRoot)}
	w.Pages = counting
	// Imported from an observation that has no page: it is no bridge, so it is promoted.
	imported := eligibleCard(31, 1)
	imported.EngramID = 999
	cards := &fakeCards{memories: []memory.CardMemory{eligibleCard(11, 2), eligibleCard(12, 1), eligibleCard(13, 1), eligibleCard(14, 1), imported}}

	report, err := SyncCards(t.Context(), cards, w, "memorycard")
	if err != nil {
		t.Fatalf("SyncCards: %v", err)
	}
	if len(report.Promoted) != 5 {
		t.Fatalf("report = %+v, want one update and four creations", report)
	}
	if counting.lists != 1 {
		t.Fatalf("the run listed the pages %d times, want once", counting.lists)
	}
}

// A page the run created is seen by the memories after it: the successor link of a superseded memory names it.
func TestSyncCards_ALinkNamesAPageCreatedEarlierInTheRun(t *testing.T) {
	vaultRoot := t.TempDir()
	w := syncCardsWriter(vaultRoot)
	successor := eligibleCard(41, 1)
	old := eligibleCard(42, 1)
	old.Status, old.SupersededBy, old.SupersededByTitle = "superseded", 41, successor.Title

	report, err := SyncCards(t.Context(), &fakeCards{memories: []memory.CardMemory{successor, old}}, w, "memorycard")
	if err != nil || len(report.Promoted) != 2 {
		t.Fatalf("report = %+v, %v", report, err)
	}
	page := readPage(t, vaultRoot, report.Promoted[1].Page.Address)
	if want := "[[" + report.Promoted[0].Page.Address; !strings.Contains(page, want) {
		t.Errorf("the superseded page does not link its successor %s:\n%s", want, page)
	}
}
