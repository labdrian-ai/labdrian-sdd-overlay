package piguard

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
)

// processAdapterSuffix ends the import path of the process adapter, engine/execrunner.
const processAdapterSuffix = "/engine/execrunner"

// execImportPath is the standard package that starts programs.
const execImportPath = "os/exec"

// Reporter is what CheckTestSources needs of the test that calls it: *testing.T and testing.TB
// satisfy it, and so does the small recorder of the tests of this package. Asking for exactly
// the two methods the scan calls means a fake has nothing else to leave unimplemented.
type Reporter interface {
	Helper()
	Errorf(format string, args ...any)
}

// CheckTestSources reads the source of the *_test.go files in dir and fails t for a test that can
// reach the CLI of the machine with the PATH of the run: one that imports the process adapter
// (engine/execrunner), or starts `pi` itself with exec.Command or exec.CommandContext. The tests
// of a package that drive the Pi adapter hand it a fake CommandRunner instead. Looking `pi` up
// (exec.LookPath) starts nothing and is allowed, because the guard's own tests do it.
// It also fails when dir holds no test file, so a wrong path cannot pass for good.
//
// What the scan reads is syntax, and it says what it cannot see. It finds the call under the name
// the file gave os/exec (`exec` or an alias), and the program named by a string literal or by a
// string constant declared with a literal at the top of a file of dir. It does not follow a
// variable, a function result, a constant declared inside a function (which could shadow a
// package one), a dot import, or a program built by joining strings: those are the limits that
// TestCheckTestSourcesStatesItsLimits pins, so a change that widens the scan shows up as a test
// to rewrite, not as a silent gap. The first line of defence is the PATH of the run, whose `pi`
// refuses to run (Install); this scan is the second.
func CheckTestSources(t Reporter, dir string) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*_test.go"))
	if err != nil || len(files) == 0 {
		t.Errorf("no test files found in %s to scan: %v", dir, err)
		return
	}
	fset := token.NewFileSet()
	parsed := map[string]*ast.File{}
	for _, path := range files {
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Errorf("parse %s: %v", filepath.Base(path), err)
			continue
		}
		parsed[path] = file
	}
	constants := stringConstants(parsed)
	for _, path := range files {
		file, ok := parsed[path]
		if !ok {
			continue
		}
		name := filepath.Base(path)
		execName := "" // empty when the file does not import os/exec under a name a selector can use
		for _, spec := range file.Imports {
			p, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				continue
			}
			if strings.HasSuffix(p, processAdapterSuffix) {
				t.Errorf("%s imports %s: the process adapter starts real programs with the PATH of the run; hand the code under test a fake CommandRunner", name, p)
			}
			if p == execImportPath {
				execName = localName(spec, "exec")
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
			if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != execName {
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
			if started, ok := stringOf(call.Args[nameArg], constants); ok && started == "pi" {
				t.Errorf("%s:%d: exec.%s(%q) starts the real CLI", name, fset.Position(call.Pos()).Line, sel.Sel.Name, started)
			}
			return true
		})
	}
}

// localName is the name a file gives an import: its alias, or the default when it has none. An
// import named `_` or `.` cannot be called through a selector, so it has no name here.
func localName(spec *ast.ImportSpec, fallback string) string {
	if spec.Name == nil {
		return fallback
	}
	if spec.Name.Name == "_" || spec.Name.Name == "." {
		return ""
	}
	return spec.Name.Name
}

// stringConstants maps each constant declared at the top of a file, with a string literal, to
// its value. A name declared in two files of the package would not compile, so a flat map is
// enough.
func stringConstants(files map[string]*ast.File) map[string]string {
	constants := map[string]string{}
	for _, file := range files {
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.CONST {
				continue
			}
			for _, spec := range gen.Specs {
				vs := spec.(*ast.ValueSpec)
				for i, name := range vs.Names {
					if i >= len(vs.Values) {
						continue
					}
					if value, ok := stringOf(vs.Values[i], nil); ok {
						constants[name.Name] = value
					}
				}
			}
		}
	}
	return constants
}

// stringOf is the value of expr when it is a string literal, or a name in constants.
func stringOf(expr ast.Expr, constants map[string]string) (string, bool) {
	switch e := expr.(type) {
	case *ast.BasicLit:
		if e.Kind == token.STRING {
			value, err := strconv.Unquote(e.Value)
			return value, err == nil
		}
	case *ast.Ident:
		value, ok := constants[e.Name]
		return value, ok
	}
	return "", false
}
