package memorycard

import (
	"reflect"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// The Source adapter serves memorycard's memories as memory.Memory over the fixtures of store_test.go: memory 1 is
// superseded by 2 and imported from Engram observation 402, 2 is mapped by another source, 3 has no import_map
// row, 4 is archived and 5 belongs to another project.

func openSource(t *testing.T, path string) *Source {
	t.Helper()
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close the store: %v", err)
		}
	})
	return NewSource(store)
}

func card(id string) memory.Identity { return memory.Identity{Source: SourceName, ID: id} }

func wantMemory1() memory.Memory {
	return memory.Memory{
		Identity: card("1"), Project: "p", Type: "decision", Title: "Old", Body: "old body", TopicKey: "a/b", Pinned: true,
		Status: memory.StatusSuperseded, Successor: card("2"), SuccessorTitle: "New", Revision: 3,
		Lineage: []memory.Identity{{Source: "engram", ID: "402"}},
	}
}

func wantMemory2() memory.Memory {
	return memory.Memory{Identity: card("2"), Project: "p", Type: "decision", Title: "New", Body: "new body", Status: memory.StatusActive, Revision: 1}
}

func wantMemory4() memory.Memory {
	return memory.Memory{
		Identity: card("4"), Project: "p", Type: "decision", Title: "Gone", Body: "archived body", TopicKey: "a/gone", Pinned: true,
		Status: memory.StatusArchived, Revision: 1,
	}
}

func TestSourceMapsAMemoryWithItsStatusSuccessorRevisionAndEngramLineage(t *testing.T) {
	source := openSource(t, listFixture(t))

	got, ok, err := source.ByID("1")
	if err != nil || !ok {
		t.Fatalf("ByID(1) = %v, %v", ok, err)
	}
	if want := wantMemory1(); !reflect.DeepEqual(got, want) {
		t.Errorf("ByID(1) = %+v\nwant %+v", got, want)
	}
	got, ok, err = source.ByID("4")
	if err != nil || !ok {
		t.Fatalf("ByID(4) = %v, %v", ok, err)
	}
	if want := wantMemory4(); !reflect.DeepEqual(got, want) {
		t.Errorf("ByID(4) = %+v\nwant %+v: an archived memory is served by id", got, want)
	}
	for _, id := range []string{"99", "not-a-number"} {
		if got, ok, err := source.ByID(id); ok || err != nil {
			t.Errorf("ByID(%q) = %+v, %v, %v, want not found and no error", id, got, ok, err)
		}
	}
}

func TestSourceCandidatesAreTheActiveAndSupersededAndMemoriesIsEveryStatus(t *testing.T) {
	source := openSource(t, listFixture(t))

	candidates, err := source.Candidates("p")
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 3 || !reflect.DeepEqual(candidates[0], wantMemory1()) || !reflect.DeepEqual(candidates[1], wantMemory2()) {
		t.Errorf("Candidates(p) = %+v, want memories 1, 2 and 3 in id order (no archived 4, no other project's 5)", candidates)
	}
	all, err := source.Memories("p")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 4 || !reflect.DeepEqual(all[0], wantMemory1()) || !reflect.DeepEqual(all[3], wantMemory4()) {
		t.Errorf("Memories(p) = %+v, want memories 1 to 4 in id order, the archived one last and mapped as ByID maps it", all)
	}
}
