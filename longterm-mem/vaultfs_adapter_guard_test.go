// This file shares package guard with engram_adapter_guard_test.go and its walk test: it uses importersOf,
// defined in the first, and writeTree and importing, defined in the second, and a different package name
// would stop it building.
package guard

import (
	"reflect"
	"testing"
)

// vaultfsAdapter is the import path of the adapter that reads and writes the files of a vault.
const vaultfsAdapter = "github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vaultfs"

// vaultfsDir is the adapter itself, exempt for the reason engram's directory is.
const vaultfsDir = "internal/vaultfs"

// TestOnlyTheCompositionRootImportsTheVaultFileSystemAdapter keeps the vault's file system out of the
// vocabulary of the packages that use it. Promote and ops declare the repository ports they need
// (promote.PrecedenceRepository, ops.PrecedenceReader), over the types of their own packages; the adapter
// that satisfies them is handed to them by the composition root. The dependency rule of the rings stops a
// domain or application package from importing an adapter, but not an adapter from importing another, so
// this test closes that gap as the Engram one does.
func TestOnlyTheCompositionRootImportsTheVaultFileSystemAdapter(t *testing.T) {
	importers, err := importersOf(".", vaultfsAdapter, vaultfsDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range importers {
		t.Errorf("%s imports the vault file system adapter: only %s may; read and write the vault through the repository port the package owns", path, compositionRoot)
	}
}

// The walk is the rule, so it is tested on a tree built for it: the composition root and the adapter are
// let through, every other package is flagged.
func TestImportersOfTheVaultFileSystemAdapterFlagsEveryoneButTheCompositionRootAndTheAdapter(t *testing.T) {
	adapter := `"` + vaultfsAdapter + `"`
	root := writeTree(t, map[string]string{
		"cmd/longterm-mem/main.go":     importing(adapter),
		"internal/vaultfs/vault.go":    importing(adapter),
		"internal/promote/promote.go":  importing(adapter),
		"internal/ops/doctor.go":       importing(`v ` + adapter),
		"internal/vault/runner.go":     importing(`_ ` + adapter),
		"internal/engram/store.go":     importing(`"fmt"`),
		"internal/vaultlayout/path.go": importing(`"fmt"`),
	})

	got, err := importersOf(root, vaultfsAdapter, vaultfsDir)
	if err != nil {
		t.Fatalf("importersOf: %v", err)
	}
	want := []string{
		"internal/ops/doctor.go",
		"internal/promote/promote.go",
		"internal/vault/runner.go",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("importersOf = %v, want %v", got, want)
	}
}
