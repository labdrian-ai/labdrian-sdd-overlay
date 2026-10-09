package status

import (
	"encoding/json"
	"errors"
	"io/fs"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/contract"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/propagator"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings"
)

// The places of the world a test installs into, and the directory a person runs status from.
const (
	home            = "/h"
	project         = "/p"
	binaryPath      = "/h/.claude/bin/gentle-ai-overlay"
	settingsPath    = "/h/.claude/settings.json"
	contractPath    = "/h/.claude/skills/_shared/minimalism-contract.md"
	registryPath    = "/p/.atl/skill-registry.md"
	wantRemediation = "run 'labdrian uninstall-hooks' then 'labdrian install-hooks'"
	restart         = "restart Claude Code to load hook changes"
	wantSpeedBump   = "the guard is a speed bump, not a security boundary"
	validContract   = "---\napplies_to_phases: [sdd-tasks, sdd-apply]\n---\nbody\n"
	labelBinary     = "binary: " + binaryPath
	labelPrompt     = "hook: UserPromptSubmit (propagate)"
	labelAgent      = `hook: PreToolUse matcher="Agent" (gate-task)`
	labelSession    = "hook: SessionEnd (sync-trigger)"
	labelReceipt    = `hook: PreToolUse matcher="Bash" (review-receipt)`
	labelShaper     = "guard: shaper clearance record (PreToolUse + permissions.deny)"
	labelProj       = "hooks: projection (UserPromptSubmit + PreToolUse gates)"
	labelApprove    = "guard: skills approve (PreToolUse Bash + file tools)"
	labelContract   = "contract: " + contractPath
	labelRegistry   = "registry: " + registryPath
	bothBlocksText  = "# R\n" + propagator.BeginMarker + "\n" + propagator.EndMarker + "\n" + propagator.AntiGenericDesignBeginMarker + "\n" + propagator.AntiGenericDesignEndMarker + "\n"
)

// memFiles is a file system in memory: the modes and the contents of the paths it holds, and the
// error to answer for a path that is made to fail. Every path asked about is recorded.
type memFiles struct {
	modes    map[string]fs.FileMode
	contents map[string]string
	failures map[string]error
	asked    []string
}

func (m *memFiles) Stat(path string) (fs.FileMode, error) {
	m.asked = append(m.asked, "stat "+path)
	if err := m.failures[path]; err != nil {
		return 0, err
	}
	mode, ok := m.modes[path]
	if !ok {
		return 0, &fs.PathError{Op: "stat", Path: path, Err: fs.ErrNotExist}
	}
	return mode, nil
}

func (m *memFiles) ReadFile(path string) ([]byte, error) {
	m.asked = append(m.asked, "read "+path)
	if err := m.failures[path]; err != nil {
		return nil, err
	}
	content, ok := m.contents[path]
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
	}
	return []byte(content), nil
}

// memSettings answers the settings of the one path it holds, or the error it is made to fail
// with. A path it knows nothing of is a file that does not exist: the document with nothing in it.
type memSettings struct {
	docs   map[string]settings.Document
	errs   map[string]error
	loaded []string
}

func (m *memSettings) Settings(path string) (settings.Document, error) {
	m.loaded = append(m.loaded, path)
	if err := m.errs[path]; err != nil {
		return settings.Document{}, err
	}
	return m.docs[path], nil
}

// world is an installation in memory, which a test then spoils.
type world struct {
	files    *memFiles
	settings *memSettings
}

// rootOf is the complete settings object the installer writes for the binary, decoded the way
// the status check is given it, so a test can take parts away.
func rootOf(t *testing.T) map[string]interface{} {
	t.Helper()
	doc := settings.Empty()
	if _, err := doc.Merge(binaryPath); err != nil {
		t.Fatal(err)
	}
	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]interface{}
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	return root
}

// docOf is the document of a settings object.
func docOf(t *testing.T, root map[string]interface{}) settings.Document {
	t.Helper()
	data, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := settings.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

// complete is an installation status finds nothing wrong with, and no registry in the project.
func complete(t *testing.T) *world {
	t.Helper()
	w := &world{
		files: &memFiles{
			modes:    map[string]fs.FileMode{binaryPath: 0o755},
			contents: map[string]string{contractPath: validContract},
			failures: map[string]error{},
		},
		settings: &memSettings{docs: map[string]settings.Document{}, errs: map[string]error{}},
	}
	w.settings.docs[settingsPath] = docOf(t, rootOf(t))
	return w
}

// editSettings replaces the settings of the world with the complete ones after edit.
func (w *world) editSettings(t *testing.T, edit func(root map[string]interface{})) {
	t.Helper()
	root := rootOf(t)
	edit(root)
	w.settings.docs[settingsPath] = docOf(t, root)
}

func (w *world) check() Report {
	return Service{Files: w.files, Settings: w.settings}.Check(Request{Home: home, Cwd: project})
}

// named is the check whose label is exactly label.
func (r Report) named(t *testing.T, label string) Check {
	t.Helper()
	for _, c := range r.Checks {
		if c.Label == label {
			return c
		}
	}
	t.Fatalf("no check is labelled %q in %v", label, r.labels())
	return Check{}
}

func (r Report) labels() []string {
	var labels []string
	for _, c := range r.Checks {
		labels = append(labels, c.Label)
	}
	return labels
}

func wantCheck(t *testing.T, got Check, level Level, note string) {
	t.Helper()
	if got.Level != level || got.Note != note {
		t.Errorf("%s: got %v %q, want %v %q", got.Label, got.Level, got.Note, level, note)
	}
}

// dropHooks takes out of the entries under event those whose commands contain word.
func dropHooks(root map[string]interface{}, event, word string) {
	hooks := root["hooks"].(map[string]interface{})
	var kept []interface{}
	for _, e := range hooks[event].([]interface{}) {
		if !strings.Contains(commandsOf(e), word) {
			kept = append(kept, e)
		}
	}
	hooks[event] = kept
}

func commandsOf(entry interface{}) string {
	var out []string
	for _, ih := range entry.(map[string]interface{})["hooks"].([]interface{}) {
		out = append(out, ih.(map[string]interface{})["command"].(string))
	}
	return strings.Join(out, "\n")
}

func TestACompleteInstallIsHealthyAndReportsEveryCheckInOrder(t *testing.T) {
	report := complete(t).check()
	want := []string{labelBinary, labelPrompt, labelAgent, labelSession, labelReceipt, labelShaper, labelProj, labelApprove, labelContract, labelRegistry}
	if got := report.labels(); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("checks\n got: %q\nwant: %q", got, want)
	}
	for _, c := range report.Checks[:len(report.Checks)-1] {
		if c.Level != OK {
			t.Errorf("%s: %v %q, want OK", c.Label, c.Level, c.Note)
		}
	}
	wantCheck(t, report.named(t, labelShaper), OK, "installed; "+wantSpeedBump)
	wantCheck(t, report.named(t, labelProj), OK, "installed; "+restart+" if this session started earlier")
	wantCheck(t, report.named(t, labelApprove), OK, "installed; "+wantSpeedBump+"; "+restart+" if this session started earlier")
	wantCheck(t, report.named(t, labelRegistry), OK, "not present (project may not use the overlay)")
	if report.Outcome() != Healthy {
		t.Errorf("Outcome = %v, want Healthy", report.Outcome())
	}
}

func TestTheSettingsAreLoadedOnceAndOnlyThePlacesOfTheInstallationAreAsked(t *testing.T) {
	w := complete(t)
	w.check()
	if strings.Join(w.settings.loaded, "|") != settingsPath {
		t.Errorf("settings loaded: %q, want only %q, once", w.settings.loaded, settingsPath)
	}
	want := "stat " + binaryPath + "|read " + contractPath + "|read " + registryPath
	if got := strings.Join(w.files.asked, "|"); got != want {
		t.Errorf("files asked: %q, want %q", got, want)
	}
}

func TestWithoutADirectoryThereIsNoRegistryToLookFor(t *testing.T) {
	w := complete(t)
	report := Service{Files: w.files, Settings: w.settings}.Check(Request{Home: home})
	for _, c := range report.Checks {
		if strings.HasPrefix(c.Label, "registry:") {
			t.Errorf("a registry check was made without a directory: %v", c)
		}
	}
	if len(report.Checks) != 9 {
		t.Errorf("%d checks, want 9", len(report.Checks))
	}
	for _, asked := range w.files.asked {
		if strings.Contains(asked, "skill-registry") {
			t.Errorf("the registry was asked for: %q", asked)
		}
	}
}

func TestTheLayoutFollowsTheHomeAsItIsGivenEvenWhenItIsEmpty(t *testing.T) {
	w := complete(t)
	report := Service{Files: w.files, Settings: w.settings}.Check(Request{Home: "", Cwd: "rel"})
	want := []string{
		"binary: .claude/bin/gentle-ai-overlay",
		"contract: .claude/skills/_shared/minimalism-contract.md",
		"registry: rel/.atl/skill-registry.md",
	}
	got := []string{report.Checks[0].Label, report.Checks[8].Label, report.Checks[9].Label}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("labels = %q, want %q", got, want)
	}
	if w.settings.loaded[0] != ".claude/settings.json" {
		t.Errorf("settings loaded from %q, want .claude/settings.json", w.settings.loaded[0])
	}
}

func TestTheBinaryNeedsToBeThereAndToHaveAnExecuteBit(t *testing.T) {
	cases := []struct {
		name   string
		mode   fs.FileMode
		absent bool
		fail   error
		level  Level
		note   string
	}{
		{"executable", 0o755, false, nil, OK, ""},
		{"the owner's bit alone", 0o100, false, nil, OK, ""},
		{"the group's bit alone", 0o010, false, nil, OK, ""},
		{"the bit of anyone", 0o001, false, nil, OK, ""},
		{"a directory has the bits", fs.ModeDir | 0o755, false, nil, OK, ""},
		{"no bit", 0o644, false, nil, Fail, "exists but not executable"},
		{"not there", 0, true, nil, Fail, "not found"},
		{"another failure is told as the system told it", 0, false, errors.New("permission denied"), Fail, "permission denied"},
		{"a wrapped absence is still an absence", 0, false, &fs.PathError{Op: "stat", Path: binaryPath, Err: fs.ErrNotExist}, Fail, "not found"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := complete(t)
			delete(w.files.modes, binaryPath)
			switch {
			case c.fail != nil:
				w.files.failures[binaryPath] = c.fail
			case !c.absent:
				w.files.modes[binaryPath] = c.mode
			}
			wantCheck(t, w.check().named(t, labelBinary), c.level, c.note)
		})
	}
}

func TestSettingsThatCannotBeReadFailEveryCheckThatLooksAtThem(t *testing.T) {
	w := complete(t)
	w.settings.errs[settingsPath] = errors.New("invalid JSON: boom")
	report := w.check()
	note := "cannot read " + settingsPath + ": invalid JSON: boom"
	for _, label := range []string{labelPrompt, labelAgent, labelSession, labelReceipt, labelShaper, labelProj, labelApprove} {
		wantCheck(t, report.named(t, label), Fail, note)
	}
	wantCheck(t, report.named(t, labelBinary), OK, "")
	if report.Outcome() != Failed {
		t.Errorf("Outcome = %v, want Failed", report.Outcome())
	}
}

func TestSettingsWithNothingInThemFailTheTwoBaseHooksAndWarnOfTheOthers(t *testing.T) {
	w := complete(t)
	w.settings.docs[settingsPath] = settings.Document{}
	report := w.check()
	absent := settingsPath + " absent or empty"
	wantCheck(t, report.named(t, labelPrompt), Fail, absent)
	wantCheck(t, report.named(t, labelAgent), Fail, absent)
	wantCheck(t, report.named(t, labelSession), Warn, absent+"; "+wantRemediation)
	wantCheck(t, report.named(t, labelReceipt), Warn, absent+"; "+wantRemediation)
	wantCheck(t, report.named(t, labelShaper), Warn, absent+"; clearance recording is unguarded ("+wantSpeedBump+"); "+wantRemediation)
	wantCheck(t, report.named(t, labelProj), Warn, absent+"; "+wantRemediation+"; "+restart)
	wantCheck(t, report.named(t, labelApprove), Warn, absent+"; the agent can run skills approve unguarded ("+wantSpeedBump+"); "+wantRemediation+"; "+restart)
	if report.Outcome() != Failed {
		t.Errorf("Outcome = %v, want Failed", report.Outcome())
	}
}

func TestSettingsWithoutAHooksObjectFailTheTwoBaseHooksWithTheirOwnWords(t *testing.T) {
	for name, root := range map[string]map[string]interface{}{
		"no hooks key":     {},
		"hooks is null":    {"hooks": nil},
		"hooks is a list":  {"hooks": []interface{}{}},
		"hooks is a value": {"hooks": "x"},
	} {
		t.Run(name, func(t *testing.T) {
			w := complete(t)
			w.settings.docs[settingsPath] = docOf(t, root)
			report := w.check()
			wantCheck(t, report.named(t, labelPrompt), Fail, "hooks key missing in settings.json")
			wantCheck(t, report.named(t, labelAgent), Fail, "hooks key missing in settings.json")
			wantCheck(t, report.named(t, labelSession), Warn, "no SessionEnd entry referencing gentle-ai-overlay; "+wantRemediation)
		})
	}
}

func TestAnEmptyHooksObjectHasNoEntryForTheBaseHooks(t *testing.T) {
	w := complete(t)
	w.settings.docs[settingsPath] = docOf(t, map[string]interface{}{"hooks": map[string]interface{}{}})
	report := w.check()
	wantCheck(t, report.named(t, labelPrompt), Fail, "no UserPromptSubmit entry referencing gentle-ai-overlay")
	wantCheck(t, report.named(t, labelAgent), Fail, `no PreToolUse entry with matcher="Agent" referencing gentle-ai-overlay`)
	wantCheck(t, report.named(t, labelReceipt), Warn, `no PreToolUse entry with matcher="Bash" referencing gentle-ai-overlay; `+wantRemediation)
}

func entry(matcher interface{}, command string) map[string]interface{} {
	e := map[string]interface{}{"hooks": []interface{}{map[string]interface{}{"type": "command", "command": command}}}
	if matcher != nil {
		e["matcher"] = matcher
	}
	return e
}

func TestTheBaseHooksNeedTheBinaryInAnEntryOfTheEventAndAgentAsTheMatcherOfTheSecond(t *testing.T) {
	cases := []struct {
		name         string
		events       map[string]interface{}
		prompt, tool Level
	}{
		{"the binary under both", map[string]interface{}{
			"UserPromptSubmit": []interface{}{entry(nil, binaryPath+" propagate")},
			"PreToolUse":       []interface{}{entry("Agent", binaryPath+" gate-task")},
		}, OK, OK},
		{"the prompt hook looks at no matcher", map[string]interface{}{
			"UserPromptSubmit": []interface{}{entry("whatever", binaryPath)},
		}, OK, Fail},
		{"someone else's entries", map[string]interface{}{
			"UserPromptSubmit": []interface{}{entry(nil, "echo hi")},
			"PreToolUse":       []interface{}{entry("Agent", "echo hi")},
		}, Fail, Fail},
		{"the identity alone is a substring, not the path", map[string]interface{}{
			"UserPromptSubmit": []interface{}{entry(nil, "/elsewhere/gentle-ai-overlay-2 propagate")},
			"PreToolUse":       []interface{}{entry("Agent", "gentle-ai-overlay")},
		}, OK, OK},
		{"another matcher", map[string]interface{}{
			"PreToolUse": []interface{}{entry("agent", binaryPath), entry("Bash", binaryPath), entry(5, binaryPath), entry(nil, binaryPath)},
		}, Fail, Fail},
		{"the matcher is found in a later entry", map[string]interface{}{
			"PreToolUse": []interface{}{entry("Bash", binaryPath), "x", 7, entry("Agent", binaryPath)},
		}, Fail, OK},
		{"entries that are not entries", map[string]interface{}{
			"UserPromptSubmit": []interface{}{"x", 1, nil, map[string]interface{}{"hooks": "x"}, map[string]interface{}{"hooks": []interface{}{7, map[string]interface{}{"command": 7}}}},
		}, Fail, Fail},
		{"events that are not lists", map[string]interface{}{"UserPromptSubmit": "x", "PreToolUse": map[string]interface{}{}}, Fail, Fail},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := complete(t)
			w.settings.docs[settingsPath] = docOf(t, map[string]interface{}{"hooks": c.events})
			report := w.check()
			if got := report.named(t, labelPrompt).Level; got != c.prompt {
				t.Errorf("UserPromptSubmit check = %v, want %v", got, c.prompt)
			}
			if got := report.named(t, labelAgent).Level; got != c.tool {
				t.Errorf("PreToolUse Agent check = %v, want %v", got, c.tool)
			}
		})
	}
}

func TestSessionEndAndTheReceiptHookNeedTheBinaryAndTheWordOfTheirFamilyInOneEntry(t *testing.T) {
	cases := []struct {
		name             string
		events           map[string]interface{}
		session, receipt Level
	}{
		{"both, with the words", map[string]interface{}{
			"SessionEnd": []interface{}{entry(nil, binaryPath+" sync-trigger")},
			"PreToolUse": []interface{}{entry("Bash", binaryPath+" review-receipt hook")},
		}, OK, OK},
		{"the binary without the words", map[string]interface{}{
			"SessionEnd": []interface{}{entry(nil, binaryPath+" other")},
			"PreToolUse": []interface{}{entry("Bash", binaryPath+" other")},
		}, Warn, Warn},
		{"the words without the binary", map[string]interface{}{
			"SessionEnd": []interface{}{entry(nil, "x sync-trigger")},
			"PreToolUse": []interface{}{entry("Bash", "x review-receipt")},
		}, Warn, Warn},
		{"the words in two entries", map[string]interface{}{
			"SessionEnd": []interface{}{entry(nil, binaryPath), entry(nil, "sync-trigger")},
			"PreToolUse": []interface{}{entry("Bash", binaryPath), entry("Bash", "review-receipt")},
		}, Warn, Warn},
		{"the words in two inner hooks of one entry", map[string]interface{}{
			"SessionEnd": []interface{}{map[string]interface{}{"hooks": []interface{}{
				map[string]interface{}{"command": binaryPath}, map[string]interface{}{"command": "sync-trigger"}}}},
		}, OK, Warn},
		{"the receipt hook under another matcher", map[string]interface{}{
			"PreToolUse": []interface{}{entry("Agent", binaryPath+" review-receipt"), entry(nil, binaryPath+" review-receipt")},
		}, Warn, Warn},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := complete(t)
			w.settings.docs[settingsPath] = docOf(t, map[string]interface{}{"hooks": c.events})
			report := w.check()
			if got := report.named(t, labelSession).Level; got != c.session {
				t.Errorf("SessionEnd check = %v, want %v", got, c.session)
			}
			if got := report.named(t, labelReceipt).Level; got != c.receipt {
				t.Errorf("review-receipt check = %v, want %v", got, c.receipt)
			}
		})
	}
}

func TestAMissingFamilyIsOnlyWarnedOfWhileAMissingBaseHookFails(t *testing.T) {
	w := complete(t)
	w.editSettings(t, func(root map[string]interface{}) {
		delete(root["hooks"].(map[string]interface{}), "SessionEnd")
		dropHooks(root, "PreToolUse", "review-receipt")
	})
	report := w.check()
	wantCheck(t, report.named(t, labelSession), Warn, "no SessionEnd entry referencing gentle-ai-overlay; "+wantRemediation)
	wantCheck(t, report.named(t, labelReceipt), Warn, `no PreToolUse entry with matcher="Bash" referencing gentle-ai-overlay; `+wantRemediation)
	if report.Outcome() != Degraded {
		t.Errorf("Outcome = %v, want Degraded", report.Outcome())
	}
	w.editSettings(t, func(root map[string]interface{}) { dropHooks(root, "PreToolUse", "gate-task") })
	if got := w.check().Outcome(); got != Failed {
		t.Errorf("Outcome without the Agent hook = %v, want Failed", got)
	}
}

func TestTheShaperGuardNamesWhatIsMissingAndIsOnlyWarnedOf(t *testing.T) {
	cases := []struct {
		name  string
		strip func(root map[string]interface{})
	}{
		{"the guard hooks", func(root map[string]interface{}) { dropHooks(root, "PreToolUse", settings.LabdrianShaperGuardIdentity) }},
		{"guard hooks of another binary are not ours", func(root map[string]interface{}) {
			for _, e := range root["hooks"].(map[string]interface{})["PreToolUse"].([]interface{}) {
				if strings.Contains(commandsOf(e), settings.LabdrianShaperGuardIdentity) {
					e.(map[string]interface{})["hooks"].([]interface{})[0].(map[string]interface{})["command"] = "/usr/bin/other " + settings.LabdrianShaperGuardIdentity
				}
			}
		}},
		{"the deny rule", func(root map[string]interface{}) { delete(root, "permissions") }},
		{"hooks disabled", func(root map[string]interface{}) { root["disableAllHooks"] = true }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := complete(t)
			w.editSettings(t, c.strip)
			parts := settings.MissingShaperClearanceGuardParts(w.settings.docs[settingsPath].Root(), binaryPath)
			if len(parts) == 0 {
				t.Fatal("the settings are missing nothing; the case proves nothing")
			}
			wantCheck(t, w.check().named(t, labelShaper), Warn,
				"missing "+strings.Join(parts, ", ")+"; clearance recording is unguarded against the model ("+wantSpeedBump+"); "+wantRemediation)
		})
	}
}

func TestTheProjectionHooksNameWhatIsMissingOrDriftedAndAskForARestart(t *testing.T) {
	cases := []struct {
		name  string
		strip func(root map[string]interface{})
	}{
		{"missing", func(root map[string]interface{}) {
			dropHooks(root, "UserPromptSubmit", "projection hook")
			dropHooks(root, "PreToolUse", "projection hook")
		}},
		{"drifted", func(root map[string]interface{}) {
			for _, e := range root["hooks"].(map[string]interface{})["UserPromptSubmit"].([]interface{}) {
				if strings.Contains(commandsOf(e), "projection hook") {
					e.(map[string]interface{})["hooks"].([]interface{})[0].(map[string]interface{})["command"] = commandsOf(e) + " # edited"
				}
			}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := complete(t)
			w.editSettings(t, c.strip)
			parts := settings.MissingProjectionHookParts(w.settings.docs[settingsPath].Root(), binaryPath)
			if len(parts) == 0 {
				t.Fatal("the settings are missing nothing; the case proves nothing")
			}
			wantCheck(t, w.check().named(t, labelProj), Warn, "missing or drifted: "+strings.Join(parts, ", ")+"; "+wantRemediation+"; "+restart)
		})
	}
}

func TestTheApproveGuardNamesWhatIsMissingOrDriftedAndTheLimitOfAGuard(t *testing.T) {
	for name, strip := range map[string]func(root map[string]interface{}){
		"missing":        func(root map[string]interface{}) { dropHooks(root, "PreToolUse", "skills guard-hook") },
		"hooks disabled": func(root map[string]interface{}) { root["disableAllHooks"] = true },
	} {
		t.Run(name, func(t *testing.T) {
			w := complete(t)
			w.editSettings(t, strip)
			parts := settings.MissingApproveGuardParts(w.settings.docs[settingsPath].Root(), binaryPath)
			if len(parts) == 0 {
				t.Fatal("the settings are missing nothing; the case proves nothing")
			}
			wantCheck(t, w.check().named(t, labelApprove), Warn,
				"missing or drifted: "+strings.Join(parts, ", ")+"; the agent can run skills approve unguarded ("+wantSpeedBump+"); "+wantRemediation+"; "+restart)
		})
	}
}

func TestTheProjectionAndApproveHooksAreMatchedByTheInstalledPathAndNotByTheIdentityAlone(t *testing.T) {
	w := complete(t)
	w.settings.docs["/other/.claude/settings.json"] = w.settings.docs[settingsPath]
	report := Service{Files: w.files, Settings: w.settings}.Check(Request{Home: "/other", Cwd: project})
	// The entries name /h/.claude/bin/gentle-ai-overlay; under another home that is not the binary.
	if report.named(t, "hooks: projection (UserPromptSubmit + PreToolUse gates)").Level != Warn {
		t.Error("the projection hooks of another binary were taken for this one's")
	}
	if report.named(t, "guard: skills approve (PreToolUse Bash + file tools)").Level != Warn {
		t.Error("the approve guard of another binary was taken for this one's")
	}
}

func TestTheContractMustBeReadableAndItsFrontmatterParse(t *testing.T) {
	_, parseErr := contract.Parse("no frontmatter here")
	if parseErr == nil {
		t.Fatal("contract.Parse accepted text with no frontmatter")
	}
	cases := []struct {
		name    string
		content string
		absent  bool
		fail    error
		level   Level
		note    string
	}{
		{"well-formed", validContract, false, nil, OK, ""},
		{"not there", "", true, nil, Fail, "not found"},
		{"no frontmatter", "no frontmatter here", false, nil, Fail, "frontmatter error: " + parseErr.Error()},
		{"an empty file has none either", "", false, nil, Fail, "frontmatter error: contract file has no YAML frontmatter (expected content between --- delimiters)"},
		{"unreadable", "", false, errors.New("is a directory"), Fail, "is a directory"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := complete(t)
			switch {
			case c.fail != nil:
				w.files.failures[contractPath] = c.fail
			case c.absent:
				delete(w.files.contents, contractPath)
			default:
				w.files.contents[contractPath] = c.content
			}
			wantCheck(t, w.check().named(t, labelContract), c.level, c.note)
		})
	}
}

func TestCheckContractIsTheContractCheckOfAnyPathOverAnyReader(t *testing.T) {
	files := &memFiles{contents: map[string]string{"/virtual/c.md": validContract}}
	wantCheck(t, CheckContract("/virtual/c.md", files), OK, "")
	if got := CheckContract("/virtual/c.md", files).Label; got != "contract: /virtual/c.md" {
		t.Errorf("label = %q", got)
	}
	wantCheck(t, CheckContract("/virtual/none.md", files), Fail, "not found")
}

func TestTheRegistryOfTheProjectIsQuietWhenAbsentLoudWhenEmptyOrUnreadableAndWarnsOfMissingBlocks(t *testing.T) {
	missingBoth := "present but scoped block(s) missing: minimalism-contract-scope, anti-generic-design-scope (run 'labdrian install-hooks' or propagate)"
	const empty = "present but EMPTY — run skill-registry refresh; do NOT conclude skills are absent (an empty registry is inconclusive, not zero)"
	cases := []struct {
		name    string
		content string
		absent  bool
		fail    error
		level   Level
		note    string
	}{
		{"absent", "", true, nil, OK, "not present (project may not use the overlay)"},
		{"unreadable", "", false, errors.New("permission denied"), Fail, "cannot read: permission denied"},
		{"empty", "", false, nil, Fail, empty},
		{"white space", " \n\t\n", false, nil, Fail, empty},
		{"neither block", "# R\n", false, nil, Warn, missingBoth},
		{"only the minimalism block", "# R\n" + propagator.BeginMarker + "\n", false, nil, Warn,
			"present but scoped block(s) missing: anti-generic-design-scope (run 'labdrian install-hooks' or propagate)"},
		{"only the design block", propagator.AntiGenericDesignBeginMarker, false, nil, Warn,
			"present but scoped block(s) missing: minimalism-contract-scope (run 'labdrian install-hooks' or propagate)"},
		{"the end markers are not read", propagator.EndMarker + propagator.AntiGenericDesignEndMarker, false, nil, Warn, missingBoth},
		{"both blocks", bothBlocksText, false, nil, OK, "scoped block present"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := complete(t)
			switch {
			case c.fail != nil:
				w.files.failures[registryPath] = c.fail
			case !c.absent:
				w.files.contents[registryPath] = c.content
			}
			wantCheck(t, w.check().named(t, labelRegistry), c.level, c.note)
		})
	}
}

func TestTheOutcomeOfAReportIsItsWorstCheck(t *testing.T) {
	c := func(l Level) Check { return Check{Label: "x", Level: l} }
	cases := []struct {
		name   string
		checks []Check
		want   Outcome
	}{
		{"no check", nil, Healthy},
		{"all OK", []Check{c(OK), c(OK)}, Healthy},
		{"a warning", []Check{c(OK), c(Warn), c(OK)}, Degraded},
		{"a failure", []Check{c(OK), c(Fail)}, Failed},
		{"a failure outweighs a warning, in either order", []Check{c(Warn), c(Fail)}, Failed},
		{"a failure outweighs a warning, the other order", []Check{c(Fail), c(Warn)}, Failed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := (Report{Checks: tc.checks}).Outcome(); got != tc.want {
				t.Errorf("Outcome = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestEveryLevelAndOutcomeHasANameForAFailureMessage(t *testing.T) {
	for _, l := range []Level{OK, Warn, Fail} {
		if l.String() == "" || strings.Contains(l.String(), "(") {
			t.Errorf("Level %d has no name: %q", int(l), l.String())
		}
	}
	for _, o := range []Outcome{Healthy, Degraded, Failed} {
		if o.String() == "" || strings.Contains(o.String(), "(") {
			t.Errorf("Outcome %d has no name: %q", int(o), o.String())
		}
	}
}
