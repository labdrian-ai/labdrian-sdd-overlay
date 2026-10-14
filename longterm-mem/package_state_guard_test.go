// This file shares package guard with the Engram guard's walk (skipDirectory, within) and its walk test (writeTree
// and importing).
package guard

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// The architecture guard reads imports and the members a package selects from a few standard ones; it cannot see a
// package-level variable, and the module has had two kinds of those that are seams in disguise: a function a test
// replaces to put a failure where the filesystem would not (register's saveInstallState) and a lock registry that
// every caller in the process shares (vecindex's dirLocks). Each hides a dependency in a global, so the code that
// needs it cannot be handed another, and a test that replaces it cannot run beside another. Both are dependencies
// now, passed in (register.InstallStateStore, vecindex.Builder). This guard keeps the next from arriving.
//
// What it flags, in the production files of the module:
//
//   - a package-level variable that holds a function (declared with a func type, or initialized with a function
//     literal);
//   - a package-level lock or once (sync.Mutex, RWMutex, Map, Once, Pool, WaitGroup);
//   - a package-level map that the package writes to (an element assigned, or delete called on it) in a function.
//
// What it leaves alone, because it is not shared state: error values and regexps, and the lookup tables that are
// built once and only read (a map no function writes). A map written only by its initializer is a table.
//
// It works on names, as the architecture guard does: a local variable that shadows a package-level map and is
// written is read as the package's, which fails closed; rename the local.

func TestProductionCodeKeepsNoStateInPackageVariables(t *testing.T) {
	problems, err := packageStateProblems(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, problem := range problems {
		t.Error(problem)
	}
}

// packageStateProblems reads every production Go file under root (the module's own: not tests, testdata, vendor,
// hidden directories or nested modules) and says which declare state in a package variable, one problem for each,
// in a stable order. A file it cannot parse is an error: the rule would guard nothing.
func packageStateProblems(root string) ([]string, error) {
	files := map[string][]*ast.File{} // package directory -> its files
	names := map[*ast.File]string{}   // file -> its path under root
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			return skipDirectory(root, path, rel, d.Name())
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		dir := filepath.ToSlash(filepath.Dir(rel))
		files[dir] = append(files[dir], file)
		names[file] = rel
		return nil
	})
	if err != nil {
		return nil, err
	}

	var problems []string
	for _, dirFiles := range files {
		problems = append(problems, packageStateProblemsOf(dirFiles, names)...)
	}
	sort.Strings(problems)
	return problems, nil
}

// packageStateProblemsOf judges the files of one package.
func packageStateProblemsOf(files []*ast.File, names map[*ast.File]string) []string {
	var problems []string
	maps := map[string]string{} // package-level map variable -> the file that declares it

	for _, file := range files {
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.VAR {
				continue
			}
			for _, spec := range gen.Specs {
				spec := spec.(*ast.ValueSpec)
				for i, name := range spec.Names {
					if name.Name == "_" {
						continue
					}
					var value ast.Expr
					if i < len(spec.Values) {
						value = spec.Values[i]
					}
					switch {
					case holdsFunction(spec.Type, value):
						problems = append(problems, names[file]+" declares the package-level variable "+name.Name+", which holds a function: a seam is a dependency the code is handed, not a global a test replaces")
					case holdsLock(spec.Type, value):
						problems = append(problems, names[file]+" declares the package-level variable "+name.Name+", a lock: the state it guards belongs to a value that owns it, and the lock with it")
					case holdsMap(spec.Type, value):
						maps[name.Name] = names[file]
					}
				}
			}
		}
	}

	written := map[string]string{} // map variable -> the file that writes it
	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				for _, name := range mapNamesWritten(n) {
					if _, packageLevel := maps[name]; packageLevel {
						written[name] = names[file]
					}
				}
				return true
			})
		}
	}
	for name, declaredIn := range maps {
		if writer, isWritten := written[name]; isWritten {
			problems = append(problems, declaredIn+" declares the package-level map "+name+", which "+writer+" writes to: shared mutable state belongs to a value that owns it")
		}
	}
	return problems
}

// holdsFunction says whether a variable declared with typ and initialized with value holds a function.
func holdsFunction(typ, value ast.Expr) bool {
	if _, isFunc := typ.(*ast.FuncType); isFunc {
		return true
	}
	_, isLiteral := value.(*ast.FuncLit)
	return isLiteral
}

// syncState are the types of package sync a package variable must not be: each is a lock or a once, which guards
// state that is shared.
var syncState = map[string]bool{"Mutex": true, "RWMutex": true, "Map": true, "Once": true, "Pool": true, "WaitGroup": true}

// holdsLock says whether a variable declared with typ and initialized with value is one of syncState, or a pointer
// to one.
func holdsLock(typ, value ast.Expr) bool {
	for _, expr := range []ast.Expr{typ, value} {
		if isSyncState(expr) {
			return true
		}
	}
	return false
}

// isSyncState says whether expr names one of syncState, as a type, as a composite literal of it or as the address
// of one.
func isSyncState(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.StarExpr:
		return isSyncState(e.X)
	case *ast.UnaryExpr:
		return isSyncState(e.X)
	case *ast.CompositeLit:
		return isSyncState(e.Type)
	case *ast.SelectorExpr:
		pkg, ok := e.X.(*ast.Ident)
		return ok && pkg.Name == "sync" && syncState[e.Sel.Name]
	}
	return false
}

// holdsMap says whether a variable declared with typ and initialized with value is a map.
func holdsMap(typ, value ast.Expr) bool {
	if _, isMap := typ.(*ast.MapType); isMap {
		return true
	}
	switch v := value.(type) {
	case *ast.CompositeLit:
		_, isMap := v.Type.(*ast.MapType)
		return isMap
	case *ast.CallExpr:
		if fn, ok := v.Fun.(*ast.Ident); ok && fn.Name == "make" && len(v.Args) > 0 {
			_, isMap := v.Args[0].(*ast.MapType)
			return isMap
		}
	}
	return false
}

// mapNamesWritten are the names of the maps node writes to: an element assigned (`m[k] = v`, `m[k] += v`,
// `m[k]++`) or removed (`delete(m, k)`).
func mapNamesWritten(node ast.Node) []string {
	var written []string
	elementOf := func(expr ast.Expr) {
		if index, ok := expr.(*ast.IndexExpr); ok {
			if name, ok := index.X.(*ast.Ident); ok {
				written = append(written, name.Name)
			}
		}
	}
	switch n := node.(type) {
	case *ast.AssignStmt:
		for _, lhs := range n.Lhs {
			elementOf(lhs)
		}
	case *ast.IncDecStmt:
		elementOf(n.X)
	case *ast.CallExpr:
		if fn, ok := n.Fun.(*ast.Ident); ok && fn.Name == "delete" && len(n.Args) > 0 {
			if name, ok := n.Args[0].(*ast.Ident); ok {
				written = append(written, name.Name)
			}
		}
	}
	return written
}

// The rule is tested on a tree built for it: each kind of package-level state is flagged, a table that is only
// read is not, the writes that make a map shared are found in another file of the package, and tests, testdata and
// state held in a value are left alone.
func TestPackageStateGuardFlagsSeamsLocksAndSharedMapsAndNothingElse(t *testing.T) {
	root := writeTree(t, map[string]string{
		"internal/a/seam.go":       "package a\n\nvar save = func(s string) error { return nil }\n",
		"internal/a/typed.go":      "package a\n\nvar clock func() int\n",
		"internal/a/lock.go":       "package a\n\nimport \"sync\"\n\nvar mu sync.Mutex\n",
		"internal/a/pointer.go":    "package a\n\nimport \"sync\"\n\nvar once = &sync.Once{}\n",
		"internal/a/map.go":        "package a\n\nvar registry = map[string]int{}\n",
		"internal/a/writer.go":     "package a\n\nfunc Add(k string) { registry[k]++ }\n",
		"internal/a/made.go":       "package a\n\nvar cache = make(map[string]bool)\n",
		"internal/a/remover.go":    "package a\n\nfunc Drop(k string) { delete(cache, k) }\n",
		"internal/a/table.go":      "package a\n\nvar stopwords = map[string]bool{\"the\": true}\n\nfunc Is(w string) bool { return stopwords[w] }\n",
		"internal/a/errors.go":     "package a\n\nimport \"errors\"\n\nvar ErrX = errors.New(\"x\")\n\nvar _ = ErrX\n",
		"internal/a/held.go":       "package a\n\nimport \"sync\"\n\ntype Builder struct {\n\tmu    sync.Mutex\n\tlocks map[string]int\n}\n\nfunc (b *Builder) Add(k string) { b.locks[k]++ }\n",
		"internal/a/local.go":      "package a\n\nfunc Local() map[string]int {\n\tm := map[string]int{}\n\tm[\"a\"] = 1\n\treturn m\n}\n",
		"internal/a/seam_test.go":  "package a\n\nvar testSeam = func() {}\n",
		"internal/a/testdata/x.go": "package a\n\nvar seam = func() {}\n",
	})

	got, err := packageStateProblems(root)
	if err != nil {
		t.Fatalf("packageStateProblems: %v", err)
	}

	var files []string
	for _, problem := range got {
		files = append(files, strings.SplitN(problem, " ", 2)[0])
	}
	want := []string{
		"internal/a/lock.go",
		"internal/a/made.go",
		"internal/a/map.go",
		"internal/a/pointer.go",
		"internal/a/seam.go",
		"internal/a/typed.go",
	}
	if !reflect.DeepEqual(files, want) {
		t.Fatalf("flagged = %v, want %v (a table nothing writes, an error, state held in a struct, a local map, tests and testdata pass)", files, want)
	}
}

func TestPackageStateGuardReportsAFileItCannotParse(t *testing.T) {
	root := writeTree(t, map[string]string{"internal/a/broken.go": "package a\n\nvar (\n"})

	if _, err := packageStateProblems(root); err == nil {
		t.Fatal("packageStateProblems accepted a file it could not parse; a guard that skips what it cannot read guards nothing")
	}
}
