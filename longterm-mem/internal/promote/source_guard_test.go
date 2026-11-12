package promote

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Promotion reads memory through the Source port it owns, over the types of internal/memory, and names no store:
// a store is an adapter the composition root wires. This pins the imports of the package's own code (its
// dependencies are those and what they import, which the module's ring guard keeps to the domain).
func TestPromotionImportsNoStoreAdapter(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		seen++
		parsed, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, spec := range parsed.Imports {
			path, _ := strconv.Unquote(spec.Path.Value)
			for _, adapter := range []string{"/internal/engram", "/internal/memorycard"} {
				if strings.HasSuffix(path, adapter) {
					t.Errorf("%s imports %s: promotion reads a store through the Source port", name, path)
				}
			}
		}
	}
	if seen == 0 {
		t.Fatal("found no production file: the guard would pass over nothing")
	}
}
