package shaper

import (
	"fmt"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/guardmarkers"
)

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
	return strings.Contains(normalized, guardmarkers.Command) || strings.Contains(text, guardmarkers.Store)
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
		strings.Contains(call.FilePath, guardmarkers.Store) ||
		strings.Contains(call.NotebookPath, guardmarkers.Store) {
		return GuardVerdict{Deny: true, Reason: guardDenyMessage}
	}
	return GuardVerdict{}
}

// GuardUnreadable is the verdict for a call the caller could not read. The guard fails closed,
// unlike the guard of the approval record, which fails open for input it cannot decode: a guard
// that cannot see the call cannot vouch for it, and the markers are narrow enough to make that
// affordable. The denial is one sentence that says what the guard could not read and what to do,
// the same for every shape the call can have: it names nothing of the decoder or of the types
// the call is read into.
func GuardUnreadable() GuardVerdict {
	return GuardVerdict{Deny: true, Reason: guardDenyMessage +
		" (this tool call could not be read as a command or a file path, so the guard denied it; send it again as a well-formed tool call)"}
}

// GuardTooLarge is the verdict for a call longer than the bound bytes the caller reads. It is
// denied like a call that cannot be read: the guard cannot vouch for a call it was not given to
// judge. The denial names the bound and what to do.
func GuardTooLarge(bound int) GuardVerdict {
	return GuardVerdict{Deny: true, Reason: fmt.Sprintf("%s (this tool call is larger than the %d bytes the guard reads, so the guard denied it; send a smaller call)",
		guardDenyMessage, bound)}
}
