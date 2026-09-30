package capability

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

// CheckEvidence verifies that every test a declaration names exists. A
// reference <dir>:<TestName> is satisfied only by a top-level function
// TestName, declared in a _test.go file directly inside <dir> under
// engineRoot, that go test would run: no receiver, no type parameters, no
// results, and one parameter of type *testing.T. A same-named function in a
// non-test file, a method, a benchmark, a TestMain, or a name whose first
// character after "Test" is a lowercase letter (which go test skips) does not
// count.
//
// The guard reads the source with go/parser. It starts no process and does
// not run go list, so it works in the same sandboxes the tests do. It proves
// the named test is declared, not that it passes and not that it runs on
// every platform: build constraints are not evaluated. Every failing
// reference is reported at once, each naming its target and capability.
//
// References are checked for format first, so one that could name a path
// outside engineRoot is refused before any file is read.
func CheckEvidence(engineRoot string, d Declaration) error {
	dirs := make(map[string]dirTests)
	var errs []error
	for _, c := range d.Claims {
		for _, ref := range c.Tests {
			where := fmt.Sprintf("target %s, capability %s", d.Target, c.Capability)
			dir, name, err := parseTestRef(ref)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", where, err))
				continue
			}
			found, ok := dirs[dir]
			if !ok {
				found = loadTestFuncs(engineRoot, dir)
				dirs[dir] = found
			}
			switch {
			case found.err != nil:
				errs = append(errs, fmt.Errorf("%s: test %q: %w", where, ref, found.err))
			case !found.names[name]:
				errs = append(errs, fmt.Errorf("%s: test %q not found: no top-level func %s(t *testing.T) in a _test.go file of %q", where, ref, name, dir))
			}
		}
	}
	return errors.Join(errs...)
}

// dirTests is what one directory's _test.go files declare, or why that could
// not be determined. It is computed once per directory per CheckEvidence
// call.
type dirTests struct {
	names map[string]bool
	err   error
}

// loadTestFuncs parses every regular _test.go file directly inside
// engineRoot/relDir and collects the names of the runnable test functions
// they declare. relDir is a validated, clean, slash-separated relative
// directory.
func loadTestFuncs(engineRoot, relDir string) dirTests {
	absDir := filepath.Join(engineRoot, filepath.FromSlash(relDir))
	entries, err := os.ReadDir(absDir)
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
		file, err := parser.ParseFile(fset, filepath.Join(absDir, entry.Name()), nil, parser.SkipObjectResolution)
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
