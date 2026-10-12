// This file shares package guard with query_adapters_guard_test.go (adapterImportsIn, adapterPackages and
// modulePath), the architecture guard (rings) and the walk test of the Engram guard (writeTree).
package guard

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// skillstalePackage is the detector of stale project skills: a function of the lock and the skill files, the
// candidate observations, the repository's evidence, a command resolver and the time, all handed to it.
const skillstalePackage = "internal/skillstale"

// TestSkillstaleNamesNoAdapter keeps the detector free of the concrete adapters, in its production files and in
// its tests alike (the dependency rule of the rings reads production code only). Its ports are its own:
// SkillSource, PathFactSource, CommandResolver and ObservationLister; internal/skillstalefs answers the first
// three on a file system and a repository, and the composition root wires it.
func TestSkillstaleNamesNoAdapter(t *testing.T) {
	problems, err := adapterImportsIn(".", skillstalePackage, adapterPackages(rings), "")
	if err != nil {
		t.Fatal(err)
	}
	for _, problem := range problems {
		t.Error(problem)
	}
}

// TestSkillstaleReachesNothingAmbient keeps the detector, tests included, off the operating system, the process
// environment, the working directory, the wall clock and the PATH: the architecture guard holds the production
// files to that, and a debt line used to hide the rest (os, time.Now, filepath.EvalSymlinks); this one also reads
// the tests, so that no case of the detector depends on the day it runs, on the machine or on the directory it
// is run from. A test that needs a file system or a PATH belongs to the adapter that has one.
func TestSkillstaleReachesNothingAmbient(t *testing.T) {
	problems, err := ambientUsesIn(".", skillstalePackage)
	if err != nil {
		t.Fatal(err)
	}
	for _, problem := range problems {
		t.Error(problem)
	}
}

// ambientImports are the packages whose import alone is the process or the machine.
var ambientImports = map[string]bool{
	"os": true, "os/exec": true, "os/signal": true, "os/user": true, "syscall": true, "runtime": true,
	"net": true, "net/http": true, "database/sql": true, "log": true, "flag": true, "embed": true,
}

// ambientMembers are the members of packages that are pure to import but not to use, by the package's name.
var ambientMembers = map[string][]string{
	"time":          {"Now", "Since", "Until", "Sleep", "After", "AfterFunc", "Tick", "NewTicker", "NewTimer"},
	"path/filepath": {"Abs", "EvalSymlinks", "Glob", "Walk", "WalkDir"},
	"io/fs":         {"Glob", "ReadDir", "ReadFile", "Stat", "WalkDir"},
	"fmt":           {"Print", "Printf", "Println", "Scan", "Scanf", "Scanln"},
}

// ambientMethods are method names that put a test in the environment or the file system whatever they are called
// on: t.Setenv, t.Chdir and t.TempDir.
var ambientMethods = []string{"Setenv", "Chdir", "TempDir"}

// ambientUsesIn reads every Go file under pkg, tests included, and says which import an ambient package, select
// an ambient member of one that is pure to import, or call a method that reaches the environment. A file it
// cannot parse, or a package that is not there, is an error: the rule would guard nothing.
func ambientUsesIn(root, pkg string) ([]string, error) {
	var problems []string
	err := filepath.WalkDir(filepath.Join(root, filepath.FromSlash(pkg)), func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		isTest := strings.HasSuffix(path, "_test.go")
		localNames := map[string]string{} // the name the file gives an import -> its path
		for _, spec := range file.Imports {
			importPath, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			if importPath == "os" && isTest {
				// A test may read a checked-in fixture (os.ReadFile) and nothing else of the system: the
				// member check below flags every other use.
				name := "os"
				if spec.Name != nil {
					name = spec.Name.Name
				}
				localNames[name] = importPath
				continue
			}
			if ambientImports[importPath] {
				problems = append(problems, rel+" imports "+importPath+": the detector reaches the process or the machine only through its ports")
			}
			if _, guarded := ambientMembers[importPath]; guarded {
				name := filepath.Base(importPath)
				if spec.Name != nil {
					name = spec.Name.Name
				}
				if name == "." {
					problems = append(problems, rel+" dot-imports "+importPath+", which would hide the ambient members it selects")
					continue
				}
				localNames[name] = importPath
			}
		}
		ast.Inspect(file, func(n ast.Node) bool {
			selector, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			for _, method := range ambientMethods {
				if selector.Sel.Name == method {
					problems = append(problems, rel+" calls "+method+": the detector's cases do not depend on the environment or on files")
				}
			}
			if pkgIdent, ok := selector.X.(*ast.Ident); ok {
				if importPath, guarded := localNames[pkgIdent.Name]; guarded {
					if importPath == "os" && selector.Sel.Name != "ReadFile" {
						problems = append(problems, rel+" uses os."+selector.Sel.Name+": a test of the detector reads a checked-in fixture with os.ReadFile and touches nothing else of the system")
					}
					for _, member := range ambientMembers[importPath] {
						if selector.Sel.Name == member {
							problems = append(problems, rel+" uses "+importPath+"."+member+": take the value as an input, or a port")
						}
					}
				}
			}
			return true
		})
		return nil
	})
	sort.Strings(problems)
	return problems, err
}

// The rule is tested on a tree built for it: an ambient import, an ambient member of a pure package through its
// own name and through an alias, a dot import, and the environment methods are flagged in production files and in
// tests, and the pure uses of the same packages pass.
func TestAmbientUsesInFlagsTheProcessTheClockAndTheEnvironmentInTestsToo(t *testing.T) {
	root := writeTree(t, map[string]string{
		"internal/skillstale/ok.go": "package p\n\nimport (\n\t\"fmt\"\n\t\"path/filepath\"\n\t\"time\"\n)\n\n" +
			"var _ = fmt.Sprintf\nvar _ time.Duration\nvar _ = filepath.IsAbs\nvar _ = time.Date\n",
		"internal/skillstale/os.go":           "package p\n\nimport \"os\"\n\nvar _ = os.Args\n",
		"internal/skillstale/clock.go":        "package p\n\nimport \"time\"\n\nvar _ = time.Now\n",
		"internal/skillstale/alias.go":        "package p\n\nimport t \"time\"\n\nvar _ = t.Since\n",
		"internal/skillstale/dot.go":          "package p\n\nimport . \"time\"\n\nvar _ = Duration(0)\n",
		"internal/skillstale/walk.go":         "package p\n\nimport \"path/filepath\"\n\nvar _ = filepath.EvalSymlinks\n",
		"internal/skillstale/print.go":        "package p\n\nimport \"fmt\"\n\nfunc f() { fmt.Println() }\n",
		"internal/skillstale/env_test.go":     "package p\n\nimport \"testing\"\n\nfunc TestX(t *testing.T) { t.Setenv(\"A\", \"b\"); t.Chdir(\".\"); _ = t.TempDir() }\n",
		"internal/skillstale/fixture_test.go": "package p\n\nimport \"os\"\n\nvar _, _ = os.ReadFile(\"f\")\n",
		"internal/skillstale/getenv_test.go":  "package p\n\nimport \"os\"\n\nvar _ = os.Getenv\n",
		"internal/skillstale/testdata/x.go":   "package p\n\nimport \"os\"\n\nvar _ = os.Args\n",
		"internal/other/clock.go":             "package p\n\nimport \"time\"\n\nvar _ = time.Now\n",
	})

	got, err := ambientUsesIn(root, skillstalePackage)
	if err != nil {
		t.Fatalf("ambientUsesIn: %v", err)
	}

	var files []string
	for _, problem := range got {
		files = append(files, strings.SplitN(problem, " ", 2)[0])
	}
	want := []string{
		"internal/skillstale/alias.go",
		"internal/skillstale/clock.go",
		"internal/skillstale/dot.go",
		"internal/skillstale/env_test.go",
		"internal/skillstale/env_test.go",
		"internal/skillstale/env_test.go",
		"internal/skillstale/getenv_test.go",
		"internal/skillstale/os.go",
		"internal/skillstale/print.go",
		"internal/skillstale/walk.go",
	}
	if !reflect.DeepEqual(files, want) {
		t.Fatalf("flagged = %v, want %v (testdata and other packages are not looked at; the pure uses of time, filepath and fmt pass)", files, want)
	}
}

func TestAmbientUsesInFailsOnAPackageThatIsNotThere(t *testing.T) {
	if _, err := ambientUsesIn(t.TempDir(), skillstalePackage); err == nil {
		t.Error("a package that is not there passed: the rule would guard nothing")
	}
}

// The adapter rule is tested too: a production file or a test of the detector that names the file system
// adapter, or any other adapter, is flagged, and there is no exception for tests.
func TestSkillstaleMayNameNoAdapterNotEvenInATest(t *testing.T) {
	imp := func(dir string) string { return `"` + modulePath + "/" + dir + `"` }
	root := writeTree(t, map[string]string{
		"internal/skillstale/skillstale.go":       importing(imp("internal/memory")),
		"internal/skillstale/fs.go":               importing(imp("internal/skillstalefs")),
		"internal/skillstale/fs_test.go":          importing(imp("internal/skillstalefs")),
		"internal/skillstale/engram_test.go":      importing(imp("internal/engram")),
		"internal/skillstale/testdata/ignored.go": importing(imp("internal/vaultfs")),
	})
	adapters := []string{"internal/engram", "internal/skillstalefs", "internal/vaultfs"}

	got, err := adapterImportsIn(root, skillstalePackage, adapters, "")
	if err != nil {
		t.Fatalf("adapterImportsIn: %v", err)
	}

	var files []string
	for _, problem := range got {
		files = append(files, strings.SplitN(problem, " ", 2)[0])
	}
	want := []string{
		"internal/skillstale/engram_test.go",
		"internal/skillstale/fs.go",
		"internal/skillstale/fs_test.go",
	}
	if !reflect.DeepEqual(files, want) {
		t.Fatalf("flagged files = %v, want %v", files, want)
	}
}
