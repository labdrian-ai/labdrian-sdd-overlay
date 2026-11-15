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

// These tests pin the one reader of a page's identity (page_identity.go): a page names the memory it was
// promoted from by the store-agnostic keys `source`, `source_id`, `source_revision` and `source_type`, and by no
// other (S7: the legacy keys of the pages written before them were retired). Each consumer of page identity is
// driven through its own public entry.

// enginePage is the page a promotion of memory id writes.
func enginePage(t *testing.T, obs memory.Observation, address string) Page {
	t.Helper()
	page, err := EmitPage(obsMemory(obs), address, nil, testInstant)
	if err != nil {
		t.Fatalf("EmitPage: %v", err)
	}
	return page
}

// The promoted-page scans find a page by its identity and read its revision, whichever spelling names them.
func TestPageIdentity_PromotedPageScansFindThePageBySourceAndID(t *testing.T) {
	vaultRoot := t.TempDir()
	obs := memory.Observation{ID: 701, Type: "decision", Title: "Identified", Content: "Body.", Project: "p-one", RevisionCount: 4}
	writePromotedPage(t, vaultRoot, enginePage(t, obs, "c-000701"))

	promoted, ok, err := findPromotedPage(pagesIn(vaultRoot), "p-one", 701)
	if err != nil {
		t.Fatalf("findPromotedPage: %v", err)
	}
	if !ok || promoted.Address != "c-000701" || promoted.Revision != 4 || promoted.Project != "p-one" {
		t.Fatalf("findPromotedPage = %+v, %v; want c-000701 at revision 4 in p-one", promoted, ok)
	}
	if _, ok, err := findPromotedPage(pagesIn(vaultRoot), "p-one", 702); err != nil || ok {
		t.Fatalf("findPromotedPage of another id = %v, %v; want not promoted", ok, err)
	}

	moved, err := findMovedPages(pagesIn(vaultRoot), "p-two", 701)
	if err != nil {
		t.Fatalf("findMovedPages: %v", err)
	}
	if len(moved) != 1 || moved[0].Address != "c-000701" || moved[0].Project != "p-one" {
		t.Fatalf("findMovedPages = %+v; want the page c-000701 of p-one", moved)
	}
}

// A page whose source is another store than the one asked for is not that store's page, even when its id is the
// same number.
func TestPageIdentity_AnIDIsOnlyThatOfItsOwnSource(t *testing.T) {
	vaultRoot := t.TempDir()
	memoryDir := filepath.Join(vaultRoot, vaultlayout.PagesDir)
	if err := os.MkdirAll(memoryDir, 0o755); err != nil {
		t.Fatal(err)
	}
	page := "---\nsource: engram\nsource_id: 7\nsource_revision: 2\nproject: p-one\naddress: c-000007\n---\n\nBody.\n"
	if err := os.WriteFile(filepath.Join(memoryDir, "c-000007.md"), []byte(page), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := findPromotedPage(pagesIn(vaultRoot), "p-one", 7); err != nil || ok {
		t.Fatalf("findPromotedPage(7) = %v, %v; a page of another store is not a memorycard page", ok, err)
	}
}

// The card scan finds the memory's own page, and the page of the memory it is bridged to, by identity.
func TestPageIdentity_CardScanFindsOwnAndBridgedPagesByIdentity(t *testing.T) {
	vaultRoot := t.TempDir()
	memoryDir := filepath.Join(vaultRoot, vaultlayout.PagesDir)
	if err := os.MkdirAll(memoryDir, 0o755); err != nil {
		t.Fatal(err)
	}
	own := "---\nsource: memorycard\nsource_id: 7\nsource_revision: 3\nproject: memorycard\naddress: c-000007\n---\n\nBody.\n"
	if err := os.WriteFile(filepath.Join(memoryDir, "c-000007.md"), []byte(own), 0o644); err != nil {
		t.Fatal(err)
	}
	w := cardWriter(vaultRoot, PrecedenceStore{})
	result, err := w.PromoteCard(t.Context(), cardMemory())
	if err != nil {
		t.Fatalf("PromoteCard: %v", err)
	}
	if result.Action.Kind != ActionAlreadyPromoted || result.Page.Address != "c-000007" {
		t.Fatalf("PromoteCard = %v at %q; want already promoted at c-000007", result.Action.Kind, result.Page.Address)
	}
	if got := pageFiles(t, vaultRoot); len(got) != 1 {
		t.Fatalf("pages = %v, want only the existing one", got)
	}

	index, err := readPages(pagesIn(vaultRoot))
	if err != nil {
		t.Fatalf("readPages: %v", err)
	}
	cardSeven := memory.Identity{Source: memorycardSource, ID: "7"}
	if got := index.ownPage("memorycard", cardSeven); got == nil || got.RawRevision != "3" || got.Address != "c-000007" {
		t.Fatalf("own page of memory 7 = %+v; want c-000007 at revision 3", got)
	}
	if got := index.addresses[addressKey{project: "memorycard", identity: cardSeven}]; got != "c-000007" {
		t.Fatalf("successor-link address of memory 7 = %q; want c-000007", got)
	}
	if own := index.ownPage("memorycard", cardSeven); own == nil || !strings.Contains(own.Block, "c-000007") {
		t.Fatalf("frontmatter block of memory 7 = %+v; want the page's block", own)
	}

	peerVault := t.TempDir()
	peerObs := memory.Observation{ID: 402, Type: "pattern", Title: "Old Page", Content: "Peer.", Project: "memorycard", RevisionCount: 2, TopicKey: "x/y"}
	writePromotedPage(t, peerVault, enginePage(t, peerObs, "c-000402"))
	bridged := cardMem(cardMemory())
	bridged.Lineage = []memory.Identity{{Source: memorycardSource, ID: "402"}}
	_, err = cardWriter(peerVault, PrecedenceStore{}).Promote(t.Context(), bridged, true)
	var refusal *BridgedError
	if !errors.As(err, &refusal) || refusal.Address != "c-000402" {
		t.Fatalf("PromoteCard err = %v; want a *BridgedError naming c-000402", err)
	}
}

// Reconcile reads the revision to adopt off the page, in either spelling.
func TestPageIdentity_ReconcileReadsTheRevisionOffThePage(t *testing.T) {
	vaultRoot := t.TempDir()
	obs := memory.Observation{ID: 711, Type: "decision", Title: "Adopt", Content: "Body.", Project: "p-one", RevisionCount: 4}
	writePromotedPageAt(t, vaultRoot, "c-000711", enginePage(t, obs, "c-000711"))

	outcome, err := Reconcile(pagesIn(vaultRoot), "p-one", "c-000711", &memPrecedence{})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if !outcome.Adopted || outcome.PromotedRevision != 4 {
		t.Fatalf("Reconcile = %+v; want the page adopted at revision 4", outcome)
	}
}

// The project move recognises the page the observation left behind by its identity, in either spelling.
func TestPageIdentity_ProjectMoveSupersedesThePageByIdentity(t *testing.T) {
	vaultRoot := t.TempDir()
	obs := memory.Observation{ID: 721, Type: "decision", Title: "Moved Decision", Content: "Body.", Project: "p-one", RevisionCount: 1, TopicKey: "longterm-mem/projectmove-fixture"}
	oldPage := enginePage(t, obs, "c-000001")
	writePromotedPage(t, vaultRoot, oldPage)
	store := PrecedenceStore{}
	seedPrecedence(store, oldPage)

	moved := obs
	moved.Project = "p-two"
	w := &Writer{Index: indexIn(vaultRoot), Log: logIn(vaultRoot), SyncState: syncStateIn(vaultRoot), Pages: pagesIn(vaultRoot), Store: store, Precedence: &memPrecedence{}, AddressMap: &memAddressMap{}, Addresses: staticAddress(testAddress), Clock: &fakeClock{at: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)}}
	result, err := w.Promote(t.Context(), obsMemory(moved), false)
	if err != nil {
		t.Fatalf("Promote after project move: %v", err)
	}
	if result.Action.Kind != ActionCreated || result.Page.Address == "c-000001" {
		t.Fatalf("Promote = %v at %q; want a fresh page", result.Action.Kind, result.Page.Address)
	}
	old := readPage(t, vaultRoot, "c-000001")
	if !strings.Contains(old, "status: superseded") || !strings.Contains(old, "[["+result.Page.Address+"|Moved Decision]]") {
		t.Fatalf("old page was not superseded toward %s; got:\n%s", result.Page.Address, old)
	}
}
