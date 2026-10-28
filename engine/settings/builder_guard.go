package settings

import "fmt"

// The builder guard family: the one PreToolUse entry on the Agent tool that runs the engine's 'roles builder-guard',
// which denies launching the builder agent when its prompt does not cite a task that the shaper marked as designed in the
// feature document (own-rdd T11d, S14: the builder never designs; see engine/cmd/roles_builder_guard.go for the contract
// with Claude Code, and engine/oddtask for what it decides).
//
// The family is owned, repaired, and removed as a unit by hookFamily, the same implementation the projection, approve
// guard and review stop-hook families use: the binary path plus one identity token tell its entry from foreign ones,
// install replaces a drifted or duplicated entry and never touches a foreign one, and uninstall removes exactly this
// entry. It shares the Agent matcher with the minimalism and design entries (gate-task), which are other families with
// other identities: neither claims, repairs, or removes the other's entry. When both answer one Agent call, Claude Code
// takes the strictest decision (deny, then defer, ask, allow; hooks reference), so the gate's allow with an updated
// prompt never lifts this guard's deny.

const (
	// LabdrianBuilderGuardIdentity is the hook verb and the dedup/uninstall identity token of the family. Combined with
	// the installed binary path it names exactly the entry this family owns, so install and uninstall never touch a
	// foreign Agent entry, even one that runs our binary with another verb or mentions the token while running another
	// program.
	LabdrianBuilderGuardIdentity = "roles builder-guard"
	// BuilderGuardMatcher is the PreToolUse matcher of the entry: the tool that launches a sub-agent.
	BuilderGuardMatcher = "Agent"
)

// builderGuardFamily is the whole family: one PreToolUse entry on the Agent tool.
var builderGuardFamily = hookFamily{
	identity: LabdrianBuilderGuardIdentity,
	specs:    []hookSpec{{event: "PreToolUse", matcher: BuilderGuardMatcher}},
	build:    buildBuilderGuardEntry,
}

// buildBuilderGuardEntry returns the settings entry for the spec of the family.
//
// NEVER-BLOCKING COMMAND, AND A MISSING BINARY IS A NO-OP. The hook reports a denial as JSON on stdout and exits 0, so the
// command forces exit 0 the same way the approve guard entries do ("|| true"): a crash or a usage error in the hook can
// never block an Agent call by the exit code of 2, which would hand its stderr to the agent as the reason. A missing
// binary lets the launch through and is reported by status-hooks, not enforced here (L39, D2): a text fallback could not
// read the citation of a prompt out of the raw hook JSON and would deny every builder launch. Claude Code runs hooks with
// "sh -c", so the command uses POSIX redirection, not "&>".
func buildBuilderGuardEntry(hookCommand string, s hookSpec) map[string]interface{} {
	bin := shellQuote(hookCommand)
	cmd := fmt.Sprintf(`command -v %s >/dev/null 2>&1 && %s %s || true`, bin, bin, LabdrianBuilderGuardIdentity)
	return map[string]interface{}{
		"matcher": s.matcher,
		"hooks": []interface{}{map[string]interface{}{
			"type":    "command",
			"command": cmd,
		}},
	}
}

// MissingBuilderGuardParts names every part of the family that is missing or has drifted in root, in a fixed order, and
// returns nil when the family is exactly what Install writes; see hookFamily.missingParts.
func MissingBuilderGuardParts(root map[string]interface{}, hookCommand string) []string {
	return builderGuardFamily.missingParts(root, hookCommand)
}

// HasBuilderGuard reports whether the builder guard is exactly in place: its entry present once, not drifted.
func HasBuilderGuard(root map[string]interface{}, hookCommand string) bool {
	return len(MissingBuilderGuardParts(root, hookCommand)) == 0
}

// BuilderGuardEntries returns, per event key, exactly the entries Install writes for the builder guard with hookCommand
// as the binary. Fixtures and status checks use it so they never retype the entry shape.
func BuilderGuardEntries(hookCommand string) map[string][]interface{} {
	return builderGuardFamily.entries(hookCommand)
}
