// This file shares package guard with skillstale_purity_guard_test.go (ambientUsesIn), query_adapters_guard_test.go
// (adapterImportsIn, adapterPackages and modulePath), the architecture guard (rings) and the walk test of the
// Engram guard (writeTree and importing).
package guard

import (
	"reflect"
	"strings"
	"testing"
)

// runtimesPackage is the use cases behind register and unregister: a function of the request, a Registrar that
// writes a runtime's configuration and a Machine that tells paths and probes pi, the last two handed to it.
const runtimesPackage = "internal/runtimes"

// TestRuntimesNamesNoAdapter keeps the use cases free of the concrete adapters, in their production files and in
// their tests alike (the dependency rule of the rings reads production code only): not the writers of the
// runtimes' configuration, which are the Registrar they are handed, and not the files of the machine.
func TestRuntimesNamesNoAdapter(t *testing.T) {
	problems, err := adapterImportsIn(".", runtimesPackage, adapterPackages(rings), "")
	if err != nil {
		t.Fatal(err)
	}
	for _, problem := range problems {
		t.Error(problem)
	}
}

// TestRuntimesReachesNothingAmbient keeps the use cases, tests included, off the operating system, the process
// environment, the working directory, the wall clock and the file system: the architecture guard holds the
// production files to that, and this one also reads the tests, so that no case of them depends on the machine it
// runs on. Where a runtime keeps its configuration and whether pi is installed are asked of the Machine.
func TestRuntimesReachesNothingAmbient(t *testing.T) {
	problems, err := ambientUsesIn(".", runtimesPackage)
	if err != nil {
		t.Fatal(err)
	}
	for _, problem := range problems {
		t.Error(problem)
	}
}

// The rule is tested on a tree built for it: a file that reads the environment or the home, touches the file
// system, or names an adapter, in a production file or in a test, is flagged, and use cases that are handed
// everything they read pass.
func TestRuntimesGuardsFlagTheMachineTheDiskAndTheAdaptersInTestsToo(t *testing.T) {
	imp := func(dir string) string { return `"` + modulePath + "/" + dir + `"` }
	root := writeTree(t, map[string]string{
		"internal/runtimes/ok.go":        "package p\n\nimport (\n\t\"errors\"\n\t\"io/fs\"\n)\n\nvar _ = errors.New\nvar _ = fs.ErrNotExist\n",
		"internal/runtimes/home.go":      "package p\n\nimport \"os\"\n\nvar _ = os.UserHomeDir\n",
		"internal/runtimes/walk.go":      "package p\n\nimport \"io/fs\"\n\nvar _ = fs.WalkDir\n",
		"internal/runtimes/clock.go":     "package p\n\nimport \"time\"\n\nvar _ = time.Now\n",
		"internal/runtimes/writer.go":    importing(imp("internal/register")),
		"internal/runtimes/durable.go":   importing(imp("internal/durable")),
		"internal/runtimes/env_test.go":  "package p\n\nimport \"testing\"\n\nfunc TestX(t *testing.T) { t.Setenv(\"HOME\", t.TempDir()) }\n",
		"internal/runtimes/file_test.go": "package p\n\nimport \"os\"\n\nvar _ = os.WriteFile\n",
	})

	ambient, err := ambientUsesIn(root, runtimesPackage)
	if err != nil {
		t.Fatalf("ambientUsesIn: %v", err)
	}
	adapters, err := adapterImportsIn(root, runtimesPackage, []string{"internal/register", "internal/durable"}, "")
	if err != nil {
		t.Fatalf("adapterImportsIn: %v", err)
	}

	flagged := map[string]bool{}
	for _, problem := range append(ambient, adapters...) {
		flagged[strings.SplitN(problem, " ", 2)[0]] = true
	}
	want := map[string]bool{
		"internal/runtimes/home.go":      true,
		"internal/runtimes/walk.go":      true,
		"internal/runtimes/clock.go":     true,
		"internal/runtimes/writer.go":    true,
		"internal/runtimes/durable.go":   true,
		"internal/runtimes/env_test.go":  true,
		"internal/runtimes/file_test.go": true,
	}
	if !reflect.DeepEqual(flagged, want) {
		t.Fatalf("flagged = %v, want %v (the pure uses of errors and of fs.ErrNotExist pass)", flagged, want)
	}
}
