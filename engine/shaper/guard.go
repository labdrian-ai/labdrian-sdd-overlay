package shaper

import (
	"strings"
)

// GuardCommandMarker is the clearance record entry point the runtime deny
// guards refuse to let a model run. It is matched anywhere in a command
// after whitespace is collapsed.
const GuardCommandMarker = "shaper clearance record"

// GuardStoreMarker is the clearance store path segment the runtime deny
// guards refuse to let a model touch. It is the fixed store directory under
// the state home.
const GuardStoreMarker = "labdrian/shaper-clearance"

// guardDenyMessage explains a refusal to the model. The guards match text
// only, so they are speed bumps and not a security boundary.
const guardDenyMessage = "labdrian shaper clearance guard: recording a clearance or touching the clearance store is reserved for the human " +
	"through the Pi /shaper-clear dialog; the model must not run it. This guard matches command and path text only, so it is a speed bump, " +
	"not a security boundary: any process running as the same OS user can still forge a clearance record, and a clearance is not a signature."

// GuardMatches reports whether text names the clearance record entry point
// or the clearance store path. Runs of whitespace and shell line
// continuations are collapsed first, so spacing alone cannot hide the entry
// point. Quoting, variables, encodings, or a script written to a file can
// still evade it.
func GuardMatches(text string) bool {
	normalized := strings.Join(strings.Fields(strings.ReplaceAll(text, "\\\n", " ")), " ")
	return strings.Contains(normalized, GuardCommandMarker) || strings.Contains(text, GuardStoreMarker)
}

// GuardCall is the tool call the clearance guard decides about: the command a shell tool is
// about to run and the paths the file tools are about to write. A field the call does not have
// is empty. What is written to a file is not part of it: the guard does not inspect file
// contents, so writing documentation that mentions the entry point is allowed.
type GuardCall struct {
	// Command is the command of a shell tool.
	Command string
	// FilePath is the path of the file tools that write a file.
	FilePath string
	// NotebookPath is the path of the notebook tool.
	NotebookPath string
}

// GuardVerdict is the decision of the clearance guard for one call. Reason is set only for a
// denial, and is what the model is told.
type GuardVerdict struct {
	Deny   bool
	Reason string
}

// DecideGuard is the Claude Code PreToolUse clearance deny guard. It denies when a command
// names the record entry point or the store path, or when a file tool's path lies in the
// store. Everything else is allowed.
func DecideGuard(call GuardCall) GuardVerdict {
	if GuardMatches(call.Command) ||
		strings.Contains(call.FilePath, GuardStoreMarker) ||
		strings.Contains(call.NotebookPath, GuardStoreMarker) {
		return GuardVerdict{Deny: true, Reason: guardDenyMessage}
	}
	return GuardVerdict{}
}

// GuardUnreadable is the verdict for a call the caller could not read, with detail saying why. The
// guard fails closed, unlike the guard of the approval record, which fails open: input it cannot
// decode is denied, because a guard that cannot see the call cannot vouch for it. The markers
// are narrow enough to make that affordable.
func GuardUnreadable(detail string) GuardVerdict {
	return GuardVerdict{Deny: true, Reason: guardDenyMessage + " (hook input could not be decoded: " + detail + ")"}
}
