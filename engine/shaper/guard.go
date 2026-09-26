package shaper

import (
	"encoding/json"
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

// guardHookInput is the subset of a Claude Code PreToolUse hook input the
// guard reads: a Bash command and the path fields of the file-writing tools.
type guardHookInput struct {
	ToolName  string `json:"tool_name"`
	ToolInput struct {
		Command      string `json:"command"`
		FilePath     string `json:"file_path"`
		NotebookPath string `json:"notebook_path"`
	} `json:"tool_input"`
}

// RunGuardHook implements the Claude Code PreToolUse clearance deny guard,
// following reviewreceipt.RunHook. It returns (0, "") to allow and
// (2, message) to deny. It denies when a Bash command names the record entry
// point or the store path, or when a file tool's path lies in the store.
// Unlike RunHook it fails closed: input it cannot decode is denied, because
// a guard that cannot see the call cannot vouch for it. File contents are
// not inspected, so writing documentation that mentions the entry point is
// allowed.
func RunGuardHook(rawInput []byte) (exitCode int, message string) {
	var in guardHookInput
	if err := json.Unmarshal(rawInput, &in); err != nil {
		return 2, guardDenyMessage + " (hook input could not be decoded: " + err.Error() + ")"
	}
	if GuardMatches(in.ToolInput.Command) ||
		strings.Contains(in.ToolInput.FilePath, GuardStoreMarker) ||
		strings.Contains(in.ToolInput.NotebookPath, GuardStoreMarker) {
		return 2, guardDenyMessage
	}
	return 0, ""
}
