package reviewreceipt

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// acknowledgeMarker is the exact substring RunHook matches inside
// tool_input.command to recognize the acknowledge-approved invocation. The
// hook never parses the lineage or executes anything of its own beyond
// Capture -- it only string-matches this marker, per the threat matrix
// (Subprocess boundary): a look-alike command (e.g. `echo
// "gentle-ai review acknowledge-approved"` in a comment or log line) still
// matches, which is intentionally conservative -- a false-positive capture
// is harmless (idempotent), while a false negative would let an
// acknowledgement burn an uncaptured receipt.
const acknowledgeMarker = "gentle-ai review acknowledge-approved"

// hookInput is the subset of the Claude Code PreToolUse Bash hook JSON
// shape RunHook needs.
type hookInput struct {
	ToolName  string `json:"tool_name"`
	ToolInput struct {
		Command string `json:"command"`
	} `json:"tool_input"`
}

// RunHook implements the fail-closed PreToolUse Bash hook: it reads the raw
// hook input JSON and, only when tool_input.command contains
// acknowledgeMarker, resolves the single active change and captures every
// surviving approved receipt before returning allow.
//
// Returns (exitCode, message):
//   - (0, "") -- allow: the command does not match, there is no
//     openspec/changes/ directory, or there is no active change to attach a
//     receipt to.
//   - (0, "") -- allow: exactly one active change existed and Capture
//     succeeded.
//   - (2, message) -- deny: more than one active change exists, or Capture
//     itself failed. Fail-closed: the acknowledgement must never proceed
//     without a persisted receipt when capture could not be resolved
//     unambiguously.
//
// Malformed or empty input is treated the same as a non-matching command --
// pass through -- because a hook that cannot even see a command is not
// looking at an acknowledge-approved invocation in the first place.
func RunHook(rawInput []byte, repoRoot string) (exitCode int, message string) {
	var hi hookInput
	if err := json.Unmarshal(rawInput, &hi); err != nil {
		return 0, ""
	}
	if !strings.Contains(hi.ToolInput.Command, acknowledgeMarker) {
		return 0, ""
	}

	change, err := DetectActiveChange(repoRoot)
	if err != nil {
		var multi *MultipleActiveChangesError
		if errors.As(err, &multi) {
			return 2, multi.Error()
		}
		return 2, fmt.Sprintf("review-receipt: %v", err)
	}
	if change == "" {
		return 0, ""
	}

	if _, err := Capture(repoRoot, change); err != nil {
		return 2, fmt.Sprintf("review-receipt: capture failed: %v", err)
	}
	return 0, ""
}
