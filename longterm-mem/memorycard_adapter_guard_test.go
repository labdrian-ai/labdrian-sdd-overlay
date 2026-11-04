package guard

import "testing"

// memorycardAdapter is the import path of the adapter that reads memorycard's SQLite database.
const memorycardAdapter = "github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memorycard"

// TestOnlyTheCompositionRootImportsTheMemorycardAdapter holds the second store to the rule the first has
// (engram_adapter_guard_test.go): promotion reads a memorycard memory through the lookup port it owns, over the
// types of internal/memory, and only the composition root names the adapter that serves it.
func TestOnlyTheCompositionRootImportsTheMemorycardAdapter(t *testing.T) {
	importers, err := importersOf(".", memorycardAdapter, "internal/memorycard")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range importers {
		t.Errorf("%s imports the memorycard adapter: only %s may; read memory through the port the package owns, over the types of internal/memory", path, compositionRoot)
	}
}
