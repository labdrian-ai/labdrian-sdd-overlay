package gate_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The gate decides what to do to a prompt from a contract's frontmatter, and for that it
// needs the parse of a contract (engine/contract). It once took the parse from the
// propagator, the package that rewrites the skill registry on disk, so the hook that runs
// before every sub-agent spawn depended on the writer of a file it never touches. The gate
// reads a contract through engine/contract only.
func TestTheGateDoesNotDependOnTheRegistryWriter(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		checked++
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		for _, imp := range parsed.Imports {
			path, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if strings.HasSuffix(path, "/engine/propagator") {
				t.Errorf("%s imports %s: the gate reads a contract through engine/contract, not through the registry writer", file, path)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no source file of the gate was checked")
	}
	if _, err := os.Stat("gate.go"); err != nil {
		t.Fatalf("the test must run in the gate's directory: %v", err)
	}
}
