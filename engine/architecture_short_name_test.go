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

// literalTwelveCuts names every place of a Go source that cuts a name at the integer literal 12: a slice expression whose
// high bound is 12 (`name[:12]`), or a call of min with 12 among its arguments (`name[:min(len(name), 12)]`). The first 12
// characters of an object name are written once, as review.ShortName (T4x, `4b9c3789c2e0-5` F2): two copies of the cut
// existed, and this is the known-error check of the class. It does not follow a number through a variable (`n := 12`),
// which it cannot see; the rule catches both copies found and every spelling of them seen.
func literalTwelveCuts(fset *token.FileSet, file *ast.File) []string {
	var hits []string
	isTwelve := func(e ast.Expr) bool {
		lit, ok := e.(*ast.BasicLit)
		return ok && lit.Kind == token.INT && lit.Value == "12"
	}
	ast.Inspect(file, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.SliceExpr:
			if n.High != nil && isTwelve(n.High) {
				hits = append(hits, fmt.Sprintf("%s: a slice cut at the literal 12", fset.Position(n.Pos())))
			}
		case *ast.CallExpr:
			if fn, ok := n.Fun.(*ast.Ident); ok && fn.Name == "min" {
				for _, arg := range n.Args {
					if isTwelve(arg) {
						hits = append(hits, fmt.Sprintf("%s: a call of min with the literal 12", fset.Position(n.Pos())))
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
	for _, root := range []string{"review", "cmd"} {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return err
			}
			hits = append(hits, literalTwelveCuts(fset, file)...)
			return nil
		})
		if err != nil {
			t.Fatalf("read %s: %v", root, err)
		}
	}
	for _, hit := range hits {
		t.Errorf("%s: write the short name of an object once, as review.ShortName", hit)
	}
}

func TestLiteralTwelveCutsSeesTheSpellingsOfTheShortName(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       int
	}{
		{"a slice", "func f(s string) string { return s[:12] }", 1},
		{"a slice cut with min", "func f(s string) string { return s[:min(len(s), 12)] }", 1},
		{"min alone", "func f(s string) int { return min(len(s), 12) }", 1},
		{"another bound", "func f(s string) string { return s[:11] }", 0},
		{"the named length", "const n = 12\nfunc f(s string) string { return s[:n] }", 0},
		{"a low bound", "func f(s string) string { return s[12:] }", 0},
	} {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, "x.go", "package x\n"+tc.body, 0)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got := len(literalTwelveCuts(fset, file)); got != tc.want {
			t.Errorf("%s: %d hits, want %d", tc.name, got, tc.want)
		}
	}
}
