// This file shares package guard with engram_adapter_guard_test.go and its walk test: it uses importersOf,
// defined in the first, and writeTree and importing, defined in the second, and a different package name
// would stop it building.
package guard

import (
	"reflect"
	"testing"
)

// projectidfsAdapter is the import path of the adapter that answers the identity rules' questions of a
// repository from the metadata git keeps on a disk.
const projectidfsAdapter = "github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/projectidfs"

// projectidfsDir is the adapter itself, exempt for the reason engram's directory is.
const projectidfsDir = "internal/projectidfs"

// TestOnlyTheCompositionRootImportsTheProjectIDAdapter keeps the disk out of the vocabulary of the identity
// rules: projectid declares the port it reads the repository through (RepositoryInspector), over its own types,
// and the composition root hands the rules, and what builds on them, the adapter that satisfies it.
func TestOnlyTheCompositionRootImportsTheProjectIDAdapter(t *testing.T) {
	importers, err := importersOf(".", projectidfsAdapter, projectidfsDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range importers {
		t.Errorf("%s imports the project identity adapter: only %s may; read the repository through the port projectid owns", path, compositionRoot)
	}
}

// The walk is the rule, so it is tested on a tree built for it: the composition root and the adapter are let
// through, every other package is flagged.
func TestImportersOfTheProjectIDAdapterFlagsEveryoneButTheCompositionRootAndTheAdapter(t *testing.T) {
	adapter := `"` + projectidfsAdapter + `"`
	root := writeTree(t, map[string]string{
		"cmd/longterm-mem/main.go":             importing(adapter),
		"internal/projectidfs/projectidfs.go":  importing(adapter),
		"internal/projectid/projectid.go":      importing(adapter),
		"internal/identityledger/ledger.go":    importing(`i ` + adapter),
		"internal/adoption/adoption.go":        importing(`_ ` + adapter),
		"internal/projectid/projectid_test.go": importing(adapter),
		"internal/vaultfs/vault.go":            importing(`"fmt"`),
	})

	got, err := importersOf(root, projectidfsAdapter, projectidfsDir)
	if err != nil {
		t.Fatalf("importersOf: %v", err)
	}
	want := []string{
		"internal/adoption/adoption.go",
		"internal/identityledger/ledger.go",
		"internal/projectid/projectid.go",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("importersOf = %v, want %v (test files are not looked at: the identity guard reads them)", got, want)
	}
}
