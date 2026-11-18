package settings

import "fmt"

// The registry refresh family: the one UserPromptSubmit entry that runs the engine's 'registry refresh' for the project
// Claude Code names, so the table of skills of its .atl/skill-registry.md is current when a prompt is sent and no
// gentle-ai hook is needed for it (labdrian-orchestrator-opencode T30 commit d, S17).
//
// The family is owned, repaired, and removed as a unit by hookFamily, the same implementation every other family uses: the
// binary path plus one identity token tell its entry from foreign ones, install replaces a drifted or duplicated entry and
// never touches a foreign one (the refresh hook of gentle-ai and the wrapper of the codegraph prompt hook included), and
// uninstall removes exactly this entry. It sits on UserPromptSubmit beside the propagate entries; the refresh writes the
// table and leaves the blocks propagate keeps, and both take the same lock on the registry.

const (
	// LabdrianRegistryRefreshIdentity is the verb and the dedup/uninstall identity token of the family. Combined with the
	// installed binary path it names exactly the entry this family owns, so install and uninstall never touch a foreign
	// entry, even one that mentions the token while running another program (gentle-ai's own 'skill-registry refresh').
	LabdrianRegistryRefreshIdentity = "registry refresh"
)

// registryRefreshFamily is the whole family: one UserPromptSubmit entry. UserPromptSubmit has no matcher.
var registryRefreshFamily = hookFamily{
	identity: LabdrianRegistryRefreshIdentity,
	specs:    []hookSpec{{event: "UserPromptSubmit"}},
	build:    buildRegistryRefreshEntry,
}

// buildRegistryRefreshEntry returns the settings entry for the spec of the family.
//
// NEVER-BLOCKING COMMAND, AND A MISSING BINARY IS A NO-OP. The refresh is a convenience: its exit is forced to 0
// ("|| true"), so an error in it can never block a prompt, and a missing binary is reported by status-hooks, not enforced
// here. --quiet keeps it silent, because a UserPromptSubmit hook's stdout is added to the prompt's context. Claude Code
// runs hooks with "sh -c", so the command uses POSIX redirection, not "&>"; CLAUDE_PROJECT_DIR is the project and the
// working directory is the fallback.
func buildRegistryRefreshEntry(hookCommand string, _ hookSpec) map[string]interface{} {
	bin := shellQuote(hookCommand)
	cmd := fmt.Sprintf(
		`command -v %s >/dev/null 2>&1 && %s %s --cwd "${CLAUDE_PROJECT_DIR:-$PWD}" --quiet || true`,
		bin, bin, LabdrianRegistryRefreshIdentity,
	)
	return map[string]interface{}{
		"hooks": []interface{}{map[string]interface{}{
			"type":    "command",
			"command": cmd,
		}},
	}
}

// MissingRegistryRefreshHookParts names every part of the family that is missing or has drifted in root, in a fixed order,
// and returns nil when the family is exactly what Install writes; see hookFamily.missingParts.
func MissingRegistryRefreshHookParts(root map[string]interface{}, hookCommand string) []string {
	return registryRefreshFamily.missingParts(root, hookCommand)
}

// HasRegistryRefreshHook reports whether the registry refresh hook is exactly in place: its entry present once, not drifted.
func HasRegistryRefreshHook(root map[string]interface{}, hookCommand string) bool {
	return len(MissingRegistryRefreshHookParts(root, hookCommand)) == 0
}

// RegistryRefreshHookEntries returns, per event key, exactly the entries Install writes for the registry refresh hook with
// hookCommand as the binary. Fixtures and status checks use it so they never retype the entry shape.
func RegistryRefreshHookEntries(hookCommand string) map[string][]interface{} {
	return registryRefreshFamily.entries(hookCommand)
}
