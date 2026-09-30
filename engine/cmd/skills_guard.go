package main

// skills subcommand, verb 'guard-hook': the Claude Code PreToolUse hook that
// denies the AGENT running `skills approve` and writing the approval record by
// hand. It is an internal hook command, not one a person runs: Claude Code starts
// it, hands it the tool call as JSON on stdin, and reads what it prints.
// `install-hooks` installs it (the settings approve-guard family); Claude Code
// loads hook changes only when it starts, so an existing install must re-run
// install-hooks and restart Claude Code. The tests feed it hook JSON directly.
//
// The decision is engine/skills' DecideApproveGuard, a pure function of the hook
// input; this file only reads stdin (bounded), calls it, and prints the result.
// It is a speed bump, not a security boundary: it matches the command text and
// the file name, so it can be bypassed (see engine/skills/approve_guard.go for
// the rule, the exemptions, and the known bypasses).
//
// The contract with Claude Code is the projection hook's:
//
//   - stdout is empty, or exactly one JSON object: to deny, {"hookSpecificOutput":
//     {"hookEventName":"PreToolUse","permissionDecision":"deny",
//     "permissionDecisionReason":"..."}}, and to allow, nothing. It never prints
//     permissionDecision "allow", which would bypass Claude Code's normal
//     permission flow.
//   - The exit code is 0 always, except 1 for a command line that is not
//     understood. It is never 2, which would block the tool call: a denial is
//     the JSON above with exit 0, and whatever goes wrong (input that cannot be
//     read or decoded, input over the bound, a stdin that fails, an answer that
//     cannot be written, a panic) the tool call goes through. This hook runs on
//     every Bash and file-edit call, so it fails open, unlike the shaper
//     clearance guard, which fails closed for its narrower markers.
//
// It reads no file, runs no subprocess, and makes no network call.

import (
	"fmt"
	"io"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// skillsGuardVerb is the 'skills' verb of the approve guard hook. It is handled
// here, before the skills core, because it reads stdin; it is not a verb a
// person runs and is not in the skills core's verb list.
const skillsGuardVerb = "guard-hook"

// beforeApproveGuardDecision is a test seam, nil outside tests. The hook calls it
// before it decides, so a test can make the hook panic where a bug in it would.
var beforeApproveGuardDecision func()

// runSkillsWithStdin routes 'skills <verb>': the guard hook, which needs stdin,
// or the skills core, which does not. Every exit(n) is followed by a return,
// because tests inject a non-terminating exit.
func runSkillsWithStdin(args []string, stdin io.Reader, stdout, stderr io.Writer, exit func(int)) {
	if len(args) > 0 && args[0] == skillsGuardVerb {
		runSkillsGuardHook(args[1:], stdin, stdout, stderr, exit)
		return
	}
	runSkillsCore(verbFromArgs(args), args, stdout, stderr, exit)
}

// runSkillsGuardHook implements 'skills guard-hook'. From the moment the command
// line has been understood, nothing it does can change the exit code from 0.
func runSkillsGuardHook(args []string, stdin io.Reader, stdout, stderr io.Writer, exit func(int)) {
	if len(args) > 0 {
		fmt.Fprintf(stderr, "error: skills guard-hook: unexpected argument %q (it takes none; the hook input is read from stdin)\n", args[0])
		exit(1)
		return
	}
	// A Go panic ends the process with status 2, which Claude Code reads as "block
	// this tool call". The decision is pure and is not expected to panic, but the
	// cost of being wrong is a session that cannot run a command, so it is caught,
	// reported in full on stderr (which Claude Code shows only in verbose mode),
	// shown to the user as one short sanitized systemMessage (an allow with a
	// warning, never a denial, so a guard that stopped guarding does not look like
	// one that allowed), and turned into the same exit 0 as every other failure.
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(stderr, "skills guard-hook: internal error: %v\n", r)
			if out, err := (projection.GateResult{Warning: guardPanicWarning(r)}).PreToolUseOutput(); err == nil {
				_, _ = stdout.Write(out) // nothing to do if the write fails: the call goes through.
			}
			exit(0)
		}
	}()
	// One byte past the bound is enough to see the input is over it, and no more
	// of an endless input is ever read. Input over the bound is not judged.
	raw, err := io.ReadAll(io.LimitReader(stdin, skills.ApproveGuardMaxInputBytes+1))
	if err != nil {
		exit(0)
		return
	}
	if beforeApproveGuardDecision != nil {
		beforeApproveGuardDecision()
	}
	verdict := skills.DecideApproveGuard(raw)
	if verdict.Deny {
		out, err := projection.GateResult{Deny: true, Reason: verdict.Reason}.PreToolUseOutput()
		if err == nil {
			_, _ = stdout.Write(out) // nothing to do if the write fails: the call goes through.
		}
	}
	exit(0)
}

// guardPanicWarning is the one line the user sees when the guard recovered from a
// panic: it names the guard and says what happened to the call. The panic value is
// sanitized and bounded by projection.PanicText, so it can never spread over lines.
func guardPanicWarning(recovered any) string {
	return "labdrian: the skills approve guard hit an internal error (" + projection.PanicText(recovered) +
		"); this tool call was not checked for 'skills approve' and was not denied."
}
