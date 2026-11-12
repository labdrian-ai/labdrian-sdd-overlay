package memory_test

import (
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// An identity is a source plus that source's own id: the ids of two stores are unrelated numbers, so the same id
// under two sources names two memories, and the same pair names one.
func TestAnIdentityIsASourceAndItsIDAndSeparatesStores(t *testing.T) {
	engram := memory.Identity{Source: "engram", ID: "7"}
	card := memory.Identity{Source: "memorycard", ID: "7"}
	if engram == card {
		t.Fatal("the same id under two sources compared equal: an identity must carry its source")
	}
	if engram != (memory.Identity{Source: "engram", ID: "7"}) {
		t.Fatal("the same source and id compared different")
	}
	held := map[memory.Identity]string{engram: "an observation", card: "a memory"}
	if held[memory.Identity{Source: "memorycard", ID: "7"}] != "a memory" || len(held) != 2 {
		t.Fatalf("identities as map keys = %v, want two distinct entries", held)
	}
}
