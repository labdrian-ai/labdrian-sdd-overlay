package settings

import (
	"fmt"
	"reflect"
)

// hookFamily is a set of hook entries this overlay owns as a unit: the entries
// are told apart from every other entry in settings.json by the installed
// binary path together with one identity token in their command, and each is
// exactly the entry this version writes for its spec. Install repairs the
// family to exactly that set, uninstall removes exactly that set, and the status
// helpers report what is missing or has drifted. The projection family and the
// approve guard family share this one implementation, so their merge and their
// status check cannot disagree with each other or with the other family's.
type hookFamily struct {
	// identity is the dedup/uninstall token: a command is the family's when it
	// contains the binary path and this token.
	identity string
	// specs lists the family's entries in install order.
	specs []hookSpec
	// build returns the settings entry this version writes for one spec.
	build func(hookCommand string, s hookSpec) map[string]interface{}
}

// hookSpec is one entry of a family: the event key it sits under and, when it
// has one, the tool matcher.
type hookSpec struct {
	event   string
	matcher string
}

func (f hookFamily) label(s hookSpec) string {
	if s.matcher == "" {
		return s.event + " " + f.identity
	}
	return fmt.Sprintf("%s matcher=%q %s", s.event, s.matcher, f.identity)
}

// events returns the event keys the family writes to, in first-use order.
func (f hookFamily) events() []string {
	var out []string
	seen := map[string]bool{}
	for _, s := range f.specs {
		if !seen[s.event] {
			seen[s.event] = true
			out = append(out, s.event)
		}
	}
	return out
}

// specsFor returns the specs of one event key.
func (f hookFamily) specsFor(event string) []hookSpec {
	var out []hookSpec
	for _, s := range f.specs {
		if s.event == event {
			out = append(out, s)
		}
	}
	return out
}

// owns reports whether a hook entry belongs to the family: it references our
// binary AND the family's identity token. With no binary path there is nothing
// to tell our entries from foreign ones (every command contains the empty
// string), so no entry is ours.
func (f hookFamily) owns(e interface{}, hookCommand string) bool {
	return hookCommand != "" && entryContainsBinary(e, hookCommand) && entryContainsBinary(e, f.identity)
}

// matchingSpec returns the index of the spec whose exact entry equals e, or -1.
func (f hookFamily) matchingSpec(e interface{}, hookCommand string, specs []hookSpec) int {
	for i, s := range specs {
		if reflect.DeepEqual(e, interface{}(f.build(hookCommand, s))) {
			return i
		}
	}
	return -1
}

// entryKind says what one entry of an event's hook list is to a family.
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

// classify is the one place that decides what each entry of one event's hook
// list is to the family, so the merge and the status check cannot disagree.
// kinds is parallel to entries; seen is parallel to specs and says which spec
// has a matching entry.
func (f hookFamily) classify(entries []interface{}, hookCommand string, specs []hookSpec) (kinds []entryKind, seen []bool) {
	kinds = make([]entryKind, len(entries))
	seen = make([]bool, len(specs))
	for n, e := range entries {
		if !f.owns(e, hookCommand) {
			kinds[n] = entryForeign
			continue
		}
		if i := f.matchingSpec(e, hookCommand, specs); i >= 0 && !seen[i] {
			seen[i] = true
			kinds[n] = entryMatched
			continue
		}
		kinds[n] = entryDrifted
	}
	return kinds, seen
}

// merge makes the family exactly the desired entries: missing entries are
// appended, and an owned entry that is stale (drifted from what this version
// writes) or a duplicate is replaced. Foreign entries are never read past the
// identity check and never rewritten. A hook list that is not an array cannot
// hold hooks, so, as in every other family, it is replaced (the original stays
// in settings.json.bak). Returns true if anything changed.
func (f hookFamily) merge(hooks map[string]interface{}, hookCommand string) bool {
	changed := false
	for _, event := range f.events() {
		specs := f.specsFor(event)
		entries, _ := hooks[event].([]interface{})
		kinds, seen := f.classify(entries, hookCommand, specs)
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
				kept = append(kept, f.build(hookCommand, s))
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

// missingParts names every part of the family that is missing or has drifted in
// root, in a fixed order. It returns nil when the family is exactly what merge
// writes. An owned entry that differs from what this version writes, or a
// duplicate, is reported as drifted; merge repairs both. Settings that are not
// shaped as Claude Code expects (hooks or an event that is not an object or an
// array) hold none of the family, so all of it is reported missing.
func (f hookFamily) missingParts(root map[string]interface{}, hookCommand string) []string {
	var parts []string
	hooks, _ := root["hooks"].(map[string]interface{})
	for _, event := range f.events() {
		specs := f.specsFor(event)
		entries, _ := hooks[event].([]interface{})
		kinds, seen := f.classify(entries, hookCommand, specs)
		drifted := 0
		for _, k := range kinds {
			if k == entryDrifted {
				drifted++
			}
		}
		for i, s := range specs {
			if !seen[i] {
				parts = append(parts, f.label(s))
			}
		}
		if drifted > 0 {
			parts = append(parts, fmt.Sprintf("%d drifted or duplicate %s %s entries", drifted, event, f.identity))
		}
	}
	return parts
}

// entries returns, per event key, exactly the entries merge writes for the
// family with hookCommand as the binary. Fixtures and status checks use it so
// they never retype the entry shape.
func (f hookFamily) entries(hookCommand string) map[string][]interface{} {
	out := map[string][]interface{}{}
	for _, s := range f.specs {
		out[s.event] = append(out[s.event], f.build(hookCommand, s))
	}
	return out
}
