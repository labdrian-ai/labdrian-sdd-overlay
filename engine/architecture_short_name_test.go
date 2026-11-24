package guard

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// shortNameHome is the one file that may cut an object name at 12 characters: the place the engine says what git names an
// object, as a path of the engine module with slashes.
const shortNameHome = "gitobject/gitobject.go"

// shortNameCuts names every place of a Go source that cuts a name at 12: a slice expression whose high bound is 12
// (`name[:12]`), or a call of min with 12 among its arguments (`name[:min(len(name), 12)]`), where 12 is the integer literal
// or a name the same file binds to that literal by its own value (`const n = 12`, `const n int = 12`, `var n = 12`, `n := 12`).
// A bound name is matched by its text across the whole file, whatever scope binds it: a name bound to 12 in one place makes a
// cut by the same name anywhere in the file a hit. Such a hit is false but loud, and is fixed by renaming the bound or by
// gitobject.Short. The first 12 characters of an object name are written once, as gitobject.Short (T4y, `36d5bc62df38-4` F3,
// which widens T4x's `4b9c3789c2e0-5` F2): two copies of the cut existed outside the walk of the first lint, and this is the
// known-error check of the class. What it does not follow: a name that a const group repeats implicitly (`const ( a = 12; b )`
// gives b no value of its own), a name bound to 12 in another file, and a value computed (`6 * 2`). A cut it does not follow
// is read by review.
func shortNameCuts(fset *token.FileSet, file *ast.File) []string {
	isLiteral := func(e ast.Expr) bool {
		lit, ok := e.(*ast.BasicLit)
		return ok && lit.Kind == token.INT && lit.Value == "12"
	}
	bound := map[string]bool{}
	bind := func(names []ast.Expr, values []ast.Expr) {
		for i, name := range names {
			if id, ok := name.(*ast.Ident); ok && i < len(values) && isLiteral(values[i]) {
				bound[id.Name] = true
			}
		}
	}
	ast.Inspect(file, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.ValueSpec:
			names := make([]ast.Expr, len(n.Names))
			for i, id := range n.Names {
				names[i] = id
			}
			bind(names, n.Values)
		case *ast.AssignStmt:
			if n.Tok == token.DEFINE {
				bind(n.Lhs, n.Rhs)
			}
		}
		return true
	})
	isTwelve := func(e ast.Expr) bool {
		if id, ok := e.(*ast.Ident); ok {
			return bound[id.Name]
		}
		return isLiteral(e)
	}
	var hits []string
	ast.Inspect(file, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.SliceExpr:
			if n.High != nil && isTwelve(n.High) {
				hits = append(hits, fmt.Sprintf("%s: a slice cut at 12", fset.Position(n.Pos())))
			}
		case *ast.CallExpr:
			if fn, ok := n.Fun.(*ast.Ident); ok && fn.Name == "min" {
				for _, arg := range n.Args {
					if isTwelve(arg) {
						hits = append(hits, fmt.Sprintf("%s: a call of min with 12", fset.Position(n.Pos())))
					}
				}
			}
		}
		return true
	})
	return hits
}

func TestTheShortNameOfAnObjectIsWrittenOnce(t *testing.T) {
	var hits []string
	checked := 0
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
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || filepath.ToSlash(path) == shortNameHome {
			return nil
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		checked++
		hits = append(hits, shortNameCuts(fset, file)...)
		return nil
	})
	if err != nil {
		t.Fatalf("read the engine: %v", err)
	}
	if checked < 100 {
		t.Fatalf("only %d files were read: the walk does not cover the engine", checked)
	}
	for _, hit := range hits {
		t.Errorf("%s: cut an object name with gitobject.Short; a cut that is not an object name takes a bound this file does not bind to 12", hit)
	}
}

func TestShortNameCutsSeesTheSpellingsOfTheShortName(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       int
	}{
		{"a slice", "func f(s string) string { return s[:12] }", 1},
		{"a slice cut with min", "func f(s string) string { return s[:min(len(s), 12)] }", 1},
		{"min alone", "func f(s string) int { return min(len(s), 12) }", 1},
		{"another bound", "func f(s string) string { return s[:11] }", 0},
		{"a low bound", "func f(s string) string { return s[12:] }", 0},
		{"a const bound to 12", "const n = 12\nfunc f(s string) string { return s[:n] }", 1},
		{"a := bound to 12", "func f(s string) string {\n\tn := 12\n\treturn s[:n]\n}", 1},
		{"a var bound to 12 in min", "var n = 12\nfunc f(s string) string { return s[:min(len(s), n)] }", 1},
		{"a const bound to 11", "const n = 11\nfunc f(s string) string { return s[:n] }", 0},
		{"a typed const bound to 12", "const n int = 12\nfunc f(s string) string { return s[:n] }", 1},
		{"a name bound to 12 and never cut", "const n = 12\nfunc f(s string) string { return s }", 0},
		{"another name cut", "const n = 12\nfunc f(s string, m int) string { return s[:m] }", 0},
		{"a name bound to 12 in another function", "func g() int {\n\tn := 12\n\treturn n\n}\nfunc f(s string, n int) string { return s[:n] }", 1},
		{"a const group that repeats 12", "const (\n\ta = 12\n\tb\n)\nfunc f(s string) string { return s[:b] }", 0},
	} {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, "x.go", "package x\n"+tc.body, 0)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got := len(shortNameCuts(fset, file)); got != tc.want {
			t.Errorf("%s: %d hits, want %d", tc.name, got, tc.want)
		}
	}
}
