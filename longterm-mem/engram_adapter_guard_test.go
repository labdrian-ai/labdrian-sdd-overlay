package guard

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// engramAdapter is the import path of the adapter that reads Engram's SQLite database.
const engramAdapter = "github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"

// compositionRoot is the one package that may name the Engram adapter: it builds the store and hands it
// to the consumers, which declare the reader ports they need (promote.Memory, query.Memory,
// skillstale.ObservationLister) over the types of internal/memory.
const compositionRoot = "cmd/longterm-mem"

// TestOnlyTheCompositionRootImportsTheEngramAdapter keeps the memory domain model out of the adapter's
// vocabulary. The dependency rule of the rings stops a domain or application package from importing an
// adapter, but not an adapter from importing another (the MCP server once carried the adapter's
// observation type in its own); this test closes that gap for the one adapter that used to define the
// model. Only non-test files count: a test may build a real store to drive a consumer through its port.
func TestOnlyTheCompositionRootImportsTheEngramAdapter(t *testing.T) {
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		dir := filepath.ToSlash(filepath.Dir(path))
		if dir == compositionRoot || dir == "internal/engram" {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, spec := range file.Imports {
			imported, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			if imported == engramAdapter {
				t.Errorf("%s imports the Engram adapter: only %s may; read memory through the reader port the package owns, over the types of internal/memory", path, compositionRoot)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
