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

// writeIdentityPage writes content as the page file name of a scratch vault.
func writeIdentityPage(t *testing.T, vaultRoot, name, content string) {
	t.Helper()
	full := filepath.Join(vaultRoot, vaultlayout.PagesDir, name)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A revision is read off the page with or without an id beside it: reconcile adopts such a page, and the refusal of
// a page with no revision still names what it needs.
func TestPageIdentity_ARevisionIsReadWithoutAnID(t *testing.T) {
	block := "---\nsource: memorycard\nsource_revision: 4\nproject: p-one\naddress: c-000801\n---\n"
	if got, ok := pageRevision(block); !ok || got != 4 {
		t.Fatalf("pageRevision(source_revision, no source_id) = %d, %v; want 4, true", got, ok)
	}
	if got, ok := pageRevision("---\nsource_revision: 5\n---\n"); ok {
		t.Fatalf("pageRevision(source_revision with no source) = %d, true; want none: the revision belongs to the source it names", got)
	}

	vaultRoot := t.TempDir()
	writeIdentityPage(t, vaultRoot, "c-000801.md", block+"\nBody.\n")
	outcome, err := Reconcile(pagesIn(vaultRoot), "p-one", "c-000801", &memPrecedence{})
	if err != nil {
		t.Fatalf("Reconcile of a page with a revision and no id: %v", err)
	}
	if !outcome.Adopted || outcome.PromotedRevision != 4 {
		t.Fatalf("Reconcile = %+v; want the page adopted at revision 4", outcome)
	}

	writeIdentityPage(t, vaultRoot, "c-000802.md", "---\nproject: p-one\naddress: c-000802\n---\n\nBody.\n")
	_, err = Reconcile(pagesIn(vaultRoot), "p-one", "c-000802", &memPrecedence{})
	if !errors.Is(err, ErrUnusablePage) || !strings.Contains(err.Error(), "source_revision") || strings.Contains(err.Error(), "engram") || strings.Contains(err.Error(), "memorycard_revision") {
		t.Fatalf("Reconcile of a page with no revision = %v; want ErrUnusablePage naming source_revision alone", err)
	}
}

// The card scan reads the memory's own page revision from the store-agnostic keys alone: the legacy keys a page
// still carries beside them name nothing and give no revision.
func TestPageIdentity_CardScanReadsTheStoreAgnosticRevisionAlone(t *testing.T) {
	vaultRoot := t.TempDir()
	writeIdentityPage(t, vaultRoot, "c-000803.md", "---\nsource: memorycard\nsource_id: 7\nsource_revision: 3\nmemorycard_revision: 9\nengram_id: 402\nengram_revision: 8\nproject: memorycard\naddress: c-000803\n---\n\nBody.\n")
	index, err := readPages(pagesIn(vaultRoot))
	if err != nil {
		t.Fatalf("readPages: %v", err)
	}
	if got := index.ownPage("memorycard", memory.Identity{Source: memorycardSource, ID: "7"}); got == nil || got.RawRevision != "3" || got.RevisionKey != "source_revision" {
		t.Fatalf("own page of memory 7 = %+v; want its store-agnostic revision 3", got)
	}
	if len(index.carriers) != 1 {
		t.Fatalf("carriers = %d identities; want only memorycard 7: the page names no other memory", len(index.carriers))
	}
}

// The reader reads the store-agnostic keys of the store the page names, and nothing else: no legacy key fills a field,
// and a page that names another store names no memory of this one.
func TestPageIdentity_ReaderReadsTheStoreAgnosticKeysAlone(t *testing.T) {
	for _, tc := range []struct {
		name      string
		block     string
		want      PageIdentity
		wantNamed bool
	}{
		{"every store-agnostic key", "---\nsource: memorycard\nsource_id: 7\nsource_revision: 2\nsource_type: decision\n---\n", PageIdentity{Source: "memorycard", ID: "7", Revision: "2", Type: "decision"}, true},
		{"the legacy revision and type fill nothing", "---\nsource: memorycard\nsource_id: 7\nmemorycard_revision: 2\nmemorycard_type: decision\n---\n", PageIdentity{Source: "memorycard", ID: "7"}, true},
		{"another store's legacy keys fill nothing", "---\nsource: memorycard\nsource_id: 9\nsource_revision: 4\nengram_id: 5\nengram_revision: 2\nengram_type: pattern\n---\n", PageIdentity{Source: "memorycard", ID: "9", Revision: "4"}, true},
		{"a page of the legacy memorycard keys", "---\nmemorycard_id: 7\nmemorycard_revision: 2\nmemorycard_type: decision\n---\n", PageIdentity{}, false},
		{"a page of the legacy engram keys", "---\nengram_id: 9\nengram_revision: 3\nengram_type: pattern\n---\n", PageIdentity{}, false},
		{"a page of another store", "---\nsource: engram\nsource_id: 9\nsource_revision: 3\n---\n", PageIdentity{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, named := identityOf(parseFrontmatterFields(tc.block), memorycardSource)
			if named != tc.wantNamed || got != tc.want {
				t.Fatalf("identityOf = %+v, %v; want %+v, %v", got, named, tc.want, tc.wantNamed)
			}
		})
	}
}

// An unparseable revision is reported under the key it was read from, by the scan and by the promotion of the
// memory itself.
func TestPageIdentity_AnUnparseableRevisionNamesTheKeyRead(t *testing.T) {
	const fields = "source: memorycard\nsource_id: 7\nsource_revision: nope"
	vaultRoot := t.TempDir()
	writeIdentityPage(t, vaultRoot, "c-000810.md", "---\nsource: memorycard\nsource_id: 810\nsource_revision: nope\nproject: p-one\naddress: c-000810\n---\n\nBody.\n")
	_, _, err := findPromotedPage(pagesIn(vaultRoot), "p-one", 810)
	if err == nil || !strings.Contains(err.Error(), `source_revision "nope"`) {
		t.Fatalf("findPromotedPage err = %v; want it to name source_revision", err)
	}

	cardRoot := t.TempDir()
	writeIdentityPage(t, cardRoot, "c-000007.md", "---\n"+fields+"\nproject: memorycard\naddress: c-000007\n---\n\nBody.\n")
	_, err = cardWriter(cardRoot, PrecedenceStore{}).PromoteCard(t.Context(), cardMemory())
	if err == nil || !strings.Contains(err.Error(), `source_revision "nope"`) {
		t.Fatalf("PromoteCard err = %v; want it to name source_revision", err)
	}
}
