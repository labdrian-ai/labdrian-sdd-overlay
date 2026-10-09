package execrunner

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The two sleeps the tests below start, by the seconds they sleep. Each duration is unlike any
// other the tests use, because it is how stopRecordedProcess recognizes the process it started: it
// reads the command line of an id and kills it only when the line is exactly the one it expects,
// so an id that was reused by an unrelated process is left alone. Both outlast every bound the
// tests wait for (killGrace and a few seconds), so a Run that waited for the grandchild is caught,
// and both end by themselves in under half a minute, so a process nobody stopped is bounded.
const (
	lingeringSeconds = "29.5" // the grandchild of the fake programs, which holds the output pipes
	strangerSeconds  = "28.5" // a process of another command line, which must never be stopped
)

const (
	// lingeringCommand is the command line of the grandchild the fake programs below leave holding
	// the output pipes.
	lingeringCommand = "/bin/sleep " + lingeringSeconds
	// strangerCommand is the command line of the process that stands for an unrelated one.
	strangerCommand = "/bin/sleep " + strangerSeconds
)

// processLister reads the command line of a process by its id. It is named by path because the PATH
// of these tests is empty (main_test.go).
const processLister = "/bin/ps"

// reporter is what stopRecordedProcessWith needs of the test that calls it: *testing.T satisfies
// it, and so does the recorder of the tests that watch it report.
type reporter interface {
	Helper()
	Errorf(format string, args ...any)
}

// requireProcessLister skips the test when the machine has no process lister. Without one a
// grandchild could not be found again to be stopped, and it would be left running for the rest of
// its sleep with nothing to say so: a visible skip is the lesser evil.
func requireProcessLister(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(processLister); err != nil {
		t.Skipf("no %s to find the lingering grandchild by, so it could not be stopped: %v", processLister, err)
	}
}

// lingeringGrandchild returns the body of a fake program that starts a grandchild holding the
// output pipes for about thirty seconds and then waits for it, and registers the cleanup that
// stops the grandchild when the test ends. The grandchild writes its own id before it becomes the
// sleep, so a program stopped before it started leaves no sleep behind and no id to follow. The
// test is skipped when there is no process lister to stop the grandchild with.
func lingeringGrandchild(t *testing.T) string {
	t.Helper()
	requireProcessLister(t)
	pidFile := filepath.Join(t.TempDir(), "sleep.pid")
	t.Cleanup(func() { stopRecordedProcess(t, pidFile) })
	return `/bin/sh -c 'echo $$ > "$1"; exec ` + lingeringCommand + `' sh '` + pidFile + `' &
wait`
}

// stopRecordedProcess kills the process whose id a script wrote to pidFile, if it is still the
// grandchild the script started. The id is followed only when the command line of the process is
// lingeringCommand: an id that has been reused by another process is left alone.
func stopRecordedProcess(t *testing.T, pidFile string) {
	t.Helper()
	stopRecordedProcessWith(t, processLister, pidFile)
}

// stopRecordedProcessWith is stopRecordedProcess with the lister named. A lister that cannot be
// run fails the test: the id could not be checked, so the process is neither stopped nor known to
// be gone, and saying nothing would leave it running unseen.
//
// The command line is read and the process is killed in two steps, and an id could be handed to
// another process between them. The window is the time between two system calls, and it opens
// only if the grandchild has died and been reaped in it, the id space has come round to that id,
// and the new process runs exactly lingeringCommand, a line no program but the fake programs of
// these tests has. Closing it needs a handle on the process (a pidfd), which the systems these
// tests run on do not all have, so the exact match of the command line is the guard and the
// bound stays as it is.
func stopRecordedProcessWith(t reporter, lister, pidFile string) {
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
	command, err := commandOf(lister, pid)
	if err != nil {
		t.Errorf("process %d (recorded in %s) was not stopped: %v", pid, pidFile, err)
		return
	}
	if command != lingeringCommand {
		return
	}
	if proc, err := os.FindProcess(pid); err == nil {
		_ = proc.Kill()
	}
}

// commandOf is the command line of the process pid, or "" when there is no such process: ps says so
// by exiting 1 with nothing printed. A process that has been killed but not yet waited for shows as
// defunct, not as its command. An error means the lister could not say: it could not be run, or it
// ran and failed for another reason (no /proc, no permission), which says nothing about the process.
func commandOf(lister string, pid int) (string, error) {
	out, err := exec.Command(lister, "-p", strconv.Itoa(pid), "-o", "args=").Output()
	var exit *exec.ExitError
	switch {
	case errors.As(err, &exit) && exit.ExitCode() == 1 && len(out) == 0:
		return "", nil
	case errors.As(err, &exit):
		return "", fmt.Errorf("%s exited %d: %s", lister, exit.ExitCode(), strings.TrimSpace(string(exit.Stderr)))
	case err != nil:
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// waitForCommand waits until the process pid has become the command line want: the script
// replaces itself with the sleep a moment after it wrote its id.
func waitForCommand(t *testing.T, pid int, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		got, err := commandOf(processLister, pid)
		if err != nil {
			t.Fatalf("cannot read the command line of process %d: %v", pid, err)
		}
		if got == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("process %d never became %q", pid, want)
}

// A process that is not the grandchild of a test is never killed through a recorded id, whatever
// the id says: here the id is that of a live sleep with another command line.
func TestStopRecordedProcessLeavesAnotherProcessAlone(t *testing.T) {
	requireProcessLister(t)
	other := exec.Command("/bin/sleep", strangerSeconds)
	if err := other.Start(); err != nil {
		t.Skipf("cannot start a sleep: %v", err)
	}
	t.Cleanup(func() { _ = other.Process.Kill(); _ = other.Wait() })
	waitForCommand(t, other.Process.Pid, strangerCommand) // the lister works, so a pass is not vacuous
	pidFile := filepath.Join(t.TempDir(), "sleep.pid")
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(other.Process.Pid)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	stopRecordedProcess(t, pidFile)

	got, err := commandOf(processLister, other.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	if got != strangerCommand {
		t.Errorf("the process of another command line was stopped: it reads %q", got)
	}
}

// The grandchild of a test, found by its command line, is killed.
func TestStopRecordedProcessKillsTheGrandchildItRecorded(t *testing.T) {
	requireProcessLister(t)
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

// reports is the recorder of the tests below: it keeps what a test would have failed with.
type reports struct{ messages []string }

func (*reports) Helper() {}
func (r *reports) Errorf(format string, args ...any) {
	r.messages = append(r.messages, strings.TrimSpace(fmt.Sprintf(format, args...)))
}

// A lister that cannot be run is not "no such process": the recorded id is then neither stopped
// nor known to be gone, and the test must say so instead of leaving a sleep running unseen.
func TestStopRecordedProcessSaysWhenItCannotListProcesses(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "sleep.pid")
	if err := os.WriteFile(pidFile, []byte("1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := &reports{}

	stopRecordedProcessWith(r, filepath.Join(t.TempDir(), "no-such-ps"), pidFile)

	wantReported(t, r, "process 1 (recorded in "+pidFile+") was not stopped")
}

// wantReported fails the test unless r holds exactly one report and it names what the test would
// have failed with: the process, the file it was recorded in, and that it was not stopped.
func wantReported(t *testing.T, r *reports, parts ...string) {
	t.Helper()
	if len(r.messages) != 1 {
		t.Fatalf("reports = %q, want exactly one", r.messages)
	}
	for _, part := range parts {
		if !strings.Contains(r.messages[0], part) {
			t.Errorf("report %q does not say %q", r.messages[0], part)
		}
	}
}

// A lister that runs and fails for a reason other than "no such process" (ps exits 1 for that one
// and prints nothing) has not said the process is gone: it must be reported, or the sleep is left
// running unseen.
func TestStopRecordedProcessSaysWhenTheListerFailsForAnotherReason(t *testing.T) {
	dir := t.TempDir()
	lister := filepath.Join(dir, "ps")
	if err := os.WriteFile(lister, []byte("#!/bin/sh\necho 'ps: cannot read /proc' >&2\nexit 2\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	pidFile := filepath.Join(dir, "sleep.pid")
	if err := os.WriteFile(pidFile, []byte("1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := &reports{}

	stopRecordedProcessWith(r, lister, pidFile)

	wantReported(t, r, "process 1 (recorded in "+pidFile+") was not stopped", "ps: cannot read /proc")
}

// A recorded id with no process behind it is not an error: the grandchild is simply gone.
func TestStopRecordedProcessIsQuietAboutAnIdWithNoProcess(t *testing.T) {
	requireProcessLister(t)
	pid := idOfAProcessThatHasEnded(t)
	pidFile := filepath.Join(t.TempDir(), "sleep.pid")
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(pid)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := &reports{}

	stopRecordedProcessWith(r, processLister, pidFile)

	if len(r.messages) != 0 {
		t.Fatalf("reports = %q, want none for an id with no process", r.messages)
	}
}

// idOfAProcessThatHasEnded starts a process, waits for it, and returns its id: an id the system
// has just given back, with no fixed number to assume anything about the largest id of a machine.
// The lister must say nothing runs under it, so a pass of the test is not vacuous; an id that was
// handed to another process in the moment between skips the test, since it says nothing then.
func idOfAProcessThatHasEnded(t *testing.T) int {
	t.Helper()
	ended := exec.Command("/bin/sh", "-c", "exit 0")
	if err := ended.Run(); err != nil {
		t.Skipf("cannot run a shell to get an id that ends: %v", err)
	}
	pid := ended.Process.Pid
	command, err := commandOf(processLister, pid)
	if err != nil {
		t.Fatalf("cannot read the command line of process %d: %v", pid, err)
	}
	if command != "" {
		t.Skipf("the id %d of the ended process was taken again by %q", pid, command)
	}
	return pid
}
