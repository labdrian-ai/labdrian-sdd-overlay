package guard

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// allowedExecImporter is the sole production file permitted to import
// "os/exec". Every subprocess call to the vault's scripts must go through
// internal/vault.Runner so it stays bounded to the vault root (R-021):
// nothing in longterm-mem is allowed to shell out to Engram's own CLI.
const allowedExecImporter = "internal/vault/runner.go"

// TestOSExecImportAllowlist statically parses every non-test .go file under
// longterm-mem/ and fails if any file other than allowedExecImporter imports
// "os/exec" (R-021).
func TestOSExecImportAllowlist(t *testing.T) {
	fset := token.NewFileSet()
	totalFiles := 0

	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "testdata", ".git":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		totalFiles++
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}

		for _, imp := range file.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			if importPath != "os/exec" {
				continue
			}
			if filepath.ToSlash(path) != allowedExecImporter {
				t.Errorf("forbidden os/exec import in %s — only %s may shell out (R-021)", path, allowedExecImporter)
			}
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walking longterm-mem/ for os/exec imports: %v", err)
	}

	// Sanity: confirm the walk actually visited production files, guarding
	// against a broken walk silently passing.
	if totalFiles == 0 {
		t.Fatal("no production .go files found under '.'; the allowlist walk may be broken")
	}
}
