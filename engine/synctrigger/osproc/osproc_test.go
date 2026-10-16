package osproc_test

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/synctrigger"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/synctrigger/osproc"
)

// These tests start real programs: small shell scripts the test writes into a directory of its
// own. They are about what the adapter does with the machine (sessions, process groups, deadlines,
// descriptors); what the trigger makes of the answers is tested, with no process, in the package
// above.

// script writes an executable shell script and returns its path.
func script(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "program")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// waitForFile returns what the file holds once it holds something, and fails the test after 2 s.
func waitForFile(t *testing.T, path string) string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
			return string(data)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("%s was not written within 2s", path)
	return ""
}

// logFile makes the file a detached program writes to.
func logFile(t *testing.T) *os.File {
	t.Helper()
	f, err := os.OpenFile(filepath.Join(t.TempDir(), "log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

func TestExecutableIsThePathOfTheRunningProgram(t *testing.T) {
	want, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	got, err := osproc.Runner{}.Executable()
	if err != nil || got != want {
		t.Errorf("Executable() = (%q, %v), want %q", got, err, want)
	}
}

// ---- Detach -----------------------------------------------------------------------------------

// The program is started with exactly the arguments it is given, one by one, however it spells
// them: nothing is split or interpreted.
func TestDetachStartsTheProgramWithTheArgumentsItIsGiven(t *testing.T) {
	dir := t.TempDir()
	record := filepath.Join(dir, "arguments")
	program := script(t, `for a in "$@"; do printf '%s\n' "$a" >> "$0.out"; done; mv "$0.out" "`+record+`"`+"\n")

	if err := (osproc.Runner{}).Detach(program, []string{"one", "two words", "--flag=$HOME", ""}, logFile(t)); err != nil {
		t.Fatalf("Detach: %v", err)
	}

	if got, want := waitForFile(t, record), "one\ntwo words\n--flag=$HOME\n\n"; got != want {
		t.Errorf("the program was started with %q, want %q", got, want)
	}
}

// Detach does not wait: it returns while the program is still running, which here cannot end until
// the test lets it.
func TestDetachReturnsWhileTheProgramIsStillRunning(t *testing.T) {
	dir := t.TempDir()
	release, finished := filepath.Join(dir, "release"), filepath.Join(dir, "finished")
	program := script(t, `while [ ! -e "`+release+`" ]; do sleep 0.02; done; echo done > "`+finished+`"`+"\n")

	returned := make(chan error, 1)
	go func() { returned <- (osproc.Runner{}).Detach(program, nil, logFile(t)) }()
	select {
	case err := <-returned:
		if err != nil {
			t.Fatalf("Detach: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Detach did not return while the program was running: it waited for it")
	}

	if _, err := os.Stat(finished); err == nil {
		t.Fatal("the program had ended before the test let it")
	}
	if err := os.WriteFile(release, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	waitForFile(t, finished)
}

// The program runs in a session of its own, which is what lets it outlive the process group of the
// caller: it is the leader of a session that is not the caller's.
func TestDetachStartsTheProgramInASessionOfItsOwn(t *testing.T) {
	dir := t.TempDir()
	pidFile, release := filepath.Join(dir, "pid"), filepath.Join(dir, "release")
	program := script(t, `echo $$ > "`+pidFile+`"; while [ ! -e "`+release+`" ]; do sleep 0.02; done`+"\n")

	if err := (osproc.Runner{}).Detach(program, nil, logFile(t)); err != nil {
		t.Fatalf("Detach: %v", err)
	}
	defer os.WriteFile(release, nil, 0o644)

	pid, err := strconv.Atoi(strings.TrimSpace(waitForFile(t, pidFile)))
	if err != nil {
		t.Fatalf("pid: %v", err)
	}
	sid, mine := sessionOf(t, pid), sessionOf(t, os.Getpid())
	if sid == mine {
		t.Errorf("the program (pid %d) is in session %s, the caller's, want a session of its own", pid, sid)
	}
	if pgid, err := syscall.Getpgid(pid); err != nil || pgid != pid {
		t.Errorf("the program (pid %d) is in process group %d (err %v), want it the leader of one of its own", pid, pgid, err)
	}
}

// sessionOf is the session of a process as ps says it (the same word for the processes of one
// session, which is all a comparison needs). A machine whose ps does not know the keyword skips the
// test.
func sessionOf(t *testing.T, pid int) string {
	t.Helper()
	out, err := exec.Command("ps", "-o", "sess=", "-p", strconv.Itoa(pid)).Output()
	if err != nil || strings.TrimSpace(string(out)) == "" {
		t.Skipf("ps cannot say the session of a process here: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// The program writes to the log file itself, on both of its output streams, with the descriptor it
// was given: nothing the caller does after Detach returns is needed for the lines to arrive.
func TestDetachGivesTheProgramTheLogForOutputAndError(t *testing.T) {
	log := logFile(t)
	program := script(t, "echo to-stdout\necho to-stderr >&2\n")

	if err := (osproc.Runner{}).Detach(program, nil, log); err != nil {
		t.Fatalf("Detach: %v", err)
	}

	got := waitForFileContaining(t, log.Name(), "to-stderr")
	if !strings.Contains(got, "to-stdout\n") || !strings.Contains(got, "to-stderr\n") {
		t.Errorf("log = %q, want both lines", got)
	}
}

func waitForFileContaining(t *testing.T, path, text string) string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(path); err == nil && strings.Contains(string(data), text) {
			return string(data)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("%s did not come to hold %q within 2s", path, text)
	return ""
}

func TestDetachSaysWhyAProgramCouldNotBeStarted(t *testing.T) {
	notExecutable := filepath.Join(t.TempDir(), "not-executable")
	if err := os.WriteFile(notExecutable, []byte("#!/bin/sh\nexit 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, program := range map[string]string{
		"absent":         filepath.Join(t.TempDir(), "absent"),
		"not executable": notExecutable,
	} {
		t.Run(name, func(t *testing.T) {
			if err := (osproc.Runner{}).Detach(program, nil, logFile(t)); err == nil {
				t.Error("Detach started a program that cannot be started")
			}
		})
	}
}

// ---- Run --------------------------------------------------------------------------------------

func TestRunReportsHowTheProgramEnded(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want synctrigger.Result
	}{
		{"exit 0", "exit 0\n", synctrigger.Result{Exit: 0}},
		{"exit 2", "exit 2\n", synctrigger.Result{Exit: 2}},
		{"exit 7", "exit 7\n", synctrigger.Result{Exit: 7}},
		{"ended by a signal", "kill -9 $$\n", synctrigger.Result{Exit: -1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := osproc.Runner{}.Run(script(t, tc.body), nil, t.TempDir(), 10*time.Second, &bytes.Buffer{})
			if err != nil || got != tc.want {
				t.Errorf("Run = (%+v, %v), want (%+v, nil): a program that ended is a result, whatever its code", got, err, tc.want)
			}
		})
	}
}

// The program is started in the directory it is given, with the arguments it is given; what it
// prints on standard error reaches the writer, and what it prints on standard output goes nowhere.
func TestRunStartsTheProgramInTheDirectoryWithTheArgumentsAndGivesItsStandardError(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	program := script(t, `echo "in $(pwd -P) with $*" >&2`+"\n"+`echo to-stdout`+"\n")
	var stderr bytes.Buffer

	got, err := osproc.Runner{}.Run(program, []string{"sync", "two words"}, dir, 10*time.Second, &stderr)

	if err != nil || got != (synctrigger.Result{}) {
		t.Fatalf("Run = (%+v, %v)", got, err)
	}
	if want := "in " + dir + " with sync two words\n"; stderr.String() != want {
		t.Errorf("stderr = %q, want %q", stderr.String(), want)
	}
}

func TestRunSaysWhyAProgramCouldNotBeRun(t *testing.T) {
	notExecutable := filepath.Join(t.TempDir(), "not-executable")
	if err := os.WriteFile(notExecutable, []byte("#!/bin/sh\nexit 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, program := range map[string]string{
		"absent":         filepath.Join(t.TempDir(), "absent"),
		"not executable": notExecutable,
	} {
		t.Run(name, func(t *testing.T) {
			got, err := osproc.Runner{}.Run(program, nil, t.TempDir(), 10*time.Second, &bytes.Buffer{})
			if err == nil || got != (synctrigger.Result{}) {
				t.Errorf("Run = (%+v, %v), want a zero Result and an error", got, err)
			}
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				t.Errorf("a program that never started is not an exit: %v", err)
			}
		})
	}
}

// A program that outlives its time is killed with everything it started, so nothing is left behind
// to hold the machine: here the program is a shell and the thing it started a sleep, and neither
// is alive when Run returns, which it does long before the sleep would have ended.
func TestRunKillsAProgramThatOutlivesItsTimeWithEverythingItStarted(t *testing.T) {
	dir := t.TempDir()
	shellPid, sleeperPid := filepath.Join(dir, "shell"), filepath.Join(dir, "sleeper")
	program := script(t, `echo $$ > "`+shellPid+`"`+"\n"+`sleep 30 &`+"\n"+`echo $! > "`+sleeperPid+`"`+"\n"+`wait`+"\n")

	start := time.Now()
	got, err := osproc.Runner{}.Run(program, nil, t.TempDir(), 300*time.Millisecond, &bytes.Buffer{})
	elapsed := time.Since(start)

	if err != nil || got != (synctrigger.Result{TimedOut: true}) {
		t.Fatalf("Run = (%+v, %v), want (TimedOut, nil)", got, err)
	}
	if elapsed > 10*time.Second {
		t.Errorf("Run took %s, want it bound by the 300ms it was given and not by the 30s sleep", elapsed)
	}
	for name, file := range map[string]string{"the shell": shellPid, "what it started": sleeperPid} {
		pid, err := strconv.Atoi(strings.TrimSpace(waitForFile(t, file)))
		if err != nil {
			t.Fatalf("%s: pid: %v", name, err)
		}
		proc, _ := os.FindProcess(pid)
		deadline := time.Now().Add(2 * time.Second)
		for proc.Signal(syscall.Signal(0)) == nil && time.Now().Before(deadline) {
			time.Sleep(20 * time.Millisecond)
		}
		if proc.Signal(syscall.Signal(0)) == nil {
			t.Errorf("%s (pid %d) is still alive after Run returned (orphan)", name, pid)
		}
	}
}

// A program that ends in time is not a timeout, however close the deadline.
func TestRunDoesNotCallAProgramThatEndedInTimeATimeout(t *testing.T) {
	got, err := osproc.Runner{}.Run(script(t, "exit 3\n"), nil, t.TempDir(), time.Minute, &bytes.Buffer{})
	if err != nil || got.TimedOut || got.Exit != 3 {
		t.Errorf("Run = (%+v, %v), want exit 3 and no timeout", got, err)
	}
}
