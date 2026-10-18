package identity

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const modulePath = "github.com/labdrian-ai/labdrian-sdd-overlay/identity"

// allowedImports is what the production code of each package of the module may import. identity is
// imported by the production code of two modules, so it pulls nothing into either: no third-party
// module and no other module of the repository. The rules (this package and resolve) are pure:
// strings in and strings out, no file system, process or clock. gitfs is the one adapter, the only
// package that reads a disk, and it reads it through the standard library alone.
var allowedImports = map[string][]string{
	".":       {"strings"},
	"resolve": {"errors", "fmt", "path/filepath", "strings", modulePath},
	"gitfs":   {"errors", "fmt", "io/fs", "os", "path/filepath", "strings", modulePath + "/resolve"},
}

func TestTheModuleDependsOnNothingButTheStandardLibraryAndOnlyItsAdapterTouchesTheDisk(t *testing.T) {
	gomod, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(gomod), "\n") {
		if directive := strings.Fields(line); len(directive) > 0 && (directive[0] == "require" || directive[0] == "replace") {
			t.Errorf("go.mod has a %q directive (%q); identity depends on the standard library only", directive[0], line)
		}
	}

	var packages []string
	err = filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == "testdata" || (strings.HasPrefix(d.Name(), ".") && path != ".")) {
			return filepath.SkipDir
		}
		if !d.IsDir() && strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			if dir := filepath.ToSlash(filepath.Dir(path)); !containsString(packages, dir) {
				packages = append(packages, dir)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(packages)

	checked := 0
	for _, pkg := range packages {
		allowed, ok := allowedImports[pkg]
		if !ok {
			t.Errorf("package %s has no entry in allowedImports: say what it may import", pkg)
			continue
		}
		files, err := filepath.Glob(filepath.Join(pkg, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, file := range files {
			if strings.HasSuffix(file, "_test.go") {
				continue
			}
			parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
			if err != nil {
				t.Fatal(err)
			}
			for _, imp := range parsed.Imports {
				checked++
				if path := strings.Trim(imp.Path.Value, `"`); !containsString(allowed, path) {
					t.Errorf("%s imports %q: package %s may import only %v", file, path, pkg, allowed)
				}
			}
		}
	}
	if checked == 0 || len(packages) != len(allowedImports) {
		t.Fatalf("examined %d imports in packages %v, want every package of allowedImports: the walk is broken or a package is gone", checked, packages)
	}
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
