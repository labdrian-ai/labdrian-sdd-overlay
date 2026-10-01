package main

// projection subcommand: 'projection hook --event UserPromptSubmit|PreToolUse'.
// It is an internal Claude Code hook command, not one a person runs: Claude Code
// starts it, hands it the event as JSON on stdin, and reads what it prints.
// `install-hooks` installs it (the settings projection family); Claude Code loads
// hook changes only when it starts, so an existing install must re-run
// install-hooks and restart Claude Code. The tests feed it hook JSON directly.
//
// UserPromptSubmit puts into the session the workflow the repository is bound to
// (see workflow bind): the decision is engine/projection's Project, a pure
// function of the binding and the workflow state, and this file only reads those
// two from disk, calls it, and prints the result. PreToolUse gates a tool call
// against the same two: engine/projection's Gate denies the file-edit tools
// while the workflow is paused and checks a longterm-mem query's project against
// the workflow's memory plan. The contract with Claude Code:
//
//   - stdout is empty, or exactly one JSON object that begins with '{'.
//     UserPromptSubmit: {"hookSpecificOutput":{"hookEventName":"UserPromptSubmit",
//     "additionalContext":"..."},"systemMessage":"..."}, either part left out
//     when it is empty. PreToolUse: to deny, {"hookSpecificOutput":{
//     "hookEventName":"PreToolUse","permissionDecision":"deny",
//     "permissionDecisionReason":"..."}}, and to allow, nothing (or only a
//     {"systemMessage":"..."} warning); it never prints permissionDecision
//     "allow", which would bypass Claude Code's normal permission flow. Nothing
//     else is ever printed to stdout.
//   - The exit code is 0 always, except 1 for a command line that is not
//     understood. It is never 2, which would block the prompt or the tool call:
//     a denial is the JSON above with exit 0, and whatever goes wrong, the
//     prompt and the tool call go through.
//   - A repository nothing is bound to, input that is not a usable hook input,
//     and a working directory outside every repository are silent: no output at
//     all, on either stream. The PreToolUse gate is silent also for a binding or
//     workflow it cannot follow and for a binding store it cannot open: it runs
//     on every tool call, and UserPromptSubmit already warns once per prompt.
//     It also answers, without reading either store, for every tool it never
//     checks: only a file-edit tool or a longterm-mem query needs the binding.
//   - A binding, or a bound workflow, that cannot be followed is one visible
//     warning (systemMessage) with nothing projected, and is never resolved by
//     removing anything. So is a binding store that cannot be opened or read
//     (a real error, not an absent binding), and so is a recovered panic (the
//     warning is sanitized and short; the full text goes to stderr).
//
// The PreToolUse gate is strictly read-only. UserPromptSubmit only reads, with one
// exception: when the bound workflow is closed it removes the binding, best effort, only if it is still the binding it read. The
// note it prints says what the removal did: removed, failed (and that
// 'labdrian workflow unbind' finishes it), or left alone. It runs no subprocess and makes no network call:
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

// onGateStoreAccess is a test seam, nil outside tests. The gate calls it at the
// moment it goes to the binding and workflow stores, so a test can prove that a
// tool the gate never checks is answered without touching them.
var onGateStoreAccess func()

// beforeGateDecision is a test seam, nil outside tests. The gate calls it before
// it decides, so a test can make the gate panic where a bug in it would.
var beforeGateDecision func()

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
// exit code from 0: a hook that fails must not fail the prompt or the tool call
// it serves.
func runProjectionHook(args []string, processCwd string, stdin io.Reader, stdout, stderr io.Writer, exit func(int)) {
	event, err := parseHookArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "error: projection hook: %v\n", err)
		exit(1)
		return
	}
	// A Go panic ends the process with status 2, which Claude Code reads as "block
	// this prompt". The projection is pure and is not expected to panic, but the
	// cost of being wrong is a session that cannot send a prompt or run a tool, so
	// it is caught, reported in full on stderr (which Claude Code shows only in
	// verbose mode), shown to the user as one short sanitized systemMessage (for
	// PreToolUse that is an allow with a warning, never a denial), and turned into
	// the same exit 0 as every other failure.
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(stderr, "projection hook: internal error: %v\n", r)
			_, _ = stdout.Write(warningOutput(event, projection.PanicWarning(event, r)))
			exit(0)
		}
	}()
	var out []byte
	if event == projection.HookEventPreToolUse {
		out = preToolUse(stdin, processCwd)
	} else {
		out = userPromptSubmit(stdin, processCwd)
	}
	if len(out) > 0 {
		_, _ = stdout.Write(out) // nothing to do if the write fails: the prompt or the call goes through.
	}
	exit(0)
}

// parseHookArgs checks the arguments of 'projection hook': --event, with the
// event to answer, and nothing else, and returns the event. UserPromptSubmit and
// PreToolUse are the events supported. The last occurrence of a repeated flag
// wins, as for the other verbs.
func parseHookArgs(args []string) (string, error) {
	event := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--event":
			if i+1 >= len(args) {
				return "", fmt.Errorf("--event requires a value")
			}
			i++
			event = args[i]
		case strings.HasPrefix(a, "-"):
			return "", fmt.Errorf("unknown flag %q", a)
		default:
			return "", fmt.Errorf("unexpected argument %q", a)
		}
	}
	supported := projection.HookEventUserPromptSubmit + ", " + projection.HookEventPreToolUse
	if event == "" {
		return "", fmt.Errorf("--event is required (supported: %s)", supported)
	}
	if event != projection.HookEventUserPromptSubmit && event != projection.HookEventPreToolUse {
		return "", fmt.Errorf("unsupported --event %q (supported: %s)", event, supported)
	}
	return event, nil
}

// warningOutput renders one warning line as the hook's only output, in the shape
// of event's own output builder, or nothing when it cannot be encoded (a warning
// must never be able to fail the prompt or the tool call). For PreToolUse it is a
// systemMessage and never a permission decision.
func warningOutput(event, warning string) []byte {
	var out []byte
	var err error
	if event == projection.HookEventPreToolUse {
		out, err = projection.GateResult{Warning: warning}.PreToolUseOutput()
	} else {
		out, err = projection.ProjectionResult{Warning: warning}.UserPromptSubmitOutput()
	}
	if err != nil {
		return nil
	}
	return out
}

// userPromptSubmit does the hook's work and returns what it prints on stdout,
// which is nothing whenever there is nothing to say. Each step that fails before
// the binding store is asked (input that cannot be read or used, a directory
// outside every repository) ends in silence: there is nothing to be loyal to. A
// store that cannot be opened or read is different, because the repository may
// well be bound, so it gets one warning.
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
	repoKey, ok := hookRepoKey(in.Cwd, processCwd)
	if !ok {
		return nil
	}
	bindings, err := newBindingStore()
	if err != nil {
		return warningOutput(projection.HookEventUserPromptSubmit, projection.StoreWarning(err))
	}
	binding, err := bindings.Load(repoKey)
	if err != nil {
		return warningOutput(projection.HookEventUserPromptSubmit, projection.StoreWarning(err))
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
		// What happened goes into the note, so it never claims a removal that
		// did not take place.
		removed, unbindErr := bindings.UnbindIfUnchanged(repoKey, binding.Binding)
		result = result.AfterUnbind(removed, unbindErr)
	}
	out, err := result.UserPromptSubmitOutput()
	if err != nil {
		return nil
	}
	return out
}

// hookRepoKey finds the repository key of the session: the input's cwd when it
// has one, else the process's. It is false when neither is inside a repository.
func hookRepoKey(inputCwd, processCwd string) (string, bool) {
	cwd := inputCwd
	if cwd == "" {
		cwd = processCwd
	}
	return observeRepoKey(cwd)
}

// preToolUse does the PreToolUse gate's work and returns what it prints on
// stdout: a denial, a warning, or nothing. Every step that cannot go on ends in
// silence and so in an allow: unusable input, another event's input, a directory
// outside every repository, a binding store that cannot be opened or read, a
// binding or workflow that cannot be followed. The gate runs on every tool call,
// and the prompt hook already warns about those, so it does not repeat the
// warning. It reads and never writes.
func preToolUse(stdin io.Reader, processCwd string) []byte {
	data, err := io.ReadAll(io.LimitReader(stdin, projection.MaxHookInputBytes+1))
	if err != nil {
		return nil
	}
	in, err := projection.ParsePreToolUseInput(data)
	if err != nil {
		return nil
	}
	if in.HookEventName != "" && in.HookEventName != projection.HookEventPreToolUse {
		return nil
	}
	// Relevance comes first, from the tool name alone: the gate runs on every
	// tool call, and only a file-edit tool or a longterm-mem query is ever checked
	// against the workflow. Any other tool is allowed here, without reading the
	// binding or the workflow log.
	if !projection.GateRelevant(in.ToolName) {
		return nil
	}
	repoKey, ok := hookRepoKey(in.Cwd, processCwd)
	if !ok {
		return nil
	}
	if onGateStoreAccess != nil {
		onGateStoreAccess()
	}
	bindings, err := newBindingStore()
	if err != nil {
		return nil
	}
	binding, err := bindings.Load(repoKey)
	if err != nil {
		return nil
	}

	gateInput := projection.GateInput{Binding: binding, ToolName: in.ToolName, ToolInput: in.ToolInput}
	if binding.Classification == projection.ClassificationOwned {
		w := loadWorkflow(binding.Binding.ProjectID, binding.Binding.WorkflowID)
		gateInput.Workflow = &w
	}
	if beforeGateDecision != nil {
		beforeGateDecision()
	}
	out, err := projection.Gate(gateInput).PreToolUseOutput()
	if err != nil {
		return nil
	}
	return out
}
