package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
)

// Two rules keep the commands testable without a switch in the program (Phase 9 unit H31):
//
//   - No package-level variable is reassigned. A test that needs another clock, another store or a
//     shorter wait builds its own deps (deps.go) and hands it to the command; it does not change
//     what every other test and the next command in the process would see.
//   - Only main.go touches the process: its arguments, its streams, its exit, its environment and
//     its working directory. main builds deps from them once, and the rest of the package is handed
//     what it needs.
//
// Both are read from the source of this package, as engine/architecture_test.go reads the imports.

// sources reads the .go files of dir, keyed by file name; tests says whether to take the test
// files or the others.
func sources(t *testing.T, dir string, tests bool) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") != tests {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		files[e.Name()] = string(data)
	}
	return files
}

func parseAll(t *testing.T, files map[string]string) map[string]*ast.File {
	t.Helper()
	fset := token.NewFileSet()
	parsed := map[string]*ast.File{}
	for name, src := range files {
		file, err := parser.ParseFile(fset, name, src, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		parsed[name] = file
	}
	return parsed
}

// packageVars lists the package-level variables the files declare.
func packageVars(files map[string]*ast.File) map[string]bool {
	vars := map[string]bool{}
	for _, file := range files {
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.VAR {
				continue
			}
			for _, spec := range gen.Specs {
				for _, name := range spec.(*ast.ValueSpec).Names {
					if name.Name != "_" {
						vars[name.Name] = true
					}
				}
			}
		}
	}
	return vars
}

// rootIdent is the variable an assignable expression is rooted in: x in x, x.f, x[i], *x, (x).
func rootIdent(e ast.Expr) *ast.Ident {
	for {
		switch v := e.(type) {
		case *ast.Ident:
			return v
		case *ast.SelectorExpr:
			e = v.X
		case *ast.IndexExpr:
			e = v.X
		case *ast.StarExpr:
			e = v.X
		case *ast.ParenExpr:
			e = v.X
		default:
			return nil
		}
	}
}

// reassignedPackageVars lists, as file:name, every place the files assign to (or change by ++ or --,
// or by assigning into) a package-level variable of the package. vars are the names of those
// variables. An identifier that the file's own scopes resolve to a local declaration is a
// different variable with the same name and is left alone; a package-level one declared in the
// same file resolves to the file's scope, and one declared in another file does not resolve.
func reassignedPackageVars(files map[string]*ast.File, vars map[string]bool) []string {
	var found []string
	for name, file := range files {
		isPackageLevel := func(id *ast.Ident) bool {
			if !vars[id.Name] {
				return false
			}
			return id.Obj == nil || id.Obj == file.Scope.Lookup(id.Name)
		}
		report := func(e ast.Expr) {
			if id := rootIdent(e); id != nil && isPackageLevel(id) {
				found = append(found, name+":"+id.Name)
			}
		}
		ast.Inspect(file, func(n ast.Node) bool {
			switch s := n.(type) {
			case *ast.AssignStmt:
				if s.Tok != token.DEFINE {
					for _, lhs := range s.Lhs {
						report(lhs)
					}
				}
			case *ast.IncDecStmt:
				report(s.X)
			case *ast.RangeStmt:
				if s.Tok == token.ASSIGN {
					for _, e := range []ast.Expr{s.Key, s.Value} {
						if e != nil {
							report(e)
						}
					}
				}
			}
			return true
		})
	}
	sort.Strings(found)
	found = slices.Compact(found)
	return found
}

func TestNoCodeOfTheCommandsReassignsAPackageLevelVariable(t *testing.T) {
	production := parseAll(t, sources(t, ".", false))
	vars := packageVars(production)
	all := parseAll(t, sources(t, ".", false))
	for name, file := range parseAll(t, sources(t, ".", true)) {
		all[name] = file
	}
	if found := reassignedPackageVars(all, vars); len(found) > 0 {
		t.Errorf("a package-level variable is reassigned (file:variable): %s\n"+
			"hand the command a deps (deps.go) with the other value instead of changing the variable", strings.Join(found, ", "))
	}
}

func TestTheCheckForAReassignedPackageVariableSeesWhatItShould(t *testing.T) {
	const decl = "package main\nvar seam func()\nvar table = map[string]int{}\nvar count int\n"
	cases := []struct {
		name, use string
		want      []string
	}{
		{"an assignment", "func f() { seam = nil }", []string{"use.go:seam"}},
		{"an assignment from a test cleanup", "func f() { saved := seam; seam = func() {}; _ = saved }", []string{"use.go:seam"}},
		{"a write into a map", "func f() { table[\"a\"] = 1 }", []string{"use.go:table"}},
		{"an increment", "func f() { count++ }", []string{"use.go:count"}},
		{"an operator assignment", "func f() { count += 2 }", []string{"use.go:count"}},
		{"a tuple assignment", "func f() { seam, count = nil, 0 }", []string{"use.go:count", "use.go:seam"}},
		{"a range assignment", "func f() { for count = range table {} }", []string{"use.go:count"}},
		{"a read", "func f() { seam(); _ = table[\"a\"]; _ = count }", nil},
		{"a local of the same name", "func f() { seam := 1; seam = 2; _ = seam }", nil},
		{"a parameter of the same name", "func f(count int) { count = 3 }", nil},
		{"a field of the same name", "type T struct{ count int }\nfunc f(t *T) { t.count = 3 }", nil},
		{"a definition", "func f() { count := 1; _ = count }", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			files := parseAll(t, map[string]string{"decl.go": decl, "use.go": "package main\n" + c.use})
			got := reassignedPackageVars(files, packageVars(files))
			if strings.Join(got, ",") != strings.Join(c.want, ",") {
				t.Errorf("reassigned = %q, want %q", got, c.want)
			}
		})
	}
}

// processUses lists, as file:os.Name, every use of the process of the machine in the files: the
// arguments, the three streams, the exit, the environment, the working directory and the
// binary's own path. The names are those of package os; a file that imports it under another
// name is read through that name.
func processUses(files map[string]*ast.File) []string {
	process := map[string]bool{
		"Args": true, "Stdin": true, "Stdout": true, "Stderr": true, "Exit": true,
		"Getenv": true, "LookupEnv": true, "Environ": true, "Setenv": true, "Unsetenv": true, "Clearenv": true, "ExpandEnv": true, "Expand": true,
		"UserHomeDir": true, "UserConfigDir": true, "UserCacheDir": true, "Getwd": true, "Chdir": true, "Executable": true, "Hostname": true,
	}
	var found []string
	for name, file := range files {
		local := ""
		for _, spec := range file.Imports {
			if strings.Trim(spec.Path.Value, `"`) != "os" {
				continue
			}
			local = "os"
			if spec.Name != nil {
				local = spec.Name.Name
			}
		}
		if local == "" || local == "_" || local == "." {
			continue
		}
		ast.Inspect(file, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok {
				if id, ok := sel.X.(*ast.Ident); ok && id.Name == local && id.Obj == nil && process[sel.Sel.Name] {
					found = append(found, name+":os."+sel.Sel.Name)
				}
			}
			return true
		})
	}
	sort.Strings(found)
	found = slices.Compact(found)
	return found
}

func TestOnlyMainTouchesTheProcess(t *testing.T) {
	files := sources(t, ".", false)
	delete(files, "main.go")
	if found := processUses(parseAll(t, files)); len(found) > 0 {
		t.Errorf("code outside main.go touches the process (file:use): %s\n"+
			"main builds deps from the process once (productionDeps); take what you need from the deps you are handed", strings.Join(found, ", "))
	}
}

func TestTheCheckForTheProcessSeesWhatItShould(t *testing.T) {
	cases := []struct {
		name, src string
		want      []string
	}{
		{"the environment", "package p\nimport \"os\"\nvar _ = os.Getenv(\"HOME\")", []string{"f.go:os.Getenv"}},
		{"the home", "package p\nimport \"os\"\nvar _ = os.UserHomeDir", []string{"f.go:os.UserHomeDir"}},
		{"the streams and the exit", "package p\nimport \"os\"\nfunc f() { _ = os.Stdout; _ = os.Stderr; _ = os.Stdin; os.Exit(1) }", []string{"f.go:os.Exit", "f.go:os.Stderr", "f.go:os.Stdin", "f.go:os.Stdout"}},
		{"the working directory", "package p\nimport \"os\"\nvar _, _ = os.Getwd()", []string{"f.go:os.Getwd"}},
		{"the arguments", "package p\nimport \"os\"\nvar _ = os.Args", []string{"f.go:os.Args"}},
		{"os under another name", "package p\nimport o \"os\"\nvar _ = o.Environ()", []string{"f.go:os.Environ"}},
		{"a file of the system", "package p\nimport \"os\"\nvar _, _ = os.ReadFile(\"x\")\nvar _ = os.Stat", nil},
		{"a local called os", "package p\nimport \"os\"\nfunc f(os struct{ Args int }) int { return os.Args }", nil},
		{"a package that is not os", "package p\nimport \"example.com/os\"\nvar _ = os.Args", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := processUses(parseAll(t, map[string]string{"f.go": c.src}))
			if strings.Join(got, ",") != strings.Join(c.want, ",") {
				t.Errorf("uses = %q, want %q", got, c.want)
			}
		})
	}
}
