// Package execrunner is the process adapter of the CommandRunner port the runtime adapters own:
// it looks a program up on the PATH of the process and runs it with a fixed argument vector.
// It is the one place in the module that starts the CLI of a runtime (today, `pi`).
package execrunner

import (
	"context"
	"fmt"
	"os/exec"
	"time"
)

// killGrace is how long Run waits, after it has stopped a program at its deadline, for the
// program's output pipes to close. A grandchild that kept them open must not hold Run past a
// deadline that has already expired.
const killGrace = 2 * time.Second

// Runner starts programs of the machine. It holds no state; New exists so a caller names the
// adapter it wires.
type Runner struct{}

// New returns the process adapter.
func New() Runner { return Runner{} }

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
