package promote

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

// A revision is read off the page with or without an id beside it, as it was before the reader existed: reconcile
// and update corroboration adopt such a page, and the refusal of a page with no revision still names what it needs.
func TestPageIdentity_ARevisionIsReadWithoutAnID(t *testing.T) {
	block := "---\nengram_revision: 4\nproject: p-one\naddress: c-000801\n---\n"
	if got, ok := frontmatterRevision(block); !ok || got != 4 {
		t.Fatalf("frontmatterRevision(engram_revision only) = %d, %v; want 4, true", got, ok)
	}
	agnostic := "---\nsource: engram\nsource_revision: 5\n---\n"
	if got, ok := frontmatterRevision(agnostic); !ok || got != 5 {
		t.Fatalf("frontmatterRevision(source_revision, no source_id) = %d, %v; want 5, true", got, ok)
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
	if !errors.Is(err, ErrUnusablePage) || !strings.Contains(err.Error(), "engram_revision") || !strings.Contains(err.Error(), "source_revision") {
		t.Fatalf("Reconcile of a page with no revision = %v; want ErrUnusablePage naming engram_revision and source_revision", err)
	}
}

// The card scan reads the memory's own page revision from the memorycard keys alone: a page that also names an
// Engram identity gives its Engram match no revision of the memorycard's.
func TestPageIdentity_CardScanKeepsTheRevisionSlotsApart(t *testing.T) {
	vaultRoot := t.TempDir()
	writeIdentityPage(t, vaultRoot, "c-000803.md", "---\nmemorycard_id: 7\nmemorycard_revision: 3\nengram_id: 402\nproject: memorycard\naddress: c-000803\n---\n\nBody.\n")
	index, err := readCardPages(pagesIn(vaultRoot))
	if err != nil {
		t.Fatalf("readCardPages: %v", err)
	}
	if got := index.own[cardKey{project: "memorycard", id: 7}]; got == nil || got.RawRevision != "3" {
		t.Fatalf("own page of memory 7 = %+v; want its memorycard revision 3", got)
	}
	if len(index.engramPages) != 1 || index.engramPages[0].EngramID != "402" || index.engramPages[0].RawRevision != "" {
		t.Fatalf("engramPages = %+v; want the page of observation 402 with no revision of the memory's", index.engramPages)
	}
}

// Precedence between the key families is per field: a store-agnostic key wins for its field, and a field the
// page names only in the legacy keys of the same store is still read from them; never from another store's.
func TestPageIdentity_ReaderFallsBackPerFieldToTheSameStoresLegacyKey(t *testing.T) {
	for _, tc := range []struct {
		name   string
		block  string
		source string
		want   PageIdentity
	}{
		{"revision and type from the legacy keys", "---\nsource: engram\nsource_id: 9\nengram_revision: 3\nengram_type: pattern\n---\n", "engram", PageIdentity{Source: "engram", ID: "9", Revision: "3", Type: "pattern"}},
		{"the agnostic revision wins, the type falls back", "---\nsource: engram\nsource_id: 9\nsource_revision: 6\nengram_revision: 3\nengram_type: pattern\n---\n", "engram", PageIdentity{Source: "engram", ID: "9", Revision: "6", Type: "pattern"}},
		{"the same for memorycard", "---\nsource: memorycard\nsource_id: 7\nmemorycard_revision: 2\nmemorycard_type: decision\n---\n", "memorycard", PageIdentity{Source: "memorycard", ID: "7", Revision: "2", Type: "decision"}},
		{"never across stores", "---\nsource: memorycard\nsource_id: 9\nsource_revision: 4\nsource_type: decision\nengram_id: 5\nengram_revision: 2\nengram_type: pattern\n---\n", "engram", PageIdentity{Source: "engram", ID: "5", Revision: "2", Type: "pattern"}},
		{"another store's legacy keys do not fill this one's", "---\nsource: engram\nsource_id: 9\nmemorycard_revision: 8\nmemorycard_type: decision\n---\n", "engram", PageIdentity{Source: "engram", ID: "9"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, named := identityOf(parseFrontmatterFields(tc.block), tc.source)
			if !named || got != tc.want {
				t.Fatalf("identityOf = %+v, %v; want %+v, true", got, named, tc.want)
			}
		})
	}
}

// An unparseable revision is reported under the key it was read from.
func TestPageIdentity_AnUnparseableRevisionNamesTheKeyRead(t *testing.T) {
	for _, tc := range []struct{ name, fields, key string }{
		{"legacy", "engram_id: 810\nengram_revision: nope", "engram_revision"},
		{"store-agnostic", "source: engram\nsource_id: 810\nsource_revision: nope", "source_revision"},
	} {
		t.Run("engram page, "+tc.name, func(t *testing.T) {
			vaultRoot := t.TempDir()
			writeIdentityPage(t, vaultRoot, "c-000810.md", "---\n"+tc.fields+"\nproject: p-one\naddress: c-000810\n---\n\nBody.\n")
			_, _, err := findPromotedPage(pagesIn(vaultRoot), "p-one", 810)
			if err == nil || !strings.Contains(err.Error(), tc.key+` "nope"`) {
				t.Fatalf("findPromotedPage err = %v; want it to name %s", err, tc.key)
			}
		})
	}
	for _, tc := range []struct{ name, fields, key string }{
		{"legacy", "memorycard_id: 7\nmemorycard_revision: nope", "memorycard_revision"},
		{"store-agnostic", "source: memorycard\nsource_id: 7\nsource_revision: nope", "source_revision"},
	} {
		t.Run("memorycard page, "+tc.name, func(t *testing.T) {
			vaultRoot := t.TempDir()
			writeIdentityPage(t, vaultRoot, "c-000007.md", "---\n"+tc.fields+"\nproject: memorycard\naddress: c-000007\n---\n\nBody.\n")
			_, err := cardWriter(vaultRoot, PrecedenceStore{}).PromoteCard(t.Context(), cardMemory())
			if err == nil || !strings.Contains(err.Error(), tc.key+` "nope"`) {
				t.Fatalf("PromoteCard err = %v; want it to name %s", err, tc.key)
			}
		})
	}
}
