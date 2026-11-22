package main

// roles subcommand, verb 'classifier-guard': the Claude Code PreToolUse hook that keeps the classifier agent to
// its one page and its one script (merge-playbook-into-sdd-overlay S5, T3c). It is an internal hook command, not
// one a person runs: the PreToolUse hook in the frontmatter of agents/classifier.md starts it, hands it the tool
// call as JSON on stdin, and reads its exit status.
//
// The decision is engine/classifierguard's, a pure function of the call; this file only reads stdin (bounded by
// hookwire.MaxToolCallBytes), has hookwire decode it, and answers with hookwire.ExitReply: exit 0 and no output to
// allow, exit 2 with the reason on stderr to block. It fails closed: an argument, a failing stdin, input that does
// not decode, input over the bound and a recovered panic each exit 2 with one line on stderr. That is safe because
// the hook is declared only in the classifier's own file, so a block never reaches another agent.

import (
	"fmt"
	"io"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/classifierguard"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/hookwire"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
)

const rolesClassifierGuardVerb = "classifier-guard"

// runRolesClassifierGuard implements 'roles classifier-guard'. Every exit(n) is followed by a return, because
// tests inject a non-terminating exit.
func runRolesClassifierGuard(args []string, stdin io.Reader, stdout, stderr io.Writer, exit func(int)) {
	blockLine := func(format string, a ...any) {
		fmt.Fprintf(stderr, "roles classifier-guard: "+format+"; this tool call is blocked.\n", a...)
		exit(hookwire.ExitBlock)
	}
	if len(args) > 0 {
		blockLine("unexpected argument %q (it takes none; the hook input is read from stdin)", args[0])
		return
	}
	// A Go panic ends the process with status 2, which is a block too; this one says why, in one line.
	defer func() {
		if r := recover(); r != nil {
			blockLine("internal error: %s", oneLine(projection.PanicText(r)))
		}
	}()
	// One byte past the bound is enough to see the input is over it, and no more of an endless input is read.
	raw, err := io.ReadAll(io.LimitReader(stdin, hookwire.MaxToolCallBytes+1))
	if err != nil {
		blockLine("cannot read the hook input (%s)", oneLine(err.Error()))
		return
	}
	call, err := hookwire.DecodeToolCall(raw)
	if err != nil {
		blockLine("cannot decode the hook input (%s)", oneLine(err.Error()))
		return
	}
	verdict := classifierguard.Decide(classifierguard.Call{
		Tool: call.Name, Command: call.Command, FilePath: call.FilePath, NotebookPath: call.NotebookPath,
	})
	reply := hookwire.ExitReply{Block: verdict.Block, Message: verdict.Reason}
	_, _ = stderr.Write(reply.MessageLine())
	exit(reply.Code())
}

// oneLine makes text one line, so a stderr message stays one.
func oneLine(text string) string {
	return strings.Join(strings.Fields(text), " ")
}
