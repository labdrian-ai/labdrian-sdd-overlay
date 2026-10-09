package execrunner

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fakeBinary writes an executable shell script called name into a fresh directory, names that
// directory as the PATH of the test, and returns the script's path. The scripts are POSIX shell.
func fakeBinary(t *testing.T, name, script string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake binaries are shell scripts")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	return path
}

func TestLookPathFindsTheProgramOnThePath(t *testing.T) {
	want := fakeBinary(t, "pi", "exit 0")
	got, err := New().LookPath("pi")
	if err != nil {
		t.Fatalf("LookPath: %v", err)
	}
	if got != want {
		t.Errorf("LookPath = %q, want %q", got, want)
	}
}

func TestLookPathReportsAMissingProgram(t *testing.T) {
	if _, err := New().LookPath("pi"); err == nil {
		t.Fatal("LookPath found a pi on an empty PATH")
	}
}

func TestRunReturnsTheCombinedOutput(t *testing.T) {
	bin := fakeBinary(t, "pi", `printf 'out\n'; printf 'err\n' >&2`)
	out, err := New().Run(context.Background(), bin)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if string(out) != "out\nerr\n" {
		t.Errorf("output = %q, want both streams", out)
	}
}

// The arguments reach the program one by one and as they are: nothing is interpreted by a shell.
func TestRunPassesTheArgumentsUninterpreted(t *testing.T) {
	bin := fakeBinary(t, "pi", `for a in "$@"; do printf '[%s]' "$a"; done`)
	hostile := `dir; $(touch injected); ` + "`id`" + ` \`
	out, err := New().Run(context.Background(), bin, "install", hostile)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if want := "[install][" + hostile + "]"; string(out) != want {
		t.Errorf("the program saw %q, want %q", out, want)
	}
}

// A failing program is an error that still carries what it printed, so the adapter can tell the
// person why.
func TestRunKeepsTheOutputOfAFailingProgram(t *testing.T) {
	bin := fakeBinary(t, "pi", `printf 'no such package\n'; exit 3`)
	out, err := New().Run(context.Background(), bin, "install", "x")
	if err == nil {
		t.Fatal("Run reported success for a program that exited 3")
	}
	if strings.TrimSpace(string(out)) != "no such package" {
		t.Errorf("output = %q, want what the program printed", out)
	}
	if !strings.Contains(err.Error(), "exit status 3") {
		t.Errorf("error = %q, want it to name the exit status", err)
	}
}

func TestRunReportsAProgramThatCannotStart(t *testing.T) {
	if _, err := New().Run(context.Background(), filepath.Join(t.TempDir(), "nothing-here")); err == nil {
		t.Fatal("Run reported success for a program that does not exist")
	}
}

// The deadline stops a program that never ends, and the error says it was the deadline.
func TestRunStopsAProgramAtItsDeadline(t *testing.T) {
	bin := fakeBinary(t, "pi", `exec /bin/sleep 30`)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	started := time.Now()
	_, err := New().Run(ctx, bin, "install", "x")
	if err == nil {
		t.Fatal("Run reported success for a program stopped at its deadline")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error = %v, want it to be the deadline", err)
	}
	if elapsed := time.Since(started); elapsed > 10*time.Second {
		t.Errorf("Run took %v to give up on a 200ms deadline", elapsed)
	}
}

// A deadline that has passed before the program starts starts nothing.
func TestRunStartsNothingAfterTheDeadline(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "ran")
	bin := fakeBinary(t, "pi", `: > '`+marker+`'`)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := New().Run(ctx, bin); err == nil {
		t.Fatal("Run reported success on a cancelled context")
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("the program ran although its context was already over")
	}
}

// A grandchild that keeps the output pipes open must not hold Run past its grace period: the
// shell below is stopped at 200ms, but the sleep it started still holds the pipes for thirty
// seconds, and Run gives up on them after killGrace instead of waiting for it. The bound is the
// grace period plus a margin for a loaded machine, far under the sleep, so a Run that waited for
// the pipes (no WaitDelay) is caught, and one that waited past the grace is too. The sleep is
// stopped when the test ends, so no process outlives it.
func TestRunDoesNotWaitForAGrandchildHoldingThePipes(t *testing.T) {
	bin := fakeBinary(t, "pi", lingeringGrandchild(t))
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	started := time.Now()
	_, err := New().Run(ctx, bin)
	if err == nil {
		t.Fatal("Run reported success for a program stopped at its deadline")
	}
	if elapsed, bound := time.Since(started), killGrace+5*time.Second; elapsed > bound {
		t.Errorf("Run took %v, want it back within %v: it waited for the grandchild that kept the pipes open", elapsed, bound)
	}
}

// lingeringCommand is the command line of the grandchild the fake programs below leave holding
// the output pipes. The duration is unlike any other the tests use, so stopRecordedProcess can
// tell it from an unrelated process that reused its id.
const lingeringCommand = "/bin/sleep 29.5"

// processLister reads the command line of a process by its id. It is named by path because the PATH
// of these tests is empty (main_test.go).
const processLister = "/bin/ps"

// lingeringGrandchild returns the body of a fake program that starts a grandchild holding the
// output pipes for about thirty seconds and then waits for it, and registers the cleanup that
// stops the grandchild when the test ends. The grandchild writes its own id before it becomes the
// sleep, so a program stopped before it started leaves no sleep behind and no id to follow.
func lingeringGrandchild(t *testing.T) string {
	t.Helper()
	pidFile := filepath.Join(t.TempDir(), "sleep.pid")
	t.Cleanup(func() { stopRecordedProcess(t, pidFile) })
	return `/bin/sh -c 'echo $$ > "$1"; exec ` + lingeringCommand + `' sh '` + pidFile + `' &
wait`
}

// stopRecordedProcess kills the process whose id a script wrote to pidFile, if it is still the
// grandchild the script started. The id is followed only when the command line of the process is
// lingeringCommand: an id that has been reused by another process is left alone, and so is one
// whose command line cannot be read.
func stopRecordedProcess(t *testing.T, pidFile string) {
	t.Helper()
	raw, err := os.ReadFile(pidFile)
	if err != nil {
		return
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Errorf("pid file %s holds %q", pidFile, raw)
		return
	}
	if commandOf(pid) != lingeringCommand {
		return
	}
	if proc, err := os.FindProcess(pid); err == nil {
		_ = proc.Kill()
	}
}

// commandOf is the command line of the process pid, or "" when there is none or it cannot be read.
// A process that has been killed but not yet waited for shows as defunct, not as its command.
func commandOf(pid int) string {
	out, err := exec.Command(processLister, "-p", strconv.Itoa(pid), "-o", "args=").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// A process that is not the grandchild of a test is never killed through a recorded id, whatever
// the id says: here the id is that of a live sleep with another command line.
func TestStopRecordedProcessLeavesAnotherProcessAlone(t *testing.T) {
	other := exec.Command("/bin/sleep", "28.5")
	if err := other.Start(); err != nil {
		t.Skipf("cannot start a sleep: %v", err)
	}
	t.Cleanup(func() { _ = other.Process.Kill(); _ = other.Wait() })
	waitForCommand(t, other.Process.Pid, "/bin/sleep 28.5") // the lister works, so a pass is not vacuous
	pidFile := filepath.Join(t.TempDir(), "sleep.pid")
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(other.Process.Pid)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	stopRecordedProcess(t, pidFile)

	if got := commandOf(other.Process.Pid); got != "/bin/sleep 28.5" {
		t.Errorf("the process of another command line was stopped: it reads %q", got)
	}
}

// The grandchild of a test, found by its command line, is killed.
func TestStopRecordedProcessKillsTheGrandchildItRecorded(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "sleep.pid")
	script := filepath.Join(dir, "start.sh")
	body := "#!/bin/sh\necho $$ > \"$1\"\nexec " + lingeringCommand + "\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	child := exec.Command(script, pidFile)
	if err := child.Start(); err != nil {
		t.Skipf("cannot start the grandchild: %v", err)
	}
	t.Cleanup(func() { _ = child.Process.Kill(); _ = child.Wait() })
	deadline := time.Now().Add(5 * time.Second)
	for {
		if raw, err := os.ReadFile(pidFile); err == nil && len(raw) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the grandchild did not record its id")
		}
		time.Sleep(10 * time.Millisecond)
	}
	waitForCommand(t, child.Process.Pid, lingeringCommand)

	stopRecordedProcess(t, pidFile)

	done := make(chan error, 1)
	go func() { done <- child.Wait() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Error("the recorded grandchild was not stopped")
	}
}

// waitForCommand waits until the process pid has become the command line want: the script
// replaces itself with the sleep a moment after it wrote its id.
func waitForCommand(t *testing.T, pid int, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if commandOf(pid) == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("process %d never became %q", pid, want)
}

func TestOutputReturnsTheTwoStreamsApart(t *testing.T) {
	bin := fakeBinary(t, "git", `printf 'out'; printf 'err' >&2`)
	stdout, stderr, err := New().Output(context.Background(), nil, bin)
	if err != nil {
		t.Fatalf("Output: %v", err)
	}
	if string(stdout) != "out" || string(stderr) != "err" {
		t.Errorf("stdout=%q stderr=%q, want them apart: out and err", stdout, stderr)
	}
}

func TestOutputKeepsWhatAFailingProgramPrinted(t *testing.T) {
	bin := fakeBinary(t, "git", `printf 'partial'; printf 'why' >&2; exit 3`)
	stdout, stderr, err := New().Output(context.Background(), nil, bin)
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 3 {
		t.Fatalf("err = %v, want an *exec.ExitError with code 3", err)
	}
	if string(stdout) != "partial" || string(stderr) != "why" {
		t.Errorf("stdout=%q stderr=%q, want what the program printed before it failed", stdout, stderr)
	}
}

// The environment given is the whole environment of the program: nothing of the process leaks
// into it, which is what lets a caller decide which variables a program sees.
func TestOutputGivesTheProgramExactlyTheEnvironmentItIsHanded(t *testing.T) {
	bin := fakeBinary(t, "git", `printf '%s|%s|%s' "${ASKED-unset}" "${LEAK-unset}" "${HOME-unset}"`)
	t.Setenv("LEAK", "from the process")
	t.Setenv("HOME", "/leaked-home")

	stdout, _, err := New().Output(context.Background(), []string{"ASKED=yes"}, bin)
	if err != nil {
		t.Fatalf("Output: %v", err)
	}
	if string(stdout) != "yes|unset|unset" {
		t.Errorf("the program saw %q, want only the variable it was handed", stdout)
	}
}

// With no environment given, the program gets the one of the process, as exec.Command does.
func TestOutputWithoutAnEnvironmentInheritsTheProcessOne(t *testing.T) {
	bin := fakeBinary(t, "git", `printf '%s' "${INHERITED-unset}"`)
	t.Setenv("INHERITED", "yes")

	stdout, _, err := New().Output(context.Background(), nil, bin)
	if err != nil {
		t.Fatalf("Output: %v", err)
	}
	if string(stdout) != "yes" {
		t.Errorf("the program saw %q, want the environment of the process", stdout)
	}
}

func TestOutputStopsAtTheDeadlineAndSaysWhy(t *testing.T) {
	bin := fakeBinary(t, "git", `exec /bin/sleep 30`)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	started := time.Now()
	_, _, err := New().Output(ctx, nil, bin)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want it to wrap context.DeadlineExceeded", err)
	}
	if elapsed := time.Since(started); elapsed > killGrace+5*time.Second {
		t.Errorf("Output took %v: it did not stop the program at its deadline", elapsed)
	}
}

// Output gives up on a grandchild that holds the pipes after the deadline, like Run.
func TestOutputDoesNotWaitForAGrandchildHoldingThePipes(t *testing.T) {
	bin := fakeBinary(t, "git", lingeringGrandchild(t))
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	started := time.Now()
	if _, _, err := New().Output(ctx, nil, bin); err == nil {
		t.Fatal("Output reported success for a program stopped at its deadline")
	}
	if elapsed, bound := time.Since(started), killGrace+5*time.Second; elapsed > bound {
		t.Errorf("Output took %v, want it back within %v", elapsed, bound)
	}
}

// ---- a bound on what Output holds in memory ----

func TestOutputStopsAProgramThatPrintsMoreThanTheBound(t *testing.T) {
	bin := fakeBinary(t, "git", `i=0; while [ $i -lt 2000 ]; do printf '0123456789'; i=$((i+1)); done`)

	// 1005 is not a multiple of the ten bytes a write carries, so an exact cut falls inside one.
	stdout, _, err := New().WithMaxOutput(1005).Output(context.Background(), nil, bin)
	if !errors.Is(err, ErrOutputTooLarge) {
		t.Fatalf("err = %v, want ErrOutputTooLarge", err)
	}
	want := strings.Repeat("0123456789", 101)[:1005]
	if string(stdout) != want {
		t.Errorf("held %d bytes, want exactly the first 1005 of the output", len(stdout))
	}
}

func TestOutputBoundsStandardErrorToo(t *testing.T) {
	bin := fakeBinary(t, "git", `i=0; while [ $i -lt 2000 ]; do printf 'xxxxxxxxxx' >&2; i=$((i+1)); done`)
	_, stderr, err := New().WithMaxOutput(1005).Output(context.Background(), nil, bin)
	if !errors.Is(err, ErrOutputTooLarge) || string(stderr) != strings.Repeat("x", 1005) {
		t.Fatalf("err = %v, held %d bytes of stderr, want ErrOutputTooLarge and exactly 1005", err, len(stderr))
	}
}

func TestOutputUnderTheBoundAndWithoutOneIsUnchanged(t *testing.T) {
	bin := fakeBinary(t, "git", `printf 'twelve bytes'`)
	for name, runner := range map[string]Runner{"under": New().WithMaxOutput(12), "none": New()} {
		stdout, _, err := runner.Output(context.Background(), nil, bin)
		if err != nil || string(stdout) != "twelve bytes" {
			t.Errorf("%s: stdout=%q err=%v, want the output whole", name, stdout, err)
		}
	}
}

// A program that goes on after its output is refused is stopped, not waited for.
func TestOutputStopsAProgramThatKeepsRunningAfterTheBound(t *testing.T) {
	bin := fakeBinary(t, "git", `trap '' PIPE
i=0; while [ $i -lt 600 ]; do printf xxxxxxxxxx; i=$((i+1)); done
exec /bin/sleep 30`)
	started := time.Now()
	_, _, err := New().WithMaxOutput(1000).Output(context.Background(), nil, bin)
	if !errors.Is(err, ErrOutputTooLarge) {
		t.Fatalf("err = %v, want ErrOutputTooLarge", err)
	}
	if elapsed, bound := time.Since(started), killGrace+5*time.Second; elapsed > bound {
		t.Errorf("Output took %v, want it back within %v: the program was waited for", elapsed, bound)
	}
}

func TestMaxOutputReportsTheBound(t *testing.T) {
	if got := New().MaxOutput(); got != 0 {
		t.Errorf("New().MaxOutput() = %d, want 0 (none)", got)
	}
	if got := New().WithMaxOutput(42).MaxOutput(); got != 42 {
		t.Errorf("MaxOutput() = %d, want 42", got)
	}
}
