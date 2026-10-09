package settings

import "fmt"

// The approve guard family: the two PreToolUse entries that run the engine's
// 'skills guard-hook', which denies the AGENT running `skills approve` and
// writing a skill's approval record by hand. Approval is a human step; the
// guard is a speed bump, not a security boundary (see engine/skills/
// approve_guard.go for what it matches and how it can be bypassed).
//
// The family is owned, repaired, and removed as a unit by hookFamily, the same
// implementation the projection family uses: the binary path plus one identity
// token tell its entries from foreign ones, install replaces a drifted or
// duplicated entry and never touches a foreign one, and uninstall removes
// exactly these entries.

const (
	// LabdrianApproveGuardIdentity is the guard's hook verb and the
	// dedup/uninstall identity token of the family. Combined with the installed
	// binary path it names exactly the entries this family owns, so install and
	// uninstall never touch a foreign entry, even one that shares the event key or
	// a matcher.
	LabdrianApproveGuardIdentity = "skills guard-hook"
	// ApproveGuardBashMatcher is the PreToolUse matcher of the entry that watches
	// Bash commands for the approve verb.
	ApproveGuardBashMatcher = "Bash"
	// ApproveGuardFileToolMatcher is the PreToolUse matcher of the entry that
	// watches the file-edit tools for a write to an approval record. It lists the
	// tools engine/skills' ApproveGuardFileTools names; a test checks the copy.
	ApproveGuardFileToolMatcher = "Write|Edit|MultiEdit|NotebookEdit"
)

// approveGuardFamily is the whole family in install order: one PreToolUse entry
// per matcher. Both run the same command; the hook tells the tools apart by the
// tool name in its input.
var approveGuardFamily = hookFamily{
	identity: LabdrianApproveGuardIdentity,
	specs: []hookSpec{
		{event: "PreToolUse", matcher: ApproveGuardBashMatcher},
		{event: "PreToolUse", matcher: ApproveGuardFileToolMatcher},
	},
	build: buildApproveGuardEntry,
}

// buildApproveGuardEntry returns the settings entry for one spec.
//
// NEVER-BLOCKING COMMAND, AND A MISSING BINARY IS A NO-OP. The hook reports a
// denial as JSON on stdout and exits 0, so the command forces exit 0 the same
// way the projection entries do ("|| true"): a crash or a usage error in the hook
// can never block a Bash or file-edit call. That is a deliberate difference from
// the shaper clearance guard, whose entries fall back to a POSIX text match and
// still deny when the binary is missing. Here a fallback could only match the
// raw hook JSON for "skills approve", and that phrase is in the content of the
// documentation, tests, and help text the agent legitimately writes, so a
// fail-closed fallback would deny ordinary edits whenever the binary is missing.
// Nothing is lost by failing open: the entry points cannot approve without the
// binary (the labdrian wrapper stops when this very path is not executable). A
// missing binary is reported by status-hooks, not enforced here. Claude Code runs
// hooks with "sh -c", so the guard uses POSIX redirection, not "&>".
func buildApproveGuardEntry(hookCommand string, s hookSpec) map[string]interface{} {
	cmd := fmt.Sprintf(
		`command -v %s >/dev/null 2>&1 && %s %s || true`,
		hookCommand, hookCommand, LabdrianApproveGuardIdentity,
	)
	return map[string]interface{}{
		"matcher": s.matcher,
		"hooks": []interface{}{map[string]interface{}{
			"type":    "command",
			"command": cmd,
		}},
	}
}

// isApproveGuardEntry reports whether a hook entry belongs to the approve guard
// family: the Merger-level name every other entry kind is reached through (see
// isProjectionEntry), so the uninstall filter and the merge read alike.
func (m owner) isApproveGuardEntry(e interface{}) bool {
	return approveGuardFamily.owns(e, m.hookCommand)
}

// mergeApproveGuard makes the approve guard family exactly the desired entries;
// see hookFamily.merge.
func (m owner) mergeApproveGuard(hooks map[string]interface{}) bool {
	return approveGuardFamily.merge(hooks, m.hookCommand)
}

// MissingApproveGuardParts names every part of the approve guard that is missing
// or has drifted in root, in a fixed order, and returns nil when the guard is
// exactly what Install writes and hooks are not globally disabled; see
// hookFamily.missingParts. Global disabling is reported because a guard that is
// installed but never run guards nothing.
func MissingApproveGuardParts(root map[string]interface{}, hookCommand string) []string {
	parts := approveGuardFamily.missingParts(root, hookCommand)
	if disabled, _ := root["disableAllHooks"].(bool); disabled {
		parts = append(parts, "hooks enabled (disableAllHooks is true)")
	}
	return parts
}

// HasApproveGuard reports whether the approve guard is fully in place: both
// entries present once, none drifted, and hooks not globally disabled. Even so
// it is a speed bump, not a security boundary.
func HasApproveGuard(root map[string]interface{}, hookCommand string) bool {
	return len(MissingApproveGuardParts(root, hookCommand)) == 0
}

// ApproveGuardHookEntries returns, per event key, exactly the entries Install
// writes for the approve guard with hookCommand as the binary. Fixtures and
// status checks use it so they never retype the entry shape.
func ApproveGuardHookEntries(hookCommand string) map[string][]interface{} {
	return approveGuardFamily.entries(hookCommand)
}
