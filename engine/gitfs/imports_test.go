package gitfs

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"sort"
	"strings"
	"testing"
)

// gitfs finds a repository by reading files and must never run git (Decision 3 of Phase 7:
// no subprocess). It imports gitprov, the package that does run git, for one pure function; this
// pins that it takes nothing else from it, so the import cannot grow into a call that starts a
// process.
func TestGitfsTakesOnlyThePointerRuleFromGitprov(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(info fs.FileInfo) bool { return !strings.HasSuffix(info.Name(), "_test.go") }, 0)
	if err != nil {
		t.Fatal(err)
	}
	used := map[string]bool{}
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			ast.Inspect(file, func(n ast.Node) bool {
				if sel, ok := n.(*ast.SelectorExpr); ok {
					if id, ok := sel.X.(*ast.Ident); ok && id.Name == "gitprov" {
						used[sel.Sel.Name] = true
					}
				}
				return true
			})
		}
	}
	var names []string
	for name := range used {
		names = append(names, name)
	}
	sort.Strings(names)
	if got := strings.Join(names, ","); got != "PointerTarget" {
		t.Fatalf("gitfs uses gitprov.{%s}, want only PointerTarget", got)
	}
}
