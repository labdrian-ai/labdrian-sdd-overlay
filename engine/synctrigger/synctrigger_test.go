package synctrigger

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The trigger is tested here with no process and no clock of the machine: a Runner that records
// what it was asked and answers what the test says, and a clock that reads the same moments every
// time. What starts real programs is engine/synctrigger/osproc, and its tests, and the ones that
// run the two together, are there.

// epoch is the moment the clock of these tests starts at, and step what each reading of it adds.
var epoch = time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)

const step = 250 * time.Millisecond

// stepClock reads epoch the first time, and a step later each time after.
func stepClock() func() time.Time {
	reads := 0
	return func() time.Time {
		at := epoch.Add(time.Duration(reads) * step)
		reads++
		return at
	}
}

// stamp is how the log writes the moment of the n-th reading of a stepClock (counting from 0),
// to the second, which is how it writes it.
func stamp(reads int) string {
	return epoch.Add(time.Duration(reads) * step).UTC().Format(time.RFC3339)
}

type detachCall struct {
	program string
	args    []string
	log     string
}

type runCall struct {
	program string
	args    []string
	dir     string
	timeout time.Duration
}

// fakeRunner answers what it was told to and records what it was asked.
type fakeRunner struct {
	executable    string
	executableErr error

	detachErr error
	// childWrites is what the detached child prints: Detach writes it to the log file it was
	// handed, as the child would through the descriptor it inherits.
	childWrites string
	detached    []detachCall

	result Result
	runErr error
	// prints is what the program prints on standard error while it runs.
	prints string
	ran    []runCall
}

func (f *fakeRunner) Executable() (string, error) { return f.executable, f.executableErr }

func (f *fakeRunner) Detach(program string, args []string, log *os.File) error {
	f.detached = append(f.detached, detachCall{program: program, args: args, log: log.Name()})
	if f.childWrites != "" {
		if _, err := io.WriteString(log, f.childWrites); err != nil {
			return err
		}
	}
	return f.detachErr
}

func (f *fakeRunner) Run(program string, args []string, dir string, timeout time.Duration, stderr io.Writer) (Result, error) {
	f.ran = append(f.ran, runCall{program: program, args: args, dir: dir, timeout: timeout})
	if f.prints != "" {
		io.WriteString(stderr, f.prints)
	}
	return f.result, f.runErr
}

// logPath is where the log of a state directory is.
func logPath(stateDir string) string { return filepath.Join(stateDir, "logs", "sync-trigger.log") }

// logText is the whole log of a state directory, "" if there is none.
func logText(t *testing.T, stateDir string) string {
	t.Helper()
	data, err := os.ReadFile(logPath(stateDir))
	if err != nil {
		return ""
	}
	return string(data)
}

// executableBinary writes a file the sync may be run from, and returns its path.
func executableBinary(t *testing.T, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "longterm-mem")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
	return path
}

// child is the Options of a RunChild whose binary can be run.
func child(t *testing.T, runner Runner) (Options, string) {
	t.Helper()
	stateDir := t.TempDir()
	return Options{Event: "session-end", Cwd: "/project", StateDir: stateDir, Binary: executableBinary(t, 0o755), Now: stepClock(), Runner: runner}, stateDir
}

// ---- RunChild ---------------------------------------------------------------------------------

// The sync is run from the project, with the one argument longterm-mem takes, under the time
// the options give, and the default when they give none.
func TestRunChild_RunsTheSyncInTheProjectUnderTheTimeItIsGiven(t *testing.T) {
	for _, tc := range []struct {
		name    string
		timeout time.Duration
		want    time.Duration
	}{
		{"the default when none is given", 0, 60 * time.Second},
		{"the default when it is negative", -time.Second, 60 * time.Second},
		{"the one given", 3 * time.Second, 3 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner := &fakeRunner{}
			o, _ := child(t, runner)
			o.Timeout = tc.timeout

			RunChild(o)

			want := []runCall{{program: o.Binary, args: []string{"sync"}, dir: "/project", timeout: tc.want}}
			if len(runner.ran) != 1 || runner.ran[0].program != want[0].program || strings.Join(runner.ran[0].args, " ") != "sync" ||
				runner.ran[0].dir != want[0].dir || runner.ran[0].timeout != want[0].timeout {
				t.Errorf("the runner was asked %+v, want %+v", runner.ran, want)
			}
		})
	}
}

func TestRunChild_WithoutABinaryRunsTheOneUnderTheStateDirectory(t *testing.T) {
	runner := &fakeRunner{}
	stateDir := t.TempDir()
	bin := filepath.Join(stateDir, "bin", "longterm-mem")
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	got := RunChild(Options{Event: "archive", Cwd: "/project", StateDir: stateDir, Now: stepClock(), Runner: runner})

	if got.Kind != "ok" || len(runner.ran) != 1 || runner.ran[0].program != bin {
		t.Errorf("RunChild = %+v, runner asked %+v, want ok from %s", got, runner.ran, bin)
	}
}

// Every outcome is one line of the log, dated by the clock when the sync ended, with the time the
// sync took (a step: the clock was read when the run began and when it ended) and the exit code.
func TestRunChild_WritesOneLineForEveryOutcomeWithTheClockItWasGiven(t *testing.T) {
	notProject := "longterm-mem: sync: --project is required: it could not be resolved from the working directory: not a git repo\n"
	for _, tc := range []struct {
		name   string
		runner *fakeRunner
		want   Outcome
	}{
		{"exit 0", &fakeRunner{}, Outcome{Kind: "ok", Exit: 0, Dur: step}},
		{"exit 2 with the sentence for a directory that is no project", &fakeRunner{result: Result{Exit: 2}, prints: notProject}, Outcome{Kind: "skip:no-project", Exit: 2, Dur: step}},
		{"exit 2 with any other words", &fakeRunner{result: Result{Exit: 2}, prints: "longterm-mem: sync: unknown flag --bogus\n"}, Outcome{Kind: "error:usage", Exit: 2, Dur: step}},
		{"exit 2 with nothing said", &fakeRunner{result: Result{Exit: 2}}, Outcome{Kind: "error:usage", Exit: 2, Dur: step}},
		{"exit 3, no vault", &fakeRunner{result: Result{Exit: 3}}, Outcome{Kind: "skip:no-vault", Exit: 3, Dur: step}},
		{"exit 4, engram unavailable", &fakeRunner{result: Result{Exit: 4}}, Outcome{Kind: "failure:engram-unavailable", Exit: 4, Dur: step}},
		{"exit 5, the vault subprocess failed", &fakeRunner{result: Result{Exit: 5}}, Outcome{Kind: "failure:vault-subprocess", Exit: 5, Dur: step}},
		{"exit 1, any other", &fakeRunner{result: Result{Exit: 1}}, Outcome{Kind: "failure:exit-1", Exit: 1, Dur: step}},
		{"exit 137", &fakeRunner{result: Result{Exit: 137}}, Outcome{Kind: "failure:exit-137", Exit: 137, Dur: step}},
		{"ended by a signal", &fakeRunner{result: Result{Exit: -1}}, Outcome{Kind: "failure:exit--1", Exit: -1, Dur: step}},
		{"killed at the timeout", &fakeRunner{result: Result{TimedOut: true}}, Outcome{Kind: "timeout", Exit: 0, Dur: step}},
		{"killed at the timeout, whatever it said while it was stopped", &fakeRunner{result: Result{TimedOut: true, Exit: 9}, runErr: errors.New("signal: killed"), prints: notProject}, Outcome{Kind: "timeout", Exit: 0, Dur: step}},
		{"could not be run", &fakeRunner{runErr: errors.New("fork/exec: resource temporarily unavailable")}, Outcome{Kind: "error:spawn", Exit: 0, Dur: step}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o, stateDir := child(t, tc.runner)

			got := RunChild(o)

			if got != tc.want {
				t.Errorf("RunChild = %+v, want %+v", got, tc.want)
			}
			wantLine := stamp(1) + " event=session-end cwd=/project outcome=" + tc.want.Kind + " exit=" + strconv.Itoa(tc.want.Exit) + " duration=250ms\n"
			if text := logText(t, stateDir); !strings.HasSuffix(text, wantLine) || strings.Count(text, "outcome=") != 1 {
				t.Errorf("log = %q, want its one outcome line to be %q", text, wantLine)
			}
		})
	}
}

// What the program says on standard error is in the log before the line that summarizes the run,
// and it is also what the exit code 2 is read with.
func TestRunChild_PutsWhatTheProgramSaysInTheLogBeforeTheOutcomeLine(t *testing.T) {
	runner := &fakeRunner{result: Result{Exit: 3}, prints: "vault not configured\n"}
	o, stateDir := child(t, runner)

	RunChild(o)

	want := "vault not configured\n" + stamp(1) + " event=session-end cwd=/project outcome=skip:no-vault exit=3 duration=250ms\n"
	if got := logText(t, stateDir); got != want {
		t.Errorf("log = %q, want %q", got, want)
	}
}

func TestRunChild_SkipsAMissingBinaryWithoutRunningAnything(t *testing.T) {
	runner := &fakeRunner{}
	o, stateDir := child(t, runner)
	o.Binary = filepath.Join(stateDir, "bin", "does-not-exist")

	got := RunChild(o)

	if got.Kind != "skip:no-binary" || len(runner.ran) != 0 {
		t.Errorf("RunChild = %+v, runner asked %+v, want skip:no-binary and nothing run", got, runner.ran)
	}
	if want := stamp(1) + " event=session-end cwd=/project outcome=skip:no-binary exit=0 duration=250ms\n"; logText(t, stateDir) != want {
		t.Errorf("log = %q, want %q", logText(t, stateDir), want)
	}
}

func TestRunChild_TakesADirectoryForAMissingBinary(t *testing.T) {
	runner := &fakeRunner{}
	o, _ := child(t, runner)
	o.Binary = t.TempDir()

	if got := RunChild(o); got.Kind != "skip:no-binary" || len(runner.ran) != 0 {
		t.Errorf("RunChild = %+v, runner asked %+v, want skip:no-binary and nothing run", got, runner.ran)
	}
}

func TestRunChild_RefusesABinaryThatCannotBeExecuted(t *testing.T) {
	runner := &fakeRunner{}
	o, stateDir := child(t, runner)
	o.Binary = executableBinary(t, 0o644)

	got := RunChild(o)

	if got.Kind != "error:binary-not-executable" || len(runner.ran) != 0 {
		t.Errorf("RunChild = %+v, runner asked %+v, want error:binary-not-executable and nothing run", got, runner.ran)
	}
	if want := stamp(1) + " event=session-end cwd=/project outcome=error:binary-not-executable exit=0 duration=250ms\n"; logText(t, stateDir) != want {
		t.Errorf("log = %q, want %q", logText(t, stateDir), want)
	}
}

func TestRunChild_RefusesBadArgumentsWithoutRunningAnything(t *testing.T) {
	for _, tc := range []struct{ name, event, cwd string }{
		{"empty-cwd", "session-end", ""},
		{"relative-cwd", "session-end", "relative/path"},
		{"invalid-event", "bogus-event", "/tmp"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner := &fakeRunner{}
			o, _ := child(t, runner)
			o.Event, o.Cwd = tc.event, tc.cwd
			var stderr strings.Builder
			o.Stderr = &stderr

			got := RunChild(o)

			if got.Kind != "error:usage" || len(runner.ran) != 0 {
				t.Errorf("RunChild = %+v, runner asked %+v, want error:usage and nothing run", got, runner.ran)
			}
			if stderr.String() != usageLine(tc.event, tc.cwd) {
				t.Errorf("stderr = %q, want %q", stderr.String(), usageLine(tc.event, tc.cwd))
			}
		})
	}
}

func TestRunChild_WithoutALogStillRunsTheSyncAndReturnsItsOutcome(t *testing.T) {
	// A state directory that is a file: the log can be neither made nor opened.
	stateDir := filepath.Join(t.TempDir(), "a-file")
	if err := os.WriteFile(stateDir, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{result: Result{Exit: 4}}
	o, _ := child(t, runner)
	o.StateDir = stateDir

	got := RunChild(o)

	if want := (Outcome{Kind: "failure:engram-unavailable", Exit: 4, Dur: step}); got != want || len(runner.ran) != 1 {
		t.Errorf("RunChild = %+v, runner asked %d times, want %+v once", got, len(runner.ran), want)
	}
}

// A trigger with no clock dates its lines with the zero time, and with no Runner it says it could
// not start the program: neither fails its caller, and neither reads the machine.
func TestRunChild_WithNoClockAndNoRunnerIsSaidInTheLog(t *testing.T) {
	o, stateDir := child(t, nil)
	o.Now = nil

	got := RunChild(o)

	if want := (Outcome{Kind: "error:spawn"}); got != want {
		t.Errorf("RunChild = %+v, want %+v", got, want)
	}
	if want := "0001-01-01T00:00:00Z event=session-end cwd=/project outcome=error:spawn exit=0 duration=0s\n"; logText(t, stateDir) != want {
		t.Errorf("log = %q, want %q", logText(t, stateDir), want)
	}
}

// ---- Run --------------------------------------------------------------------------------------

func exampleChildArgv(event, cwd, stateDir string) []string {
	return []string{"a-child", "--event", event, "--cwd", cwd, "--state", stateDir}
}

// parent is the Options of a Run that can start its child.
func parent(t *testing.T, runner Runner) (Options, string, *strings.Builder) {
	t.Helper()
	var stderr strings.Builder
	stateDir := t.TempDir()
	return Options{Event: "archive", Cwd: "/project", StateDir: stateDir, Self: "/bin/self", ChildArgv: exampleChildArgv, Stderr: &stderr, Now: stepClock(), Runner: runner}, stateDir, &stderr
}

// The child is started as the caller's command line says, with the log as its output and its
// error, and Run says nothing and writes nothing of its own.
func TestRun_StartsTheChildWithTheArgumentsTheCallerGivesAndTheLogForItsOutput(t *testing.T) {
	runner := &fakeRunner{childWrites: "the child's line\n"}
	o, stateDir, stderr := parent(t, runner)
	var asked [][3]string
	o.ChildArgv = func(event, cwd, state string) []string {
		asked = append(asked, [3]string{event, cwd, state})
		return []string{"first", "--event=" + event, "a b", cwd, state}
	}

	if got := Run(o); got != 0 {
		t.Fatalf("Run() = %d, want 0", got)
	}

	if len(runner.detached) != 1 {
		t.Fatalf("the child was started %d times, want once", len(runner.detached))
	}
	d := runner.detached[0]
	if d.program != "/bin/self" || strings.Join(d.args, "\n") != "first\n--event=archive\na b\n/project\n"+stateDir {
		t.Errorf("the child was started as %s %q, want /bin/self with the arguments the caller gave", d.program, d.args)
	}
	if d.log != logPath(stateDir) {
		t.Errorf("the child's output goes to %s, want the log %s", d.log, logPath(stateDir))
	}
	if len(asked) != 1 || asked[0] != [3]string{"archive", "/project", stateDir} {
		t.Errorf("ChildArgv was asked %v, want once, for (archive, /project, %s)", asked, stateDir)
	}
	if got := logText(t, stateDir); got != "the child's line\n" {
		t.Errorf("log = %q, want only what the child wrote", got)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want nothing", stderr.String())
	}
}

// Without a Self, the path of the running program is the child.
func TestRun_StartsThePathOfTheRunningProgramWhenNoSelfIsGiven(t *testing.T) {
	runner := &fakeRunner{executable: "/usr/local/bin/engine"}
	o, _, _ := parent(t, runner)
	o.Self = ""

	Run(o)

	if len(runner.detached) != 1 || runner.detached[0].program != "/usr/local/bin/engine" {
		t.Errorf("started %+v, want the child to be /usr/local/bin/engine", runner.detached)
	}
}

func TestRun_SaysItCouldNotFindItselfOnStderrAndInTheLog(t *testing.T) {
	runner := &fakeRunner{executableErr: errors.New("no /proc")}
	o, stateDir, stderr := parent(t, runner)
	o.Self = ""

	if got := Run(o); got != 0 {
		t.Fatalf("Run() = %d, want 0", got)
	}

	if want := "sync-trigger: error:self: no /proc\n"; stderr.String() != want {
		t.Errorf("stderr = %q, want %q", stderr.String(), want)
	}
	if want := stamp(0) + " event=archive cwd=/project outcome=error:self exit=0 duration=0s\n"; logText(t, stateDir) != want {
		t.Errorf("log = %q, want %q", logText(t, stateDir), want)
	}
	if len(runner.detached) != 0 {
		t.Errorf("a child was started: %+v", runner.detached)
	}
}

// Without a command line there is no child to start, and no guessing one: the log and stderr say
// so, and nothing is started.
func TestRun_WithoutChildArgv_ErrorSpawnAndNothingStarted(t *testing.T) {
	runner := &fakeRunner{}
	o, stateDir, stderr := parent(t, runner)
	o.ChildArgv = nil

	if got := Run(o); got != 0 {
		t.Fatalf("Run() = %d, want 0", got)
	}

	if want := "sync-trigger: error:spawn: no command line was given for the child\n"; stderr.String() != want {
		t.Errorf("stderr = %q, want %q", stderr.String(), want)
	}
	if want := stamp(0) + " event=archive cwd=/project outcome=error:spawn exit=0 duration=0s\n"; logText(t, stateDir) != want {
		t.Errorf("log = %q, want %q", logText(t, stateDir), want)
	}
	if len(runner.detached) != 0 {
		t.Errorf("a child was started: %+v", runner.detached)
	}
}

func TestRun_SaysAChildThatCouldNotStartOnStderrAndInTheLog(t *testing.T) {
	runner := &fakeRunner{detachErr: errors.New("fork/exec /bin/self: permission denied")}
	o, stateDir, stderr := parent(t, runner)

	if got := Run(o); got != 0 {
		t.Fatalf("Run() = %d, want 0", got)
	}

	if want := "sync-trigger: error:spawn: fork/exec /bin/self: permission denied\n"; stderr.String() != want {
		t.Errorf("stderr = %q, want %q", stderr.String(), want)
	}
	if want := stamp(0) + " event=archive cwd=/project outcome=error:spawn exit=0 duration=0s\n"; logText(t, stateDir) != want {
		t.Errorf("log = %q, want %q", logText(t, stateDir), want)
	}
}

func TestRun_WithNoRunnerSaysSoLikeAnyOtherFailureToStart(t *testing.T) {
	for _, tc := range []struct{ name, self, kind, said string }{
		{"to start the child", "/bin/self", "error:spawn", "sync-trigger: error:spawn: no runner was given\n"},
		{"to find itself", "", "error:self", "sync-trigger: error:self: no runner was given\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o, stateDir, stderr := parent(t, nil)
			o.Self = tc.self

			if got := Run(o); got != 0 {
				t.Fatalf("Run() = %d, want 0", got)
			}

			if stderr.String() != tc.said {
				t.Errorf("stderr = %q, want %q", stderr.String(), tc.said)
			}
			if want := stamp(0) + " event=archive cwd=/project outcome=" + tc.kind + " exit=0 duration=0s\n"; logText(t, stateDir) != want {
				t.Errorf("log = %q, want %q", logText(t, stateDir), want)
			}
		})
	}
}

// A relative cwd (from a "${CLAUDE_PROJECT_DIR:-$PWD}" fallback whose $PWD was itself relative, or
// any other caller) is not a usage error: it is made absolute before the caller is asked for the
// command line, so the child is never handed a path that depends on its own working directory.
func TestRun_RelativeCwdIsMadeAbsoluteBeforeTheCallerIsAsked(t *testing.T) {
	projectDir := t.TempDir()
	// Run resolves a relative cwd against the process's own directory, so this test has to move
	// it. No test of this package runs in parallel, and the directory is put back when it ends.
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(projectDir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	t.Cleanup(func() { os.Chdir(oldWd) })
	resolved, err := os.Getwd() // the directory as the process names it, with its symlinks resolved
	if err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{}
	o, _, stderr := parent(t, runner)
	o.Cwd = "."
	var cwd string
	o.ChildArgv = func(_, c, _ string) []string {
		cwd = c
		return []string{"x", c}
	}

	Run(o)

	if cwd != resolved {
		t.Errorf("ChildArgv was given cwd %q, want the absolute %q", cwd, resolved)
	}
	if len(runner.detached) != 1 || runner.detached[0].args[1] != resolved {
		t.Errorf("the child was started with %+v, want the absolute cwd %q", runner.detached, resolved)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want a relative cwd resolved and not refused", stderr.String())
	}
}

func TestRun_UnwritableLogDir_ErrorLog(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	runner := &fakeRunner{}
	o, stateDir, stderr := parent(t, runner)
	logsDir := filepath.Join(stateDir, "logs")
	if err := os.MkdirAll(logsDir, 0o500); err != nil {
		t.Fatalf("mkdir logs: %v", err)
	}
	t.Cleanup(func() { os.Chmod(logsDir, 0o700) })

	if got := Run(o); got != 0 {
		t.Fatalf("Run() = %d, want 0", got)
	}

	if !strings.HasPrefix(stderr.String(), "sync-trigger: error:log: ") || !strings.HasSuffix(stderr.String(), "\n") {
		t.Errorf("stderr = %q, want one line that starts with `sync-trigger: error:log: `", stderr.String())
	}
	if len(runner.detached) != 0 {
		t.Errorf("a child was started without a log: %+v", runner.detached)
	}
}

// ---- the rest ---------------------------------------------------------------------------------

func TestAppendLogWritesTheMomentTheEventTheDirectoryTheOutcomeTheExitAndTheDurationRounded(t *testing.T) {
	var b strings.Builder

	appendLog(&b, time.Date(2026, 3, 4, 7, 6, 7, 900_000_000, time.FixedZone("x", 2*3600)), "archive", "/p", "failure:exit-7", 7, 1234567*time.Microsecond)

	if want := "2026-03-04T05:06:07Z event=archive cwd=/p outcome=failure:exit-7 exit=7 duration=1.235s\n"; b.String() != want {
		t.Errorf("appendLog wrote %q, want %q", b.String(), want)
	}
}

func TestNoRunnerFailsEveryCallWithTheSameWords(t *testing.T) {
	var r Runner = noRunner{}
	if _, err := r.Executable(); err == nil || err.Error() != "no runner was given" {
		t.Errorf("Executable error = %v", err)
	}
	if err := r.Detach("x", nil, nil); err == nil || err.Error() != "no runner was given" {
		t.Errorf("Detach error = %v", err)
	}
	if _, err := r.Run("x", nil, "", 0, nil); err == nil || err.Error() != "no runner was given" {
		t.Errorf("Run error = %v", err)
	}
}
