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

// A page of another store's memory bridges a memory only in the memory's own project: a page of the same piece of
// memory left in another project is not a page of this project, and the memory is promoted into its own.
func TestPromote_APageOfTheLineageMemoryInAnotherProjectDoesNotBridge(t *testing.T) {
	w, _, cardAddress, observation := bridgedWorld(t)
	observation.Project = "elsewhere"
	observation.Lineage = []memory.Identity{cardSevenIdentity}

	result, err := w.Promote(t.Context(), observation, true)

	var refusal *BridgedError
	if errors.As(err, &refusal) {
		t.Fatalf("Promote = %v, want the observation promoted into its own project, not bridged to %s", err, refusal.Address)
	}
	if err != nil || result.Action.Kind != ActionCreated || result.Page.Address == cardAddress {
		t.Fatalf("Promote = (%+v, %v), want a page created of its own", result, err)
	}
}
