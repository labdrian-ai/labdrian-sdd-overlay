package promote

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vaultlayout"
)

func cardWriter(vaultRoot string, store PrecedenceStore) *Writer {
	clock := &fakeClock{at: time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC)}
	return &Writer{Index: indexIn(vaultRoot), Log: logIn(vaultRoot), SyncState: syncStateIn(vaultRoot), Pages: pagesIn(vaultRoot), Store: store, Precedence: &memPrecedence{}, Clock: clock, AddressMap: &memAddressMap{}, Addresses: staticAddress(testAddress)}
}

func cardMemory() memory.CardMemory {
	return memory.CardMemory{ID: 7, Project: "memorycard", Type: "decision", Title: "Card Decision", Body: "A memory that lives in memorycard.", Revision: 3}
}

func pageFiles(t *testing.T, vaultRoot string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(vaultRoot, vaultlayout.PagesDir))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// A memorycard promote creates one page that names the memory, and no engram_id.
func TestPromoteCard_CreatesOnePage(t *testing.T) {
	vaultRoot := t.TempDir()
	w := cardWriter(vaultRoot, PrecedenceStore{})

	result, err := w.PromoteCard(t.Context(), cardMemory())
	if err != nil {
		t.Fatalf("PromoteCard: %v", err)
	}
	if result.Action.Kind != ActionCreated {
		t.Fatalf("Action.Kind = %v, want created", result.Action.Kind)
	}
	if got := pageFiles(t, vaultRoot); len(got) != 1 {
		t.Fatalf("pages = %v, want exactly one", got)
	}
	raw, err := os.ReadFile(filepath.Join(vaultRoot, result.Page.Path))
	if err != nil {
		t.Fatal(err)
	}
	fm, _ := frontmatterBlock(string(raw))
	fields := parseFrontmatterFields(fm)
	for key, want := range map[string]string{"memorycard_id": "7", "memorycard_revision": "3", "memorycard_type": "decision", "project": "memorycard", "address": testAddress} {
		if fields[key] != want {
			t.Errorf("frontmatter %s = %q, want %q", key, fields[key], want)
		}
	}
	if _, has := fields[engramIDField]; has {
		t.Errorf("a memorycard page must not carry engram_id; frontmatter:\n%s", fm)
	}
	if !strings.Contains(string(raw), "A memory that lives in memorycard.") {
		t.Errorf("page lacks the memory body:\n%s", raw)
	}
	for _, d := range LintPage(result.Page, nil, nil, nil) {
		switch d.Rule {
		case "required-fields", "enum", "address-format":
			t.Errorf("lint %s: %s", d.Rule, d.Detail)
		}
	}
}

// Promoting the same memory twice leaves one page, and the second promote writes nothing.
func TestPromoteCard_TwiceLeavesOnePage(t *testing.T) {
	vaultRoot := t.TempDir()
	w := cardWriter(vaultRoot, PrecedenceStore{})
	first, err := w.PromoteCard(t.Context(), cardMemory())
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(vaultRoot, first.Page.Path))

	// A second promote asks for no address: it must not need an allocator.
	again := cardWriter(vaultRoot, w.Store)
	again.Addresses, again.AddressMap = nil, nil
	changed := cardMemory() // the same revision: nothing newer to write
	changed.Body = "Edited since."
	second, err := again.PromoteCard(t.Context(), changed)
	if err != nil {
		t.Fatalf("second PromoteCard: %v", err)
	}
	if second.Action.Kind != ActionAlreadyPromoted || second.Page.Address != first.Page.Address {
		t.Fatalf("second = %v at %q, want already_promoted at %q", second.Action.Kind, second.Page.Address, first.Page.Address)
	}
	if got := pageFiles(t, vaultRoot); len(got) != 1 {
		t.Fatalf("pages = %v, want one", got)
	}
	after, _ := os.ReadFile(filepath.Join(vaultRoot, first.Page.Path))
	if string(before) != string(after) {
		t.Fatalf("the existing page was written by the second promote")
	}
}

// A memorycard page is updated in place when the memory's revision is newer, and an equal revision writes nothing.
func TestPromoteCard_NewerRevisionUpdatesThePageInPlace(t *testing.T) {
	vaultRoot := t.TempDir()
	w := cardWriter(vaultRoot, PrecedenceStore{})
	first, err := w.PromoteCard(t.Context(), cardMemory())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(vaultRoot, first.Page.Path)

	newer := cardMemory()
	newer.Revision, newer.Body = 4, "A new version of the memory."
	again := cardWriter(vaultRoot, w.Store)
	again.Precedence = w.Precedence
	again.Addresses, again.AddressMap = nil, nil
	second, err := again.PromoteCard(t.Context(), newer)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if second.Action.Kind != ActionUpdated || second.Page.Address != first.Page.Address {
		t.Fatalf("second = %v at %q, want updated at %q", second.Action.Kind, second.Page.Address, first.Page.Address)
	}
	if got := pageFiles(t, vaultRoot); len(got) != 1 {
		t.Fatalf("pages = %v, want one", got)
	}
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), "memorycard_revision: 4\n") || !strings.Contains(string(raw), "A new version of the memory.") {
		t.Fatalf("page not brought to revision 4:\n%s", raw)
	}

	before := raw
	third, err := again.PromoteCard(t.Context(), newer)
	if err != nil || third.Action.Kind != ActionAlreadyPromoted {
		t.Fatalf("third = %v, %v, want already_promoted", third.Action.Kind, err)
	}
	if after, _ := os.ReadFile(path); string(after) != string(before) {
		t.Fatalf("an equal revision wrote the page")
	}
}

// A page edited by hand since it was written is not overwritten by a newer revision, as on the Engram path.
func TestPromoteCard_NewerRevisionDoesNotOverwriteALocalEdit(t *testing.T) {
	vaultRoot := t.TempDir()
	w := cardWriter(vaultRoot, PrecedenceStore{})
	first, err := w.PromoteCard(t.Context(), cardMemory())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(vaultRoot, first.Page.Path)
	raw, _ := os.ReadFile(path)
	edited := string(raw) + "\nA human added this.\n"
	if err := os.WriteFile(path, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	newer := cardMemory()
	newer.Revision = 4
	res, err := w.PromoteCard(t.Context(), newer)
	if err != nil || res.Action.Kind != ActionSkippedLocalEdit {
		t.Fatalf("result = %v, %v, want skipped_local_edit", res.Action.Kind, err)
	}
	if after, _ := os.ReadFile(path); string(after) != edited {
		t.Fatalf("the edited page was overwritten")
	}
}

// A bridged memory is refused, the message names the existing page, no page is created and the
// existing page is byte-identical.
func TestPromoteCard_BridgedMemoryIsRefused(t *testing.T) {
	vaultRoot := t.TempDir()
	engramObs := memory.Observation{ID: 402, Type: "pattern", Title: "Old Engram Page", Content: "Engram.", Project: "memorycard", RevisionCount: 2, TopicKey: "x/y"}
	seeded, err := EmitPage(engramObs, "c-000402", nil, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	full := writePromotedPage(t, vaultRoot, seeded)
	before, _ := os.ReadFile(full)
	store := PrecedenceStore{}
	seedPrecedence(store, seeded)

	w := cardWriter(vaultRoot, store)
	bridged := cardMemory()
	bridged.EngramID = 402
	result, err := w.PromoteCard(t.Context(), bridged)

	var refusal *BridgedError
	if !errors.As(err, &refusal) {
		t.Fatalf("err = %v, want a *BridgedError", err)
	}
	if refusal.Address != "c-000402" || !strings.Contains(err.Error(), "c-000402") {
		t.Errorf("refusal %q does not name the existing page c-000402", err)
	}
	if result.Action.Kind != ActionNone {
		t.Errorf("Action.Kind = %v, want none", result.Action.Kind)
	}
	if got := pageFiles(t, vaultRoot); len(got) != 1 {
		t.Fatalf("pages = %v, want only the existing one", got)
	}
	after, _ := os.ReadFile(full)
	if string(before) != string(after) {
		t.Fatalf("the existing page changed")
	}
	if len(store) != 1 {
		t.Errorf("precedence store has %d entries, want the one it had", len(store))
	}
}

// A memory with no bridge that holds the content of an Engram observation which has a page is refused as a
// bridged one is: the message names that page, nothing is created, and the page is byte-identical.
func TestPromoteCard_SameContentAsAnObservationWithAPageIsRefused(t *testing.T) {
	vaultRoot := t.TempDir()
	engramObs := memory.Observation{ID: 402, Type: "pattern", Title: "Old Engram Page", Content: "Engram.", Project: "memorycard", RevisionCount: 2, TopicKey: "x/y"}
	seeded, err := EmitPage(engramObs, "c-000402", nil, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	full := writePromotedPage(t, vaultRoot, seeded)
	before, _ := os.ReadFile(full)
	store := PrecedenceStore{}
	seedPrecedence(store, seeded)

	twin := cardMemory()
	twin.SameContentEngramIDs = []int64{401, 402}
	_, err = cardWriter(vaultRoot, store).PromoteCard(t.Context(), twin)
	var refusal *BridgedError
	if !errors.As(err, &refusal) || !refusal.SameContent || refusal.Address != "c-000402" || refusal.EngramID != 402 {
		t.Fatalf("err = %v, want a same-content *BridgedError naming c-000402 and observation 402", err)
	}
	if !strings.Contains(err.Error(), "same content") || !strings.Contains(err.Error(), "c-000402") {
		t.Errorf("message %q does not say so", err)
	}
	if got := pageFiles(t, vaultRoot); len(got) != 1 {
		t.Fatalf("pages = %v, want only the existing one", got)
	}
	if after, _ := os.ReadFile(full); string(before) != string(after) {
		t.Fatalf("the existing page changed")
	}

	// The same candidates with no page among them are no bridge: the memory is promoted.
	free := cardMemory()
	free.SameContentEngramIDs = []int64{401}
	if r, err := cardWriter(t.TempDir(), PrecedenceStore{}).PromoteCard(t.Context(), free); err != nil || r.Action.Kind != ActionCreated {
		t.Fatalf("result = %v, err = %v, want created", r.Action.Kind, err)
	}
}

func TestCardContentHash_IsMemorycardsHash(t *testing.T) {
	a := memory.CardContentHash("Title  Here", "Body\n\ttext ")
	b := memory.CardContentHash("title here", "BODY text")
	if a != b || len(a) != 64 {
		t.Fatalf("hashes %q and %q: case and white space runs must not matter", a, b)
	}
	// sha256("title here body text"), the normalised text memorycard hashes.
	if a != "cc259695be5df92e8d55d06de3518889bf8cbf47fd3aa74c5ed5182616219ba5" {
		t.Errorf("hash = %s, want the sha256 of %q", a, "title here body text")
	}
}

// A memory whose engram_id has no page is not bridged to anything: it is promoted.
func TestPromoteCard_ImportedButWithoutAPageIsPromoted(t *testing.T) {
	vaultRoot := t.TempDir()
	w := cardWriter(vaultRoot, PrecedenceStore{})
	imported := cardMemory()
	imported.EngramID = 999
	result, err := w.PromoteCard(t.Context(), imported)
	if err != nil || result.Action.Kind != ActionCreated {
		t.Fatalf("result = %v, err = %v, want created", result.Action.Kind, err)
	}
}

// The lookup seam: an id memorycard does not hold is the not-found error, as for Engram.
func TestExplicitPromoteCard_UnknownIDIsNotFound(t *testing.T) {
	w := cardWriter(t.TempDir(), PrecedenceStore{})
	lookup := func(int64) (memory.CardMemory, bool, error) { return memory.CardMemory{}, false, nil }
	_, err := ExplicitPromoteCard(t.Context(), w, lookup, 5)
	if !errors.Is(err, ErrObservationNotFound) {
		t.Fatalf("err = %v, want ErrObservationNotFound", err)
	}
}

// A superseded memory is marked so, and relates to the page of its successor only.
func TestPromoteCard_SupersededByLinksTheSuccessorPage(t *testing.T) {
	vaultRoot := t.TempDir()
	w := cardWriter(vaultRoot, PrecedenceStore{})
	successor := cardMemory()
	successor.ID = 8
	successor.Title = "Successor"
	if _, err := w.PromoteCard(t.Context(), successor); err != nil {
		t.Fatal(err)
	}

	old := cardMemory()
	old.Status, old.SupersededBy, old.SupersededByTitle = "superseded", 8, "Successor"
	w.Addresses = staticAddress("c-000043")
	result, err := w.PromoteCard(t.Context(), old)
	if err != nil {
		t.Fatal(err)
	}
	fm, _ := frontmatterBlock(result.Page.Frontmatter)
	fields := parseFrontmatterFields(fm)
	if fields[statusField] != "superseded" {
		t.Errorf("status = %q, want superseded", fields[statusField])
	}
	if !strings.Contains(result.Page.Frontmatter, "[[c-000042|Successor]]") {
		t.Errorf("related does not link the successor page:\n%s", result.Page.Frontmatter)
	}
}
