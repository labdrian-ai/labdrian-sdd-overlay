package analyzers_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/analyzers"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
)

// The tests of this file never run a real analyzer. Each tool is a small shell script in a temporary directory that
// is the whole PATH of the test, so the adapter starts exactly the program the test wrote, and a tool that is missing
// is really missing. The script appends where it ran and with which arguments to a log the test reads.

type stub struct {
	exit   int
	stdout string
	stderr string
	sleep  bool
	late   bool // with sleep: print first, and only then outlive the deadline
}

// bin makes the directory of the tools of the test, puts it alone on the PATH and returns the log the tools write.
func bin(t *testing.T, tools map[string]stub) (log string) {
	t.Helper()
	dir := t.TempDir()
	log = filepath.Join(t.TempDir(), "calls.log")
	for name, s := range tools {
		script := "#!/bin/sh\nprintf '%s|%s\\n' \"$(pwd)\" \"$*\" >> '" + log + "'\n"
		if s.sleep && !s.late {
			script += "exec /bin/sleep 5\n"
		}
		script += fmt.Sprintf("printf '%%s' '%s'\nprintf '%%s' '%s' >&2\n", s.stdout, s.stderr)
		if s.sleep && s.late {
			script += "exec /bin/sleep 5\n"
		}
		script += fmt.Sprintf("exit %d\n", s.exit)
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
	return log
}

func calls(t *testing.T, log string) []string {
	t.Helper()
	data, err := os.ReadFile(log)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}

func tools(t *testing.T) analyzers.Tools {
	t.Helper()
	return analyzers.New(analyzers.ExecRunner{}, analyzers.Options{Environ: os.Environ(), Timeout: time.Minute})
}

// copyOf makes a copy of a tree with a module "mod" that has the packages a and b and the files given.
func copyOf(t *testing.T, files ...string) string {
	t.Helper()
	root := t.TempDir()
	for _, rel := range append([]string{"mod/go.mod", "mod/a/x.go", "mod/b/y.go", "scripts/a.sh"}, files...) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestDeadcodeRunsOverTheWholeModuleOfTheCopyAndReturnsItsFindingLines(t *testing.T) {
	log := bin(t, map[string]stub{"deadcode": {stdout: "a/x.go:3:6: unreachable func: Old\nsome remark that is no finding\nb/y.go:1:1: unreachable func: Dead\n"}})
	root := copyOf(t)

	got, err := tools(t).Deadcode(root, "mod")

	want := []string{"a/x.go:3:6: unreachable func: Old", "b/y.go:1:1: unreachable func: Dead"}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("Deadcode = %q, %v, want %q, nil", got, err, want)
	}
	if ran := calls(t, log); !reflect.DeepEqual(ran, []string{filepath.Join(root, "mod") + "|-test ./..."}) {
		t.Errorf("deadcode ran as %q, want it in the module of the copy with -test ./...", ran)
	}
}

func TestACleanToolIsNoFindings(t *testing.T) {
	bin(t, map[string]stub{"deadcode": {}})
	root := copyOf(t)

	got, err := tools(t).Deadcode(root, "mod")

	if err != nil || len(got) != 0 {
		t.Errorf("Deadcode = %q, %v, want none, nil", got, err)
	}
}

func TestAToolThatExitsNonZeroWithFindingsHasRun(t *testing.T) {
	bin(t, map[string]stub{"staticcheck": {exit: 1, stdout: "a/x.go:4:2: this is wrong (SA1000)\n"}})
	root := copyOf(t)

	got, err := tools(t).Staticcheck(root, "mod", []string{"./a"})

	want := []string{"a/x.go:4:2: this is wrong (SA1000)"}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("Staticcheck = %q, %v, want %q, nil", got, err, want)
	}
}

func TestAToolThatExitsNonZeroWithNoFindingLineDidNotRun(t *testing.T) {
	cases := map[string]func(analyzers.Tools, string) error{
		"deadcode": func(a analyzers.Tools, root string) error { _, err := a.Deadcode(root, "mod"); return err },
		"staticcheck": func(a analyzers.Tools, root string) error {
			_, err := a.Staticcheck(root, "mod", []string{"./a"})
			return err
		},
		"shellcheck": func(a analyzers.Tools, root string) error {
			_, err := a.Shellcheck(root, []string{"scripts/a.sh"})
			return err
		},
	}
	for name, run := range cases {
		t.Run(name, func(t *testing.T) {
			bin(t, map[string]stub{name: {exit: 3, stdout: "-: build failed\n", stderr: "toolchain exploded"}})

			err := run(tools(t), copyOf(t))

			if !errors.Is(err, app.ErrToolDidNotRun) || !strings.Contains(err.Error(), name) || !strings.Contains(err.Error(), "toolchain exploded") {
				t.Errorf("error = %v, want ErrToolDidNotRun naming %s and what it said", err, name)
			}
		})
	}
}

func TestAToolThatIsNotOnThePathDidNotRun(t *testing.T) {
	bin(t, map[string]stub{})
	root := copyOf(t)

	_, deadErr := tools(t).Deadcode(root, "mod")
	_, staticErr := tools(t).Staticcheck(root, "mod", []string{"./a"})
	_, shellErr := tools(t).Shellcheck(root, []string{"scripts/a.sh"})

	for name, err := range map[string]error{"deadcode": deadErr, "staticcheck": staticErr, "shellcheck": shellErr} {
		if !errors.Is(err, app.ErrToolDidNotRun) || !strings.Contains(err.Error(), name) {
			t.Errorf("%s error = %v, want ErrToolDidNotRun naming it", name, err)
		}
	}
}

func TestAToolThatOutlivesItsTimeDidNotRun(t *testing.T) {
	bin(t, map[string]stub{"deadcode": {sleep: true}})
	a := analyzers.New(analyzers.ExecRunner{}, analyzers.Options{Environ: os.Environ(), Timeout: 200 * time.Millisecond})

	_, err := a.Deadcode(copyOf(t), "mod")

	if !errors.Is(err, app.ErrToolDidNotRun) {
		t.Errorf("error = %v, want ErrToolDidNotRun for a tool stopped at its deadline", err)
	}
}

func TestAToolStoppedAtItsDeadlineDidNotRunEvenAfterItPrintedAFinding(t *testing.T) {
	bin(t, map[string]stub{"deadcode": {sleep: true, late: true, stdout: "a/x.go:3:6: unreachable func: Old\n"}})
	a := analyzers.New(analyzers.ExecRunner{}, analyzers.Options{Environ: os.Environ(), Timeout: 200 * time.Millisecond})

	got, err := a.Deadcode(copyOf(t), "mod")

	if !errors.Is(err, app.ErrToolDidNotRun) || len(got) != 0 {
		t.Errorf("Deadcode = %q, %v, want ErrToolDidNotRun: what a stopped program printed is not a whole answer", got, err)
	}
}

func TestAModuleTheCopyDoesNotHaveHasNoFindingsAndRunsNoTool(t *testing.T) {
	log := bin(t, map[string]stub{"deadcode": {stdout: "a.go:1:1: unreachable func: X\n"}, "staticcheck": {stdout: "a.go:1:1: wrong\n"}})
	root := t.TempDir()

	dead, deadErr := tools(t).Deadcode(root, "mod")
	static, staticErr := tools(t).Staticcheck(root, "mod", []string{"."})

	if deadErr != nil || staticErr != nil || len(dead) != 0 || len(static) != 0 {
		t.Errorf("Deadcode = %q, %v; Staticcheck = %q, %v, want none: the module is new at the other side", dead, deadErr, static, staticErr)
	}
	if ran := calls(t, log); len(ran) != 0 {
		t.Errorf("tools ran as %q, want none", ran)
	}
}

func TestStaticcheckRunsOnTheExistingPackagesOnly(t *testing.T) {
	log := bin(t, map[string]stub{"staticcheck": {}})
	root := copyOf(t)

	_, err := tools(t).Staticcheck(root, "mod", []string{"./a", "./gone", "./b"})
	_, errNone := tools(t).Staticcheck(root, "mod", []string{"./gone"})

	if err != nil || errNone != nil {
		t.Fatalf("Staticcheck errors = %v, %v, want none", err, errNone)
	}
	want := []string{filepath.Join(root, "mod") + "|./a ./b"}
	if ran := calls(t, log); !reflect.DeepEqual(ran, want) {
		t.Errorf("staticcheck ran as %q, want %q: a package that is not in the copy is not asked for, and no package is no run", ran, want)
	}
}

func TestShellcheckRunsOnTheExistingFilesOnlyInTheRootOfTheCopy(t *testing.T) {
	log := bin(t, map[string]stub{"shellcheck": {exit: 1, stdout: "scripts/a.sh:3:5: warning: quote this [SC2086]\n"}})
	root := copyOf(t)

	got, err := tools(t).Shellcheck(root, []string{"scripts/a.sh", "scripts/gone.sh"})
	_, errNone := tools(t).Shellcheck(root, []string{"scripts/gone.sh"})

	want := []string{"scripts/a.sh:3:5: warning: quote this [SC2086]"}
	if err != nil || errNone != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("Shellcheck = %q, %v (%v), want %q, nil", got, err, errNone, want)
	}
	if ran := calls(t, log); !reflect.DeepEqual(ran, []string{root + "|-f gcc -- scripts/a.sh"}) {
		t.Errorf("shellcheck ran as %q, want -f gcc -- and only the file the copy has", ran)
	}
}

func TestShellcheckEndsItsOptionsBeforeTheFilesSoAFileNamedLikeAnOptionIsAFile(t *testing.T) {
	log := bin(t, map[string]stub{"shellcheck": {}})
	root := copyOf(t, "-x.sh")

	if _, err := tools(t).Shellcheck(root, []string{"-x.sh", "scripts/a.sh"}); err != nil {
		t.Fatalf("Shellcheck: %v", err)
	}

	if ran := calls(t, log); !reflect.DeepEqual(ran, []string{root + "|-f gcc -- -x.sh scripts/a.sh"}) {
		t.Errorf("shellcheck ran as %q, want -f gcc -- and then the files", ran)
	}
}

// probeTool makes a tool named name that appends the variables it was started with to a log, and puts it alone on the PATH.
func probeTool(t *testing.T, name string) (log string) {
	t.Helper()
	dir := t.TempDir()
	log = filepath.Join(t.TempDir(), "exported.log")
	script := "#!/bin/sh\nexport -p >> '" + log + "'\nexit 0\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	return log
}

func TestToolsStartedWithNoEnvironmentDoNotInheritTheOneOfTheProcess(t *testing.T) {
	for name, environ := range map[string][]string{"a nil one": nil, "an empty one": {}} {
		t.Run(name, func(t *testing.T) {
			log := probeTool(t, "deadcode")
			t.Setenv("LABDRIAN_PROBE_LEAK", "leaked")
			a := analyzers.New(analyzers.ExecRunner{}, analyzers.Options{Environ: environ, Timeout: time.Minute})

			if _, err := a.Deadcode(copyOf(t), "mod"); err != nil {
				t.Fatalf("Deadcode: %v", err)
			}

			data, err := os.ReadFile(log)
			if err != nil || strings.Contains(string(data), "LABDRIAN_PROBE_LEAK") {
				t.Errorf("the tool read its log with %v and saw the variable of the process: %v, want a start under none of them", err, strings.Contains(string(data), "LABDRIAN_PROBE_LEAK"))
			}
		})
	}
}

func TestExecRunnerGivenNoEnvironmentDoesNotInheritTheOneOfTheProcess(t *testing.T) {
	for name, env := range map[string][]string{"a nil one": nil, "an empty one": {}} {
		t.Run(name, func(t *testing.T) {
			log := probeTool(t, "probe")
			t.Setenv("LABDRIAN_PROBE_LEAK", "leaked")

			_, _, err := analyzers.ExecRunner{}.Output(context.Background(), t.TempDir(), env, "probe")

			data, readErr := os.ReadFile(log)
			if err != nil || readErr != nil || strings.Contains(string(data), "LABDRIAN_PROBE_LEAK") {
				t.Errorf("Output = %v, log read %v, variable of the process seen: %v, want a start under none of them", err, readErr, strings.Contains(string(data), "LABDRIAN_PROBE_LEAK"))
			}
		})
	}
}

const untidyDiff = "diff current/go.mod tidy/go.mod\n--- current/go.mod\n+++ tidy/go.mod\n@@ -1,4 +1,3 @@\n module example.com/u\n-require github.com/x/y v1.2.3\n"

func TestTidyRanWhenItExitsZeroOrPrintsADiff(t *testing.T) {
	log := bin(t, map[string]stub{"go": {}})
	root := copyOf(t)

	got, err := tools(t).Tidy(root, "mod")

	if err != nil || len(got) != 0 {
		t.Errorf("Tidy = %q, %v, want none, nil for a tidy module", got, err)
	}
	log = bin(t, map[string]stub{"go": {exit: 1, stdout: untidyDiff}})
	got, err = tools(t).Tidy(root, "mod")

	want := []string{"go.mod: require github.com/x/y v1.2.3"}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("Tidy = %q, %v, want %q, nil: a non-zero exit after a diff is a run", got, err, want)
	}
	if ran := calls(t, log); !reflect.DeepEqual(ran, []string{filepath.Join(root, "mod") + "|mod tidy -diff"}) {
		t.Errorf("go ran as %q, want `go mod tidy -diff` in the module of the copy", ran)
	}
}

func TestTidyDidNotRunWhenItFailsWithoutADiff(t *testing.T) {
	root := copyOf(t)
	for name, tc := range map[string]struct {
		tools map[string]stub
		a     func(t *testing.T) analyzers.Tools
	}{
		"not found": {map[string]stub{}, tools},
		"failed":    {map[string]stub{"go": {exit: 1, stderr: "go: example.com/x@v1.0.0: missing go.sum entry"}}, tools},
		"the deadline": {map[string]stub{"go": {sleep: true}}, func(*testing.T) analyzers.Tools {
			return analyzers.New(analyzers.ExecRunner{}, analyzers.Options{Environ: os.Environ(), Timeout: 200 * time.Millisecond})
		}},
	} {
		t.Run(name, func(t *testing.T) {
			bin(t, tc.tools)

			_, err := tc.a(t).Tidy(root, "mod")

			if !errors.Is(err, app.ErrToolDidNotRun) || !strings.Contains(err.Error(), "go mod tidy") {
				t.Errorf("error = %v, want ErrToolDidNotRun naming go mod tidy", err)
			}
		})
	}
}

func TestTidyStartsNothingForAModuleTheCopyLacks(t *testing.T) {
	log := bin(t, map[string]stub{"go": {exit: 1, stdout: untidyDiff}})

	got, err := tools(t).Tidy(t.TempDir(), "mod")

	if err != nil || len(got) != 0 || len(calls(t, log)) != 0 {
		t.Errorf("Tidy = %q, %v, calls = %q, want none and no start: the module is new at the other side", got, err, calls(t, log))
	}
}
