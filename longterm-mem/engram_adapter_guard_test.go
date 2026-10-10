package guard

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// engramAdapter is the import path of the adapter that reads Engram's SQLite database.
const engramAdapter = "github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"

// compositionRoot is the directory whose packages may name the Engram adapter: it builds the store and
// hands it to the consumers, which declare the reader ports they need (promote.Memory, query.Memory,
// skillstale.ObservationLister) over the types of internal/memory. Its subpackages count as the root.
const compositionRoot = "cmd/longterm-mem"

// adapterDir is the adapter itself. It is exempt because a package may import itself and its own
// subpackages: the rule is about who else knows the adapter's vocabulary, not about the adapter.
const adapterDir = "internal/engram"

// TestOnlyTheCompositionRootImportsTheEngramAdapter keeps the memory domain model out of the adapter's
// vocabulary. The dependency rule of the rings stops a domain or application package from importing an
// adapter, but not an adapter from importing another (the MCP server once carried the adapter's
// observation type in its own); this test closes that gap for the one adapter that used to define the
// model. Only non-test files count: a test may build a real store to drive a consumer through its port.
func TestOnlyTheCompositionRootImportsTheEngramAdapter(t *testing.T) {
	importers, err := adapterImporters(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range importers {
		t.Errorf("%s imports the Engram adapter: only %s may; read memory through the reader port the package owns, over the types of internal/memory", path, compositionRoot)
	}
}

// adapterImporters lists, as slash-separated paths relative to root, the production Go files that import
// the Engram adapter and are neither part of the composition root nor of the adapter. It reads the
// import path itself, so an alias, a dot import and a blank import all count.
//
// It looks only at this module's own production code: it skips test files, directories named testdata or
// vendor, hidden and underscore directories (the go tool's own rule), and any directory below root that
// holds a go.mod (another module). A file it cannot parse is an error, not a skip.
func adapterImporters(root string) ([]string, error) {
	var found []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			return skipDirectory(root, path, rel, d.Name())
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		dir := filepath.ToSlash(filepath.Dir(rel))
		if within(dir, compositionRoot) || within(dir, adapterDir) {
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
				found = append(found, rel)
			}
		}
		return nil
	})
	sort.Strings(found)
	return found, err
}

// skipDirectory decides whether the walk goes into a directory.
func skipDirectory(root, path, rel, name string) error {
	if rel == "." {
		return nil
	}
	if name == "testdata" || name == "vendor" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
		return filepath.SkipDir
	}
	if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil && path != root {
		return filepath.SkipDir
	}
	return nil
}

// within reports whether dir is base or a directory below it.
func within(dir, base string) bool {
	return dir == base || strings.HasPrefix(dir, base+"/")
}
