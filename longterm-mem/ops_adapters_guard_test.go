// This file shares package guard with query_adapters_guard_test.go: it uses adapterImportsIn and adapterPackages,
// defined there, and writeTree and importing, defined in engram_adapter_guard_walk_test.go.
package guard

import (
	"reflect"
	"strings"
	"testing"
)

// opsPackage is the package that reads the vault, the sync state, Engram's live rows, the embedding index and the
// embedding backend through ports it owns.
const opsPackage = "internal/ops"

// opsTestMayDriveARealVault is the one adapter a test of the ops package may import: the vault file system
// adapter, to put real files behind the ports a case drives (the golden files of the diagnostics are recorded
// over them). It does not import ops, so the import is no cycle. The adapters of the embedding index and of the
// backend are out of reach of its tests too: the port is driven by a fake, and the composition root holds the
// adapter to the port (cmd/longterm-mem, ops_ports_test.go).
const opsTestMayDriveARealVault = "internal/vaultfs"

// TestOpsNamesNoAdapter keeps the ops package free of the concrete adapters, in its production files and in its
// tests alike (the dependency rule of the rings reads production code only, and a debt line would hide an import
// in a test as well), but for the vault file system adapter in a test. The ports it reads through are its own:
// the vault's files (RootInspector, PageReader, AddressMapReader, PrecedenceReader, IndexReader, LogReader,
// SyncStateReader) and the embedding index (EmbeddingIndexReader), with the two failures of the backend told by
// what the error says (EmbedModelMissing) and an index nobody built by NoEmbeddingIndex.
func TestOpsNamesNoAdapter(t *testing.T) {
	problems, err := adapterImportsIn(".", opsPackage, adapterPackages(rings), opsTestMayDriveARealVault)
	if err != nil {
		t.Fatal(err)
	}
	for _, problem := range problems {
		t.Error(problem)
	}
}

// The rule is tested on a tree built for it: a production file naming any adapter is flagged, the vault file
// system adapter included, and a test file may name that one and no other, the embedding index and the embedding
// client among them.
func TestOpsMayDriveARealVaultInATestAndNamesNoOtherAdapter(t *testing.T) {
	imp := func(dir string) string { return `"` + modulePath + "/" + dir + `"` }
	root := writeTree(t, map[string]string{
		"internal/ops/doctor.go":        importing(imp("internal/memory")),
		"internal/ops/vecindex.go":      importing(imp("internal/vecindex")),
		"internal/ops/embed.go":         importing(imp("internal/embed")),
		"internal/ops/vaultfs.go":       importing(imp("internal/vaultfs")),
		"internal/ops/golden_test.go":   importing(imp("internal/vaultfs")),
		"internal/ops/index_test.go":    importing(imp("internal/vecindex")),
		"internal/ops/backend_test.go":  importing(`e ` + imp("internal/embed")),
		"internal/ops/testdata/file.go": importing(imp("internal/vecindex")),
	})
	adapters := []string{"internal/embed", "internal/vaultfs", "internal/vecindex"}

	got, err := adapterImportsIn(root, opsPackage, adapters, opsTestMayDriveARealVault)
	if err != nil {
		t.Fatalf("adapterImportsIn: %v", err)
	}

	var files []string
	for _, problem := range got {
		files = append(files, strings.SplitN(problem, " ", 2)[0])
	}
	want := []string{
		"internal/ops/backend_test.go",
		"internal/ops/embed.go",
		"internal/ops/index_test.go",
		"internal/ops/vaultfs.go",
		"internal/ops/vecindex.go",
	}
	if !reflect.DeepEqual(files, want) {
		t.Fatalf("flagged files = %v, want %v", files, want)
	}
}
