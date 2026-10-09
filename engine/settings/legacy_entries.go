package settings

import (
	"fmt"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/guardmarkers"
)

// The builders below write the entries of the five hook families the overlay installs besides the
// projection and approve guard families (those two build theirs next to their identity, in
// projection.go and approve_guard.go): the minimalism pair, the anti-generic-design pair, the
// SessionEnd sync-trigger entry, the PreToolUse/Bash review-receipt entry, and the shaper clearance
// guard. Each entry is the exact JSON shape Claude Code reads, and the command inside it is the
// text an installed settings.json holds, so a change here is a change to every machine's file.

// minimalismFamily is the minimalism-contract pair: the UserPromptSubmit entry that propagates
// the contract into the project's registry, and the PreToolUse/Agent entry that injects its path
// into sub-agent prompts. An installed entry is kept as it is (keepingOne), so an older command
// line stays until the owner decides how an installed entry is upgraded.
var minimalismFamily = hookFamily{
	identity: LabdrianMinimalismIdentity,
	specs: []hookSpec{
		{event: "UserPromptSubmit"},
		{event: "PreToolUse", matcher: "Agent"},
	},
	build:  buildMinimalismEntry,
	upkeep: keepingOne,
}

// buildMinimalismEntry returns the entry of the minimalism family for one spec.
func buildMinimalismEntry(hookCommand string, s hookSpec) map[string]interface{} {
	if s.event == "UserPromptSubmit" {
		return buildUserPromptSubmitEntry(hookCommand)
	}
	return buildPreToolUseEntry(hookCommand)
}

// designFamily is the anti-generic-design pair, shaped like the minimalism pair and told apart from
// it by its own identity, because both pairs run the same binary.
var designFamily = hookFamily{
	identity: LabdrianDesignIdentity,
	specs: []hookSpec{
		{event: "UserPromptSubmit"},
		{event: "PreToolUse", matcher: "Agent"},
	},
	build:  buildDesignEntry,
	upkeep: keepingOne,
}

// buildDesignEntry returns the entry of the design family for one spec.
func buildDesignEntry(hookCommand string, s hookSpec) map[string]interface{} {
	if s.event == "UserPromptSubmit" {
		return buildDesignUserPromptSubmitEntry(hookCommand)
	}
	return buildDesignPreToolUseEntry(hookCommand)
}

// buildUserPromptSubmitEntry returns the hook entry for propagate.
// The hook runs 'gentle-ai-overlay propagate' to ensure the scoped block
// exists in the current project's .atl/skill-registry.md.
//
// VERIFIED SHAPE (Claude Code docs):
//
//	{"hooks":[{"type":"command","command":"<bash>"}]}
//
// Missing-binary safety: the command uses 'bash -c' with a guard:
//
//	command -v <binary> &>/dev/null && <binary> ... || true
//
// This ensures the hook exits 0 even if the binary is absent, so no Agent
// call is ever blocked by a missing binary.
func buildUserPromptSubmitEntry(hookCommand string) map[string]interface{} {
	// CLAUDE_PROJECT_DIR is the env var Claude Code sets for hooks to point at
	// the project root. The :- fallback to "." keeps the command functional
	// when the env var is absent (e.g. local testing), but the primary path
	// is always anchored to the project root when Claude Code fires the hook.
	cmd := fmt.Sprintf(
		`command -v %s &>/dev/null && %s propagate --registry "${CLAUDE_PROJECT_DIR:-.}/.atl/skill-registry.md" --contract-file ~/.claude/skills/_shared/minimalism-contract.md || true`,
		hookCommand, hookCommand,
	)
	return map[string]interface{}{
		"hooks": []interface{}{map[string]interface{}{
			"type":    "command",
			"command": cmd,
		}},
	}
}

// buildPreToolUseEntry returns the hook entry for gate-task (matcher: Agent tool).
//
// VERIFIED SHAPE (Claude Code 2.1.185):
//
//	{"matcher":"Agent","hooks":[{"type":"command","command":"<bash>"}]}
//
// The matcher MUST be "Agent" (not "Task") — empirically verified: the sub-agent
// spawn tool is named "Agent" in CC 2.1.185. A "Task" matcher fires on nothing.
//
// F-PATH: the --contract-path flag passes the ABSOLUTE path ($HOME-expanded) so
// a sub-agent in any project cwd can read the contract file. The injected bare
// path line in the Agent prompt will be this absolute path.
//
// Missing-binary safety: same guard pattern as UserPromptSubmit.
func buildPreToolUseEntry(hookCommand string) map[string]interface{} {
	cmd := fmt.Sprintf(
		`command -v %s &>/dev/null && %s gate-task --contract-file ~/.claude/skills/_shared/minimalism-contract.md --contract-path "$HOME/.claude/skills/_shared/minimalism-contract.md" || true`,
		hookCommand, hookCommand,
	)
	return map[string]interface{}{
		"matcher": "Agent",
		"hooks": []interface{}{map[string]interface{}{
			"type":    "command",
			"command": cmd,
		}},
	}
}

// buildDesignUserPromptSubmitEntry returns the UserPromptSubmit entry that
// propagates the anti-generic-design guard. It uses --embedded-contract so the
// contract text ships in the engine binary (no external file dependency), and
// the --embedded-contract argument doubles as this entry's dedup/uninstall
// identity.
//
// Same missing-binary guard as the minimalism entry.
func buildDesignUserPromptSubmitEntry(hookCommand string) map[string]interface{} {
	cmd := fmt.Sprintf(
		`command -v %s &>/dev/null && %s propagate --registry "${CLAUDE_PROJECT_DIR:-.}/.atl/skill-registry.md" --embedded-contract %s || true`,
		hookCommand, hookCommand, embeddedDesignName,
	)
	return map[string]interface{}{
		"hooks": []interface{}{map[string]interface{}{
			"type":    "command",
			"command": cmd,
		}},
	}
}

// buildDesignPreToolUseEntry returns the PreToolUse/Agent entry that injects
// the anti-generic-design contract path into in-scope sub-agent prompts. The
// contract content is embedded (--embedded-contract); --contract-path is the
// absolute path the engine emits as the bare injected line so a sub-agent in
// any cwd can resolve it.
//
// Same missing-binary guard and matcher="Agent" as the minimalism entry.
func buildDesignPreToolUseEntry(hookCommand string) map[string]interface{} {
	cmd := fmt.Sprintf(
		`command -v %s &>/dev/null && %s gate-task --embedded-contract %s --contract-path "$HOME/.claude/skills/_shared/anti-generic-design.md" || true`,
		hookCommand, hookCommand, embeddedDesignName,
	)
	return map[string]interface{}{
		"matcher": "Agent",
		"hooks": []interface{}{map[string]interface{}{
			"type":    "command",
			"command": cmd,
		}},
	}
}

// syncTriggerFamily is the single SessionEnd entry that runs the sync-trigger at session close.
// Nothing is ever installed on Stop: SessionEnd fires once per session, Stop fires per turn.
var syncTriggerFamily = hookFamily{
	identity: LabdrianSyncTriggerIdentity,
	specs:    []hookSpec{{event: "SessionEnd"}},
	build:    buildSyncTriggerSessionEndEntry,
	upkeep:   keepingOne,
}

// reviewReceiptFamily is the single PreToolUse/Bash entry that captures review receipts before an
// acknowledge-approved invocation burns them.
var reviewReceiptFamily = hookFamily{
	identity: LabdrianReviewReceiptIdentity,
	specs:    []hookSpec{{event: "PreToolUse", matcher: "Bash"}},
	build:    buildReviewReceiptPreToolUseEntry,
	upkeep:   keepingOne,
}

// buildSyncTriggerSessionEndEntry returns the SessionEnd entry that invokes
// the shared sync-trigger runner at session close.
//
// VERIFIED SHAPE (Claude Code docs): SessionEnd supports a "matcher" field
// like PreToolUse, but omitting one (as here) matches every SessionEnd
// event, the same way a matcher-less UserPromptSubmit entry does:
//
//	{"hooks":[{"type":"command","command":"<bash>"}]}
//
// SessionEnd hooks run under a short default timeout, which is why
// synctrigger.Run detaches its child immediately and returns rather than
// waiting on the sync to finish.
//
// The --cwd fallback is "${CLAUDE_PROJECT_DIR:-$PWD}", not "-.": "." is a
// relative path, and synctrigger.Run rejects a non-absolute --cwd as
// error:usage before it even opens its log, so a relative fallback would
// silently disable the sync whenever CLAUDE_PROJECT_DIR is unset. $PWD is
// always absolute.
//
// Missing-binary safety: same guard pattern as the other entries, but
// POSIX ">/dev/null 2>&1" rather than the bash-only "&>/dev/null" -- Claude
// Code invokes hooks via "sh -c" (dash on most systems), where "&>" is not
// a redirection operator and the guard would be silently inert.
func buildSyncTriggerSessionEndEntry(hookCommand string, _ hookSpec) map[string]interface{} {
	cmd := fmt.Sprintf(
		`command -v %s >/dev/null 2>&1 && %s %s --event session-end --cwd "${CLAUDE_PROJECT_DIR:-$PWD}" || true`,
		hookCommand, hookCommand, LabdrianSyncTriggerIdentity,
	)
	return map[string]interface{}{
		"hooks": []interface{}{map[string]interface{}{
			"type":    "command",
			"command": cmd,
		}},
	}
}

// buildReviewReceiptPreToolUseEntry returns the PreToolUse/Bash entry that
// runs the fail-closed review-receipt capture hook before every Bash tool
// call.
//
// FAIL-CLOSED DEVIATION FROM THE OTHER HOOK ENTRIES: every other builder in
// this file guards the missing-binary case with
// "command -v X && X ... || true", which forces the shell command to exit 0
// no matter what the hook itself returns — appropriate for the fail-SAFE
// gate-task/propagate/sync-trigger hooks, which must never block a call.
// The review-receipt hook is the opposite: it must fail CLOSED, so a
// capture error blocks the acknowledge-approved Bash call (PreToolUse deny
// on non-zero exit). The command below only short-circuits to exit 0 when
// the binary itself is missing (the same "don't block on an absent
// installation" guard as every other entry); once the binary is found, its
// own exit code — 0 (allow) or 2 (deny) for the verdict of reviewreceipt.Service.CheckCommand — is the
// command's exit code, unmasked by "|| true".
func buildReviewReceiptPreToolUseEntry(hookCommand string, _ hookSpec) map[string]interface{} {
	cmd := fmt.Sprintf(
		`command -v %s >/dev/null 2>&1 || exit 0; %s %s hook --cwd "${CLAUDE_PROJECT_DIR:-$PWD}"`,
		hookCommand, hookCommand, LabdrianReviewReceiptIdentity,
	)
	return map[string]interface{}{
		"matcher": "Bash",
		"hooks": []interface{}{map[string]interface{}{
			"type":    "command",
			"command": cmd,
		}},
	}
}

// buildShaperGuardPreToolUseEntry returns one PreToolUse entry of the shaper
// clearance deny guard. Once the binary is found, its own exit code, 0
// (allow) or 2 (deny) from the shaper guard hook (shaper.DecideGuard), is the command's exit code.
//
// FAIL-CLOSED FOR THE GUARDED MARKERS: unlike the review-receipt entry, a
// missing binary does not simply exit 0. The command falls back to a POSIX
// case match over the raw hook input and still denies (exit 2) when it names
// the record entry point or the store path, and allows everything else, so a
// removed binary neither disables the guard nor blocks every tool call. The
// fallback does not collapse whitespace. Both paths match text only: this
// guard is a speed bump, not a security boundary.
func (m owner) buildShaperGuardPreToolUseEntry(matcher string) map[string]interface{} {
	cmd := fmt.Sprintf(
		`command -v %s >/dev/null 2>&1 || { case "$(cat)" in *'%s'*|*'%s'*) echo 'labdrian shaper clearance guard: gentle-ai-overlay is missing; denying a clearance record or store access (a speed bump, not a security boundary)' >&2; exit 2;; esac; exit 0; }; %s %s`,
		m.hookCommand, guardmarkers.Command, guardmarkers.Store, m.hookCommand, LabdrianShaperGuardIdentity,
	)
	return map[string]interface{}{
		"matcher": matcher,
		"hooks": []interface{}{map[string]interface{}{
			"type":    "command",
			"command": cmd,
		}},
	}
}
