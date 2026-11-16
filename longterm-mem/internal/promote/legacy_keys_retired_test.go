package promote

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vaultlayout"
)

// S7: the legacy identity keys of the pages written before the store-agnostic ones (engram_*, memorycard_*) are
// retired. A page that carries only them names no memory: it is no promoted page.

// A page carrying only the legacy memorycard keys is not the page of the memory they name: a sync promotes the
// memory again and leaves the legacy page as it is.
func TestLegacyKeysRetired_ALegacyOnlyPageIsNotThePageOfItsMemory(t *testing.T) {
	vaultRoot := t.TempDir()
	legacy := "---\ntype: concept\naddress: c-000900\nmemorycard_id: 11\nmemorycard_type: decision\nmemorycard_revision: 2\nproject: memorycard\n---\n\nBody 11.\n"
	writeIdentityPage(t, vaultRoot, "c-000900.md", legacy)
	w := syncCardsWriter(vaultRoot)
	cards := &fakeCards{memories: []memory.CardMemory{eligibleCard(11, 2)}}

	report, err := SyncCards(t.Context(), cards, w, "memorycard")
	if err != nil {
		t.Fatalf("SyncCards: %v", err)
	}
	if len(report.Promoted) != 1 || report.Promoted[0].Action.Kind != ActionCreated || report.Skipped != 0 || report.Bridged != 0 || len(report.Failed) != 0 {
		t.Fatalf("report = %+v, want one created page: the legacy page is no page of memory 11", report)
	}
	if report.Promoted[0].Page.Address == "c-000900" {
		t.Errorf("the new page took the address of the legacy page")
	}
	if got := pageFiles(t, vaultRoot); len(got) != 2 {
		t.Errorf("pages = %v, want the legacy page and the new one", got)
	}
	raw, err := os.ReadFile(filepath.Join(vaultRoot, vaultlayout.PagesDir, "c-000900.md"))
	if err != nil || string(raw) != legacy {
		t.Errorf("the legacy page = %q, %v; want it untouched", raw, err)
	}
}

// Whatever the legacy keys name, no page of them names a memory: not the Engram family, not the memorycard one, and
// not the store-agnostic id of a page whose source is a store no longer served.
func TestLegacyKeysRetired_NoLegacyPageNamesAMemory(t *testing.T) {
	cases := map[string]string{
		"engram keys":                       "engram_id: 9\nengram_revision: 2\nengram_type: decision\n",
		"memorycard keys":                   "memorycard_id: 9\nmemorycard_revision: 2\nmemorycard_type: decision\n",
		"source engram":                     "source: engram\nsource_id: 9\nsource_revision: 2\n",
		"source_id with no source":          "source_id: 9\nsource_revision: 2\n",
		"memorycard keys beside engram ids": "memorycard_id: 9\nengram_id: 9\n",
	}
	for name, keys := range cases {
		t.Run(name, func(t *testing.T) {
			page := Page{Frontmatter: "---\ntype: concept\naddress: c-000001\n" + keys + "project: p\n---\n"}
			if ref, named := PageMemoryRef(page); named {
				t.Errorf("PageMemoryRef = %+v, true; want no memory", ref)
			}
		})
	}
}

// A page that names its memory in the store-agnostic keys is still read, and its revision and type are those keys'
// alone: a legacy revision beside them is not a fallback.
func TestLegacyKeysRetired_TheStoreAgnosticKeysAreTheOnlyOnesRead(t *testing.T) {
	vaultRoot := t.TempDir()
	writeIdentityPage(t, vaultRoot, "c-000001.md", "---\naddress: c-000001\nsource: memorycard\nsource_id: 7\nmemorycard_revision: 8\nproject: p\n---\n\nBody.\n")
	writeIdentityPage(t, vaultRoot, "c-000002.md", "---\naddress: c-000002\nsource: memorycard\nsource_id: 8\nsource_revision: 3\nmemorycard_revision: 9\nproject: p\n---\n\nBody.\n")
	index, err := readPages(pagesIn(vaultRoot))
	if err != nil {
		t.Fatalf("readPages: %v", err)
	}
	first := index.ownPage("p", memory.Identity{Source: memorycardSource, ID: "7"})
	if first == nil || first.RawRevision != "" {
		t.Errorf("the page of memory 7 = %+v; want it found with no revision (memorycard_revision is not read)", first)
	}
	second := index.ownPage("p", memory.Identity{Source: memorycardSource, ID: "8"})
	if second == nil || second.RawRevision != "3" || second.RevisionKey != sourceRevisionField {
		t.Errorf("the page of memory 8 = %+v; want revision 3 read from source_revision", second)
	}
	ref, named := PageMemoryRef(Page{Frontmatter: "---\nsource: memorycard\nsource_id: 7\n---\n"})
	if !named || ref != (MemoryRef{Source: memorycardSource, ID: 7}) {
		t.Errorf("PageMemoryRef(source: memorycard, source_id: 7) = %+v, %v; want memorycard 7", ref, named)
	}
}
