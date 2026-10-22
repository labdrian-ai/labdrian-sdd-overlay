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
// caller: it is the leader of a session that is not the caller's. The program the test detaches
// must still end with the test, or every run of the suite leaves one behind.
func TestDetachStartsTheProgramInASessionOfItsOwn(t *testing.T) {
	pid := 0
	t.Run("session", func(t *testing.T) {
		pidFile := filepath.Join(t.TempDir(), "pid")
		// The program runs while its own script exists: removing the test's directories, however
		// the test ended, is what ends it.
		program := script(t, `echo $$ > "`+pidFile+`"; while [ -e "$0" ]; do sleep 0.02; done`+"\n")

		if err := (osproc.Runner{}).Detach(program, nil, logFile(t)); err != nil {
			t.Fatalf("Detach: %v", err)
		}

		var err error
		pid, err = strconv.Atoi(strings.TrimSpace(waitForFile(t, pidFile)))
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
	})
	if pid != 0 {
		assertEnded(t, pid)
	}
}

// assertEnded waits for a program this process started and never waited for. Such a program stays
// a zombie until reaped, and a zombie still answers signals, so ending is proved by reaping it.
func assertEnded(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var status syscall.WaitStatus
		got, err := syscall.Wait4(pid, &status, syscall.WNOHANG, nil)
		if err != nil {
			t.Errorf("waiting for the detached program (pid %d): %v", pid, err)
			return
		}
		if got == pid {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Errorf("the detached program (pid %d) is still running after its test ended (orphan)", pid)
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
	shell, sleeper := untilTheProgramHasWrittenItsPids(t, func(timeout time.Duration, shellFile, sleeperFile string) {
		start := time.Now()
		got, err := osproc.Runner{}.Run(script(t, pidScript(shellFile, sleeperFile)), nil, t.TempDir(), timeout, &bytes.Buffer{})
		elapsed := time.Since(start)

		if err != nil || got != (synctrigger.Result{TimedOut: true}) {
			t.Fatalf("Run = (%+v, %v), want (TimedOut, nil)", got, err)
		}
		if elapsed > 20*time.Second {
			t.Errorf("Run took %s, want it bound by the %s it was given and not by the 30s sleep", elapsed, timeout)
		}
	})

	assertGone(t, "the shell", shell)
	assertGone(t, "what it started", sleeper)
}

// The retry is what makes the test above hold on a loaded machine, so it is shown on a script that
// is slow on purpose: it takes half a second to write anything, the first deadline of 300 ms kills
// it before it has, and the run is repeated with twice the time (and again if that is still early).
func TestAnAttemptKilledBeforeTheScriptWroteItsPidsIsRepeatedWithTwiceTheTime(t *testing.T) {
	var timeouts []time.Duration

	shell, sleeper := untilTheProgramHasWrittenItsPids(t, func(timeout time.Duration, shellFile, sleeperFile string) {
		timeouts = append(timeouts, timeout)
		got, err := osproc.Runner{}.Run(script(t, "sleep 0.5\n"+pidScript(shellFile, sleeperFile)), nil, t.TempDir(), timeout, &bytes.Buffer{})
		if err != nil || !got.TimedOut {
			t.Fatalf("Run = (%+v, %v), want (TimedOut, nil)", got, err)
		}
	})

	if len(timeouts) < 2 || timeouts[0] != 300*time.Millisecond || timeouts[1] != 600*time.Millisecond {
		t.Errorf("the deadlines were %v, want 300ms and then twice that (and twice again if still early)", timeouts)
	}
	for i := 1; i < len(timeouts); i++ {
		if timeouts[i] != 2*timeouts[i-1] {
			t.Errorf("deadline %d is %v after %v, want twice", i, timeouts[i], timeouts[i-1])
		}
	}
	assertGone(t, "the shell", shell)
	assertGone(t, "what it started", sleeper)
}

// An attempt counts only when both pid files hold a pid: a script killed after the first and
// before the second, or between creating a file and writing it, leaves a partial record, and a
// partial record is no proof of anything.
func TestAnAttemptWhoseRecordOfPidsIsPartialIsRepeated(t *testing.T) {
	const absent = "<no file>"
	for name, partial := range map[string][2]string{
		"only the shell":            {"111\n", absent},
		"only what it started":      {absent, "222\n"},
		"a shell file still empty":  {"", "222\n"},
		"a shell that is no number": {"not a pid\n", "222\n"},
	} {
		t.Run(name, func(t *testing.T) {
			attempts := 0

			shell, sleeper := untilTheProgramHasWrittenItsPids(t, func(_ time.Duration, shellFile, sleeperFile string) {
				attempts++
				content := [2]string{"111\n", "222\n"}
				if attempts == 1 {
					content = partial
				}
				for i, file := range []string{shellFile, sleeperFile} {
					if content[i] == absent {
						continue
					}
					if err := os.WriteFile(file, []byte(content[i]), 0o644); err != nil {
						t.Fatal(err)
					}
				}
			})

			if attempts != 2 || shell != 111 || sleeper != 222 {
				t.Errorf("%d attempts, pids (%d, %d), want 2 attempts and (111, 222)", attempts, shell, sleeper)
			}
		})
	}
}

// The deadline of a run starts with the run, and a script needs a moment to start, a long one on a
// machine under load. A run that was killed before the script wrote its pid files says nothing
// about the kill, so it is repeated with twice the time (from 300 ms, six times at most, 18.9 s if
// every one is needed) until the script has written both before it was killed; a test that fails
// does so for what the kill did, never for being early.
func untilTheProgramHasWrittenItsPids(t *testing.T, attempt func(timeout time.Duration, shellFile, sleeperFile string)) (shell, sleeper int) {
	t.Helper()
	timeout := 300 * time.Millisecond
	for i := 0; i < 6; i++ {
		dir := t.TempDir()
		shellFile, sleeperFile := filepath.Join(dir, "shell"), filepath.Join(dir, "sleeper")
		attempt(timeout, shellFile, sleeperFile)
		s, errS := readPid(shellFile)
		p, errP := readPid(sleeperFile)
		if errS == nil && errP == nil {
			return s, p
		}
		timeout *= 2
	}
	t.Fatal("the program was killed before it wrote its pid files on every attempt, up to a deadline of 9.6s")
	return 0, 0
}

// pidScript is a shell that writes its pid to shellFile, starts a sleep of 30 seconds, writes the
// pid of the sleep to sleeperFile and waits for it.
func pidScript(shellFile, sleeperFile string) string {
	return `echo $$ > "` + shellFile + `"` + "\n" + `sleep 30 &` + "\n" + `echo $! > "` + sleeperFile + `"` + "\n" + `wait` + "\n"
}

// readPid is the pid a file holds, or an error when it is absent, empty or not a number.
func readPid(file string) (int, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}

// assertGone fails the test when the process is still there after up to 5 seconds. A process the
// kill has just orphaned is reaped by init a moment later, so it is waited for; one that is never
// gone was never killed.
func assertGone(t *testing.T, what string, pid int) {
	t.Helper()
	proc, _ := os.FindProcess(pid)
	deadline := time.Now().Add(5 * time.Second)
	for proc.Signal(syscall.Signal(0)) == nil && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if proc.Signal(syscall.Signal(0)) == nil {
		t.Errorf("%s (pid %d) is still alive after Run returned (orphan)", what, pid)
	}
}

// A program that ends in time is not a timeout, however close the deadline.
func TestRunDoesNotCallAProgramThatEndedInTimeATimeout(t *testing.T) {
	got, err := osproc.Runner{}.Run(script(t, "exit 3\n"), nil, t.TempDir(), time.Minute, &bytes.Buffer{})
	if err != nil || got.TimedOut || got.Exit != 3 {
		t.Errorf("Run = (%+v, %v), want exit 3 and no timeout", got, err)
	}
}
