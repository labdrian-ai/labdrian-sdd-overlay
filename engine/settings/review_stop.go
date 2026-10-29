package settings

import "fmt"

// The review stop-hook family: the one Stop entry that runs the engine's 'review stop-hook', which blocks the stop of an
// agent whose branch has a commit that misses anything 'review check' asks of it, once for each session, commit and
// thing it misses, and names the command for each (own-rdd T6a, T6b6, S6, L18; see engine/cmd/review_stop_hook.go for
// the contract with Claude Code).
//
// The family is owned, repaired, and removed as a unit by hookFamily, the same implementation the projection and approve
// guard families use: the binary path plus one identity token tell its entry from foreign ones, install replaces a
// drifted or duplicated entry and never touches a foreign one, and uninstall removes exactly this entry. It is the only
// family on Stop, which fires every time the agent is about to stop; a foreign Stop entry shares the event and nothing
// else.

const (
	// LabdrianReviewStopIdentity is the hook verb and the dedup/uninstall identity token of the family. Combined with the
	// installed binary path it names exactly the entry this family owns, so install and uninstall never touch a foreign
	// Stop entry, even one that runs our binary with another verb or mentions the token while running another program.
	LabdrianReviewStopIdentity = "review stop-hook"
)

// reviewStopFamily is the whole family: one Stop entry. Stop has no matcher.
var reviewStopFamily = hookFamily{
	identity: LabdrianReviewStopIdentity,
	specs:    []hookSpec{{event: "Stop"}},
	build:    buildReviewStopEntry,
}

// buildReviewStopEntry returns the settings entry for the spec of the family.
//
// NEVER-BLOCKING COMMAND, AND A MISSING BINARY IS A NO-OP. The hook reports a block as JSON on stdout and exits 0, so the
// command forces exit 0 the same way the projection and approve guard entries do ("|| true"): a crash or a usage error in
// the hook can never block a stop, which is what an exit of 2 would do, with its stderr as the reason the agent reads. A
// missing binary is reported by status-hooks, not enforced here. Claude Code runs hooks with "sh -c", so the command uses
// POSIX redirection, not "&>".
func buildReviewStopEntry(hookCommand string, _ hookSpec) map[string]interface{} {
	bin := shellQuote(hookCommand)
	cmd := fmt.Sprintf(`command -v %s >/dev/null 2>&1 && %s %s || true`, bin, bin, LabdrianReviewStopIdentity)
	return map[string]interface{}{
		"hooks": []interface{}{map[string]interface{}{
			"type":    "command",
			"command": cmd,
		}},
	}
}

// MissingReviewStopHookParts names every part of the family that is missing or has drifted in root, in a fixed order, and
// returns nil when the family is exactly what Install writes; see hookFamily.missingParts.
func MissingReviewStopHookParts(root map[string]interface{}, hookCommand string) []string {
	return reviewStopFamily.missingParts(root, hookCommand)
}

// HasReviewStopHook reports whether the review stop-hook is exactly in place: its entry present once, not drifted.
func HasReviewStopHook(root map[string]interface{}, hookCommand string) bool {
	return len(MissingReviewStopHookParts(root, hookCommand)) == 0
}

// ReviewStopHookEntries returns, per event key, exactly the entries Install writes for the review stop-hook with
// hookCommand as the binary. Fixtures and status checks use it so they never retype the entry shape.
func ReviewStopHookEntries(hookCommand string) map[string][]interface{} {
	return reviewStopFamily.entries(hookCommand)
}
