package main

import (
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings"
)

// checkProjectionHooks reports whether the projection hook family (the
// UserPromptSubmit context projection and the two PreToolUse gates) is exactly
// in place in Claude Code settings. Like the other post-upgrade families, a
// missing or drifted part is WARN/degraded with the same remediation, and an
// unreadable settings file is a hard FAIL. hookCommand is the installed binary
// path, because the entries are matched exactly, not by substring alone.
//
// Installing the hooks is not enough for a running session: Claude Code loads
// hook changes only when it starts, so the note says to restart it.
func checkProjectionHooks(root map[string]interface{}, settingsErr error, settingsPath, hookCommand string) checkResult {
	label := "hooks: projection (UserPromptSubmit + PreToolUse gates)"
	if settingsErr != nil {
		return checkResult{label: label, ok: false, note: "cannot read " + settingsPath + ": " + settingsErr.Error()}
	}
	const restart = "restart Claude Code to load hook changes"
	if root == nil {
		return checkResult{label: label, ok: true, degraded: true, note: settingsPath + " absent or empty; " + remediationNote + "; " + restart}
	}
	if missing := settings.MissingProjectionHookParts(root, hookCommand); len(missing) > 0 {
		return checkResult{label: label, ok: true, degraded: true,
			note: "missing or drifted: " + strings.Join(missing, ", ") + "; " + remediationNote + "; " + restart}
	}
	return checkResult{label: label, ok: true, note: "installed; " + restart + " if this session started earlier"}
}
