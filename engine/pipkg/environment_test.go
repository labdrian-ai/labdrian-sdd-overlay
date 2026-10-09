package pipkg_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestThePackageReadsNoEnvironment reads the source of the package: a call to os.Getenv,
// os.LookupEnv, os.Environ or os.UserHomeDir is a second place that decides what the
// configuration of a run is. The composition root reads it once and hands the package its
// Options, which is what a test points at a temporary directory or a fixed ref.
func TestThePackageReadsNoEnvironment(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var scanned int
	for _, entry := range entries {
		name := entry.Name()
		if !entry.Type().IsRegular() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		scanned++
		file, err := parser.ParseFile(fset, filepath.Join(".", name), nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != "os" {
				return true
			}
			switch sel.Sel.Name {
			case "Getenv", "LookupEnv", "Environ", "UserHomeDir":
				t.Errorf("%s:%s: os.%s reads the environment the package should have been given in its Options", name, strconv.Itoa(fset.Position(call.Pos()).Line), sel.Sel.Name)
			}
			return true
		})
	}
	if scanned == 0 {
		t.Fatal("no production file was scanned; the walk is broken")
	}
}
