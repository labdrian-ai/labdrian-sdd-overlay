// Package osproc is the adapter of the Runner port of engine/synctrigger for the operating
// system: it starts the programs of the machine, in sessions and process groups of their own,
// and kills a whole group when a program outlives its time. It is the one place the trigger
// reaches os/exec and syscall, which is also where a platform that has no sessions or process
// groups will need its own file.
package osproc

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/synctrigger"
)

// waitDelay is how long Run waits, after it has killed a program at its deadline, for the pipes
// of the program to close. A grandchild that kept them open must not hold Run past the deadline.
const waitDelay = 5 * time.Second

// Runner starts programs with os/exec. It holds no state.
type Runner struct{}

var _ synctrigger.Runner = Runner{}

// Executable is the path of the running program.
func (Runner) Executable() (string, error) { return os.Executable() }

// Detach starts program with args under a new session (Setsid), so that it survives the teardown
// of the process group of its caller, with log for standard output and standard error. Its standard
// input is the null device, which is what os/exec gives a command that has none. It does not wait.
func (Runner) Detach(program string, args []string, log *os.File) error {
	cmd := exec.Command(program, args...)
	cmd.Stdout = log
	cmd.Stderr = log
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd.Start()
}

// Run starts program with args in dir, in a process group of its own, and waits for it. Standard
// error goes to stderr and standard output nowhere. When the timeout passes first, the whole group
// is killed, so a program that started others leaves no orphan, and the Result says TimedOut: that
// is what a deadline that has passed means, whatever else the program reported as it was stopped.
func (Runner) Run(program string, args []string, dir string, timeout time.Duration, stderr io.Writer) (synctrigger.Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, program, args...)
	cmd.Dir = dir
	cmd.Stderr = stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = waitDelay

	err := cmd.Run()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return synctrigger.Result{TimedOut: true}, nil
	}
	if err == nil {
		return synctrigger.Result{}, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return synctrigger.Result{Exit: exitErr.ExitCode()}, nil
	}
	return synctrigger.Result{}, err
}
