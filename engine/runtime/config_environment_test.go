package runtime_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"testing"
)

// TestAdaptersReadNoEnvironment reads the source of the package: a call to os.UserHomeDir,
// os.Environ, or os.Getenv/os.LookupEnv, whatever it is given (a literal, a named constant of the
// package, or anything else), is a place that decides what the configuration of a run is. The
// composition root reads it once and hands the adapters a Config and their options (HOME,
// XDG_CONFIG_HOME, CODEX_HOME, OVERLAY_DIR, LABDRIAN_OVERLAY_DIR, STATE_DIR,
// LABDRIAN_PI_SKIP_SUBAGENTS, LABDRIAN_PI_DEPLOY_REF); a read in the package is a second place, and
// the one that made --config-root differ between runtimes. There is no exception: the last one,
// the OpenCode contract lookup, moved to the composition root with H26.
func TestAdaptersReadNoEnvironment(t *testing.T) {
	fset := token.NewFileSet()
	files := nonTestGoFiles(t, ".")
	constants := packageStringConstants(t, fset, files)
	for _, path := range files {
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
			case "Environ":
				t.Errorf("%s: os.Environ() reads the whole environment the adapter should have been given in its Config", where)
			case "Getenv", "LookupEnv":
				name, known := "", false
				if len(call.Args) == 1 {
					name, known = stringValue(call.Args[0], constants)
				}
				if !known {
					t.Errorf("%s: os.%s reads a variable the scan cannot name; the adapter should have been given it in its Config", where, sel.Sel.Name)
					return true
				}
				t.Errorf("%s: os.%s(%q) reads a variable the adapter should have been given in its Config or its options", where, sel.Sel.Name, name)
			}
			return true
		})
	}
}

// packageStringConstants maps the name of every package-level string constant declared with a
// literal in the given files to its value, so a read through a named constant is read by name.
// It is a map of the package, so a constant of the same name declared inside a function would be
// read as the package one: the scan does not follow shadowing. No file of the package does that,
// and a function-level constant that read the environment under a package name would be found
// by a reader of the diff, not by this scan.
func packageStringConstants(t *testing.T, fset *token.FileSet, paths []string) map[string]string {
	t.Helper()
	constants := map[string]string{}
	for _, path := range paths {
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
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
					if lit, ok := vs.Values[i].(*ast.BasicLit); ok && lit.Kind == token.STRING {
						if value, err := strconv.Unquote(lit.Value); err == nil {
							constants[name.Name] = value
						}
					}
				}
			}
		}
	}
	return constants
}

// stringValue is the value of expr when it is a string literal or a named constant of the package.
func stringValue(expr ast.Expr, constants map[string]string) (string, bool) {
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
