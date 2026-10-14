// This file shares package guard with skillstale_purity_guard_test.go (ambientUsesIn, whose rule it applies to
// another package), query_adapters_guard_test.go (adapterImportsIn, adapterPackages and modulePath), the
// architecture guard (rings) and the walk test of the Engram guard (writeTree and importing).
package guard

import (
	"reflect"
	"strings"
	"testing"
)

// ingestPackage is the extraction and the identity of what is ingested: the provenance block, the slug, the id of
// a source and the chunk boundary, all functions of the text and the origin they are handed. Where a relative
// path begins is asked of the SourceReader port the package owns (ingest.ResolveOrigin), which internal/sourcefs
// answers on a file system and the composition root wires.
const ingestPackage = "internal/ingest"

// TestIngestNamesNoAdapter keeps the package free of the concrete adapters, in its production files and in its
// tests alike (the dependency rule of the rings reads production code only): not sourcefs, which reads the file
// system for it, and none of the others.
func TestIngestNamesNoAdapter(t *testing.T) {
	problems, err := adapterImportsIn(".", ingestPackage, adapterPackages(rings), "")
	if err != nil {
		t.Fatal(err)
	}
	for _, problem := range problems {
		t.Error(problem)
	}
}

// TestIngestReachesNothingAmbient keeps the package, tests included, off the operating system (no file, no
// directory, no process, no environment), the wall clock and the working directory: the architecture guard holds
// the production files to that, and a debt line used to hide the one call that did not (filepath.Abs); this one
// also reads the tests, so that no case of the package depends on the machine, the day it runs or the directory it
// is run from. A test that needs a file system belongs to the adapter that has one (internal/sourcefs), and the
// ingestion use case is tested over a SourceReader that answers what the case needs.
func TestIngestReachesNothingAmbient(t *testing.T) {
	problems, err := ambientUsesIn(".", ingestPackage)
	if err != nil {
		t.Fatal(err)
	}
	for _, problem := range problems {
		t.Error(problem)
	}
}

// The rule is tested on a tree built for it: the resolution of a path against the process, the walking of a
// directory, a process, the file system's own members, the clock and an adapter, in a production file or a test
// of the package, are flagged, and a package that is handed everything it reads passes.
func TestIngestGuardsFlagTheDiskTheProcessTheClockAndTheAdaptersInTestsToo(t *testing.T) {
	imp := func(dir string) string { return `"` + modulePath + "/" + dir + `"` }
	root := writeTree(t, map[string]string{
		"internal/ingest/ok.go": "package p\n\nimport (\n\t\"errors\"\n\t\"path/filepath\"\n\t\"strings\"\n\t\"time\"\n)\n\n" +
			"var _ = errors.New\nvar _ = filepath.IsAbs\nvar _ = filepath.Join\nvar _ = filepath.Clean\nvar _ = filepath.Base\n" +
			"var _ = filepath.Ext\nvar _ = strings.TrimSpace\nvar _ time.Time\n",
		"internal/ingest/abs.go":       "package p\n\nimport \"path/filepath\"\n\nvar _ = filepath.Abs\n",
		"internal/ingest/links.go":     "package p\n\nimport \"path/filepath\"\n\nvar _ = filepath.EvalSymlinks\n",
		"internal/ingest/walk.go":      "package p\n\nimport \"path/filepath\"\n\nvar _ = filepath.WalkDir\n",
		"internal/ingest/glob.go":      "package p\n\nimport \"path/filepath\"\n\nvar _ = filepath.Glob\n",
		"internal/ingest/os.go":        "package p\n\nimport \"os\"\n\nvar _ = os.Getwd\n",
		"internal/ingest/exec.go":      "package p\n\nimport \"os/exec\"\n\nvar _ = exec.Command\n",
		"internal/ingest/clock.go":     "package p\n\nimport \"time\"\n\nvar _ = time.Now\n",
		"internal/ingest/readdir.go":   "package p\n\nimport \"io/fs\"\n\nvar _ = fs.ReadDir\n",
		"internal/ingest/sourcefs.go":  importing(imp("internal/sourcefs")),
		"internal/ingest/dir_test.go":  "package p\n\nimport \"testing\"\n\nfunc TestX(t *testing.T) { t.Chdir(t.TempDir()) }\n",
		"internal/ingest/cwd_test.go":  "package p\n\nimport \"os\"\n\nvar _ = os.Getwd\n",
		"internal/ingest/real_test.go": importing(imp("internal/sourcefs")),
	})

	ambient, err := ambientUsesIn(root, ingestPackage)
	if err != nil {
		t.Fatalf("ambientUsesIn: %v", err)
	}
	adapters, err := adapterImportsIn(root, ingestPackage, []string{"internal/sourcefs"}, "")
	if err != nil {
		t.Fatalf("adapterImportsIn: %v", err)
	}

	flagged := map[string]bool{}
	for _, problem := range append(ambient, adapters...) {
		flagged[strings.SplitN(problem, " ", 2)[0]] = true
	}
	want := map[string]bool{
		"internal/ingest/abs.go":       true,
		"internal/ingest/links.go":     true,
		"internal/ingest/walk.go":      true,
		"internal/ingest/glob.go":      true,
		"internal/ingest/os.go":        true,
		"internal/ingest/exec.go":      true,
		"internal/ingest/clock.go":     true,
		"internal/ingest/readdir.go":   true,
		"internal/ingest/sourcefs.go":  true,
		"internal/ingest/dir_test.go":  true,
		"internal/ingest/cwd_test.go":  true,
		"internal/ingest/real_test.go": true,
	}
	if !reflect.DeepEqual(flagged, want) {
		t.Fatalf("flagged = %v, want %v (the pure uses of errors, filepath.IsAbs, Join, Clean, Base and Ext, strings and the time type pass)", flagged, want)
	}
}
