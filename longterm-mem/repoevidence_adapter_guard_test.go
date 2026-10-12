// This file shares package guard with engram_adapter_guard_test.go and its walk test: it uses importersOf,
// defined in the first, and writeTree and importing, defined in the second, and a different package name
// would stop it building.
package guard

import (
	"reflect"
	"testing"
)

// repoevidenceAdapter is the import path of the adapter that answers staleness's questions of a repository on a
// file system and git.
const repoevidenceAdapter = "github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/repoevidence"

// repoevidenceDir is the adapter itself, exempt for the reason engram's directory is.
const repoevidenceDir = "internal/repoevidence"

// TestOnlyTheCompositionRootImportsTheRepoEvidenceAdapter keeps the file system and git out of the vocabulary of
// the staleness rule: staleness declares the port it reads the repository through (RepoEvidence), over its own
// types, and the composition root hands it, and what builds on it, the adapter that satisfies it.
func TestOnlyTheCompositionRootImportsTheRepoEvidenceAdapter(t *testing.T) {
	importers, err := importersOf(".", repoevidenceAdapter, repoevidenceDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range importers {
		t.Errorf("%s imports the repository evidence adapter: only %s may; read the repository through the port staleness owns", path, compositionRoot)
	}
}

// The walk is the rule, so it is tested on a tree built for it: the composition root and the adapter are let
// through, every other package is flagged.
func TestImportersOfTheRepoEvidenceAdapterFlagsEveryoneButTheCompositionRootAndTheAdapter(t *testing.T) {
	adapter := `"` + repoevidenceAdapter + `"`
	root := writeTree(t, map[string]string{
		"cmd/longterm-mem/main.go":              importing(adapter),
		"internal/repoevidence/repoevidence.go": importing(adapter),
		"internal/staleness/staleness.go":       importing(adapter),
		"internal/skillstalefs/project.go":      importing(`e ` + adapter),
		"internal/skillstale/skillstale.go":     importing(`_ ` + adapter),
		"internal/staleness/staleness_test.go":  importing(adapter),
		"internal/vaultfs/vault.go":             importing(`"fmt"`),
	})

	got, err := importersOf(root, repoevidenceAdapter, repoevidenceDir)
	if err != nil {
		t.Fatalf("importersOf: %v", err)
	}
	want := []string{
		"internal/skillstale/skillstale.go",
		"internal/skillstalefs/project.go",
		"internal/staleness/staleness.go",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("importersOf = %v, want %v (test files are not looked at: staleness's own guard reads them)", got, want)
	}
}
