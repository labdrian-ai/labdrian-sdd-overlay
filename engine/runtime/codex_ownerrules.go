package runtime

import (
	"errors"
	"path/filepath"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/core"
)

// The Codex adapter keeps the owner rules (instructions/owner-rules.md) in one marked block of each
// of three files, with the writer the Claude and OpenCode adapters use (MarkedBlock, through
// OwnerRulesBlock), so every byte outside the block is the owner's and stays:
//
//	~/AGENTS.md              (the config's home)
//	<codex root>/AGENTS.md   (the root is $CODEX_HOME, else ~/.codex)
//	<codex root>/agents.md
//
// What Codex loads, read from the installed codex 0.167.0 binary and the owner's config.toml (not
// from a run): the global instructions come from AGENTS.override.md, else AGENTS.md, in the Codex
// root, and a session's project documents come from AGENTS.override.md, else AGENTS.md, of each
// directory on the walk from its working directory up to the project root. Nothing in the binary
// names a lower-case agents.md, and config.toml sets no project_doc_fallback_filenames, so no Codex
// path reads <codex root>/agents.md; it is written all the same and reported.

// codexUnreadNote is said beside the file no Codex path reads.
const codexUnreadNote = "Codex reads no file named agents.md (it loads AGENTS.override.md and AGENTS.md, and any name project_doc_fallback_filenames lists)"

// codexRulesFile is one file the block is written in.
type codexRulesFile struct {
	path string
	// note, when it is not empty, is what to say about the file beside its state.
	note string
}

// rulesFiles are the files the block is written in, the home one only when the home is known. The
// second result is the sentence that says the home file was left out, empty when it was not.
func (a CodexAdapter) rulesFiles() ([]codexRulesFile, string) {
	var files []codexRulesFile
	skipped := ""
	if a.home != "" {
		files = append(files, codexRulesFile{path: filepath.Join(a.home, "AGENTS.md")})
	} else {
		skipped = "the home directory is not known, so ~/AGENTS.md was left out"
	}
	files = append(files,
		codexRulesFile{path: filepath.Join(a.root, "AGENTS.md")},
		codexRulesFile{path: filepath.Join(a.root, "agents.md"), note: codexUnreadNote})
	return files, skipped
}

// withOwnerRules is the result of an install or update whose manifest is in place: it writes the
// owner rules block in each file and adds what it did to message. It reports partial when the rules
// could not be read or a file could not be written (the manifest stays installed; a file that was
// not written stays as it was); the other files are still written. An adapter without a source
// writes nothing and reports the manifest alone.
func (a CodexAdapter) withOwnerRules(action core.Action, message string) core.LifecycleResult {
	if a.rules == nil {
		return a.result(action, core.CapabilityRestartRequired, message)
	}
	body, err := a.rules.OwnerRules()
	if errors.Is(err, ErrNoOverlayCheckout) {
		return a.result(action, core.CapabilityRestartRequired, message+"; owner rules not installed: "+err.Error())
	}
	if err != nil {
		return a.result(action, core.CapabilityPartial, message+"; owner rules could not be read: "+err.Error())
	}
	files, skipped := a.rulesFiles()
	status := core.CapabilityRestartRequired
	for _, file := range files {
		change, err := OwnerRulesBlock.WriteFile(file.path, body)
		if err != nil {
			status = core.CapabilityPartial
			message += "; owner rules could not be written: " + err.Error()
			continue
		}
		switch change {
		case BlockAdded:
			message += "; owner rules written to " + file.path
		case BlockReplaced:
			message += "; owner rules updated in " + file.path
		default:
			message += "; owner rules already current in " + file.path
		}
		if file.note != "" {
			message += " (note: " + file.note + ")"
		}
	}
	if skipped != "" {
		message += "; " + skipped
	}
	return a.result(action, status, message)
}

// removeOwnerRules is message with what removing the block from each file did. It is partial when a
// file could not be changed (markers that are not one pair, a changed file): the others are still
// handled. An adapter without a source never wrote a block and leaves the files alone.
func (a CodexAdapter) removeOwnerRules(message string) (string, core.CapabilityStatus) {
	if a.rules == nil {
		return message, core.CapabilityRestartRequired
	}
	files, _ := a.rulesFiles()
	status := core.CapabilityRestartRequired
	for _, file := range files {
		removed, err := OwnerRulesBlock.RemoveFile(file.path)
		switch {
		case err != nil:
			status = core.CapabilityPartial
			message += "; owner rules could not be removed: " + err.Error()
		case removed:
			message += "; owner rules removed from " + file.path
		}
	}
	return message, status
}

// ownerRulesReasons are the lines a status adds: the state of the block in each file, or why it is
// not known. A status checks nothing and says nothing when the adapter has no source. A file's
// state is never a failure by itself: the status of Codex is partial until its activation can be
// proven, whatever the files hold.
func (a CodexAdapter) ownerRulesReasons() []string {
	if a.rules == nil {
		return nil
	}
	body, err := a.rules.OwnerRules()
	if errors.Is(err, ErrNoOverlayCheckout) {
		return []string{"owner rules not checked: " + err.Error()}
	}
	if err != nil {
		return []string{"Codex owner rules could not be read: " + err.Error()}
	}
	files, skipped := a.rulesFiles()
	var reasons []string
	for _, file := range files {
		reason := ""
		state, err := OwnerRulesBlock.StateOfFile(file.path, body)
		switch {
		case err != nil:
			reason = "owner rules block could not be checked: " + err.Error()
		case state == BlockMissing:
			reason = "owner rules block is missing in " + file.path + "; run 'engine runtime install --target codex'"
		case state == BlockStale:
			reason = "owner rules block is stale in " + file.path + "; run 'engine runtime update --target codex'"
		default:
			reason = "owner rules block is current in " + file.path
		}
		if file.note != "" {
			reason += " (note: " + file.note + ")"
		}
		reasons = append(reasons, reason)
	}
	if skipped != "" {
		reasons = append(reasons, skipped)
	}
	return reasons
}

// codexWithReasons is result with more reasons after its own.
func codexWithReasons(result core.LifecycleResult, more []string) core.LifecycleResult {
	if len(more) == 0 {
		return result
	}
	reasons := append(append([]string(nil), result.Reasons...), more...)
	return core.NewLifecycleResult(result.Target, result.Action, result.Status, result.Message, reasons)
}
