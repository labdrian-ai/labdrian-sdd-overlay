package register

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/durable"
)

// installStateFileName is install-state.json's file name inside a target's
// stateDir (D9; see installstate.go for the schema itself).
const installStateFileName = "install-state.json"

// ErrConflict is Decide's ActionRefuse outcome (D9): a same-named entry
// already exists in the runtime's own config, install-state has no record
// for this target, and the entry's canonical ownership fingerprint
// (ownership.go) is not the one we would write — it is not ours, whatever
// it is named, and writing over it would destroy someone else's
// configuration (R-016/R-017 "Untagged same-named entry is refused, not
// overwritten"). cmd_register.go maps this to exit code 6. Every
// jsonInstall caller returns this wrapped with errors.Is still resolving
// it, so callers can branch on it without string matching.
//
// The text carries the way out, and deliberately does not repeat the
// "register:" prefix its wrapper already adds. This message is the only
// instruction a user staring at exit 6 gets, and it used to be both
// tripled ("register: claude: register: claude: register: ...", one
// prefix per layer) and a dead end: it named the situation and no action.
// Its wrapper names the config file, so between them the message says
// which file, what is wrong, and what to do about it.
//
// The de-duplication rule that produced that wrapper is now stated once,
// for the whole package: exactly ONE layer names the command, and it is
// the per-target wrap in jsonInstall/tomlInstall ("register: <target>: ")
// or jsonUninstall/tomlUninstall ("unregister: <target>: "), because only
// those know which direction is running. Every helper underneath them
// names the file and the failure and no command at all -- naming one
// there is how `unregister` came to report its own failures as
// "longterm-mem: register: claude: register: read ...", a command that
// was never run, twice (errormessage_test.go pins the convention,
// cmd/longterm-mem/register_messages_test.go pins the resulting lines).
var ErrConflict = errors.New("an entry with this name already exists and is not owned by longterm-mem; to hand it to longterm-mem, remove that entry by hand and run register again")

// saveInstallState is InstallState.Save, indirected the way promote's
// nowFunc is, so a test can put a failure exactly in the window
// installWithRollback exists for, without depending on how the filesystem
// happens to fail.
//
// This comment used to claim the rollback below could never run outside a
// test, on the reasoning that anything making the state directory
// unwritable would first break LoadInstallState. That was false, and it is
// worth naming the exact counter-example rather than just deleting the
// claim: LoadInstallState treats a MISSING install-state.json as an empty
// state and no error (installstate.go), so a state directory at mode 0o500
// — readable and searchable, not writable — loads perfectly and only fails
// later at Save's os.CreateTemp. Every fresh install against a directory
// the user (or a restrictive umask, or a partially restored backup) left
// unwritable takes exactly that path. TestJSONInstall_
// RollbackIsReachableWithoutStubbingTheSave pins it with no stub at all.
//
// The comment is worth this much space because of what it cost: an error
// path documented as unreachable is a path nobody writes a test for, and
// this one went on to spend its whole life as the only non-atomic write to
// a user's own config file, with a mode-preservation argument that never
// did anything. A false claim of unreachability is not a harmless comment;
// it is a standing instruction not to look.
var saveInstallState = func(s *InstallState, path string) error { return s.Save(path) }

// uninstallCannotDeriveOwnership is the entryOwned argument jsonUninstall
// and tomlUninstall pass to Decide, named rather than spelled `false` at
// the call site because the reason is not obvious and matters.
//
// Deriving ownership from an entry's content (Decide's adopt row) requires
// knowing the entry longterm-mem WOULD write, which requires the resolved
// binary path. Install has it — it is about to write that entry. Uninstall
// has nothing to write and is never told a binary path (Unregister takes
// target, configRoot, stateDir), so it cannot rebuild the comparison and
// must not guess at one.
//
// At THIS layer the consequence is bounded and recoverable: with
// install-state.json lost, `unregister` reports the entry unmanaged
// (exit 6) and leaves it alone, which is the safe direction, and running
// `register` first adopts the entry and restores the record, after which
// `unregister` removes it normally.
//
// That recovery used not to survive the packaged uninstall, and it is
// worth recording why, because the comment half of this defect was fixed
// a round before its behavioural half. bin/labdrian-overlay's
// `longterm-mem uninstall` treated exit 6 exactly as exit 0 — "this
// target has nothing of ours left" — cleared the target from its own
// tracking file, and, once the last tracked target was cleared, deleted
// the shared binary at ~/.labdrian-overlay/bin/longterm-mem. So on a
// machine that lost install-state.json, one packaged uninstall left every
// runtime config still carrying longterm-mem's own MCP entry, pointed at
// a binary that no longer existed, and removed the only tool that could
// have adopted them back.
//
// The overlay now keeps a target tracked on exit 6 (and on exit 2, its
// twin: both mean "the entry was not removed"), so the binary-removal
// guard holds and the recovery above stays reachable —
// engine/shelltest/overlay_longterm_mem_test.sh pins both. The bounded,
// recoverable outcome this comment claims is therefore true of the
// shipped caller, not only of this package.
//
// Making uninstall self-heal on its own would mean threading a binary
// path through Unregister's signature and cmd_unregister's flags — a
// public API change, not a bug fix.
const uninstallCannotDeriveOwnership = false

// adoptExistingEntry is Decide's ActionAdopt outcome: the runtime's own
// config already holds exactly the entry this install would write, so the
// only thing missing is longterm-mem's ownership record. Restore it.
//
// Unlike the insert/replace path this deliberately does NOT go through
// installWithRollback: that helper exists to undo a config write when the
// install-state write then fails, and there is no config write here to
// undo. Rewriting the config with the bytes it already has would be a
// pointless edit to a file longterm-mem does not own, and would churn its
// .bak for nothing.
func adoptExistingEntry(target string, state *InstallState, statePath, fingerprint string) error {
	state.Set(target, TargetRecord{Fingerprint: fingerprint})
	if err := saveInstallState(state, statePath); err != nil {
		return fmt.Errorf("register: %s: %w", target, err)
	}
	return nil
}

// jsonInstall is the shared JSON-writer install flow every JSON-backed
// runtime target (claude.go, opencode.go) drives through its own
// containerKey/memberKey/entry shape (11b.8): read the current entry (if
// any) from the runtime's own config, decide the action via the shared D9
// semantics table (Decide), and act on it — insert or replace via
// WriteMember plus recording the new fingerprint in install-state.json,
// adopt an entry that is already exactly ours by recording it, refuse via
// ErrConflict, or do nothing. The runtime's own config file is never
// touched on adopt, refuse or noop.
func jsonInstall(target, configPath, stateDir, containerKey, memberKey string, entry json.RawMessage) error {
	statePath := filepath.Join(stateDir, installStateFileName)
	state, err := LoadInstallState(statePath)
	if err != nil {
		return fmt.Errorf("register: %s: %w", target, err)
	}
	record, recordPresent := state.Get(target)

	current, entryPresent, err := readMember(configPath, containerKey, memberKey)
	if err != nil {
		return fmt.Errorf("register: %s: %w", target, err)
	}

	// fingerprintMatches compares install-state's recorded fingerprint
	// against the entry this call is ABOUT TO WRITE (not the bytes
	// currently on disk, decide.go's Decide doc comment) — so a reinstall
	// with the same desired entry (e.g. the same resolved binary path) is
	// a true no-op (skips WriteMember, .bak, and install-state.Save
	// entirely), while a reinstall whose desired entry changed (e.g. a
	// different binary path) always replaces in place, matching R-016/
	// R-017's "Reinstall is idempotent" scenario.
	fingerprintMatches := entryPresent && recordPresent && record.Fingerprint == Fingerprint(entry)

	// entryOwned is the other comparison, and the one that survives a lost
	// install-state.json: the entry CURRENTLY on disk against the entry
	// this call would write, both reduced to their canonical ownership
	// fingerprint (ownership.go) rather than hashed as raw bytes. That is
	// the same derivation engine/runtime's read-only adapter uses to
	// answer this question about the same file, so `doctor` and `register`
	// cannot disagree about whose entry it is — and an entry ours that
	// something re-serialized is adopted rather than met with exit 6. See
	// Decide's own doc comment for why this concedes nothing to a
	// genuinely foreign entry.
	//
	// The empty-fingerprint guard is load-bearing: an unparseable entry
	// has no fingerprint at all, and without this two entries neither of
	// which is ours would compare "" == "" into an adoption.
	ownedFingerprint := ownershipFingerprintJSON(entry)
	entryOwned := entryPresent && current != nil && ownedFingerprint != "" &&
		ownershipFingerprintJSON(current) == ownedFingerprint

	switch Decide(entryPresent, recordPresent, entryOwned, fingerprintMatches) {
	case ActionNoop:
		return nil
	case ActionRefuse:
		return fmt.Errorf("register: %s: %s: %w", target, configPath, ErrConflict)
	case ActionAdopt:
		return adoptExistingEntry(target, state, statePath, Fingerprint(entry))
	case ActionInsert, ActionReplace:
		return installWithRollback(target, configPath, memberKey, state, statePath, Fingerprint(entry), func() error {
			return WriteMember(configPath, containerKey, memberKey, entry)
		})
	default:
		return fmt.Errorf("register: %s: internal error: unreachable Decide outcome", target)
	}
}

// tomlInstall is jsonInstall's TOML counterpart (12a): read
// whether tableKey.memberKey already exists in the runtime's own
// config.toml, decide the action via the shared D9 semantics table
// (Decide), and act on it — insert or replace via WriteTOMLSection plus
// recording the new fingerprint in install-state.json, adopt a section
// that is already exactly ours by recording it, refuse via ErrConflict, or
// do nothing. It shares jsonInstall's own
// installWithRollback helper rather than re-implementing the config-write/
// install-state-write rollback window, so a TOML-specific writer can never
// reopen the exact CRITICAL 11b-1 fixed for JSON (see installWithRollback's
// doc comment).
func tomlInstall(target, configPath, stateDir, tableKey, memberKey, binary string, newSection []byte) error {
	statePath := filepath.Join(stateDir, installStateFileName)
	state, err := LoadInstallState(statePath)
	if err != nil {
		return fmt.Errorf("register: %s: %w", target, err)
	}
	record, recordPresent := state.Get(target)

	current, entryPresent, err := readTOMLSection(configPath, tableKey, memberKey)
	if err != nil {
		return fmt.Errorf("register: %s: %w", target, err)
	}

	// See jsonInstall's identical comments: fingerprintMatches compares
	// against the section this call is ABOUT TO WRITE, not the bytes
	// currently on disk, while entryOwned compares the section on disk
	// through the canonical ownership derivation (ownership.go) — for
	// TOML that means trailing newlines are trimmed on both sides, which
	// is exactly what engine/runtime's read-only adapter does, so a config
	// whose final line carries no newline is still recognized as ours.
	fingerprintMatches := entryPresent && recordPresent && record.Fingerprint == Fingerprint(newSection)
	ownedFingerprint := ownershipFingerprintTOML(newSection)
	entryOwned := entryPresent && current != nil &&
		ownershipFingerprintTOML(current) == ownedFingerprint

	switch Decide(entryPresent, recordPresent, entryOwned, fingerprintMatches) {
	case ActionNoop:
		return nil
	case ActionRefuse:
		return fmt.Errorf("register: %s: %s: %w", target, configPath, ErrConflict)
	case ActionAdopt:
		return adoptExistingEntry(target, state, statePath, Fingerprint(newSection))
	case ActionInsert, ActionReplace:
		return installWithRollback(target, configPath, memberKey, state, statePath, Fingerprint(newSection), func() error {
			return WriteTOMLSection(configPath, tableKey, memberKey, binary, newSection)
		})
	default:
		return fmt.Errorf("register: %s: internal error: unreachable Decide outcome", target)
	}
}

// jsonUninstall is jsonInstall's inverse (12b.4, R-019): decide via the
// SAME shared D9 semantics table (Decide, 12b.7), then act on it in the
// opposite direction — remove the config entry (when present) and clear
// install-state's record, leave an unmanaged entry untouched and reported,
// or do nothing. See UnregisterOutcome's own doc comments for exactly how
// each Action maps.
//
// Unlike jsonInstall's fingerprintMatches (which compares against the
// entry the caller is ABOUT TO WRITE), uninstall has nothing new to write
// — there is only ever one candidate to compare install-state's recorded
// fingerprint against: the entry CURRENTLY on disk.
//
// No installWithRollback-style guard is needed here the way jsonInstall's
// install-state-write failure needed one (11b-1 CRITICAL): if the config
// removal succeeds but saveInstallState then fails, the next run sees
// entryPresent=false, recordPresent=true — Decide's ActionReplace, which
// jsonUninstall (like jsonInstall) simply retries: entryPresent is false,
// so no config write is attempted, and clearing the stale record is
// retried. A stale record with no matching config entry is already a
// state Decide's table understands on both sides; it is never mistaken for
// someone else's entry (that requires entryPresent=true), so it can never
// regress into the 11b-1 failure mode.
func jsonUninstall(target, configPath, stateDir, containerKey, memberKey string) (UnregisterOutcome, error) {
	statePath := filepath.Join(stateDir, installStateFileName)
	state, err := LoadInstallState(statePath)
	if err != nil {
		return 0, fmt.Errorf("unregister: %s: %w", target, err)
	}
	record, recordPresent := state.Get(target)

	current, entryPresent, err := readMember(configPath, containerKey, memberKey)
	if err != nil {
		return 0, fmt.Errorf("unregister: %s: %w", target, err)
	}
	fingerprintMatches := entryPresent && recordPresent && current != nil && record.Fingerprint == Fingerprint(current)

	switch Decide(entryPresent, recordPresent, uninstallCannotDeriveOwnership, fingerprintMatches) {
	case ActionInsert:
		return UnregisterNoop, nil
	case ActionRefuse, ActionAdopt:
		return UnregisterUnmanaged, nil
	case ActionNoop, ActionReplace:
		if entryPresent {
			if err := RemoveMember(configPath, containerKey, memberKey); err != nil {
				return 0, fmt.Errorf("unregister: %s: %w", target, err)
			}
		}
		state.Delete(target)
		if err := saveInstallState(state, statePath); err != nil {
			return 0, fmt.Errorf("unregister: %s: %w", target, err)
		}
		return UnregisterRemoved, nil
	default:
		return 0, fmt.Errorf("unregister: %s: internal error: unreachable Decide outcome", target)
	}
}

// tomlUninstall is jsonUninstall's TOML counterpart, exactly as tomlInstall
// is jsonInstall's — see jsonUninstall's own doc comment for the shared
// Decide-reinterpretation rule and why no rollback guard is needed.
func tomlUninstall(target, configPath, stateDir, tableKey, memberKey string) (UnregisterOutcome, error) {
	statePath := filepath.Join(stateDir, installStateFileName)
	state, err := LoadInstallState(statePath)
	if err != nil {
		return 0, fmt.Errorf("unregister: %s: %w", target, err)
	}
	record, recordPresent := state.Get(target)

	current, entryPresent, err := readTOMLSection(configPath, tableKey, memberKey)
	if err != nil {
		return 0, fmt.Errorf("unregister: %s: %w", target, err)
	}
	fingerprintMatches := entryPresent && recordPresent && current != nil && record.Fingerprint == Fingerprint(current)

	switch Decide(entryPresent, recordPresent, uninstallCannotDeriveOwnership, fingerprintMatches) {
	case ActionInsert:
		return UnregisterNoop, nil
	case ActionRefuse, ActionAdopt:
		return UnregisterUnmanaged, nil
	case ActionNoop, ActionReplace:
		if entryPresent {
			if err := RemoveTOMLSection(configPath, tableKey, memberKey); err != nil {
				return 0, fmt.Errorf("unregister: %s: %w", target, err)
			}
		}
		state.Delete(target)
		if err := saveInstallState(state, statePath); err != nil {
			return 0, fmt.Errorf("unregister: %s: %w", target, err)
		}
		return UnregisterRemoved, nil
	default:
		return 0, fmt.Errorf("unregister: %s: internal error: unreachable Decide outcome", target)
	}
}

// installWithRollback performs the two-effect sequence every runtime
// writer's install flow shares (D9), format-agnostic: call writeConfig to
// make the runtime config edit permanent, then record fingerprint in
// install-state.json under target. The config write and the install-state
// write are two separate effects, and the window between them is the one
// failure a re-run cannot recover from: a config carrying a longterm-mem
// entry install-state has no record of is exactly the shape ActionRefuse
// is for, so every later run would refuse over an entry longterm-mem
// itself wrote (11b-1 CRITICAL). If saveInstallState fails, writeConfig's
// effect is therefore rolled back to the bytes configPath had before it
// ran, and the caller is told the entry was withdrawn.
//
// The rollback goes through durable.WriteFile, exactly like the forward
// write it undoes. It used to be a plain os.WriteFile — truncate the user's
// config, then write the old bytes back — which made the undo strictly less
// safe than the do: a crash or a full disk during the restore left
// ~/.claude.json truncated, with no entry, no valid JSON, and a broken
// editor. It also carried a captured file mode into os.WriteFile's perm
// argument, which does nothing: that argument applies only when the call
// CREATES the file, and by this point writeConfig's own rename has always
// just created it. Restoring bytes while silently re-permissioning the file
// to 0o600 is not a restore.
func installWithRollback(target, configPath, label string, state *InstallState, statePath, fingerprint string, writeConfig func() error) error {
	before, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("register: %s: read %s: %w", target, configPath, err)
	}
	if err := writeConfig(); err != nil {
		return fmt.Errorf("register: %s: %w", target, err)
	}
	state.Set(target, TargetRecord{Fingerprint: fingerprint})
	if err := saveInstallState(state, statePath); err != nil {
		if rollbackErr := durable.WriteFile(configPath, before, configCreatePerm); rollbackErr != nil {
			return fmt.Errorf("register: %s: %w (and restoring %s failed: %v — remove the %q entry by hand before re-running)", target, err, configPath, rollbackErr, label)
		}
		return fmt.Errorf("register: %s: %w (the %q entry was withdrawn from %s)", target, err, label, configPath)
	}
	return nil
}

// readMember reads the current value at containerKey.memberKey inside the
// JSON document at path. This is a read-only inspection, not the byte-
// preserving Splice write path — decoding into a generic map is fine here;
// only WRITES must never decode-reencode the whole document (D9's
// byte-identity contract binds Splice/WriteMember, not this read side). A
// config file that does not exist yet, or that has no containerKey or
// memberKey, is not an error — it just means the entry is not present.
func readMember(path, containerKey, memberKey string) (json.RawMessage, bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("read %s: %w", path, err)
	}

	var doc map[string]json.RawMessage
	if err := json.Unmarshal(emptyDocumentAsObject(raw), &doc); err != nil {
		return nil, false, fmt.Errorf("parse %s: %w", path, err)
	}
	containerRaw, ok := doc[containerKey]
	if !ok {
		return nil, false, nil
	}
	var container map[string]json.RawMessage
	if err := json.Unmarshal(containerRaw, &container); err != nil {
		return nil, false, fmt.Errorf("parse %s %q: %w", path, containerKey, err)
	}
	val, ok := container[memberKey]
	return val, ok, nil
}

// readTOMLSection reads the raw bytes of the table located at
// tableKey.memberKey inside the TOML document at path — the TOML analogue
// of readMember's read-only scope, and the source of the "bytes currently
// on disk" both tomlInstall (entryOwned) and tomlUninstall
// (fingerprintMatches) compare against, exactly as jsonInstall/
// jsonUninstall get those bytes from readMember for free.
//
// A narrower readTOMLPresence wrapper used to sit here, returning only the
// found flag for tomlInstall, on the grounds that install "never needs the
// section's actual content". Deriving ownership from an entry's own bytes
// (Decide's adopt row) is exactly needing it, so the wrapper was deleted
// rather than left as a second, weaker way to ask the same question.
//
// A config file that does not exist yet, or that has no matching table, is
// not an error — it just means the section is not present.
func readTOMLSection(path, tableKey, memberKey string) ([]byte, bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("read %s: %w", path, err)
	}
	loc := locateTOMLSection(raw, tableKey, memberKey)
	if !loc.found {
		return nil, false, nil
	}
	section := raw[loc.start:loc.end]
	if !tomlSectionIsAnEntry(section) {
		// A header with no `command =` line is not an entry -- the exact
		// rule engine/runtime's read-only adapter applies to this same
		// file for `doctor`/`status` (tomlSectionFingerprint: "a header
		// with no command line is not a real entry -- nothing a register
		// step would have written"). Reporting it as PRESENT here was the
		// one place this package and that adapter still disagreed about a
		// real file, and the disagreement had teeth: a stray bodyless
		// `[mcp_servers.longterm-mem]` header read as absent to doctor and
		// as a foreign entry to register, which refused it with exit 6 and
		// left it -- an installation that reports itself missing and
		// refuses to install, forever, over a section holding nothing.
		//
		// Calling it absent is safe in both directions. longterm-mem never
		// writes a section without a command line, so such a section is
		// provably not one of ours being protected; and it is not a
		// working entry for anyone else either, since codex has nothing to
		// launch without it. Install then fills the empty header in place
		// (tomlsplice.go locates and replaces the same span), after
		// replaceConfig has backed the original file up to .bak.
		//
		// Uninstall correspondingly leaves such a header alone rather than
		// deleting a section it does not consider ours, which is the same
		// answer it gives for any other entry it did not write.
		return nil, false, nil
	}
	return section, true, nil
}

// tomlSectionIsAnEntry reports whether a located codex section is a real
// MCP server entry, i.e. carries a `command =` line. It reproduces
// engine/runtime's codexCommandLine test, so both sides of D9 answer
// "does this entry exist?" with one rule.
func tomlSectionIsAnEntry(section []byte) bool {
	for _, line := range bytes.Split(section, []byte("\n")) {
		if tomlCommandLinePattern.Match(line) {
			return true
		}
	}
	return false
}
