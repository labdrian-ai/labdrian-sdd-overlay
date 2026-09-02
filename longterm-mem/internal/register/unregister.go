package register

import (
	"fmt"
	"path/filepath"
)

// UnregisterOutcome reports what Unregister actually did to one runtime's
// configuration (R-019).
type UnregisterOutcome int

const (
	// UnregisterNoop: neither the runtime config entry nor an install-state
	// record for this target existed — nothing to do (Decide's
	// ActionInsert outcome, reinterpreted for the uninstall direction: "not
	// installed" reads the same whether you are about to install or
	// already tried to uninstall).
	UnregisterNoop UnregisterOutcome = iota
	// UnregisterRemoved: the ownership-tagged entry (when present) was
	// removed from the runtime's own config, and install-state's record
	// for this target was cleared. Covers both Decide's ActionNoop
	// (entry present, ours, unmodified) and ActionReplace (ours but
	// stale/hand-edited, or the entry was already deleted out from under
	// install-state) outcomes: either way, this target no longer owns
	// anything once Unregister returns.
	UnregisterRemoved
	// UnregisterUnmanaged: a same-named entry exists in the runtime's own
	// config, but install-state has no record for this target — it is not
	// ours, so it is left in place and reported, never removed (Decide's
	// ActionRefuse outcome, reinterpreted the same way ActionInsert is
	// above: "not ours" is the same conflict whether install or
	// unregister asks about it).
	UnregisterUnmanaged
)

// String renders o as the word cmd_unregister.go (12b.5) prints for
// --target all's per-target report line.
func (o UnregisterOutcome) String() string {
	switch o {
	case UnregisterNoop:
		return "noop"
	case UnregisterRemoved:
		return "removed"
	case UnregisterUnmanaged:
		return "unmanaged"
	default:
		return "unknown"
	}
}

// Unregister removes the ownership-tagged longterm-mem MCP entry from
// target's own runtime configuration (claude, opencode, codex) and clears
// its install-state.json record (R-019). Unlike Register*'s three separate
// per-runtime entry points — which each need their own entry SHAPE
// (claudeEntry, opencodeEntry, codex's section template) to write —
// unregister has nothing runtime-specific to write, only somewhere to look
// and a format to edit, so one function dispatches by target name here
// rather than three thin per-runtime wrappers plus a second switch at the
// cmd layer (cmd_unregister.go, 12b.5, mirrors cmd_register.go's
// registerTarget one level higher only because Register* needed the entry
// shape argument this does not).
//
// jsonUninstall and tomlUninstall (writer.go) share Decide (decide.go) with
// jsonInstall/tomlInstall — the exact call site, not a second copy of the
// D9 semantics table — reinterpreted for the uninstall direction; see
// UnregisterOutcome's own doc comments for how each Action maps (12b.7).
func Unregister(target, configRoot, stateDir string) (UnregisterOutcome, error) {
	switch target {
	case claudeTarget:
		configPath := filepath.Join(configRoot, claudeConfigFileName)
		return jsonUninstall(claudeTarget, configPath, stateDir, claudeContainerKey, "longterm-mem")
	case opencodeTarget:
		configPath := filepath.Join(configRoot, opencodeConfigFileName)
		return jsonUninstall(opencodeTarget, configPath, stateDir, opencodeContainerKey, "longterm-mem")
	case codexTarget:
		configPath := filepath.Join(configRoot, codexConfigFileName)
		return tomlUninstall(codexTarget, configPath, stateDir, codexTableKey, "longterm-mem")
	default:
		return 0, fmt.Errorf("register: %s: unknown target", target)
	}
}
