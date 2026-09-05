package guard

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// allowedExecImporters are the ONLY production files permitted to import
// "os/exec" (R-021). Nothing in longterm-mem is allowed to shell out to
// Engram's own CLI, and every other subprocess is confined to one of these
// two files so the bound it runs under is stated in one place.
//
//   - internal/vault/runner.go runs the claude-obsidian vault's own
//     scripts, bounded to a resolved vault root.
//   - internal/repohistory/history.go reads git history, bounded to one
//     repository root and writing nothing. It is here because absence from
//     the working tree is NOT evidence that a path was removed: measured
//     against a real database, absent paths split into files genuinely
//     extirpated, files recorded under a different root, files RENAMED, and
//     files belonging to another repository. Only git can tell those apart,
//     and a memory wrongly reported as describing something gone is a
//     memory somebody deletes. Reproducing that judgement by parsing
//     packfiles in-process would be a far larger, far more fragile surface
//     than one read-only invocation of the tool that owns the answer.
//
// Adding a third entry is a change to R-021, not a convenience: it widens
// the one boundary this test exists to hold.
var allowedExecImporters = map[string]bool{
	"internal/vault/runner.go":        true,
	"internal/repohistory/history.go": true,
}

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
		if !allowedExecImporters[offender] {
			t.Errorf("forbidden os/exec import in %s — only %s may shell out (R-021)", offender, allowedNames())
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

// allowedNames renders the allowlist for a failure message, sorted so the
// message is stable.
func allowedNames() string {
	names := make([]string, 0, len(allowedExecImporters))
	for name := range allowedExecImporters {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// TestOSExecImportAllowlistStillRefusesOthers: widening an allowlist is the
// easy way to turn a guard into a rubber stamp. This asserts the list is a
// list and not a shrug -- a plausible third file is still forbidden.
func TestOSExecImportAllowlistStillRefusesOthers(t *testing.T) {
	for _, path := range []string{
		"internal/ops/doctor.go",
		"cmd/longterm-mem/main.go",
		"internal/repohistory/other.go",
	} {
		if allowedExecImporters[path] {
			t.Errorf("%s must not be allowed to shell out (R-021)", path)
		}
	}
	if len(allowedExecImporters) != 2 {
		t.Errorf("the allowlist holds %d entries; every addition is a change to R-021 and must be argued for, not slipped in", len(allowedExecImporters))
	}
}
