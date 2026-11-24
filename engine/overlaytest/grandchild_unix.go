//go:build unix

package overlaytest

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// grandchildCommand is the command line of the process a Grandchild stub starts. The duration is unlike any other the
// tests use, because it is how the cleanup recognizes the process it started: it kills an id only when the command line
// of that id is exactly this one, so an id that another process reused is left alone.
const grandchildCommand = "/bin/sleep 30.7"

// grandchildLister reads the command line and the state of a process by its id. It is named by path because a test may
// run with an empty PATH.
const grandchildLister = "/bin/ps"

// Grandchild is a program body for a test of a bound on a run: the shell forks a grandchild that sleeps for 30 seconds and
// waits for it, so only a kill that reaches every process the run started ends the sleep early.
type Grandchild struct {
	// Body is the text of the program, to run with sh.
	Body    string
	pidFile string
}

// NewGrandchild returns the stub and registers the cleanup that kills the grandchild if it is still alive when the test
// ends. It kills the one process it started and nothing else: its id comes from the stub's own file and is followed only
// while its command line is grandchildCommand. The test is skipped when the machine has no /bin/sh, /bin/sleep or
// /bin/ps, since without a lister a grandchild could not be found again to be stopped.
func NewGrandchild(t testing.TB) *Grandchild {
	t.Helper()
	for _, path := range []string{"/bin/sh", "/bin/sleep", grandchildLister} {
		if _, err := os.Stat(path); err != nil {
			t.Skipf("no %s to run and follow the grandchild: %v", path, err)
		}
	}
	g := &Grandchild{pidFile: t.TempDir() + "/grandchild.pid"}
	g.Body = "x=$(" + grandchildCommand + " & echo $! > '" + g.pidFile + "'; wait)\n"
	t.Cleanup(func() {
		if pid, ok := g.pid(t); ok && g.isGrandchild(t, pid) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	})
	return g
}

// pid is the id the stub wrote, and false when it wrote none.
func (g *Grandchild) pid(t testing.TB) (int, bool) {
	t.Helper()
	raw, err := os.ReadFile(g.pidFile)
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil || pid <= 1 {
		t.Errorf("pid file %s holds %q", g.pidFile, raw)
		return 0, false
	}
	return pid, true
}

// isGrandchild says whether pid is a live process (not a zombie) whose command line is the stub's sleep. A lister that
// cannot be run is an error of the test: the id could not be checked.
func (g *Grandchild) isGrandchild(t testing.TB, pid int) bool {
	t.Helper()
	out, err := exec.Command(grandchildLister, "-p", strconv.Itoa(pid), "-o", "stat=", "-o", "args=").Output()
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 && len(out) == 0 {
		return false
	}
	if err != nil {
		t.Errorf("process %d was not checked: %v", pid, err)
		return false
	}
	state, command, _ := strings.Cut(strings.TrimSpace(string(out)), " ")
	return !strings.HasPrefix(state, "Z") && strings.TrimSpace(command) == grandchildCommand
}

// RequireGone fails the test unless the grandchild is gone within the time given, polling every 10 ms. A grandchild that
// never started (the run was stopped before its shell forked it) is gone.
func (g *Grandchild) RequireGone(t testing.TB, within time.Duration) {
	t.Helper()
	deadline := time.Now().Add(within)
	for {
		pid, ok := g.pid(t)
		if !ok || !g.isGrandchild(t, pid) {
			return
		}
		if time.Now().After(deadline) {
			t.Errorf("the grandchild (process %d) is still alive %v after the run returned", pid, within)
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Started fails the test unless the stub has written the id of a grandchild: a test of the kill must know the stub ran.
func (g *Grandchild) Started(t testing.TB) {
	t.Helper()
	if _, ok := g.pid(t); !ok {
		t.Errorf("the grandchild stub did not write its id to %s: it was not run", g.pidFile)
	}
}
