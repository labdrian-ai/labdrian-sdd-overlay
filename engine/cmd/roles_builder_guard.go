package main

// roles subcommand, verb 'builder-guard': the Claude Code PreToolUse hook on the Agent tool that denies
// launching the `builder` agent when its prompt does not cite a task of a feature document that the shaper
// designed (own-rdd S14: the builder never designs). It is an internal hook command, not one a person runs:
// Claude Code starts it, hands it the tool call as JSON on stdin, and reads what it prints.
//
// The decision is engine/oddtask's, a pure function of the prompt and the feature document; this file only
// reads stdin (bounded by hookwire.MaxAgentCallBytes), has hookwire read it into a call, hands the prompt of
// a builder launch to the use case of engine/oddtask/app, and has hookwire write the denial. It is a speed
// bump against forgetting the shaper, not a security boundary.
//
// The contract with Claude Code is the approve guard's (skills guard-hook):
//
//   - stdout is empty, or exactly one JSON object: to deny, {"hookSpecificOutput":{"hookEventName":
//     "PreToolUse","permissionDecision":"deny","permissionDecisionReason":"..."}}, and to allow, nothing. It
//     never prints permissionDecision "allow", which would bypass Claude Code's normal permission flow.
//   - The exit code is 0 always, except 1 for a command line that is not understood. A denial is the JSON
//     above with exit 0; it is never exit 2.
//
// It fails closed for the builder and open for the rest. Once the call is decoded and its subagent_type is
// exactly "builder", every failure denies: a document that cannot be read, a prompt that cites nothing, a
// panic. Input it cannot decode, or a stdin that fails, cannot be told to be a builder launch and is allowed,
// with one line on stderr: denying it would block every Agent call of the session. Input over the bound is
// denied whatever the agent, because the agent controls the length of its own prompt. Every other agent is
// allowed with no output and no file read.

import (
	"errors"
	"fmt"
	"io"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/hookwire"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/oddtask"
	oddapp "github.com/labdrian-ai/labdrian-sdd-overlay/engine/oddtask/app"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
)

const (
	rolesBuilderGuardVerb = "builder-guard"
	// builderAgent is the subagent_type of the agent the guard keeps from designing.
	builderAgent = "builder"
)

// runRolesBuilderGuard implements 'roles builder-guard'. Every exit(n) is followed by a return, because tests
// inject a non-terminating exit. From the moment the command line has been understood, nothing it does can
// change the exit code from 0.
func runRolesBuilderGuard(args []string, stdin io.Reader, stdout, stderr io.Writer, exit func(int), docs oddapp.Documents) {
	if len(args) > 0 {
		fmt.Fprintf(stderr, "error: roles builder-guard: unexpected argument %q (it takes none; the hook input is read from stdin)\n", args[0])
		exit(1)
		return
	}
	// A Go panic ends the process with status 2, which Claude Code reads as "block this tool call". A panic of
	// a builder launch is a failure to check it, and is denied as one; a panic before the call is known to be a
	// builder launch lets the call through, like any input that cannot be told to be one.
	builder := false
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(stderr, "roles builder-guard: internal error: %s\n", projection.PanicText(r))
			if builder {
				denyBuilderLaunch(stdout, oddtask.ReasonInternal)
			}
			exit(0)
		}
	}()
	// One byte past the bound is enough to see the input is over it, and no more of an endless input is read.
	raw, err := io.ReadAll(io.LimitReader(stdin, hookwire.MaxAgentCallBytes+1))
	if err != nil {
		fmt.Fprintf(stderr, "roles builder-guard: cannot read the hook input (%v); this Agent call was not checked.\n", err)
		exit(0)
		return
	}
	call, err := hookwire.DecodeAgentCall(raw)
	switch {
	case errors.Is(err, hookwire.ErrTooLarge):
		denyBuilderLaunch(stdout, oddtask.TooLargeReason(hookwire.MaxAgentCallBytes))
		exit(0)
		return
	case err != nil:
		fmt.Fprintf(stderr, "roles builder-guard: cannot decode the hook input (%v); this Agent call was not checked.\n", err)
		exit(0)
		return
	case call.SubagentType != builderAgent:
		exit(0)
		return
	}
	builder = true
	if reason := oddapp.New(docs).Check(call.Prompt, call.Cwd); reason != "" {
		denyBuilderLaunch(stdout, reason)
	}
	exit(0)
}

// denyBuilderLaunch writes the denial for reason. A write that fails changes nothing: the call goes through.
func denyBuilderLaunch(stdout io.Writer, reason oddtask.Reason) {
	out, err := hookwire.PreToolUseReply{Deny: true, Reason: oddtask.Message(reason)}.Encode()
	if err == nil {
		_, _ = stdout.Write(out)
	}
}
