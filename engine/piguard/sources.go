package piguard

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// processAdapterSuffix ends the import path of the process adapter, engine/execrunner.
const processAdapterSuffix = "/engine/execrunner"

// CheckTestSources reads the source of the *_test.go files in dir and fails t for a test that can
// reach the CLI of the machine with the PATH of the run: one that imports the process adapter
// (engine/execrunner), or starts `pi` itself with exec.Command or exec.CommandContext. The tests
// of a package that drives the Pi adapter hand it a fake CommandRunner instead. Looking `pi` up
// (exec.LookPath) starts nothing and is allowed, because the guard's own tests do it.
// It also fails when dir holds no test file, so a wrong path cannot pass for good.
func CheckTestSources(t testing.TB, dir string) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*_test.go"))
	if err != nil || len(files) == 0 {
		t.Errorf("no test files found in %s to scan: %v", dir, err)
		return
	}
	fset := token.NewFileSet()
	for _, path := range files {
		name := filepath.Base(path)
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Errorf("parse %s: %v", name, err)
			continue
		}
		for _, spec := range file.Imports {
			if p, err := strconv.Unquote(spec.Path.Value); err == nil && strings.HasSuffix(p, processAdapterSuffix) {
				t.Errorf("%s imports %s: the process adapter starts real programs with the PATH of the run; hand the code under test a fake CommandRunner", name, p)
			}
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
			if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != "exec" {
				return true
			}
			// exec.Command(name, ...) and exec.CommandContext(ctx, name, ...) start a program.
			nameArg := 0
			switch sel.Sel.Name {
			case "Command":
			case "CommandContext":
				nameArg = 1
			default:
				return true
			}
			if nameArg >= len(call.Args) {
				return true
			}
			if lit, ok := call.Args[nameArg].(*ast.BasicLit); ok && lit.Kind == token.STRING {
				if started, err := strconv.Unquote(lit.Value); err == nil && started == "pi" {
					t.Errorf("%s:%d: exec.%s(%q) starts the real CLI", name, fset.Position(call.Pos()).Line, sel.Sel.Name, started)
				}
			}
			return true
		})
	}
}
