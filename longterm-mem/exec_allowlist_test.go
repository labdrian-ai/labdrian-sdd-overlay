package guard

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// allowedExecImporter is the sole production file permitted to import
// "os/exec". Every subprocess call to the vault's scripts must go through
// internal/vault.Runner so it stays bounded to the vault root (R-021):
// nothing in longterm-mem is allowed to shell out to Engram's own CLI.
const allowedExecImporter = "internal/vault/runner.go"

// findOSExecImporters walks root and returns the slash-separated paths
// (relative to root) of every non-test .go file that imports "os/exec",
// plus the total number of production .go files visited.
//
// It is factored out of TestOSExecImportAllowlist so the walk rule itself
// can be regression-tested independently of the real longterm-mem/ tree
// (see TestOSExecImportAllowlistCatchesTestdataPackage).
//
// Only ".git" is pruned. A directory named "testdata" is deliberately NOT
// pruned: Go's own toolchain treats "testdata" as special only for its own
// package-discovery purposes, but nothing stops a "testdata" directory from
// holding a genuine compiled Go package — internal/ops/testdata/fixture.go
// is exactly that, a real "package testdata" reused by other tests, not
// inert fixture data — so the walk must still see its .go files. Non-.go
// content under testdata (fixtures, golden files, YAML) is already excluded
// by the ".go" suffix check below, so there is nothing left for a directory
// skip to usefully prune.
func findOSExecImporters(root string) (offenders []string, totalFiles int, walkErr error) {
	fset := token.NewFileSet()

	walkErr = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
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
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				rel = path
			}
			offenders = append(offenders, filepath.ToSlash(rel))
		}

		return nil
	})
	return offenders, totalFiles, walkErr
}

// TestOSExecImportAllowlist statically parses every non-test .go file under
// longterm-mem/ and fails if any file other than allowedExecImporter imports
// "os/exec" (R-021).
func TestOSExecImportAllowlist(t *testing.T) {
	offenders, totalFiles, err := findOSExecImporters(".")
	if err != nil {
		t.Fatalf("walking longterm-mem/ for os/exec imports: %v", err)
	}

	for _, offender := range offenders {
		if offender != allowedExecImporter {
			t.Errorf("forbidden os/exec import in %s — only %s may shell out (R-021)", offender, allowedExecImporter)
		}
	}

	// Sanity: confirm the walk actually visited production files, guarding
	// against a broken walk silently passing.
	if totalFiles == 0 {
		t.Fatal("no production .go files found under '.'; the allowlist walk may be broken")
	}
}

// TestOSExecImportAllowlistCatchesTestdataPackage is a regression test for
// the blind spot the walk above closes: a directory named "testdata" can
// hold a genuine compiled Go package (internal/ops/testdata/fixture.go is
// exactly that), so an os/exec import under such a directory must still be
// caught. It builds a synthetic tree whose only production file lives under
// a "testdata" directory and imports "os/exec", then asserts the walk
// reports it.
func TestOSExecImportAllowlistCatchesTestdataPackage(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "internal", "fake", "testdata")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	src := "package testdata\n\nimport \"os/exec\"\n\nvar _ = exec.Command\n"
	if err := os.WriteFile(filepath.Join(dir, "fixture.go"), []byte(src), 0o644); err != nil {
		t.Fatalf("write synthetic fixture: %v", err)
	}

	offenders, _, err := findOSExecImporters(root)
	if err != nil {
		t.Fatalf("findOSExecImporters: %v", err)
	}

	want := "internal/fake/testdata/fixture.go"
	found := false
	for _, o := range offenders {
		if o == want {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("findOSExecImporters: expected to catch the os/exec import under a testdata/ directory (%s), got offenders=%v", want, offenders)
	}
}
