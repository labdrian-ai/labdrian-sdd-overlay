package promote

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vaultlayout"
)

// A successor link names the page of the successor the way the successor's own page resolves, so a page that
// carries no address is linked by the same name in both (the one a page falls back to is its file).
func TestScanFor_ASuccessorPageWithNoAddressIsLinkedByTheNameItsOwnPageResolvesTo(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, vaultlayout.PageFile("c-000050"))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("---\nmemorycard_id: 9\nmemorycard_revision: 1\nproject: memorycard\n---\n# Successor\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	index, err := readPages(pagesIn(root))
	if err != nil {
		t.Fatalf("readPages: %v", err)
	}
	successor := memory.Identity{Source: memorycardSource, ID: "9"}
	own := index.ownPage("memorycard", successor)
	if own == nil || own.address() == "" {
		t.Fatalf("the page of the successor is not found, or has no name: %+v", own)
	}

	scan := index.scanFor(memory.Memory{
		Identity: memory.Identity{Source: memorycardSource, ID: "8"}, Project: "memorycard",
		Successor: successor, SuccessorTitle: "Successor",
	})

	if scan.successor != own.address() {
		t.Errorf("successor = %q, want %q, the name the successor's own page resolves to", scan.successor, own.address())
	}
}

// A page of another store's memory bridges a memory whatever project the page was promoted under, as it has since
// C22-FU: a memory whose lineage page is in another project is skipped as bridged, not given a second page.
func TestSync_AMemoryBridgedByALineagePageInAnotherProjectIsSkippedAsBridged(t *testing.T) {
	w, vaultRoot, _, observation := bridgedWorld(t)
	observation.Project = "elsewhere"
	observation.Lineage = []memory.Identity{cardSevenIdentity}

	report, err := Sync(t.Context(), Deps{Source: fixedSource{memories: []memory.Memory{observation}}, Writer: w}, "elsewhere")

	var refusal *BridgedError
	if _, explicitErr := w.Promote(t.Context(), observation, true); !errors.As(explicitErr, &refusal) {
		t.Errorf("explicit Promote = %v, want a *BridgedError", explicitErr)
	}
	if err != nil || report.Bridged != 1 || len(report.Promoted) != 0 || len(report.Failed) != 0 {
		t.Fatalf("Sync = (%+v, %v), want the memory counted bridged and nothing promoted", report, err)
	}
	if got := pageFiles(t, vaultRoot); len(got) != 1 {
		t.Fatalf("pages = %v, want only the lineage memory's", got)
	}
}
