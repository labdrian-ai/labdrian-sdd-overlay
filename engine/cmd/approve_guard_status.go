package main

import (
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings"
)

// checkApproveGuard reports whether the skills approve guard (the two
// PreToolUse entries that run 'skills guard-hook') is exactly in place in Claude
// Code settings, and hooks are not globally disabled. Like the other
// post-upgrade families, a missing or drifted part is WARN/degraded with the same
// remediation, and an unreadable settings file is a hard FAIL. hookCommand is
// the installed binary path, because the entries are matched exactly, not by
// substring alone.
//
// Installing the hooks is not enough for a running session: Claude Code loads
// hook changes only when it starts, so the note says to restart it. And even
// installed, the guard is a speed bump, not a security boundary, so the note
// says that too.
func checkApproveGuard(root map[string]interface{}, settingsErr error, settingsPath, hookCommand string) checkResult {
	label := "guard: skills approve (PreToolUse Bash + file tools)"
	if settingsErr != nil {
		return checkResult{label: label, ok: false, note: "cannot read " + settingsPath + ": " + settingsErr.Error()}
	}
	const limit = "the guard is a speed bump, not a security boundary"
	const restart = "restart Claude Code to load hook changes"
	if root == nil {
		return checkResult{label: label, ok: true, degraded: true,
			note: settingsPath + " absent or empty; the agent can run skills approve unguarded (" + limit + "); " + remediationNote + "; " + restart}
	}
	if missing := settings.MissingApproveGuardParts(root, hookCommand); len(missing) > 0 {
		return checkResult{label: label, ok: true, degraded: true,
			note: "missing or drifted: " + strings.Join(missing, ", ") + "; the agent can run skills approve unguarded (" + limit + "); " + remediationNote + "; " + restart}
	}
	return checkResult{label: label, ok: true, note: "installed; " + limit + "; " + restart + " if this session started earlier"}
}
