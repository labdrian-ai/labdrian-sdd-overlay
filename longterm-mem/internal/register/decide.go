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
	// name, install-state has no record for it, and the entry is not the
	// one longterm-mem would write — it is not ours, and writing over it
	// would destroy someone else's configuration (R-016/R-017 "Untagged
	// same-named entry is refused, not overwritten").
	ActionRefuse
	// ActionAdopt: the runtime config already has an entry with this name
	// and install-state has no record for it — but the entry is the one
	// this call is about to write, which no other program produces
	// (compared through the canonical ownership fingerprint, ownership.go,
	// so a re-serialization of our own entry still counts as ours). The
	// ownership record was lost, not the ownership: re-record the
	// fingerprint and leave the config alone.
	ActionAdopt
	// ActionNoop: install-state owns this target, the runtime config
	// entry is present, and its fingerprint matches what install-state
	// last recorded — nothing to do.
	ActionNoop
)

// Decide is the D9 semantics table as one pure function, shared by every
// runtime writer (claude.go, opencode.go, codex.go in 11b/12a) for both
// install and uninstall call sites (12b.7).
//
// entryPresent is whether the runtime config already has a same-named
// entry. recordPresent is whether install-state.json has an ownership
// record for this target.
//
// entryOwned is whether the entry CURRENTLY ON DISK is the one
// longterm-mem is about to write, judged by the canonical ownership
// fingerprint (ownership.go): identical content, whatever the key order
// or the insignificant whitespace it happens to be serialized with. It is
// consulted only when there is no record to settle ownership, and it is
// the whole of the difference between refuse and adopt.
//
// fingerprintMatches is whether the record's fingerprint matches the entry
// longterm-mem is about to write; it is a don't-care unless both
// entryPresent and recordPresent are true, since there is nothing to
// compare a fingerprint against otherwise.
//
//	entryPresent  recordPresent  entryOwned  fingerprintMatches  →  Action
//	false         false          —           —                      insert
//	false         true           —           —                      replace
//	true          false          false       —                      refuse
//	true          false          true        —                      adopt
//	true          true           —           false                  replace
//	true          true           —           true                   noop
//
// Why adopt exists at all: install-state.json is one small file in
// longterm-mem's own state directory, and losing it (a restored backup
// that predates the install, a wiped state directory, a --state-dir typo)
// used to be unrecoverable. Every runtime's entry suddenly read as someone
// else's: register refused all three with exit 6, unregister reported all
// three unmanaged, and the only way out was hand-editing three config
// files longterm-mem itself had written. Re-deriving ownership from the
// entry's own content — through the SAME canonical fingerprint
// engine/runtime's read-only adapter uses to answer this question about
// this very file (LongtermMemAdapter's ownedClaudeFingerprint and
// friends; ownership.go reproduces that canonicalization, and
// ownership_test.go pins it) — makes that state self-healing, while
// conceding nothing: an entry whose content differs from ours at all is
// still refused, so a third party's server can never be adopted.
//
// "Content", not "bytes", is the operative word, and the difference is
// not cosmetic. Hashing the raw bytes here left the lockout standing for
// every entry longterm-mem wrote that something then re-serialized — a
// runtime's settings UI, a formatter, `jq . > config`, a hand tidy-up —
// because the read-only adapter called those entries ours and this
// comparison did not. Two derivations of one fact that disagree do not
// have a stricter side and a looser side; they have a side that is wrong
// about a real file on a real machine.
func Decide(entryPresent, recordPresent, entryOwned, fingerprintMatches bool) Action {
	switch {
	case !entryPresent && !recordPresent:
		return ActionInsert
	case !entryPresent && recordPresent:
		return ActionReplace
	case entryPresent && !recordPresent && entryOwned:
		return ActionAdopt
	case entryPresent && !recordPresent:
		return ActionRefuse
	case fingerprintMatches:
		return ActionNoop
	default:
		return ActionReplace
	}
}
