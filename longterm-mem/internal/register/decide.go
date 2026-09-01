package register

// Action is the D9 semantics table's outcome for one runtime writer's
// install decision.
type Action int

const (
	// ActionInsert: no entry with this name exists in the runtime config,
	// and install-state has no record for this target — write a fresh
	// entry.
	ActionInsert Action = iota
	// ActionReplace: install-state already owns this target (a record
	// exists), and either the runtime config entry is absent (deleted out
	// from under us) or present but stale relative to that record —
	// (re)write it in place.
	ActionReplace
	// ActionRefuse: the runtime config already has an entry with this
	// name, but install-state has no record for it — it is not ours, and
	// writing over it would destroy someone else's configuration
	// (R-016/R-017 "Untagged same-named entry is refused, not
	// overwritten").
	ActionRefuse
	// ActionNoop: install-state owns this target, the runtime config
	// entry is present, and its fingerprint matches what install-state
	// last recorded — nothing to do.
	ActionNoop
)

// String renders a as the name used throughout D9's documentation and
// writer error/status messages (e.g. reporting an ActionRefuse conflict).
func (a Action) String() string {
	switch a {
	case ActionInsert:
		return "insert"
	case ActionReplace:
		return "replace"
	case ActionRefuse:
		return "refuse"
	case ActionNoop:
		return "noop"
	default:
		return "unknown"
	}
}

// Decide is the D9 semantics table as one pure function, shared by every
// runtime writer (claude.go, opencode.go, codex.go in 11b/12a) for both
// install and uninstall call sites (12b.7).
//
// entryPresent is whether the runtime config already has a same-named
// entry. recordPresent is whether install-state.json has an ownership
// record for this target. fingerprintMatches is whether that record's
// fingerprint matches the entry longterm-mem is about to write; it is a
// don't-care unless both entryPresent and recordPresent are true, since
// there is nothing to compare a fingerprint against otherwise.
//
//	entryPresent  recordPresent  fingerprintMatches  →  Action
//	false         false          —                      insert
//	false         true           —                      replace
//	true          false          —                      refuse
//	true          true           false                  replace
//	true          true           true                   noop
func Decide(entryPresent, recordPresent, fingerprintMatches bool) Action {
	switch {
	case !entryPresent && !recordPresent:
		return ActionInsert
	case !entryPresent && recordPresent:
		return ActionReplace
	case entryPresent && !recordPresent:
		return ActionRefuse
	case fingerprintMatches:
		return ActionNoop
	default:
		return ActionReplace
	}
}
