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
// promoted from by the store-agnostic keys `source`, `source_id`, `source_revision` and `source_type`, or, as
// every page written so far does, by the legacy keys of its store (`engram_id`/`engram_revision`/`engram_type`,
// `memorycard_id`/`memorycard_revision`/`memorycard_type`). Each consumer of page identity is driven through its
// own public entry with both spellings of the same page: the legacy case passes before the reader exists and
// holds the previous behavior; the store-agnostic case is the rule.

// asStoreAgnostic rewrites the frontmatter an Engram render carries (legacy keys) into the store-agnostic
// spelling of the same identity: `source: engram`, `source_id`, `source_revision`, `source_type`, and no
// `engram_sync_id`. Nothing else in the block or the body moves.
func asStoreAgnostic(t *testing.T, page Page) Page {
	t.Helper()
	var lines []string
	for _, line := range strings.Split(page.Frontmatter, "\n") {
		switch {
		case strings.HasPrefix(line, "engram_id: "):
			lines = append(lines, "source: engram", "source_id: "+strings.TrimPrefix(line, "engram_id: "))
		case strings.HasPrefix(line, "engram_sync_id:"):
		case strings.HasPrefix(line, "engram_type: "):
			lines = append(lines, "source_type: "+strings.TrimPrefix(line, "engram_type: "))
		case strings.HasPrefix(line, "engram_revision: "):
			lines = append(lines, "source_revision: "+strings.TrimPrefix(line, "engram_revision: "))
		default:
			lines = append(lines, line)
		}
	}
	converted := page
	converted.Frontmatter = strings.Join(lines, "\n")
	if strings.Contains(converted.Frontmatter, "engram_") {
		t.Fatalf("fixture still carries a legacy key:\n%s", converted.Frontmatter)
	}
	return converted
}

// enginePage is the page a promotion of observation id writes, in the spelling named by agnostic.
func enginePage(t *testing.T, obs memory.Observation, address string, agnostic bool) Page {
	t.Helper()
	page, err := EmitPage(obsMemory(obs), address, nil, testInstant)
	if err != nil {
		t.Fatalf("EmitPage: %v", err)
	}
	if agnostic {
		return asStoreAgnostic(t, page)
	}
	return page
}

// spellings runs body once for the legacy spelling and once for the store-agnostic one.
func spellings(t *testing.T, body func(t *testing.T, agnostic bool)) {
	t.Helper()
	t.Run("legacy keys", func(t *testing.T) { body(t, false) })
	t.Run("store-agnostic keys", func(t *testing.T) { body(t, true) })
}

// The promoted-page scans find a page by its identity and read its revision, whichever spelling names them.
func TestPageIdentity_PromotedPageScansFindThePageBySourceAndID(t *testing.T) {
	spellings(t, func(t *testing.T, agnostic bool) {
		vaultRoot := t.TempDir()
		obs := memory.Observation{ID: 701, Type: "decision", Title: "Identified", Content: "Body.", Project: "p-one", RevisionCount: 4}
		writePromotedPage(t, vaultRoot, enginePage(t, obs, "c-000701", agnostic))

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
	})
}

// A page whose source is another store than the one asked for is not that store's page, even when its id is the
// same number.
func TestPageIdentity_AnIDIsOnlyThatOfItsOwnSource(t *testing.T) {
	vaultRoot := t.TempDir()
	memoryDir := filepath.Join(vaultRoot, vaultlayout.PagesDir)
	if err := os.MkdirAll(memoryDir, 0o755); err != nil {
		t.Fatal(err)
	}
	page := "---\nsource: memorycard\nsource_id: 7\nsource_revision: 2\nproject: p-one\naddress: c-000007\n---\n\nBody.\n"
	if err := os.WriteFile(filepath.Join(memoryDir, "c-000007.md"), []byte(page), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := findPromotedPage(pagesIn(vaultRoot), "p-one", 7); err != nil || ok {
		t.Fatalf("findPromotedPage(engram 7) = %v, %v; a memorycard page is not an Engram page", ok, err)
	}
}

// The reader: the store-agnostic keys win for their source, a legacy key still names the identity it always did,
// and a page names one identity per source.
func TestPageIdentity_ReaderPrefersStoreAgnosticKeysAndReadsTheLegacyOnes(t *testing.T) {
	for _, tc := range []struct {
		name   string
		block  string
		source string
		want   PageIdentity
		named  bool
	}{
		{"agnostic", "---\nsource: engram\nsource_id: 9\nsource_revision: 3\nsource_type: decision\n---\n", "engram", PageIdentity{Source: "engram", ID: "9", Revision: "3", Type: "decision"}, true},
		{"legacy engram", "---\nengram_id: 5\nengram_revision: 2\nengram_type: pattern\n---\n", "engram", PageIdentity{Source: "engram", ID: "5", Revision: "2", Type: "pattern"}, true},
		{"legacy memorycard", "---\nmemorycard_id: 6\nmemorycard_revision: 1\nmemorycard_type: decision\n---\n", "memorycard", PageIdentity{Source: "memorycard", ID: "6", Revision: "1", Type: "decision"}, true},
		{"agnostic wins over legacy", "---\nsource: engram\nsource_id: 9\nengram_id: 5\n---\n", "engram", PageIdentity{Source: "engram", ID: "9"}, true},
		{"another source's agnostic keys do not name this one", "---\nsource: memorycard\nsource_id: 9\n---\n", "engram", PageIdentity{}, false},
		{"a legacy key beside another source's agnostic keys still names its own", "---\nsource: memorycard\nsource_id: 9\nengram_id: 5\n---\n", "engram", PageIdentity{Source: "engram", ID: "5"}, true},
		{"a source with no id names nothing", "---\nsource: engram\n---\n", "engram", PageIdentity{}, false},
		{"the sources list is not a source", "---\nsources: []\n---\n", "engram", PageIdentity{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, named := identityOf(parseFrontmatterFields(tc.block), tc.source)
			if got != tc.want || named != tc.named {
				t.Fatalf("identityOf = %+v, %v; want %+v, %v", got, named, tc.want, tc.named)
			}
		})
	}
}

// The card scan finds the memory's own page, and the Engram page it is bridged to, by identity.
func TestPageIdentity_CardScanFindsOwnAndBridgedPagesByIdentity(t *testing.T) {
	spellings(t, func(t *testing.T, agnostic bool) {
		vaultRoot := t.TempDir()
		memoryDir := filepath.Join(vaultRoot, vaultlayout.PagesDir)
		if err := os.MkdirAll(memoryDir, 0o755); err != nil {
			t.Fatal(err)
		}
		own := "---\nmemorycard_id: 7\nmemorycard_revision: 3\nproject: memorycard\naddress: c-000007\n---\n\nBody.\n"
		if agnostic {
			own = "---\nsource: memorycard\nsource_id: 7\nsource_revision: 3\nproject: memorycard\naddress: c-000007\n---\n\nBody.\n"
		}
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

		index, err := readCardPages(pagesIn(vaultRoot))
		if err != nil {
			t.Fatalf("readCardPages: %v", err)
		}
		if got := index.own[cardKey{project: "memorycard", id: 7}]; got == nil || got.RawRevision != "3" || got.Address != "c-000007" {
			t.Fatalf("own page of memory 7 = %+v; want c-000007 at revision 3", got)
		}
		if got := index.byCardID["memorycard"][7]; got != "c-000007" {
			t.Fatalf("successor-link address of memory 7 = %q; want c-000007", got)
		}
		if block, ok := index.blocks[cardKey{project: "memorycard", id: 7}]; !ok || !strings.Contains(block, "c-000007") {
			t.Fatalf("frontmatter block of memory 7 = %q, %v; want the page's block", block, ok)
		}

		engramVault := t.TempDir()
		engramObs := memory.Observation{ID: 402, Type: "pattern", Title: "Old Engram Page", Content: "Engram.", Project: "memorycard", RevisionCount: 2, TopicKey: "x/y"}
		writePromotedPage(t, engramVault, enginePage(t, engramObs, "c-000402", agnostic))
		bridged := cardMemory()
		bridged.EngramID = 402
		_, err = cardWriter(engramVault, PrecedenceStore{}).PromoteCard(t.Context(), bridged)
		var refusal *BridgedError
		if !errors.As(err, &refusal) || refusal.Address != "c-000402" {
			t.Fatalf("PromoteCard err = %v; want a *BridgedError naming c-000402", err)
		}
	})
}

// Update corroboration recognises an interrupted update of its own page by the revision and the identity the page
// carries, in either spelling, and does not take the revision or type the observation moves for a local edit.
func TestPageIdentity_UpdateCorroborationReadsRevisionAndIdentity(t *testing.T) {
	spellings(t, func(t *testing.T, agnostic bool) {
		vaultRoot := t.TempDir()
		obs := memory.Observation{ID: 709, Type: "decision", Title: "Interrupted", Content: "V1 body.", Project: "p-one", RevisionCount: 1}
		first := enginePage(t, obs, "c-000709", agnostic)
		existingPath := writePromotedPage(t, vaultRoot, first)
		store := PrecedenceStore{}
		seedPrecedence(store, first)

		obs.RevisionCount, obs.Content = 2, "V2 body."
		interrupted := enginePage(t, obs, "c-000709", agnostic)
		if err := os.WriteFile(existingPath, []byte(interrupted.Frontmatter+interrupted.Body), 0o644); err != nil {
			t.Fatal(err)
		}
		obs.RevisionCount, obs.Content, obs.Type = 3, "V3 body.", "pattern"
		retry := enginePage(t, obs, "c-000709", agnostic)

		action, err := UpdateInPlace(pagesIn(vaultRoot), store, retry)
		if err != nil {
			t.Fatalf("UpdateInPlace: %v", err)
		}
		if action.Kind != ActionUpdated {
			t.Fatalf("action.Kind = %v, want ActionUpdated: the page is our own interrupted write", action.Kind)
		}
		got, err := os.ReadFile(existingPath)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != retry.Frontmatter+retry.Body {
			t.Fatalf("page was not refreshed to the retry's render; got:\n%s", got)
		}
		if entry, _ := store.Get("c-000709"); entry.PromotedRevision != 3 {
			t.Fatalf("recorded revision = %d, want 3", entry.PromotedRevision)
		}
	})
}

// Reconcile reads the revision to adopt off the page, in either spelling.
func TestPageIdentity_ReconcileReadsTheRevisionOffThePage(t *testing.T) {
	spellings(t, func(t *testing.T, agnostic bool) {
		vaultRoot := t.TempDir()
		obs := memory.Observation{ID: 711, Type: "decision", Title: "Adopt", Content: "Body.", Project: "p-one", RevisionCount: 4}
		writePromotedPageAt(t, vaultRoot, "c-000711", enginePage(t, obs, "c-000711", agnostic))

		outcome, err := Reconcile(pagesIn(vaultRoot), "p-one", "c-000711", &memPrecedence{})
		if err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
		if !outcome.Adopted || outcome.PromotedRevision != 4 {
			t.Fatalf("Reconcile = %+v; want the page adopted at revision 4", outcome)
		}
	})
}

// A memorycard page is not an Engram page: reconcile still refuses it for the revision it does not carry, as it
// does today (it adopts only the revision of an Engram identity).
func TestPageIdentity_ReconcileStillRefusesAPageThatIsNotEngrams(t *testing.T) {
	vaultRoot := t.TempDir()
	page := "---\nsource: memorycard\nsource_id: 7\nsource_revision: 3\nproject: p-one\naddress: c-000007\n---\n\nBody.\n"
	full := filepath.Join(vaultRoot, vaultlayout.PagesDir, "c-000007.md")
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(page), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Reconcile(pagesIn(vaultRoot), "p-one", "c-000007", &memPrecedence{})
	if !errors.Is(err, ErrUnusablePage) {
		t.Fatalf("Reconcile err = %v; want ErrUnusablePage", err)
	}
}

// The project move recognises the page the observation left behind by its identity, in either spelling.
func TestPageIdentity_ProjectMoveSupersedesThePageByIdentity(t *testing.T) {
	spellings(t, func(t *testing.T, agnostic bool) {
		vaultRoot := t.TempDir()
		obs := memory.Observation{ID: 721, Type: "decision", Title: "Moved Decision", Content: "Body.", Project: "p-one", RevisionCount: 1, TopicKey: "longterm-mem/projectmove-fixture"}
		oldPage := enginePage(t, obs, "c-000001", agnostic)
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
	})
}

// PageObservationID names the observation of an Engram page, in either spelling, and nothing for a page of
// another source.
func TestPageIdentity_PageObservationIDReadsAnEngramIdentity(t *testing.T) {
	for _, tc := range []struct {
		frontmatter string
		wantID      int64
		wantNamed   bool
	}{
		{"---\nengram_id: 12\n---\n", 12, true},
		{"---\nsource: engram\nsource_id: 12\n---\n", 12, true},
		{"---\nsource: engram\nsource_id: 0\n---\n", 0, false},
		{"---\nsource: engram\nsource_id: x\n---\n", 0, false},
		{"---\nsource: memorycard\nsource_id: 12\n---\n", 0, false},
	} {
		id, named := PageObservationID(Page{Frontmatter: tc.frontmatter})
		if id != tc.wantID || named != tc.wantNamed {
			t.Errorf("PageObservationID(%q) = %d, %v; want %d, %v", tc.frontmatter, id, named, tc.wantID, tc.wantNamed)
		}
	}
}
