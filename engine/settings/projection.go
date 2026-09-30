package settings

import (
	"fmt"
	"reflect"
)

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

// projectionSpec is one entry of the projection family, keyed by event.
type projectionSpec struct {
	event    string
	matcher  string
	hookName string
}

// projectionSpecs is the whole family in install order: one UserPromptSubmit
// entry and two PreToolUse entries.
var projectionSpecs = []projectionSpec{
	{event: "UserPromptSubmit", hookName: "UserPromptSubmit"},
	{event: "PreToolUse", matcher: ProjectionEditToolMatcher, hookName: "PreToolUse"},
	{event: "PreToolUse", matcher: ProjectionMemoryQueryMatcher, hookName: "PreToolUse"},
}

// projectionEvents are the event keys the family writes to.
var projectionEvents = []string{"UserPromptSubmit", "PreToolUse"}

func (s projectionSpec) label() string {
	if s.matcher == "" {
		return s.event + " " + LabdrianProjectionIdentity
	}
	return fmt.Sprintf("%s matcher=%q %s", s.event, s.matcher, LabdrianProjectionIdentity)
}

// buildProjectionEntry returns the settings entry for one spec.
//
// NEVER-BLOCKING COMMAND: the projection hooks report through JSON on stdout
// and exit 0; a denial is a JSON decision, not exit code 2. So the command
// forces exit 0 the same way the propagate and gate-task entries do
// ("|| true"), and a missing binary is a silent no-op. Claude Code runs hooks
// with "sh -c", so the guard uses POSIX redirection, not "&>".
func buildProjectionEntry(hookCommand string, s projectionSpec) map[string]interface{} {
	cmd := fmt.Sprintf(
		`command -v %s >/dev/null 2>&1 && %s %s --event %s || true`,
		hookCommand, hookCommand, LabdrianProjectionIdentity, s.hookName,
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

// isProjectionEntry reports whether a hook entry belongs to the projection
// family: it references our binary AND the projection identity token. With no
// binary path there is nothing to tell our entries from foreign ones (every
// command contains the empty string), so no entry is ours.
func isProjectionEntry(e interface{}, hookCommand string) bool {
	return hookCommand != "" && entryContainsBinary(e, hookCommand) && entryContainsBinary(e, LabdrianProjectionIdentity)
}

func (m *Merger) isProjectionEntry(e interface{}) bool {
	return isProjectionEntry(e, m.hookCommand)
}

// projectionSpecsFor returns the specs of one event key.
func projectionSpecsFor(event string) []projectionSpec {
	var out []projectionSpec
	for _, s := range projectionSpecs {
		if s.event == event {
			out = append(out, s)
		}
	}
	return out
}

// matchingSpec returns the index of the spec whose exact entry equals e, or -1.
func matchingSpec(e interface{}, hookCommand string, specs []projectionSpec) int {
	for i, s := range specs {
		if reflect.DeepEqual(e, interface{}(buildProjectionEntry(hookCommand, s))) {
			return i
		}
	}
	return -1
}

// entryKind says what one entry of an event's hook list is to the family.
type entryKind int

const (
	// entryForeign is not ours: never read past the identity check.
	entryForeign entryKind = iota
	// entryMatched is exactly what this version writes for a spec, seen once.
	entryMatched
	// entryDrifted is ours but differs from what this version writes, or
	// repeats an entry already matched.
	entryDrifted
)

// classifyProjectionEntries is the one place that decides what each entry of
// one event's hook list is to the family, so the merge and the status check
// cannot disagree. kinds is parallel to entries; seen is parallel to specs and
// says which spec has a matching entry.
func classifyProjectionEntries(entries []interface{}, hookCommand string, specs []projectionSpec) (kinds []entryKind, seen []bool) {
	kinds = make([]entryKind, len(entries))
	seen = make([]bool, len(specs))
	for n, e := range entries {
		if !isProjectionEntry(e, hookCommand) {
			kinds[n] = entryForeign
			continue
		}
		if i := matchingSpec(e, hookCommand, specs); i >= 0 && !seen[i] {
			seen[i] = true
			kinds[n] = entryMatched
			continue
		}
		kinds[n] = entryDrifted
	}
	return kinds, seen
}

// mergeProjection makes the projection family exactly the desired entries:
// missing entries are appended, and an owned entry that is stale (drifted from
// what this version writes) or a duplicate is replaced. Foreign entries are
// never read past the identity check and never rewritten. A hook list that is
// not an array cannot hold hooks, so, as in every other family, it is replaced
// (the original stays in settings.json.bak). Returns true if anything changed.
func (m *Merger) mergeProjection(hooks map[string]interface{}) bool {
	changed := false
	for _, event := range projectionEvents {
		specs := projectionSpecsFor(event)
		entries, _ := hooks[event].([]interface{})
		kinds, seen := classifyProjectionEntries(entries, m.hookCommand, specs)
		kept := make([]interface{}, 0, len(entries)+len(specs))
		keyChanged := false
		for n, e := range entries {
			if kinds[n] == entryDrifted {
				keyChanged = true
				continue
			}
			kept = append(kept, e)
		}
		for i, s := range specs {
			if !seen[i] {
				kept = append(kept, buildProjectionEntry(m.hookCommand, s))
				keyChanged = true
			}
		}
		if keyChanged {
			hooks[event] = kept
			changed = true
		}
	}
	return changed
}

// MissingProjectionHookParts names every part of the projection family that is
// missing or has drifted in root, in a fixed order. It returns nil when the
// family is exactly what Install writes. An owned entry that differs from what
// this version writes, or a duplicate, is reported as drifted; Install repairs
// both. Settings that are not shaped as Claude Code expects (hooks or an event
// that is not an object or an array) hold none of the family, so all of it is
// reported missing.
func MissingProjectionHookParts(root map[string]interface{}, hookCommand string) []string {
	var parts []string
	hooks, _ := root["hooks"].(map[string]interface{})
	for _, event := range projectionEvents {
		specs := projectionSpecsFor(event)
		entries, _ := hooks[event].([]interface{})
		kinds, seen := classifyProjectionEntries(entries, hookCommand, specs)
		drifted := 0
		for _, k := range kinds {
			if k == entryDrifted {
				drifted++
			}
		}
		for i, s := range specs {
			if !seen[i] {
				parts = append(parts, s.label())
			}
		}
		if drifted > 0 {
			parts = append(parts, fmt.Sprintf("%d drifted or duplicate %s %s entries", drifted, event, LabdrianProjectionIdentity))
		}
	}
	return parts
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
	out := map[string][]interface{}{}
	for _, s := range projectionSpecs {
		out[s.event] = append(out[s.event], buildProjectionEntry(hookCommand, s))
	}
	return out
}
