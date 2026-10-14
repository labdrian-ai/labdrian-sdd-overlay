// This file shares package guard with skillstale_purity_guard_test.go (ambientUsesIn), query_adapters_guard_test.go
// (adapterImportsIn, adapterPackages and modulePath), the architecture guard (rings) and the Engram guard and its
// walk test (importersOf, writeTree and importing, compositionRoot).
package guard

import (
	"reflect"
	"strings"
	"testing"
)

// vaultregPackage is the rule that finds the vault of a project and the use case that reads and seeds the registry
// it finds it in: a function of the flag, the environment's view and the registry, and a Store port that holds the
// registry, all handed to it.
const vaultregPackage = "internal/vaultreg"

// vaultregfsAdapter is the import path of the adapter that keeps the registry of vaults in a file.
const vaultregfsAdapter = "github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vaultregfs"

// vaultregfsDir is the adapter itself, exempt for the reason engram's directory is.
const vaultregfsDir = "internal/vaultregfs"

// TestVaultregNamesNoAdapter keeps the vault rule free of the concrete adapters, in its production files and in its
// tests alike (the dependency rule of the rings reads production code only): not the registry file, whose Store it
// is handed, and not the durable replacement or the vault's own files.
func TestVaultregNamesNoAdapter(t *testing.T) {
	problems, err := adapterImportsIn(".", vaultregPackage, adapterPackages(rings), "")
	if err != nil {
		t.Fatal(err)
	}
	for _, problem := range problems {
		t.Error(problem)
	}
}

// TestVaultregReachesNothingAmbient keeps the vault rule, tests included, off the operating system, the process
// environment, the working directory, the wall clock and the file system: the architecture guard holds the
// production files to that, and this one also reads the tests, so that no case of the rule depends on the machine
// it runs on. What the rule reads of the machine it is handed (Environment), and a test that needs a file
// belongs to the adapter that has one.
func TestVaultregReachesNothingAmbient(t *testing.T) {
	problems, err := ambientUsesIn(".", vaultregPackage)
	if err != nil {
		t.Fatal(err)
	}
	for _, problem := range problems {
		t.Error(problem)
	}
}

// TestOnlyTheCompositionRootImportsTheVaultRegistryAdapter keeps the file out of the vocabulary of the vault rule:
// vaultreg declares the Store port it reads the registry through, over its own types, and the composition root
// hands the use case the adapter that satisfies it.
func TestOnlyTheCompositionRootImportsTheVaultRegistryAdapter(t *testing.T) {
	importers, err := importersOf(".", vaultregfsAdapter, vaultregfsDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range importers {
		t.Errorf("%s imports the vault registry adapter: only %s may; read the registry through the Store port vaultreg owns", path, compositionRoot)
	}
}

// The walk is the rule, so it is tested on a tree built for it: the composition root and the adapter are let
// through, every other package is flagged.
func TestImportersOfTheVaultRegistryAdapterFlagsEveryoneButTheCompositionRootAndTheAdapter(t *testing.T) {
	adapter := `"` + vaultregfsAdapter + `"`
	root := writeTree(t, map[string]string{
		"cmd/longterm-mem/main.go":           importing(adapter),
		"internal/vaultregfs/store.go":       importing(adapter),
		"internal/vaultreg/registry.go":      importing(adapter),
		"internal/adoption/adoption.go":      importing(`_ ` + adapter),
		"internal/ops/status.go":             importing(`r ` + adapter),
		"internal/vaultreg/registry_test.go": importing(adapter),
		"internal/vaultfs/vault.go":          importing(`"fmt"`),
	})

	got, err := importersOf(root, vaultregfsAdapter, vaultregfsDir)
	if err != nil {
		t.Fatalf("importersOf: %v", err)
	}
	want := []string{
		"internal/adoption/adoption.go",
		"internal/ops/status.go",
		"internal/vaultreg/registry.go",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("importersOf = %v, want %v (test files are not looked at: the vault rule's own guard reads them)", got, want)
	}
}

// The rule is tested on a tree built for it: a file that reads the environment or the home, touches the file
// system, reads the clock, or names an adapter, in a production file or in a test, is flagged, and a rule that is
// handed everything it reads passes.
func TestVaultregGuardsFlagTheMachineTheClockTheDiskAndTheAdaptersInTestsToo(t *testing.T) {
	imp := func(dir string) string { return `"` + modulePath + "/" + dir + `"` }
	root := writeTree(t, map[string]string{
		"internal/vaultreg/ok.go":        "package p\n\nimport (\n\t\"errors\"\n\t\"fmt\"\n\t\"path/filepath\"\n\t\"strings\"\n)\n\nvar _ = errors.New\nvar _ = fmt.Errorf\nvar _ = filepath.Join\nvar _ = filepath.IsAbs\nvar _ = strings.HasPrefix\n",
		"internal/vaultreg/home.go":      "package p\n\nimport \"os\"\n\nvar _ = os.UserHomeDir\n",
		"internal/vaultreg/abs.go":       "package p\n\nimport \"path/filepath\"\n\nvar _ = filepath.Abs\n",
		"internal/vaultreg/clock.go":     "package p\n\nimport \"time\"\n\nvar _ = time.Now\n",
		"internal/vaultreg/store.go":     importing(imp("internal/vaultregfs")),
		"internal/vaultreg/durable.go":   importing(imp("internal/durable")),
		"internal/vaultreg/env_test.go":  "package p\n\nimport \"testing\"\n\nfunc TestX(t *testing.T) { t.Setenv(\"HOME\", t.TempDir()) }\n",
		"internal/vaultreg/file_test.go": "package p\n\nimport \"os\"\n\nvar _ = os.WriteFile\n",
	})

	ambient, err := ambientUsesIn(root, vaultregPackage)
	if err != nil {
		t.Fatalf("ambientUsesIn: %v", err)
	}
	adapters, err := adapterImportsIn(root, vaultregPackage, []string{"internal/vaultregfs", "internal/durable"}, "")
	if err != nil {
		t.Fatalf("adapterImportsIn: %v", err)
	}

	flagged := map[string]bool{}
	for _, problem := range append(ambient, adapters...) {
		flagged[strings.SplitN(problem, " ", 2)[0]] = true
	}
	want := map[string]bool{
		"internal/vaultreg/home.go":      true,
		"internal/vaultreg/abs.go":       true,
		"internal/vaultreg/clock.go":     true,
		"internal/vaultreg/store.go":     true,
		"internal/vaultreg/durable.go":   true,
		"internal/vaultreg/env_test.go":  true,
		"internal/vaultreg/file_test.go": true,
	}
	if !reflect.DeepEqual(flagged, want) {
		t.Fatalf("flagged = %v, want %v (the pure uses of errors, fmt, filepath.Join, filepath.IsAbs and strings pass)", flagged, want)
	}
}
