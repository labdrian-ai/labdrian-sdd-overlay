package testhome

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The guard test of a package fails when the package has the line that enters the home and not the TestMain,
// and says nothing when a package has neither. This test is for the second case: every package of the module
// that has tests carries both, so a package added later starts in a home of its own or fails here.

// exempt are the packages that may not use Run: this one, whose tests change the process's environment and set
// it back themselves, to prove what Run does.
var exempt = map[string]bool{"internal/testhome": true}

const (
	runLine   = "testhome.Run(m)"
	checkLine = "testhome.Check(t)"
)

// packagesWithoutAHome walks the Go module at root and names, in a stable order, each package directory that
// has test files but no TestMain entering the home of the run, or no test that checks it did.
func packagesWithoutAHome(root string) ([]string, error) {
	type found struct{ tests, run, check bool }
	byDir := map[string]*found{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if name := entry.Name(); path != root && (name == "testdata" || name == "vendor" || strings.HasPrefix(name, ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		dir, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}
		dir = filepath.ToSlash(dir)
		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		f := byDir[dir]
		if f == nil {
			f = &found{}
			byDir[dir] = f
		}
		f.tests = true
		f.run = f.run || strings.Contains(string(source), runLine)
		f.check = f.check || strings.Contains(string(source), checkLine)
		return nil
	})
	if err != nil {
		return nil, err
	}
	var problems []string
	for dir, f := range byDir {
		if exempt[dir] {
			continue
		}
		if !f.run {
			problems = append(problems, dir+": no TestMain enters the home of the run (os.Exit(testhome.Run(m)))")
		}
		if !f.check {
			problems = append(problems, dir+": no test proves it did (testhome.Check(t))")
		}
	}
	sort.Strings(problems)
	return problems, nil
}

func TestEveryPackageWithTestsRunsInAHomeOfItsOwn(t *testing.T) {
	problems, err := packagesWithoutAHome(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for _, problem := range problems {
		t.Error(problem)
	}
}

// The walk is held to a built tree: it flags a package with neither line, with only one, and a test directory
// nested in a package that has no tests of its own; it passes a package with both, ignores testdata, vendor and
// hidden directories, and leaves this package's own directory alone.
func TestPackagesWithoutAHome_FlagsWhatIsMissingAndNothingElse(t *testing.T) {
	root := t.TempDir()
	write := func(rel, source string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	main := "func TestMain(m *testing.M) { os.Exit(testhome.Run(m)) }\n"
	guard := "func TestHome(t *testing.T) { testhome.Check(t) }\n"

	write("good/a_test.go", main)
	write("good/b_test.go", guard) // the two lines may live in different files of the package
	write("noboth/a_test.go", "package noboth\n")
	write("nomain/a_test.go", guard)
	write("noguard/a_test.go", main)
	write("nested/inner/a_test.go", "package inner\n")
	write("nested/prod.go", "package nested\n")
	write("withdata/ok_test.go", main+guard)
	write("withdata/testdata/x/a_test.go", "package x\n")
	write("vendor/v/a_test.go", "package v\n")
	write(".hidden/a_test.go", "package hidden\n")
	write("internal/testhome/a_test.go", "package testhome\n")
	write("production/only.go", "package production\n")

	got, err := packagesWithoutAHome(root)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{
		"nested/inner: no TestMain enters the home of the run (os.Exit(testhome.Run(m)))",
		"nested/inner: no test proves it did (testhome.Check(t))",
		"nomain: no TestMain enters the home of the run (os.Exit(testhome.Run(m)))",
		"noboth: no TestMain enters the home of the run (os.Exit(testhome.Run(m)))",
		"noboth: no test proves it did (testhome.Check(t))",
		"noguard: no test proves it did (testhome.Check(t))",
	}
	sort.Strings(want)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("problems =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
