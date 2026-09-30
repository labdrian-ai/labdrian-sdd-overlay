package main

// projection subcommand: 'projection hook --event UserPromptSubmit'. It is an
// internal Claude Code hook command, not one a person runs: Claude Code starts
// it before each prompt, hands it the event as JSON on stdin, and adds what it
// prints to the session's context. `install-hooks` does not install it yet, so
// today it runs only when something feeds it hook JSON, as the tests do.
//
// It puts into the session the workflow the repository is bound to (see
// workflow bind): the decision is engine/projection's Project, a pure function
// of the binding and the workflow state, and this file only reads those two
// from disk, calls it, and prints the result. The contract with Claude Code:
//
//   - stdout is empty, or exactly one JSON object that begins with '{':
//     {"hookSpecificOutput":{"hookEventName":"UserPromptSubmit",
//     "additionalContext":"..."},"systemMessage":"..."}, either part left out
//     when it is empty. Nothing else is ever printed to stdout.
//   - The exit code is 0 always, except 1 for a command line that is not
//     understood. It is never 2, which would block the prompt: whatever goes
//     wrong, the prompt goes through.
//   - A repository nothing is bound to, input that is not a usable hook input,
//     and a working directory outside every repository are silent: no output at
//     all, on either stream.
//   - A binding, or a bound workflow, that cannot be followed is one visible
//     warning (systemMessage) with nothing projected, and is never resolved by
//     removing anything.
//
// The hook only reads, with one exception: when the bound workflow is closed it
// removes the binding, best effort, only if it is still the binding it read, and
// ignores a failure to do so. It runs no subprocess and makes no network call:
// the repository is found by walking the filesystem, as the binding verbs do.

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
)

// beforeHookUnbind is a test seam, nil outside tests. The hook calls it after it
// decided to remove the binding of a closed workflow and before it does: the
// window in which another process can bind the next workflow, which
// projection.Store.UnbindIfUnchanged must then refuse to remove.
var beforeHookUnbind func()

// runProjection implements the 'projection <action>' subcommand.
func runProjection(args []string) {
	cwd, _ := os.Getwd() // best-effort; "" leaves the hook with only the input's directory.
	runProjectionCore(args, cwd, os.Stdin, os.Stdout, os.Stderr, os.Exit)
}

// runProjectionCore is the testable core of the projection subcommand. Every
// exit(n) is followed by a return, because tests inject a non-terminating exit.
// processCwd is the working directory of the process, used when the hook input
// names none.
func runProjectionCore(args []string, processCwd string, stdin io.Reader, stdout, stderr io.Writer, exit func(int)) {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		fmt.Fprintln(stderr, "error: projection requires an action: hook")
		exit(1)
		return
	}
	switch args[0] {
	case "hook":
		runProjectionHook(args[1:], processCwd, stdin, stdout, stderr, exit)
	default:
		fmt.Fprintf(stderr, "error: projection: unknown action %q (expected hook)\n", args[0])
		exit(1)
	}
}

// runProjectionHook implements 'projection hook --event <event>'. From the
// moment the command line has been understood, nothing it does can change the
// exit code from 0: a hook that fails must not fail the prompt it serves.
func runProjectionHook(args []string, processCwd string, stdin io.Reader, stdout, stderr io.Writer, exit func(int)) {
	if err := parseHookArgs(args); err != nil {
		fmt.Fprintf(stderr, "error: projection hook: %v\n", err)
		exit(1)
		return
	}
	// A Go panic ends the process with status 2, which Claude Code reads as "block
	// this prompt". The projection is pure and is not expected to panic, but the
	// cost of being wrong is a session that cannot send a prompt, so it is caught,
	// reported on stderr (which Claude Code shows only in verbose mode), and turned
	// into the same exit 0 as every other failure.
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(stderr, "projection hook: internal error: %v\n", r)
			exit(0)
		}
	}()
	if out := userPromptSubmit(stdin, processCwd); len(out) > 0 {
		_, _ = stdout.Write(out) // nothing to do if the write fails: the prompt goes through.
	}
	exit(0)
}

// parseHookArgs checks the arguments of 'projection hook': --event, with the
// event to answer, and nothing else. UserPromptSubmit is the only event
// supported. The last occurrence of a repeated flag wins, as for the other verbs.
func parseHookArgs(args []string) error {
	event := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--event":
			if i+1 >= len(args) {
				return fmt.Errorf("--event requires a value")
			}
			i++
			event = args[i]
		case strings.HasPrefix(a, "-"):
			return fmt.Errorf("unknown flag %q", a)
		default:
			return fmt.Errorf("unexpected argument %q", a)
		}
	}
	if event == "" {
		return fmt.Errorf("--event is required (supported: %s)", projection.HookEventUserPromptSubmit)
	}
	if event != projection.HookEventUserPromptSubmit {
		return fmt.Errorf("unsupported --event %q (supported: %s)", event, projection.HookEventUserPromptSubmit)
	}
	return nil
}

// userPromptSubmit does the hook's work and returns what it prints on stdout,
// which is nothing whenever there is nothing to say. Each step that fails, before
// there is a binding to be loyal to, ends in silence.
func userPromptSubmit(stdin io.Reader, processCwd string) []byte {
	// One byte past the cap is enough for ParseHookInput to see the input is over
	// it, and no more of an endless input is ever read.
	data, err := io.ReadAll(io.LimitReader(stdin, projection.MaxHookInputBytes+1))
	if err != nil {
		return nil
	}
	in, err := projection.ParseHookInput(data)
	if err != nil {
		return nil
	}
	// The event flag decides the shape of the answer; input that names another
	// event was not meant for this command.
	if in.HookEventName != "" && in.HookEventName != projection.HookEventUserPromptSubmit {
		return nil
	}
	cwd := in.Cwd
	if cwd == "" {
		cwd = processCwd
	}
	repoKey, ok := observeRepoKey(cwd)
	if !ok {
		return nil
	}
	bindings, err := projection.NewStore()
	if err != nil {
		return nil
	}
	binding, err := bindings.Load(repoKey)
	if err != nil {
		return nil
	}

	input := projection.ProjectionInput{Binding: binding}
	if binding.Classification == projection.ClassificationOwned {
		w := loadWorkflow(binding.Binding.ProjectID, binding.Binding.WorkflowID)
		input.Workflow = &w
	}
	result := projection.Project(input)
	if result.Unbind {
		if beforeHookUnbind != nil {
			beforeHookUnbind()
		}
		// Only the binding that was read, and only best effort: a fresh binding
		// made since stays, and a failure to remove this one is not the prompt's
		// problem (the next prompt sees the closed workflow again and retries).
		_, _ = bindings.UnbindIfUnchanged(repoKey, binding.Binding)
	}
	out, err := result.UserPromptSubmitOutput()
	if err != nil {
		return nil
	}
	return out
}
