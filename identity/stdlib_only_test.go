package identity

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// identity is imported by the production code of two modules, so it must pull nothing into
// either: no third-party module, no other module of the repository, no file system, process or
// clock. Its go.mod requires nothing, and its production code imports the strings package and
// nothing else it does not need.
func TestTheModuleDependsOnNothingButThePureStandardLibrary(t *testing.T) {
	gomod, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(gomod), "\n") {
		if directive := strings.Fields(line); len(directive) > 0 && (directive[0] == "require" || directive[0] == "replace") {
			t.Errorf("go.mod has a %q directive (%q); identity depends on the standard library only", directive[0], line)
		}
	}

	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	pure := map[string]bool{"strings": true}
	checked := 0
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
			if path := strings.Trim(imp.Path.Value, `"`); !pure[path] {
				t.Errorf("%s imports %q: the rules are pure strings in and strings out, so it may import only %v", file, path, "strings")
			}
		}
	}
	if checked == 0 {
		t.Fatal("no imports were examined; the walk is broken")
	}
}
