package overlaytest_test

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
)

// entryWith is an entry point that lists the modules the way bin/labdrian-overlay does: one name to a line
// in the array the loader walks.
func entryWith(modules ...string) string {
	return "#!/usr/bin/env bash\nset -euo pipefail\n\n_modules=(\n  " + strings.Join(modules, "\n  ") + "\n)\nfor _module in \"${_modules[@]}\"; do :; done\n"
}

// fakeScript lays out an entry point and the modules beside it, in a directory of its own, and returns the
// path of the entry point.
func fakeScript(t *testing.T, entry string, modules map[string]string) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "bin")
	script := filepath.Join(bin, "labdrian-overlay")
	if err := os.MkdirAll(overlaytest.LibDir(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(script, []byte(entry), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range modules {
		if err := os.WriteFile(overlaytest.ModulePath(script, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return script
}

// recorder is a testing.TB whose Fatalf records the message and ends the goroutine it was called in,
// which is what a failed test does, so that a helper that fails a test can be shown to.
type recorder struct {
	testing.TB
	message string
}

func (r *recorder) Helper() {}
func (r *recorder) Fatalf(format string, args ...any) {
	r.message = fmt.Sprintf(format, args...)
	runtime.Goexit()
}

// failureOf runs f against a recorder, in a goroutine of its own, and returns what it failed with ("" when
// it did not fail).
func failureOf(t *testing.T, f func(testing.TB)) string {
	t.Helper()
	r := &recorder{TB: t}
	var done sync.WaitGroup
	done.Add(1)
	go func() {
		defer done.Done()
		f(r)
	}()
	done.Wait()
	return r.message
}

func TestScriptModulesAreTheWordsOfTheArrayOfTheEntryPointInTheOrderItListsThem(t *testing.T) {
	script := fakeScript(t, entryWith("common", "locations", "apply"), nil)

	got := overlaytest.ScriptModules(t, script)

	if want := []string{"common", "locations", "apply"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ScriptModules = %q, want %q: the order the entry point sources them in", got, want)
	}
}

func TestAnEntryPointWithNoArrayOfModulesHasNone(t *testing.T) {
	script := fakeScript(t, "#!/usr/bin/env bash\necho one file\n", nil)

	if got := overlaytest.ScriptModules(t, script); got != nil {
		t.Errorf("ScriptModules of a script that is one file = %q, want none", got)
	}
	if got := overlaytest.ScriptFilesOf(t, script); !reflect.DeepEqual(got, []string{script}) {
		t.Errorf("ScriptFilesOf of a script that is one file = %q, want the file alone", got)
	}
}

func TestScriptFilesOfAreTheEntryPointThenEachModuleInTheOrderListed(t *testing.T) {
	script := fakeScript(t, entryWith("second", "first"), map[string]string{"first": "f\n", "second": "s\n", "unlisted": "u\n"})

	got := overlaytest.ScriptFilesOf(t, script)

	want := []string{script, overlaytest.ModulePath(script, "second"), overlaytest.ModulePath(script, "first")}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ScriptFilesOf = %q, want %q: a module the entry point does not list is not part of the script", got, want)
	}
}

func TestScriptFilesOfFailsForAModuleThatIsListedAndNotThere(t *testing.T) {
	script := fakeScript(t, entryWith("present", "absent"), map[string]string{"present": "p\n"})

	message := failureOf(t, func(tb testing.TB) { overlaytest.ScriptFilesOf(tb, script) })

	if !strings.Contains(message, `"absent"`) || !strings.Contains(message, overlaytest.ModulePath(script, "absent")) {
		t.Errorf("the failure for a listed module that is not there = %q, want it to name the module and where it should be", message)
	}
}

func TestScriptTextOfIsTheTextOfTheFilesOneAfterTheOtherEachEndingInANewline(t *testing.T) {
	script := fakeScript(t, entryWith("a", "b"), map[string]string{"a": "alpha()", "b": "beta() { :; }\n"})
	entry, err := os.ReadFile(script)
	if err != nil {
		t.Fatal(err)
	}

	got := overlaytest.ScriptTextOf(t, script)

	if want := string(entry) + "alpha()\n" + "beta() { :; }\n"; got != want {
		t.Errorf("ScriptTextOf = %q, want %q: a file that does not end in a newline must not run into the next one", got, want)
	}
}

func TestCopyScriptOfCopiesTheEntryPointAndEveryModuleItListsWithTheirModes(t *testing.T) {
	src := fakeScript(t, entryWith("one", "two"), map[string]string{"one": "uno()\n", "two": "dos()\n", "unlisted": "tres()\n"})
	root := t.TempDir()

	copied := overlaytest.CopyScriptOf(t, src, root)

	if want := filepath.Join(root, "bin", "labdrian-overlay"); copied != want {
		t.Fatalf("CopyScriptOf = %s, want %s: the entry point under bin/ of the root it was given", copied, want)
	}
	for _, rel := range []string{"labdrian-overlay", "lib/one.sh", "lib/two.sh"} {
		want, errA := os.ReadFile(filepath.Join(filepath.Dir(src), rel))
		got, errB := os.ReadFile(filepath.Join(filepath.Dir(copied), rel))
		if errA != nil || errB != nil || string(want) != string(got) {
			t.Errorf("%s was not copied as it is (%v, %v)", rel, errA, errB)
		}
		infoA, _ := os.Stat(filepath.Join(filepath.Dir(src), rel))
		infoB, errStat := os.Stat(filepath.Join(filepath.Dir(copied), rel))
		if errStat == nil && infoA.Mode().Perm() != infoB.Mode().Perm() {
			t.Errorf("%s has mode %v and its copy %v", rel, infoA.Mode().Perm(), infoB.Mode().Perm())
		}
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(copied), "lib", "unlisted.sh")); err == nil {
		t.Error("a module the entry point does not list was copied: it is not part of the script")
	}
}

func TestCopyScriptCopiesTheScriptOfThisCheckoutSoThatItIsTheSameScript(t *testing.T) {
	copied := overlaytest.CopyScript(t, t.TempDir())

	want, got := overlaytest.ScriptTextOf(t, overlaytest.Script(t)), overlaytest.ScriptTextOf(t, copied)
	if want != got {
		t.Errorf("the text of the copy differs from the text of the script of this checkout")
	}
	if info, err := os.Stat(copied); err != nil || info.Mode().Perm()&0o111 == 0 {
		t.Errorf("the copy of the entry point is not executable (%v, %v)", info, err)
	}
}
