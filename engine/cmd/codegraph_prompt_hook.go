package main

// The 'codegraph-prompt-hook' subcommand: the wrapper install-hooks puts in place of the bare
// `codegraph prompt-hook` UserPromptSubmit command of Claude Code (S19, L87). A task notification is not
// something the user wrote, so exploring its text with codegraph adds nothing: the wrapper prints nothing
// for one and exits 0. Every other input goes to `codegraph prompt-hook` on the PATH with the same stdin
// bytes, and its stdout, stderr and exit code come back unchanged.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"syscall"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/execrunner"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/hookwire"
)

// codegraphHookMaxOutput bounds what the wrapper holds of one stream of codegraph, so that a runaway
// program cannot fill memory. A hook that prints this much is far past what reaches a model.
const codegraphHookMaxOutput = 64 << 20

// errCodegraphTooMuch is what the runner reports when codegraph printed more than the bound; the tests name it
// through this value because no test of the package imports the process adapter.
var errCodegraphTooMuch = execrunner.ErrOutputTooLarge

// codegraphRunner starts codegraph with the hook input on its stdin; execrunner.Runner is the program's.
type codegraphRunner interface {
	OutputStdin(ctx context.Context, env []string, stdin []byte, bin string, args ...string) (stdout, stderr []byte, err error)
}

// shellExitCode is the code `sh -c` would give for a program that ended with code, or, when a signal ended it
// (code is -1), 128 plus the signal number. A signal ending that the error does not name is 1.
func shellExitCode(err error, code int) int {
	if code >= 0 {
		return code
	}
	var sys interface{ Sys() any }
	if errors.As(err, &sys) {
		if ws, ok := sys.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			return 128 + int(ws.Signal())
		}
	}
	return 1
}

// runCodegraphPromptHook implements the 'codegraph-prompt-hook' subcommand. The runner is handed in, so a
// test never starts the real codegraph; the dispatcher gives it the process adapter.
func runCodegraphPromptHook(p process, runner codegraphRunner, _ []string) {
	input, err := io.ReadAll(p.stdin)
	if err != nil {
		fmt.Fprintf(p.stderr, "error: codegraph-prompt-hook: read stdin: %v\n", err)
		p.exit(1)
		return
	}
	if prompt, ok := hookwire.PromptText(input); ok && hookwire.IsTaskNotification(prompt) {
		return
	}
	// A nil environment is the one of the process, which is what the bare command gave codegraph.
	stdout, stderr, err := runner.OutputStdin(context.Background(), nil, input, "codegraph", "prompt-hook")
	_, _ = p.stdout.Write(stdout)
	_, _ = p.stderr.Write(stderr)
	if err == nil {
		return
	}
	var exited interface{ ExitCode() int }
	switch {
	case errors.Is(err, errCodegraphTooMuch): // before the exit code: a stopped program reports both
		fmt.Fprintf(p.stderr, "codegraph: %v\n", err)
		p.exit(1)
	case errors.As(err, &exited):
		p.exit(shellExitCode(err, exited.ExitCode()))
	case errors.Is(err, fs.ErrPermission):
		fmt.Fprintf(p.stderr, "codegraph: %v\n", err)
		p.exit(126)
	default: // not found, as `sh -c` answers for the bare command
		fmt.Fprintf(p.stderr, "codegraph: %v\n", err)
		p.exit(127)
	}
}
