// This file shares package guard with engram_adapter_guard_test.go and its walk test: it uses importersOf,
// defined in the first, and writeTree and importing, defined in the second, and a different package name
// would stop it building.
package guard

import (
	"reflect"
	"testing"
)

// skillstalefsAdapter is the import path of the adapter that answers skillstale's questions on a file system
// and a git repository.
const skillstalefsAdapter = "github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/skillstalefs"

// skillstalefsDir is the adapter itself, exempt for the reason engram's directory is.
const skillstalefsDir = "internal/skillstalefs"

// TestOnlyTheCompositionRootImportsTheSkillstaleFileSystemAdapter keeps the file system and the PATH out of the
// vocabulary of the detector: skillstale declares the ports it reads through (SkillSource, PathFactSource,
// CommandResolver), over its own types, and the composition root hands it the adapter that satisfies them.
func TestOnlyTheCompositionRootImportsTheSkillstaleFileSystemAdapter(t *testing.T) {
	importers, err := importersOf(".", skillstalefsAdapter, skillstalefsDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range importers {
		t.Errorf("%s imports the skillstale file system adapter: only %s may; read the project through the ports the detector owns", path, compositionRoot)
	}
}

// The walk is the rule, so it is tested on a tree built for it: the composition root and the adapter are let
// through, every other package is flagged.
func TestImportersOfTheSkillstaleFileSystemAdapterFlagsEveryoneButTheCompositionRootAndTheAdapter(t *testing.T) {
	adapter := `"` + skillstalefsAdapter + `"`
	root := writeTree(t, map[string]string{
		"cmd/longterm-mem/main.go":               importing(adapter),
		"internal/skillstalefs/project.go":       importing(adapter),
		"internal/skillstale/skillstale.go":      importing(adapter),
		"internal/staleness/staleness.go":        importing(`s ` + adapter),
		"internal/ops/doctor.go":                 importing(`_ ` + adapter),
		"internal/skillstale/skillstale_test.go": importing(adapter),
		"internal/vaultfs/vault.go":              importing(`"fmt"`),
	})

	got, err := importersOf(root, skillstalefsAdapter, skillstalefsDir)
	if err != nil {
		t.Fatalf("importersOf: %v", err)
	}
	want := []string{
		"internal/ops/doctor.go",
		"internal/skillstale/skillstale.go",
		"internal/staleness/staleness.go",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("importersOf = %v, want %v (test files are not looked at: skillstale's own guard reads them)", got, want)
	}
}
