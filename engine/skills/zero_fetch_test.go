package skills

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// allowedImports is the fixed set of stdlib packages permitted in production
// (non-test) files of the engine/skills package (ADR-15).
//
// Any import outside this set turns this test RED. Adding a new import requires
// a reviewer to consciously widen this list — making a stealth network/exec
// dependency impossible to land silently. Notably absent: net, net/http, os/exec,
// and any package whose path contains "git".
var allowedImports = map[string]bool{
	"bufio":         true,
	"bytes":         true,
	"fmt":           true,
	"io":            true,
	"io/fs":         true,
	"os":            true,
	"path/filepath": true,
	"reflect":       true,
	"regexp":        true,
	"sort":          true,
	"strings":       true,
}

// TestZeroFetchImportAllowlist statically parses every non-test .go file in
// engine/skills/ and asserts that each import path is a member of allowedImports
// (R-131, R-132, SC-69).
func TestZeroFetchImportAllowlist(t *testing.T) {
	fset := token.NewFileSet()

	// ParseDir with a filter that excludes _test.go files.
	pkgs, err := parser.ParseDir(fset, ".", func(fi fs.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("go/parser.ParseDir: %v", err)
	}

	for _, pkg := range pkgs {
		for filename, file := range pkg.Files {
			base := filepath.Base(filename)
			for _, imp := range file.Imports {
				// ImportSpec.Path.Value is a double-quoted string literal.
				path := strings.Trim(imp.Path.Value, `"`)
				if !allowedImports[path] {
					t.Errorf("forbidden/unexpected import %q in %s — widen allowedImports only after reviewer approval", path, base)
				}
			}
		}
	}

	// Verify the test itself found at least one file (guard against an empty walk
	// silently passing).
	totalFiles := 0
	for _, pkg := range pkgs {
		totalFiles += len(pkg.Files)
	}
	if totalFiles == 0 {
		t.Fatal("no production .go files found under '.'; the allowlist walk may be broken")
	}

	// Sanity: confirm the AST import type is what we expect.
	var _ *ast.ImportSpec
}
