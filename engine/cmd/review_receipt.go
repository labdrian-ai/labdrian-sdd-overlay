package main

// The 'review-receipt capture|hook' subcommand.

import (
	"fmt"
	"io"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/hookwire"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewreceipt"
)

// runReviewReceipt implements the 'review-receipt <verb>' subcommand.
// Verbs: capture, hook.
//
// Every p.exit in this file is followed by a return, and a service that could not be built is
// followed by one too: the exit of the program never returns, but a test hands in an exit that
// does, and the command must not go on with what it does not have. These are not dead code.
func runReviewReceipt(p process, d deps, args []string) {
	if len(args) < 1 {
		fmt.Fprintln(p.stderr, "error: review-receipt requires a verb: capture, hook")
		p.exit(1)
		return
	}
	switch args[0] {
	case "capture":
		runReviewReceiptCapture(p, d, args[1:])
	case "hook":
		runReviewReceiptHook(p, d, args[1:])
	default:
		fmt.Fprintf(p.stderr, "error: review-receipt: unknown verb %q (expected capture or hook)\n", args[0])
		p.exit(1)
	}
}

// parseReviewReceiptArgs extracts --cwd and --change from args.
func parseReviewReceiptArgs(args []string) (cwd, change string) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--cwd":
			i++
			if i < len(args) {
				cwd = args[i]
			}
		case "--change":
			i++
			if i < len(args) {
				change = args[i]
			}
		}
	}
	return
}

// buildReviewReceiptService builds the review receipt capture both verbs run on, asking git in
// environ. When it cannot be built it says why on stderr, in the words of the verb (prefix) and as a
// set-up failure, which tells it apart from a failure of the capture itself, and exits with code,
// the one the verb's contract gives a failure: 1 for the capture a person runs, 2 for the
// hook, where exit code 2 denies the acknowledgement. The exit is injected; a caller whose exit
// returns gets no service.
func buildReviewReceiptService(cwd string, environ []string, stderr io.Writer, exit func(int), prefix string, code int) *reviewreceipt.Service {
	svc, err := newReviewReceiptService(cwd, environ)
	if err != nil {
		fmt.Fprintf(stderr, "%s: set up failed: %v\n", prefix, err)
		exit(code)
		return nil
	}
	return svc
}

// runReviewReceiptCapture implements 'review-receipt capture --cwd <repo>
// [--change <name>]'. With --change, captures directly into that change.
// Without it, auto-detects the single active change: zero active changes is
// a no-op (exit 0); more than one is a loud failure (exit 1) naming
// --change as the remedy.
func runReviewReceiptCapture(p process, d deps, args []string) {
	cwd, change := parseReviewReceiptArgs(args)
	if cwd == "" {
		fmt.Fprintln(p.stderr, "error: --cwd is required")
		p.exit(1)
		return
	}

	svc := buildReviewReceiptService(cwd, d.environment(), p.stderr, p.exit, "error: review-receipt capture", 1)
	if svc == nil {
		return // the exit returned (a test's does): there is no service to go on with
	}

	if change == "" {
		detected, err := svc.DetectActiveChange()
		if err != nil {
			fmt.Fprintf(p.stderr, "error: %v\n", err)
			p.exit(1)
			return
		}
		if detected == "" {
			fmt.Fprintln(p.stdout, "review-receipt capture: no active change; nothing to capture")
			return
		}
		change = detected
	}

	captured, err := svc.Capture(change)
	if err != nil {
		fmt.Fprintf(p.stderr, "error: review-receipt capture: %v\n", err)
		p.exit(1)
		return
	}
	fmt.Fprintf(p.stdout, "review-receipt capture: %d receipt(s) captured for %q\n", len(captured), change)
}

// runReviewReceiptHook implements 'review-receipt hook --cwd <repo>': the
// fail-closed PreToolUse Bash hook entry point. Reads the raw hook input
// JSON from stdin, has engine/hookwire read the command out of it, asks
// reviewreceipt.Service.CheckCommand, and exits with the status of its
// verdict, printing its reason (if any) to stderr.
func runReviewReceiptHook(p process, d deps, args []string) {
	cwd, _ := parseReviewReceiptArgs(args)
	if cwd == "" {
		fmt.Fprintln(p.stderr, "error: --cwd is required")
		p.exit(hookwire.ExitBlock)
		return
	}

	raw, err := io.ReadAll(p.stdin)
	if err != nil {
		// Fail closed: an unreadable hook input is treated the same as any
		// other capture-resolution failure -- deny rather than silently
		// allow an acknowledgement this hook could not even inspect.
		fmt.Fprintf(p.stderr, "review-receipt hook: read stdin: %v\n", err)
		p.exit(hookwire.ExitBlock)
		return
	}

	// Fail closed, as for an unreadable input: a hook that cannot be set up cannot guard
	// the acknowledgement it was started for, so its set-up failure exits 2 like a denial.
	svc := buildReviewReceiptService(cwd, d.environment(), p.stderr, p.exit, "review-receipt hook", hookwire.ExitBlock)
	if svc == nil {
		return // the exit returned (a test's does): there is no service to go on with
	}

	// Malformed or empty input is treated the same as a command that is not an
	// acknowledgement -- pass through -- because a hook that cannot even see a command is
	// not looking at an acknowledge-approved invocation in the first place.
	command, err := hookwire.DecodeCommand(raw)
	if err != nil {
		p.exit(hookwire.ExitAllow)
		return
	}
	verdict := svc.CheckCommand(command)
	reply := hookwire.ExitReply{Block: verdict.Deny, Message: verdict.Reason}
	_, _ = p.stderr.Write(reply.MessageLine())
	p.exit(reply.Code())
}
