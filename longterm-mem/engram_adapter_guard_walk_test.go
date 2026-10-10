// This file shares package guard with engram_adapter_guard_test.go: the tests below call
// adapterImporters, which is defined there, and a different package name would stop them building.
package guard

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir for %s: %v", name, err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return root
}

func importing(importLine string) string {
	return "package p\n\nimport " + importLine + "\n"
}

// The walk is the rule, so the rule is tested on trees built for it: who is allowed, who is flagged, and
// what the walk does not look at.
func TestAdapterImportersFlagsEveryoneButTheCompositionRootAndTheAdapter(t *testing.T) {
	adapter := `"` + engramAdapter + `"`
	root := writeTree(t, map[string]string{
		"cmd/longterm-mem/main.go":          importing(adapter),
		"cmd/longterm-mem/wiring/wire.go":   importing(adapter),
		"internal/engram/store.go":          importing(adapter),
		"internal/engram/sub/helper.go":     importing(adapter),
		"internal/query/query.go":           importing(adapter),
		"internal/mcpserver/server.go":      importing(`e ` + adapter),
		"internal/promote/promote.go":       importing(`. ` + adapter),
		"internal/skillstale/skillstale.go": importing(`_ ` + adapter),
		"internal/ops/ops.go":               importing(`"fmt"`),
	})

	got, err := adapterImporters(root)
	if err != nil {
		t.Fatalf("adapterImporters: %v", err)
	}
	want := []string{
		"internal/mcpserver/server.go",
		"internal/promote/promote.go",
		"internal/query/query.go",
		"internal/skillstale/skillstale.go",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("adapterImporters = %v, want %v (the root and its subpackages, the adapter and its subpackages are exempt; an alias, a dot and a blank import still count)", got, want)
	}
}

func TestAdapterImportersLooksOnlyAtTheModulesOwnProductionCode(t *testing.T) {
	adapter := `"` + engramAdapter + `"`
	root := writeTree(t, map[string]string{
		"internal/query/query_test.go":      importing(adapter), // a test may drive a consumer through a real store
		"internal/query/testdata/sample.go": importing(adapter),
		"vendor/example/v.go":               importing(adapter),
		".hidden/h.go":                      importing(adapter),
		"tools/go.mod":                      "module example.com/tools\n",
		"tools/t.go":                        importing(adapter), // another module
		"internal/query/notes.txt":          adapter,
	})

	got, err := adapterImporters(root)
	if err != nil {
		t.Fatalf("adapterImporters: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("adapterImporters = %v, want none: test files, testdata, vendor, hidden directories, nested modules and non-Go files are not this module's production code", got)
	}
}

func TestAdapterImportersReportsAFileItCannotParse(t *testing.T) {
	root := writeTree(t, map[string]string{"internal/query/broken.go": "package p\n\nimport (\n"})
	if _, err := adapterImporters(root); err == nil {
		t.Fatal("adapterImporters accepted a file it could not parse; a guard that skips what it cannot read guards nothing")
	}
}
