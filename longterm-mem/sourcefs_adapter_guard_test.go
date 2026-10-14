// This file shares package guard with engram_adapter_guard_test.go and its walk test: it uses importersOf,
// defined in the first, and writeTree and importing, defined in the second, and a different package name
// would stop it building.
package guard

import (
	"reflect"
	"testing"
)

// sourcefsAdapter is the import path of the adapter that answers what ingestion asks of the machine that holds
// the local sources it reads, on the file system of the machine it runs on.
const sourcefsAdapter = "github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/sourcefs"

// sourcefsDir is the adapter itself, exempt for the reason engram's directory is.
const sourcefsDir = "internal/sourcefs"

// TestOnlyTheCompositionRootImportsTheSourceFSAdapter keeps the file system out of the vocabulary of ingestion:
// ingest declares the port it asks the machine through (SourceReader), over its own types, and the composition
// root hands the ingestion use case the adapter that satisfies it.
func TestOnlyTheCompositionRootImportsTheSourceFSAdapter(t *testing.T) {
	importers, err := importersOf(".", sourcefsAdapter, sourcefsDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range importers {
		t.Errorf("%s imports the local sources adapter: only %s may; ask the machine through the port ingest owns", path, compositionRoot)
	}
}

// The walk is the rule, so it is tested on a tree built for it: the composition root and the adapter are let
// through, every other package is flagged.
func TestImportersOfTheSourceFSAdapterFlagsEveryoneButTheCompositionRootAndTheAdapter(t *testing.T) {
	adapter := `"` + sourcefsAdapter + `"`
	root := writeTree(t, map[string]string{
		"cmd/longterm-mem/main.go":       importing(adapter),
		"internal/sourcefs/sourcefs.go":  importing(adapter),
		"internal/ingest/source.go":      importing(adapter),
		"internal/promote/writer.go":     importing(`s ` + adapter),
		"internal/query/query.go":        importing(`_ ` + adapter),
		"internal/ingest/source_test.go": importing(adapter),
		"internal/vaultfs/vault.go":      importing(`"fmt"`),
	})

	got, err := importersOf(root, sourcefsAdapter, sourcefsDir)
	if err != nil {
		t.Fatalf("importersOf: %v", err)
	}
	want := []string{
		"internal/ingest/source.go",
		"internal/promote/writer.go",
		"internal/query/query.go",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("importersOf = %v, want %v (test files are not looked at: ingest's own guard reads them)", got, want)
	}
}
