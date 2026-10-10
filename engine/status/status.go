// Package status is the use case behind `engine status`, the doctor of an installation: it looks
// at what the overlay installs into a machine (the binary, the hooks and guards in Claude Code's
// settings.json, the minimalism contract) and at the registry of the project it is run in, and
// reports one Check for each, in a fixed order, as OK, a warning or a failure.
//
// It is an application package. The decisions are here, over values: what a settings object must
// hold for a hook family to count as installed, which absences are hard failures and which only
// ask for the upgrade path, when a registry is empty and when it is merely missing a block. What
// they are decided over is reached through two ports the package owns, Files and SettingsSource;
// the adapters are status/fsfiles and settings/settingsfile, and the composition root hands them
// in. The package reads no environment, takes no working directory from the process, prints
// nothing and exits nowhere: the home and the directory are in the Request, the words of a check
// are its Label and Note, and the line a person reads and the exit code are the command's.
package status

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/contract"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/propagator"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings"
)

// Level is how a check came out.
type Level int

const (
	// OK means nothing is wrong, or nothing is wrong that the overlay installs.
	OK Level = iota
	// Warn means the installation works but is behind: a family of hooks added by a later version
	// is not there yet, or a registry lacks a block. It asks for an action and breaks nothing.
	Warn
	// Fail means something the overlay needs is missing, unreadable or broken.
	Fail
)

func (l Level) String() string {
	switch l {
	case OK:
		return "ok"
	case Warn:
		return "warn"
	case Fail:
		return "fail"
	}
	return fmt.Sprintf("Level(%d)", int(l))
}

// Check is the result of one question put to the installation: what was asked (Label, which
// names the file it was asked of when it was asked of one), how it came out, and the words that
// say why when there is something to say.
type Check struct {
	Label string
	Level Level
	Note  string
}

// Outcome is how a whole report came out: the worst level of its checks.
type Outcome int

const (
	// Healthy means every check is OK.
	Healthy Outcome = iota
	// Degraded means no check failed and at least one warns.
	Degraded
	// Failed means at least one check failed.
	Failed
)

func (o Outcome) String() string {
	switch o {
	case Healthy:
		return "healthy"
	case Degraded:
		return "degraded"
	case Failed:
		return "failed"
	}
	return fmt.Sprintf("Outcome(%d)", int(o))
}

// Report is the checks of one run, in the order they were made.
type Report struct {
	Checks []Check
}

// Outcome is the worst level among the checks, so one failure fails the report whatever else
// warns, and a report with no failure and a warning is degraded.
func (r Report) Outcome() Outcome {
	outcome := Healthy
	for _, c := range r.Checks {
		switch c.Level {
		case Fail:
			return Failed
		case Warn:
			outcome = Degraded
		}
	}
	return outcome
}

// FileReader reads a file whole. A file that is not there is an error for which errors.Is(err,
// fs.ErrNotExist) holds; any other error is a file that cannot be read.
type FileReader interface {
	ReadFile(path string) ([]byte, error)
}

// Files is the part of the file system the status of an installation looks at: whether a path
// exists and with what permission bits (an absent path is an error that satisfies fs.ErrNotExist),
// and the content of a file.
type Files interface {
	FileReader
	Stat(path string) (fs.FileMode, error)
}

// SettingsSource reads the settings.json at a path. A file that is not there, or that holds the
// JSON value null, is the document with nothing in it and no error: the hooks are simply absent.
// A file that cannot be read, or that is not a JSON object, is an error.
type SettingsSource interface {
	Settings(path string) (settings.Document, error)
}

// Request is where to look: the home directory whose .claude holds the installation, as the
// process has it (an empty home gives paths relative to the working directory, as it always has),
// and the directory the person runs from, whose .atl may hold a registry. An empty Cwd means the
// directory could not be determined, and the registry is not looked for.
type Request struct {
	Home string
	Cwd  string
}

// Service makes the checks of an installation over the ports it is given.
type Service struct {
	Files    Files
	Settings SettingsSource
}

// binaryIdentity is the substring that tells the overlay's hook entries from every other
// entry in settings.json: the name of the installed binary.
const binaryIdentity = "gentle-ai-overlay"

// Words that more than one check says.
const (
	// remediation is the upgrade path of every family a later version added to the install.
	remediation = "run 'labdrian uninstall-hooks' then 'labdrian install-hooks'"
	// restartNote says that Claude Code loads its hooks when it starts.
	restartNote = "restart Claude Code to load hook changes"
	// speedBump is what even an installed guard is: it is matched on text and does not stop a
	// determined agent.
	speedBump = "the guard is a speed bump, not a security boundary"
)

// Check makes every check, in the order a report lists them: the binary; the two base hooks,
// whose absence is a failure; the four families added later, whose absence is a warning; the
// contract; and, when the request names a directory, the registry of the project there.
//
// Both ports are required: a Service without one is a wiring mistake and Check panics saying which,
// rather than answering a report that hides it.
func (s Service) Check(req Request) Report {
	if s.Files == nil {
		panic("status: Service.Files is nil")
	}
	if s.Settings == nil {
		panic("status: Service.Settings is nil")
	}
	binaryPath := filepath.Join(req.Home, ".claude", "bin", binaryIdentity)
	settingsPath := filepath.Join(req.Home, ".claude", "settings.json")
	contractPath := filepath.Join(req.Home, ".claude", "skills", "_shared", "minimalism-contract.md")

	doc, err := s.Settings.Settings(settingsPath)
	loaded := loadedSettings{path: settingsPath, root: doc.Root(), err: err}

	checks := []Check{
		checkBinary(binaryPath, s.Files),
		checkUserPromptSubmitHook(loaded),
		checkPreToolUseHook(loaded),
		checkSessionEndHook(loaded),
		checkReviewReceiptHook(loaded),
		checkShaperClearanceGuard(loaded),
		checkProjectionHooks(loaded, binaryPath),
		checkApproveGuard(loaded, binaryPath),
		CheckContract(contractPath, s.Files),
	}
	if req.Cwd != "" {
		checks = append(checks, checkRegistry(filepath.Join(req.Cwd, ".atl", "skill-registry.md"), s.Files))
	}
	return Report{Checks: checks}
}

// loadedSettings is what reading settings.json gave: its object, nil when the file is absent or
// holds null, or the error that stopped the read.
type loadedSettings struct {
	path string
	root map[string]interface{}
	err  error
}

// beforeEntries is the answer of a check that needs no look at the entries, because the settings
// cannot give it any: a file that could not be read fails every check that asks, and a file that is
// absent or holds nothing is the check's own level, with its advice after the fixed words. The
// second result is false when there are settings to look into.
func (s loadedSettings) beforeEntries(label string, whenAbsent Level, advice string) (Check, bool) {
	if s.err != nil {
		return Check{Label: label, Level: Fail, Note: "cannot read " + s.path + ": " + s.err.Error()}, true
	}
	if s.root == nil {
		return Check{Label: label, Level: whenAbsent, Note: s.path + " absent or empty" + advice}, true
	}
	return Check{}, false
}

// checkBinary verifies the engine binary is present and executable: any execute bit is enough.
func checkBinary(binaryPath string, files Files) Check {
	label := "binary: " + binaryPath
	mode, err := files.Stat(binaryPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Check{Label: label, Level: Fail, Note: "not found"}
		}
		return Check{Label: label, Level: Fail, Note: err.Error()}
	}
	if mode&0o111 == 0 {
		return Check{Label: label, Level: Fail, Note: "exists but not executable"}
	}
	return Check{Label: label, Level: OK}
}

// checkUserPromptSubmitHook verifies a UserPromptSubmit entry references our binary.
func checkUserPromptSubmitHook(s loadedSettings) Check {
	label := "hook: UserPromptSubmit (propagate)"
	if c, done := s.beforeEntries(label, Fail, ""); done {
		return c
	}
	if !settings.HasHooksObject(s.root) {
		return Check{Label: label, Level: Fail, Note: "hooks key missing in settings.json"}
	}
	if settings.HasHookContaining(s.root, "UserPromptSubmit", binaryIdentity) {
		return Check{Label: label, Level: OK}
	}
	return Check{Label: label, Level: Fail, Note: "no UserPromptSubmit entry referencing " + binaryIdentity}
}

// checkPreToolUseHook verifies the PreToolUse entry for the Agent tool references our binary.
func checkPreToolUseHook(s loadedSettings) Check {
	label := `hook: PreToolUse matcher="Agent" (gate-task)`
	if c, done := s.beforeEntries(label, Fail, ""); done {
		return c
	}
	if !settings.HasHooksObject(s.root) {
		return Check{Label: label, Level: Fail, Note: "hooks key missing in settings.json"}
	}
	if settings.HasMatchedHookContaining(s.root, "PreToolUse", "Agent", binaryIdentity) {
		return Check{Label: label, Level: OK}
	}
	return Check{Label: label, Level: Fail, Note: `no PreToolUse entry with matcher="Agent" referencing ` + binaryIdentity}
}

// checkSessionEndHook verifies the SessionEnd sync-trigger entry references our binary and the
// sync-trigger identity token. Unreadable settings is a hard failure like the other hook checks; a
// missing entry only warns, with the remediation: a machine with the first two families and no
// SessionEnd is one that has not run the upgrade path, not a broken installation.
func checkSessionEndHook(s loadedSettings) Check {
	label := "hook: SessionEnd (sync-trigger)"
	if c, done := s.beforeEntries(label, Warn, "; "+remediation); done {
		return c
	}
	if settings.HasHookContaining(s.root, "SessionEnd", binaryIdentity, settings.LabdrianSyncTriggerIdentity) {
		return Check{Label: label, Level: OK}
	}
	return Check{Label: label, Level: Warn, Note: "no SessionEnd entry referencing " + binaryIdentity + "; " + remediation}
}

// checkReviewReceiptHook verifies the PreToolUse entry for Bash references our binary and the
// review-receipt identity token. It is judged as checkSessionEndHook is.
func checkReviewReceiptHook(s loadedSettings) Check {
	label := `hook: PreToolUse matcher="Bash" (review-receipt)`
	if c, done := s.beforeEntries(label, Warn, "; "+remediation); done {
		return c
	}
	if settings.HasMatchedHookContaining(s.root, "PreToolUse", "Bash", binaryIdentity, settings.LabdrianReviewReceiptIdentity) {
		return Check{Label: label, Level: OK}
	}
	return Check{Label: label, Level: Warn, Note: `no PreToolUse entry with matcher="Bash" referencing ` + binaryIdentity + "; " + remediation}
}

// checkShaperClearanceGuard reports whether the shaper clearance deny guard (both PreToolUse
// entries and the permissions.deny backstop) is installed. A missing part warns with the same
// remediation as the other families added later. Even installed, the guard is a speed bump.
func checkShaperClearanceGuard(s loadedSettings) Check {
	label := "guard: shaper clearance record (PreToolUse + permissions.deny)"
	if c, done := s.beforeEntries(label, Warn, "; clearance recording is unguarded ("+speedBump+"); "+remediation); done {
		return c
	}
	if missing := settings.MissingShaperClearanceGuardParts(s.root, binaryIdentity); len(missing) > 0 {
		return Check{Label: label, Level: Warn,
			Note: "missing " + strings.Join(missing, ", ") + "; clearance recording is unguarded against the model (" + speedBump + "); " + remediation}
	}
	return Check{Label: label, Level: OK, Note: "installed; " + speedBump}
}

// checkProjectionHooks reports whether the projection hook family (the UserPromptSubmit context
// projection and the two PreToolUse gates) is exactly in place. A missing or drifted part warns
// with the same remediation, and an unreadable settings file fails. hookCommand is the installed
// binary path, because the entries are matched exactly, not by the identity alone.
//
// Installing the hooks is not enough for a running session: Claude Code loads hook changes only
// when it starts, so the note says to restart it.
func checkProjectionHooks(s loadedSettings, hookCommand string) Check {
	label := "hooks: projection (UserPromptSubmit + PreToolUse gates)"
	if c, done := s.beforeEntries(label, Warn, "; "+remediation+"; "+restartNote); done {
		return c
	}
	if missing := settings.MissingProjectionHookParts(s.root, hookCommand); len(missing) > 0 {
		return Check{Label: label, Level: Warn, Note: "missing or drifted: " + strings.Join(missing, ", ") + "; " + remediation + "; " + restartNote}
	}
	return Check{Label: label, Level: OK, Note: "installed; " + restartNote + " if this session started earlier"}
}

// checkApproveGuard reports whether the skills approve guard (the two PreToolUse entries that run
// 'skills guard-hook') is exactly in place, and hooks are not globally disabled. It is judged as
// checkProjectionHooks is, and even installed it is a speed bump, which the note says.
func checkApproveGuard(s loadedSettings, hookCommand string) Check {
	label := "guard: skills approve (PreToolUse Bash + file tools)"
	if c, done := s.beforeEntries(label, Warn, "; the agent can run skills approve unguarded ("+speedBump+"); "+remediation+"; "+restartNote); done {
		return c
	}
	if missing := settings.MissingApproveGuardParts(s.root, hookCommand); len(missing) > 0 {
		return Check{Label: label, Level: Warn,
			Note: "missing or drifted: " + strings.Join(missing, ", ") + "; the agent can run skills approve unguarded (" + speedBump + "); " + remediation + "; " + restartNote}
	}
	return Check{Label: label, Level: OK, Note: "installed; " + speedBump + "; " + restartNote + " if this session started earlier"}
}

// CheckContract verifies the contract at path is readable and its frontmatter parses.
func CheckContract(path string, files FileReader) Check {
	label := "contract: " + path
	data, err := files.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Check{Label: label, Level: Fail, Note: "not found"}
		}
		return Check{Label: label, Level: Fail, Note: err.Error()}
	}
	if _, err := contract.Parse(string(data)); err != nil {
		return Check{Label: label, Level: Fail, Note: "frontmatter error: " + err.Error()}
	}
	return Check{Label: label, Level: OK}
}

// checkRegistry reports on the registry of the project, distinguishing the outcomes by what each
// would do to a person who trusted it (registry authoritative, fail loud):
//
//   - absent: OK, quiet. A project that does not use the overlay is not a problem; this is the
//     only branch that stays silently OK.
//   - unreadable (a real error, not an absence): a failure. A genuine error must surface, never be
//     downgraded to an OK note.
//   - present but empty or only white space: a failure. An emptied registry is the incident's
//     misread state: it must be loud, never taken for zero skills.
//   - present, a scoped block missing: a warning that names the blocks. Actionable, not fatal.
//   - present with both scoped blocks: OK.
func checkRegistry(path string, files FileReader) Check {
	label := "registry: " + path
	data, err := files.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Check{Label: label, Level: OK, Note: "not present (project may not use the overlay)"}
		}
		return Check{Label: label, Level: Fail, Note: "cannot read: " + err.Error()}
	}
	content := string(data)
	if strings.TrimSpace(content) == "" {
		return Check{
			Label: label,
			Level: Fail,
			Note:  "present but EMPTY — run skill-registry refresh; do NOT conclude skills are absent (an empty registry is inconclusive, not zero)",
		}
	}
	hasMinimalism := strings.Contains(content, propagator.BeginMarker)
	hasDesign := strings.Contains(content, propagator.AntiGenericDesignBeginMarker)
	if hasMinimalism && hasDesign {
		return Check{Label: label, Level: OK, Note: "scoped block present"}
	}

	var missing []string
	if !hasMinimalism {
		missing = append(missing, "minimalism-contract-scope")
	}
	if !hasDesign {
		missing = append(missing, "anti-generic-design-scope")
	}
	return Check{Label: label, Level: Warn,
		Note: fmt.Sprintf("present but scoped block(s) missing: %s (run 'labdrian install-hooks' or propagate)", strings.Join(missing, ", "))}
}
