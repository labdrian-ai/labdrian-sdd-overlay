// This file shares package guard with skillstale_purity_guard_test.go (ambientUsesIn, whose rule it applies to
// other packages), query_adapters_guard_test.go (adapterImportsIn, adapterPackages and modulePath), the
// architecture guard (rings) and the walk test of the Engram guard (writeTree and importing).
package guard

import (
	"reflect"
	"strings"
	"testing"
)

// identityPackages are the packages that decide a project's identity: the rules that derive it from what a
// repository looks like (projectid), which read the repository through a port they own
// (projectid.RepositoryInspector) that internal/projectidfs answers on a disk, and the use case that lets what
// is already stored choose among the names it derives (adoption), which reads the stores, the ledger and the
// clock through ports of its own. The composition root wires the adapters of both.
var identityPackages = []string{"internal/projectid", "internal/adoption"}

// TestIdentityPackagesNameNoAdapter keeps the rules free of the concrete adapters, in their production files and
// in their tests alike (the dependency rule of the rings reads production code only): not projectidfs, which
// reads the repository's metadata, and not repohistory, which asks git.
func TestIdentityPackagesNameNoAdapter(t *testing.T) {
	for _, pkg := range identityPackages {
		problems, err := adapterImportsIn(".", pkg, adapterPackages(rings), "")
		if err != nil {
			t.Fatal(err)
		}
		for _, problem := range problems {
			t.Error(problem)
		}
	}
}

// TestIdentityPackagesReachNothingAmbient keeps the rules, tests included, off the operating system (no file, no
// process, no git, no environment), the wall clock and the working directory: the architecture guard holds the
// production files to that, and this one also reads the tests, so that no case of the rules depends on the
// machine, the day it runs or the directory it is run from. A test that needs a repository on a disk belongs to
// the adapter that reads one.
func TestIdentityPackagesReachNothingAmbient(t *testing.T) {
	for _, pkg := range identityPackages {
		problems, err := ambientUsesIn(".", pkg)
		if err != nil {
			t.Fatal(err)
		}
		for _, problem := range problems {
			t.Error(problem)
		}
	}
}

// The rules are tested on a tree built for them: a file that runs a process (git), reads the clock, touches the
// file system or the working directory, or names an adapter, in a production file or in a test, is flagged, and
// rules that are handed everything they read pass.
func TestIdentityGuardsFlagTheProcessTheClockTheDiskAndTheAdaptersInTestsToo(t *testing.T) {
	imp := func(dir string) string { return `"` + modulePath + "/" + dir + `"` }
	root := writeTree(t, map[string]string{
		"internal/projectid/ok.go":       "package p\n\nimport (\n\t\"errors\"\n\t\"path/filepath\"\n\t\"strings\"\n)\n\nvar _ = errors.New\nvar _ = filepath.Join\nvar _ = strings.TrimSpace\n",
		"internal/projectid/exec.go":     "package p\n\nimport \"os/exec\"\n\nvar _ = exec.Command\n",
		"internal/projectid/clock.go":    "package p\n\nimport \"time\"\n\nvar _ = time.Now\n",
		"internal/projectid/abs.go":      "package p\n\nimport \"path/filepath\"\n\nvar _ = filepath.Abs\n",
		"internal/projectid/links.go":    "package p\n\nimport \"path/filepath\"\n\nvar _ = filepath.EvalSymlinks\n",
		"internal/projectid/os.go":       "package p\n\nimport \"os\"\n\nvar _ = os.ReadFile\n",
		"internal/projectid/fs.go":       importing(imp("internal/projectidfs")),
		"internal/projectid/history.go":  importing(imp("internal/repohistory")),
		"internal/projectid/dir_test.go": "package p\n\nimport \"testing\"\n\nfunc TestX(t *testing.T) { t.Chdir(t.TempDir()) }\n",
		"internal/projectid/git_test.go": "package p\n\nimport (\n\t\"os/exec\"\n\t\"testing\"\n)\n\nfunc TestY(t *testing.T) { _ = exec.Command(\"git\") }\n",
	})

	ambient, err := ambientUsesIn(root, "internal/projectid")
	if err != nil {
		t.Fatalf("ambientUsesIn: %v", err)
	}
	adapters, err := adapterImportsIn(root, "internal/projectid", []string{"internal/projectidfs", "internal/repohistory"}, "")
	if err != nil {
		t.Fatalf("adapterImportsIn: %v", err)
	}

	flagged := map[string]bool{}
	for _, problem := range append(ambient, adapters...) {
		flagged[strings.SplitN(problem, " ", 2)[0]] = true
	}
	want := map[string]bool{
		"internal/projectid/exec.go":     true,
		"internal/projectid/clock.go":    true,
		"internal/projectid/abs.go":      true,
		"internal/projectid/links.go":    true,
		"internal/projectid/os.go":       true,
		"internal/projectid/fs.go":       true,
		"internal/projectid/history.go":  true,
		"internal/projectid/dir_test.go": true,
		"internal/projectid/git_test.go": true,
	}
	if !reflect.DeepEqual(flagged, want) {
		t.Fatalf("flagged = %v, want %v (the pure uses of errors, filepath.Join and strings pass)", flagged, want)
	}
}

// The same walk reads the adoption use case: the stores it asks (Engram, the vault registry), the ledger file and
// the adapters of the identity rules are not its to name, and the wall clock and the file system are not its to
// reach, in a production file or in a test. What it is handed through its ports is all it uses.
func TestIdentityGuardsReadTheAdoptionUseCaseToo(t *testing.T) {
	imp := func(dir string) string { return `"` + modulePath + "/" + dir + `"` }
	root := writeTree(t, map[string]string{
		"internal/adoption/ok.go":          "package p\n\nimport (\n\t\"fmt\"\n\t\"time\"\n)\n\nvar _ = fmt.Sprintf\nvar _ time.Time\n",
		"internal/adoption/clock.go":       "package p\n\nimport \"time\"\n\nvar _ = time.Now\n",
		"internal/adoption/engram.go":      importing(imp("internal/engram")),
		"internal/adoption/registry.go":    importing(imp("internal/vaultreg")),
		"internal/adoption/ledger.go":      importing(imp("internal/identityledger")),
		"internal/adoption/inspector.go":   importing(imp("internal/projectidfs")),
		"internal/adoption/env_test.go":    "package p\n\nimport \"testing\"\n\nfunc TestX(t *testing.T) { t.Setenv(\"HOME\", t.TempDir()) }\n",
		"internal/adoption/stdout_test.go": "package p\n\nimport \"fmt\"\n\nvar _ = fmt.Println\n",
	})

	ambient, err := ambientUsesIn(root, "internal/adoption")
	if err != nil {
		t.Fatalf("ambientUsesIn: %v", err)
	}
	adapters, err := adapterImportsIn(root, "internal/adoption", []string{"internal/engram", "internal/vaultreg", "internal/identityledger", "internal/projectidfs"}, "")
	if err != nil {
		t.Fatalf("adapterImportsIn: %v", err)
	}

	flagged := map[string]bool{}
	for _, problem := range append(ambient, adapters...) {
		flagged[strings.SplitN(problem, " ", 2)[0]] = true
	}
	want := map[string]bool{
		"internal/adoption/clock.go":       true,
		"internal/adoption/engram.go":      true,
		"internal/adoption/registry.go":    true,
		"internal/adoption/ledger.go":      true,
		"internal/adoption/inspector.go":   true,
		"internal/adoption/env_test.go":    true,
		"internal/adoption/stdout_test.go": true,
	}
	if !reflect.DeepEqual(flagged, want) {
		t.Fatalf("flagged = %v, want %v (fmt.Sprintf and the time type pass)", flagged, want)
	}
}
