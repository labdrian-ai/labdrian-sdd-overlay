package promote

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// fixedSource is a Source over a fixed list of memories: the candidates and every memory of a project are the
// same list, as a store with nothing retired serves them.
type fixedSource struct{ memories []memory.Memory }

func (s fixedSource) Candidates(string) ([]memory.Memory, error) { return s.memories, nil }
func (s fixedSource) Memories(string) ([]memory.Memory, error)   { return s.memories, nil }
func (s fixedSource) ByID(id string) (memory.Memory, bool, error) {
	for _, m := range s.memories {
		if m.Identity.ID == id {
			return m, true, nil
		}
	}
	return memory.Memory{}, false, nil
}

var cardSevenIdentity = memory.Identity{Source: memorycardSource, ID: "7"}

// bridgedWorld is a vault in which the memorycard memory 7 already has a page, and the pinned memory 402 of the same
// project: the memory a sync would promote, whose lineage names that memory.
func bridgedWorld(t *testing.T) (w *Writer, vaultRoot, cardAddress string, observation memory.Memory) {
	t.Helper()
	vaultRoot = t.TempDir()
	w = syncCardsWriter(vaultRoot)
	promoted, err := w.PromoteCard(t.Context(), cardMemory())
	if err != nil || promoted.Action.Kind != ActionCreated {
		t.Fatalf("PromoteCard = (%+v, %v), want the memorycard memory promoted", promoted, err)
	}
	observation = obsMemory(memory.Observation{ID: 402, Type: "decision", Title: "Twin", Content: "Twin.", Project: "memorycard", RevisionCount: 1, Pinned: true})
	return w, vaultRoot, promoted.Page.Address, observation
}

// An explicit promote of such a memory is refused as a bridged memorycard memory is: a *BridgedError that
// names the page of the memory it is bridged to and the memory, and writes nothing.
func TestPromote_AnObservationWhoseLineageMemoryOwnsAPageIsRefused(t *testing.T) {
	w, vaultRoot, cardAddress, observation := bridgedWorld(t)
	observation.Lineage = []memory.Identity{cardSevenIdentity}

	result, err := w.Promote(t.Context(), observation, true)

	var refusal *BridgedError
	if !errors.As(err, &refusal) {
		t.Fatalf("Promote = (%+v, %v), want a *BridgedError", result, err)
	}
	if refusal.Address != cardAddress || refusal.Peer != cardSevenIdentity || refusal.SameContent {
		t.Errorf("refusal = %+v, want the page %s of memorycard memory 7, not a same-content one", refusal, cardAddress)
	}
	if want := "memorycard memory 402 is bridged to memorycard memory 7"; !strings.Contains(err.Error(), want) {
		t.Errorf("refusal %q does not say %q", err, want)
	}
	if got := pageFiles(t, vaultRoot); len(got) != 1 {
		t.Fatalf("pages = %v, want only the memorycard memory's", got)
	}
}

// An observation that only holds the content of a memory that has a page, with no import record, is refused too,
// told from a bridged one: the refusal says the content is the same.
func TestPromote_AnObservationMirroredByAMemoryWithAPageIsRefusedAsTheSameContent(t *testing.T) {
	w, _, cardAddress, observation := bridgedWorld(t)
	observation.Mirrored = []memory.Identity{cardSevenIdentity}

	_, err := w.Promote(t.Context(), observation, true)

	var refusal *BridgedError
	if !errors.As(err, &refusal) || !refusal.SameContent || refusal.Address != cardAddress {
		t.Fatalf("Promote = %v, want a same-content *BridgedError naming %s", err, cardAddress)
	}
	if !strings.Contains(err.Error(), "same content") {
		t.Errorf("refusal %q does not say the content is the same", err)
	}
}

// The page of a lineage memory outranks the page of a mirror when both have one, as the page of the memory a memory
// was imported from has always outranked the pages of its content twins.
func TestPromote_TheLineagePageOutranksAMirrorsPage(t *testing.T) {
	w, _, cardAddress, observation := bridgedWorld(t)
	other, err := w.PromoteCard(t.Context(), memory.CardMemory{ID: 8, Project: "memorycard", Type: "decision", Title: "Mirror", Body: "Mirror.", Revision: 1})
	if err != nil {
		t.Fatalf("PromoteCard: %v", err)
	}
	observation.Lineage = []memory.Identity{cardSevenIdentity}
	observation.Mirrored = []memory.Identity{{Source: memorycardSource, ID: "8"}}

	_, err = w.Promote(t.Context(), observation, true)

	var refusal *BridgedError
	if !errors.As(err, &refusal) || refusal.SameContent || refusal.Address != cardAddress || refusal.Address == other.Page.Address {
		t.Fatalf("Promote = %v, want the refusal to name %s as a bridge, not the mirror %s", err, cardAddress, other.Page.Address)
	}
}

// The revision of the page a memory is bridged to is never read: the page is another memory's, and its revision
// says nothing of this one. A page whose revision cannot be parsed bridges as any other does, and is no failure.
func TestBridged_TheRevisionOfTheBridgedPageIsNeverRead(t *testing.T) {
	vaultRoot := t.TempDir()
	// The bridge: the memory's lineage names memory 402 (another part of the observation it was imported from), and
	// its page, with a revision that cannot be parsed, is that memory's.
	writeIdentityPage(t, vaultRoot, "c-000900.md", "---\nsource: memorycard\nsource_id: 402\nsource_revision: not-a-number\nproject: memorycard\naddress: c-000900\n---\n\nBody.\n")
	w := syncCardsWriter(vaultRoot)
	bridged := cardMemory()
	bridged.Pinned = true
	lineage := map[int64][]memory.Identity{bridged.ID: {memoryID("402")}}

	report, err := SyncCards(t.Context(), &fakeCards{memories: []memory.CardMemory{bridged}, lineage: lineage}, w, "memorycard")

	if err != nil || report.Bridged != 1 || len(report.Failed) != 0 || len(report.Promoted) != 0 {
		t.Fatalf("SyncCards = (%+v, %v), want the memory bridged: the revision of the page it is bridged to is not read", report, err)
	}
	bridgedMemory := cardMem(bridged)
	bridgedMemory.Lineage = lineage[bridged.ID]
	_, err = w.Promote(t.Context(), bridgedMemory, true)
	var refusal *BridgedError
	if !errors.As(err, &refusal) {
		t.Fatalf("PromoteCard = %v, want a *BridgedError and not the unparseable revision of the bridged page", err)
	}
}

// Every match the pages are read into names the revision key it was read from, whether the page carries a revision
// or lacks one: an error about a revision that cannot be parsed names the key, so a match without one would name
// nothing. A legacy key a page carries beside the store-agnostic ones makes no match of its own (S7).
func TestReadPages_EveryMatchNamesTheRevisionKeyItWasReadFrom(t *testing.T) {
	vaultRoot := t.TempDir()
	writeIdentityPage(t, vaultRoot, "c-000001.md", "---\nsource: memorycard\nsource_id: 1\nsource_revision: 2\nproject: p\naddress: c-000001\n---\n\nBody.\n")
	writeIdentityPage(t, vaultRoot, "c-000002.md", "---\nsource: memorycard\nsource_id: 2\nproject: p\naddress: c-000002\n---\n\nBody.\n")
	writeIdentityPage(t, vaultRoot, "c-000003.md", "---\nsource: memorycard\nsource_id: 3\nsource_revision: 4\nmemorycard_id: 8\nengram_id: 9\nproject: p\naddress: c-000003\n---\n\nBody.\n")
	writeIdentityPage(t, vaultRoot, "c-000004.md", "---\nmemorycard_id: 4\nmemorycard_revision: 5\nengram_id: 9\nproject: p\naddress: c-000004\n---\n\nBody.\n")

	index, err := readPages(pagesIn(vaultRoot))
	if err != nil {
		t.Fatalf("readPages: %v", err)
	}

	matches := 0
	for identity, carried := range index.carriers {
		for _, match := range carried {
			matches++
			if match.RevisionKey == "" {
				t.Errorf("the match of %+v on %s names no revision key", identity, match.File)
			}
		}
	}
	if matches != 3 {
		t.Fatalf("read %d matches, want the 3 the three pages of the store-agnostic keys name (the fourth carries legacy keys only)", matches)
	}
}

// A memorycard memory whose project changed leaves its page behind in the old project: the memory gets a page of
// its own in the new one, and the old page says it is superseded by it, as the page of a memory that moved always did.
func TestPromote_AMemorycardMemoryThatChangedProjectSupersedesThePageItLeftBehind(t *testing.T) {
	vaultRoot := t.TempDir()
	w := syncCardsWriter(vaultRoot)
	old := cardMemory()
	old.Project = "old-project"
	first, err := w.PromoteCard(t.Context(), old)
	if err != nil || first.Action.Kind != ActionCreated {
		t.Fatalf("PromoteCard = (%+v, %v), want the page in the old project", first, err)
	}

	moved, err := w.PromoteCard(t.Context(), cardMemory())

	if err != nil || moved.Action.Kind != ActionCreated || moved.Page.Address == first.Page.Address {
		t.Fatalf("PromoteCard = (%+v, %v), want a new page in the new project", moved, err)
	}
	raw, err := os.ReadFile(filepath.Join(vaultRoot, first.Page.Path))
	if err != nil {
		t.Fatalf("read the old page: %v", err)
	}
	if !strings.Contains(string(raw), "status: superseded") || !strings.Contains(string(raw), "[["+moved.Page.Address) {
		t.Fatalf("the page left behind does not say it is superseded by %s:\n%s", moved.Page.Address, raw)
	}
}
