package settings

import "fmt"

const (
	// LabdrianProjectionIdentity is the projection hook verb and the
	// dedup/uninstall identity token of the projection family. Combined with the
	// installed binary path it names exactly the entries this family owns, so
	// install and uninstall never touch a foreign entry, even one that shares an
	// event key or a matcher.
	LabdrianProjectionIdentity = "projection hook"
	// ProjectionEditToolMatcher is the PreToolUse matcher of the entry that
	// runs the paused-workflow edit gate. It lists the same file-edit tools the
	// shaper guard lists.
	ProjectionEditToolMatcher = "Write|Edit|MultiEdit|NotebookEdit"
	// ProjectionMemoryQueryMatcher is the PreToolUse matcher of the entry that
	// runs the longterm-mem project gate. Claude Code reads a matcher holding
	// characters outside letters, digits, underscore, and pipe as a regular
	// expression, so this is the gate's own tool-name pattern: it covers
	// install-specific prefixes such as mcp__plugin_x_longterm-mem__query, and it
	// is not narrower than what the hook itself re-checks.
	ProjectionMemoryQueryMatcher = `^mcp__([A-Za-z0-9-]+_)*longterm-mem__query$`
)

// projectionFamily is the whole family in install order: one UserPromptSubmit
// entry and two PreToolUse entries.
var projectionFamily = hookFamily{
	identity: LabdrianProjectionIdentity,
	specs: []hookSpec{
		{event: "UserPromptSubmit"},
		{event: "PreToolUse", matcher: ProjectionEditToolMatcher},
		{event: "PreToolUse", matcher: ProjectionMemoryQueryMatcher},
	},
	build: buildProjectionEntry,
}

// buildProjectionEntry returns the settings entry for one spec.
//
// NEVER-BLOCKING COMMAND: the projection hooks report through JSON on stdout
// and exit 0; a denial is a JSON decision, not exit code 2. So the command
// forces exit 0 the same way the propagate and gate-task entries do
// ("|| true"), and a missing binary is a silent no-op. Claude Code runs hooks
// with "sh -c", so the guard uses POSIX redirection, not "&>".
func buildProjectionEntry(hookCommand string, s hookSpec) map[string]interface{} {
	cmd := fmt.Sprintf(
		`command -v %s >/dev/null 2>&1 && %s %s --event %s || true`,
		hookCommand, hookCommand, LabdrianProjectionIdentity, s.event,
	)
	entry := map[string]interface{}{
		"hooks": []interface{}{map[string]interface{}{
			"type":    "command",
			"command": cmd,
		}},
	}
	if s.matcher != "" {
		entry["matcher"] = s.matcher
	}
	return entry
}

// MissingProjectionHookParts names every part of the projection family that is
// missing or has drifted in root, in a fixed order, and returns nil when the
// family is exactly what Install writes; see hookFamily.missingParts.
func MissingProjectionHookParts(root map[string]interface{}, hookCommand string) []string {
	return projectionFamily.missingParts(root, hookCommand)
}

// HasProjectionHooks reports whether the projection family is exactly in place:
// every entry present once, none drifted.
func HasProjectionHooks(root map[string]interface{}, hookCommand string) bool {
	return len(MissingProjectionHookParts(root, hookCommand)) == 0
}

// ProjectionHookEntries returns, per event key, exactly the entries Install
// writes for the projection family with hookCommand as the binary. Fixtures and
// status checks use it so they never retype the entry shape.
func ProjectionHookEntries(hookCommand string) map[string][]interface{} {
	return projectionFamily.entries(hookCommand)
}
