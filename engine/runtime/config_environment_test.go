package runtime_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"testing"
)

// configVariables are the environment variables the composition root reads once and hands the
// adapters in a Config. No adapter reads them, and none asks the system for the home directory.
var configVariables = map[string]bool{
	"HOME": true, "XDG_CONFIG_HOME": true, "CODEX_HOME": true, "OVERLAY_DIR": true, "STATE_DIR": true,
}

// TestAdaptersTakeTheirDirectoriesFromConfigAndNotFromTheEnvironment reads the source of the
// package: a call to os.UserHomeDir, or to os.Getenv/os.LookupEnv of a variable Config carries,
// is a second place that decides what a home is, and the one that made --config-root differ
// between runtimes.
func TestAdaptersTakeTheirDirectoriesFromConfigAndNotFromTheEnvironment(t *testing.T) {
	fset := token.NewFileSet()
	for _, path := range nonTestGoFiles(t, ".") {
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
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
			pkg, ok := sel.X.(*ast.Ident)
			if !ok || pkg.Name != "os" {
				return true
			}
			where := filepath.Base(path) + ":" + strconv.Itoa(fset.Position(call.Pos()).Line)
			switch sel.Sel.Name {
			case "UserHomeDir":
				t.Errorf("%s: os.UserHomeDir() decides a home the adapter should have been given in its Config", where)
			case "Getenv", "LookupEnv":
				if len(call.Args) != 1 {
					return true
				}
				lit, ok := call.Args[0].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				if name, err := strconv.Unquote(lit.Value); err == nil && configVariables[name] {
					t.Errorf("%s: os.%s(%q) reads a variable the adapter should have been given in its Config", where, sel.Sel.Name, name)
				}
			}
			return true
		})
	}
}
