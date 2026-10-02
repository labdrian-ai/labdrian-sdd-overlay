// Package capabilitytest is test tooling for the capability declarations: the adapter
// of capability.TestCatalog that reads the engine's test sources, and the guard that
// every test a declaration names exists. The rule itself, that a named test must exist,
// is capability.CheckEvidence and is pure; what reads the files is here, which is why it
// is not part of engine/capability, whose declarations, validation and report stay free
// of the file system. Nothing in the program imports it; the tests that guard the
// shipped declarations do.
package capabilitytest

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/capability"
)

// NewCatalog returns the capability.TestCatalog that answers from the test sources of the
// engine tree fsys, whose root is the engine root. A reference <dir>:<TestName> is
// satisfied only by a top-level function TestName, declared in a _test.go file directly
// inside <dir>, that go test would run: no receiver, no type parameters, no results, and
// one parameter of type *testing.T. A same-named function in a non-test file, a method, a
// benchmark, a TestMain, or a name whose first character after "Test" is a lowercase
// letter (which go test skips) does not count.
//
// The catalog reads the source with go/parser. It starts no process and does not run go
// list, so it works in the same sandboxes the tests do. It proves the named test is
// declared, not that it passes and not that it runs on every platform: build constraints
// are not evaluated. It reads through fsys only, so it cannot reach a path fsys does not
// hold (os.DirFS refuses one that climbs out of its root), and it parses a directory once,
// when it is first asked about it: the answer for a directory, including why it could not
// be given, is kept for the life of the catalog. A catalog is not safe for concurrent use.
func NewCatalog(fsys fs.FS) capability.TestCatalog {
	return &sourceCatalog{fsys: fsys, dirs: make(map[string]dirTests)}
}

// sourceCatalog is the catalog NewCatalog builds.
type sourceCatalog struct {
	fsys fs.FS
	dirs map[string]dirTests
}

// HasTest implements capability.TestCatalog.
func (c *sourceCatalog) HasTest(dir, name string) (bool, error) {
	found, ok := c.dirs[dir]
	if !ok {
		found = loadTestFuncs(c.fsys, dir)
		c.dirs[dir] = found
	}
	if found.err != nil {
		return false, found.err
	}
	return found.names[name], nil
}

// dirTests is what one directory's _test.go files declare, or why that could not be
// determined.
type dirTests struct {
	names map[string]bool
	err   error
}

// loadTestFuncs parses every regular _test.go file directly inside relDir of fsys and
// collects the names of the runnable test functions they declare. relDir is a validated,
// clean, slash-separated relative directory.
func loadTestFuncs(fsys fs.FS, relDir string) dirTests {
	entries, err := fs.ReadDir(fsys, relDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return dirTests{err: fmt.Errorf("directory %q not found under the engine root", relDir)}
		}
		return dirTests{err: fmt.Errorf("read directory %q: %w", relDir, err)}
	}

	names := make(map[string]bool)
	fset := token.NewFileSet()
	for _, entry := range entries {
		// A directory or symlink whose name ends in _test.go is not a test
		// source file.
		if !entry.Type().IsRegular() || !strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		src, err := fs.ReadFile(fsys, path.Join(relDir, entry.Name()))
		if err != nil {
			return dirTests{err: fmt.Errorf("read %s/%s: %w", relDir, entry.Name(), err)}
		}
		file, err := parser.ParseFile(fset, path.Join(relDir, entry.Name()), src, parser.SkipObjectResolution)
		if err != nil {
			// A test file that does not parse cannot be trusted to declare
			// anything, so the whole directory is unusable as evidence.
			return dirTests{err: fmt.Errorf("parse %s/%s: %w", relDir, entry.Name(), err)}
		}
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && isRunnableTest(fn) {
				names[fn.Name.Name] = true
			}
		}
	}
	return dirTests{names: names}
}

// isRunnableTest reports whether fn is a function go test runs as a test. It
// applies the same rules as cmd/go: a top-level function named Test followed
// by anything except a lowercase letter, with no type parameters, no
// results, and exactly one parameter of type *T or *pkg.T (a TestMain takes a
// *testing.M and a benchmark a *testing.B, so neither matches).
func isRunnableTest(fn *ast.FuncDecl) bool {
	if fn.Recv != nil || fn.Type.TypeParams != nil {
		return false
	}
	rest, ok := strings.CutPrefix(fn.Name.Name, "Test")
	if !ok {
		return false
	}
	// go test skips TestXxx when Xxx begins with a lowercase letter. An empty
	// rest decodes to the replacement rune, which is not lowercase.
	if first, _ := utf8.DecodeRuneInString(rest); unicode.IsLower(first) {
		return false
	}
	if fn.Type.Results != nil && len(fn.Type.Results.List) > 0 {
		return false
	}
	params := fn.Type.Params.List
	if len(params) != 1 || len(params[0].Names) > 1 {
		return false
	}
	star, ok := params[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	switch typ := star.X.(type) {
	case *ast.Ident:
		return typ.Name == "T"
	case *ast.SelectorExpr:
		return typ.Sel.Name == "T"
	}
	return false
}
