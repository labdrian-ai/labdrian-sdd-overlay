// Package execrunner is the process adapter of the ports that start a program: the
// CommandRunner the runtime adapters own, and the Runner of the git adapter of the package
// builder (pipkg/gitsource). It looks a program up on the PATH of the process and runs it with a
// fixed argument vector. It is the one place in the module that starts a program of the machine
// (the CLI of a runtime, today `pi`, and `git`). The composition root wires it into the Pi
// adapter and the git adapter (cmd); the runtime adapters and their tests see only their port,
// and no test of the runtime or of cmd imports this package.
package execrunner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"time"
)

// killGrace is how long Run waits, after it has stopped a program at its deadline, for the
// program's output pipes to close. A grandchild that kept them open must not hold Run past a
// deadline that has already expired.
const killGrace = 2 * time.Second

// ErrOutputTooLarge is what Output returns when a program prints more on either stream than the
// bound the runner was given, after it has stopped the program.
var ErrOutputTooLarge = errors.New("output is larger than the bound")

// Runner starts programs of the machine. It holds no state of a run; New exists so a caller names
// the adapter it wires.
type Runner struct {
	maxOutput int64
}

// New returns the process adapter.
func New() Runner { return Runner{} }

// WithMaxOutput is the runner with a bound, in bytes, on what Output holds of each stream: a
// program that prints more is stopped and Output returns ErrOutputTooLarge with the stream cut
// at exactly the bound. Zero is no bound,
// which is what New returns. Run, which returns a combined stream, is not bounded.
func (r Runner) WithMaxOutput(n int64) Runner {
	r.maxOutput = n
	return r
}

// MaxOutput is the bound WithMaxOutput set, in bytes; zero is none.
func (r Runner) MaxOutput() int64 { return r.maxOutput }

// cappedBuffer collects what a program prints up to a bound and stops the program when it goes
// over, so a runaway output cannot fill memory. The cut is exact: a write that crosses the bound
// is accepted up to the bound and refused for the rest, so what is held when Output fails is a
// prefix of the output that is exactly the bound long.
type cappedBuffer struct {
	buf    bytes.Buffer
	max    int64
	cancel context.CancelFunc
	over   bool
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	if c.max > 0 && int64(c.buf.Len()+len(p)) > c.max {
		room := int(c.max) - c.buf.Len()
		c.buf.Write(p[:room]) // a bytes.Buffer write does not fail
		c.over = true
		c.cancel()
		return room, ErrOutputTooLarge
	}
	return c.buf.Write(p)
}

// LookPath is the path of the program name on the PATH of the process.
func (Runner) LookPath(name string) (string, error) { return exec.LookPath(name) }

// Run starts bin with args as a vector, never as a shell string, so no argument is interpreted,
// and waits for it. It returns what the program printed on both streams, even when it failed.
// The program is stopped when ctx ends, and the error then wraps the reason (context.DeadlineExceeded
// for a deadline) next to what the stopped process reported.
func (Runner) Run(ctx context.Context, bin string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.WaitDelay = killGrace
	out, err := cmd.CombinedOutput()
	if err != nil && ctx.Err() != nil {
		return out, fmt.Errorf("%w (%v)", ctx.Err(), err)
	}
	return out, err
}

// Output starts bin with args as a vector, like Run, and returns what it printed on standard
// output and on standard error apart, even when it failed, because a caller that parses the
// first must not find the second in it. env is the whole environment of the program: nothing of
// the process is added to it. A nil env gives the program the environment of the process, as
// exec.Command does. The program is stopped when ctx ends, and the error then wraps the reason;
// it is stopped too when it prints more than the bound of WithMaxOutput on a stream.
func (r Runner) Output(ctx context.Context, env []string, bin string, args ...string) (stdout, stderr []byte, err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = env
	cmd.WaitDelay = killGrace
	out := &cappedBuffer{max: r.maxOutput, cancel: cancel}
	errOut := &cappedBuffer{max: r.maxOutput, cancel: cancel}
	cmd.Stdout = out
	cmd.Stderr = errOut
	err = cmd.Run()
	switch {
	case out.over || errOut.over:
		err = fmt.Errorf("%w (%d bytes at most)", ErrOutputTooLarge, r.maxOutput)
	case err != nil && ctx.Err() != nil:
		err = fmt.Errorf("%w (%v)", ctx.Err(), err)
	}
	return out.buf.Bytes(), errOut.buf.Bytes(), err
}
