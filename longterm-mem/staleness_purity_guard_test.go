// This file shares package guard with skillstale_purity_guard_test.go (ambientUsesIn, whose rule it applies to
// another package), query_adapters_guard_test.go (adapterImportsIn, adapterPackages and modulePath), the
// architecture guard (rings) and the walk test of the Engram guard (writeTree).
package guard

import (
	"reflect"
	"strings"
	"testing"
)

// stalenessPackage is the rule that finds the memories a repository disagrees with: a function of the memories
// and of the repository evidence it is handed (RepoEvidence), which internal/repoevidence answers on a directory
// and git and the composition root wires.
const stalenessPackage = "internal/staleness"

// TestStalenessNamesNoAdapter keeps the rule free of the concrete adapters, in its production files and in its
// tests alike (the dependency rule of the rings reads production code only): not repohistory, which asks git,
// and not repoevidence, which walks the tree and answers for it.
func TestStalenessNamesNoAdapter(t *testing.T) {
	problems, err := adapterImportsIn(".", stalenessPackage, adapterPackages(rings), "")
	if err != nil {
		t.Fatal(err)
	}
	for _, problem := range problems {
		t.Error(problem)
	}
}

// TestStalenessReachesNothingAmbient keeps the rule, tests included, off the operating system (no walking of a
// directory, no process, no environment), the wall clock and the working directory: the architecture guard holds
// the production files to that, and this one also reads the tests, so that no case of the rule depends on the
// machine, the day it runs or the directory it is run from. A test that needs a file system or git belongs to
// the adapter that has one.
func TestStalenessReachesNothingAmbient(t *testing.T) {
	problems, err := ambientUsesIn(".", stalenessPackage)
	if err != nil {
		t.Fatal(err)
	}
	for _, problem := range problems {
		t.Error(problem)
	}
}

// The rule is tested on a tree built for it: the walking of a directory, a process, the file system's own
// members and an adapter, in a production file or a test of the package, are flagged, and a rule that is handed
// everything it reads passes.
func TestStalenessGuardsFlagTheWalkTheProcessAndTheAdaptersInTestsToo(t *testing.T) {
	imp := func(dir string) string { return `"` + modulePath + "/" + dir + `"` }
	root := writeTree(t, map[string]string{
		"internal/staleness/ok.go":       "package p\n\nimport (\n\t\"sort\"\n\t\"strings\"\n\t\"time\"\n)\n\nvar _ = sort.Strings\nvar _ = strings.Split\nvar _ time.Duration\n",
		"internal/staleness/walk.go":     "package p\n\nimport \"path/filepath\"\n\nvar _ = filepath.WalkDir\n",
		"internal/staleness/exec.go":     "package p\n\nimport \"os/exec\"\n\nvar _ = exec.Command\n",
		"internal/staleness/os.go":       "package p\n\nimport \"os\"\n\nvar _ = os.Stat\n",
		"internal/staleness/history.go":  importing(imp("internal/repohistory")),
		"internal/staleness/evidence.go": importing(imp("internal/repoevidence")),
		"internal/staleness/walk_test.go": "package p\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\n" +
			"func TestX(t *testing.T) { _, _ = os.ReadDir(t.TempDir()) }\n",
		"internal/staleness/git_test.go": importing(imp("internal/repohistory")),
	})

	ambient, err := ambientUsesIn(root, stalenessPackage)
	if err != nil {
		t.Fatalf("ambientUsesIn: %v", err)
	}
	adapters, err := adapterImportsIn(root, stalenessPackage, []string{"internal/repohistory", "internal/repoevidence"}, "")
	if err != nil {
		t.Fatalf("adapterImportsIn: %v", err)
	}

	var files []string
	for _, problem := range append(ambient, adapters...) {
		files = append(files, strings.SplitN(problem, " ", 2)[0])
	}
	flagged := map[string]bool{}
	for _, f := range files {
		flagged[f] = true
	}
	want := map[string]bool{
		"internal/staleness/walk.go":      true,
		"internal/staleness/exec.go":      true,
		"internal/staleness/os.go":        true,
		"internal/staleness/history.go":   true,
		"internal/staleness/evidence.go":  true,
		"internal/staleness/walk_test.go": true,
		"internal/staleness/git_test.go":  true,
	}
	if !reflect.DeepEqual(flagged, want) {
		t.Fatalf("flagged = %v, want %v (the pure uses of sort, strings and time pass)", files, want)
	}
}
