package domain

import (
	"fmt"
	"strings"
	"testing"
)

// TestHooksActionsRegistered verifies all three hooks actions are in Actions() with
// correct TargetAgnostic and Mutating values (R-009 Scenario 9.1).
func TestHooksActionsRegistered(t *testing.T) {
	actions := Actions()
	byCmd := make(map[string]Action)
	for _, a := range actions {
		byCmd[a.Command] = a
		// status-hooks is nested under Estado's Also slice, not top-level.
		for _, sub := range a.Also {
			byCmd[sub.Command] = sub
		}
	}

	for _, cmd := range []string{"status-hooks", "install-hooks", "uninstall-hooks"} {
		a, ok := byCmd[cmd]
		if !ok {
			t.Errorf("Actions() must contain %q", cmd)
			continue
		}
		if !a.TargetAgnostic {
			t.Errorf("%s must have TargetAgnostic: true", cmd)
		}
	}

	if sh, ok := byCmd["status-hooks"]; ok && sh.Mutating {
		t.Error("status-hooks must have Mutating: false (read-only)")
	}
	for _, cmd := range []string{"install-hooks", "uninstall-hooks"} {
		if a, ok := byCmd[cmd]; ok && !a.Mutating {
			t.Errorf("%s must have Mutating: true", cmd)
		}
	}
}

// TestBackwardsCompatZeroValues verifies existing actions have zero-value TargetAgnostic
// and empty ConfirmMessage, preserving prior behavior (R-010 Scenario 10.4, backwards-compat).
func TestBackwardsCompatZeroValues(t *testing.T) {
	for _, a := range Actions() {
		if a.Command == "apply" || a.Command == "capture" {
			if a.TargetAgnostic {
				t.Errorf("%s must have TargetAgnostic: false (backwards compat)", a.Command)
			}
			if a.ConfirmMessage != "" {
				t.Errorf("%s must have empty ConfirmMessage (backwards compat), got %q", a.Command, a.ConfirmMessage)
			}
		}
	}
}

// TestSkillsActionsRegistered verifies the single top-level "skills" action
// (status verb) is registered with correct field values, and that validate/
// list are merged into it via Also rather than appearing as separate
// top-level entries.
func TestSkillsActionsRegistered(t *testing.T) {
	var skills Action
	found := false
	for _, a := range Actions() {
		if a.Command == "skills" {
			skills = a
			found = true
			break
		}
	}
	if !found {
		t.Fatal("Actions() must contain a top-level skills action")
	}
	if len(skills.Args) == 0 || skills.Args[0] != "status" {
		t.Errorf("top-level skills action Args[0] = %v, want [status ...]", skills.Args)
	}
	if !skills.TargetAgnostic {
		t.Error("skills action must have TargetAgnostic: true")
	}
	if skills.Mutating {
		t.Error("skills action must have Mutating: false (read-only)")
	}
	if skills.Hint == "" {
		t.Error("skills action must have a non-empty Hint")
	}

	if len(skills.Also) != 2 {
		t.Fatalf("skills action must have exactly 2 Also entries, got %d: %+v", len(skills.Also), skills.Also)
	}
	byVerb := make(map[string]Action)
	for _, sub := range skills.Also {
		if sub.Command != "skills" {
			t.Errorf("Also entry must have Command %q, got %q", "skills", sub.Command)
		}
		if len(sub.Args) == 0 {
			t.Fatalf("Also entry must have a non-empty Args, got %+v", sub)
		}
		byVerb[sub.Args[0]] = sub
	}
	for _, verb := range []string{"validate", "list"} {
		sub, ok := byVerb[verb]
		if !ok {
			t.Errorf("skills Also must contain a verb %q, got %+v", verb, skills.Also)
			continue
		}
		if !sub.TargetAgnostic {
			t.Errorf("skills %s (Also) must have TargetAgnostic: true", verb)
		}
	}
}

// TestSkillRegistryRefreshNotInTUIMenu is a regression guard: the
// "Actualizar registry SDD Codex" (skill-registry refresh) action was
// intentionally removed from the TUI menu as part of the 11->7 simplification.
// The backend capability (bin/labdrian-overlay skill-registry refresh) is
// untouched and remains reachable via the bash CLI directly — only its TUI
// menu entry is dropped, and it must not exist anywhere in the menu, top-level
// or nested under Also.
func TestSkillRegistryRefreshNotInTUIMenu(t *testing.T) {
	for _, a := range Actions() {
		if a.Command == "skill-registry" {
			t.Errorf("Actions() must NOT contain a top-level skill-registry action, found %+v", a)
		}
		for _, sub := range a.Also {
			if sub.Command == "skill-registry" {
				t.Errorf("Actions() must NOT contain a nested skill-registry action, found %+v under %q", sub, a.Name)
			}
		}
	}
}

// TestActionMenuShapeAndOrder locks in the 11->7 simplification (now 12: 9
// plus the three longterm-mem rows): top-level actions in the order matching
// the "Verificar → Capturar → Aplicar → Restaurar" flow copy (viewActions),
// with Capturar coming before Aplicar and restore immediately after apply
// (R-011: a recovery action belongs next to the deploy action it undoes), and
// the longterm-mem block last, after skills.
//
// The sequence is compared as Command+Args, not Command alone: the three
// longterm-mem entries share one Command and are distinguished only by their
// positional subcommand, so a Command-keyed comparison would accept them in
// any order — including status after uninstall.
func TestActionMenuShapeAndOrder(t *testing.T) {
	actions := Actions()
	if len(actions) != 12 {
		t.Fatalf("expected 12 top-level actions, got %d: %v", len(actions), actions)
	}

	want := []string{
		"status", "sync-check", "capture", "apply", "restore", "self-update",
		"install-hooks", "uninstall-hooks", "skills status",
		"longterm-mem status", "longterm-mem install", "longterm-mem uninstall",
	}
	got := make([]string, len(actions))
	for i, a := range actions {
		got[i] = strings.Join(append([]string{a.Command}, a.Args...), " ")
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("action %d: Command+Args = %q, want %q (full sequence: %v)", i, got[i], w, got)
		}
	}
}

// TestSelfUpdateActionRegistered verifies Actions() registers the
// "Actualizar repositorio" entry (R-003) with TargetAgnostic and Mutating
// both true, positioned immediately after "restore" (this slice inserted
// restore between apply and self-update, R-011) and immediately before
// "install-hooks" per design.md D7's repo-maintenance grouping.
func TestSelfUpdateActionRegistered(t *testing.T) {
	actions := Actions()
	idx := -1
	for i, a := range actions {
		if a.Command == "self-update" {
			idx = i
			break
		}
	}
	if idx == -1 {
		t.Fatal(`Actions() must contain an entry with Command "self-update"`)
	}

	a := actions[idx]
	if a.Name != "Actualizar repositorio" {
		t.Errorf("self-update Name = %q, want %q", a.Name, "Actualizar repositorio")
	}
	if !a.Mutating {
		t.Error("self-update must have Mutating: true")
	}
	if !a.TargetAgnostic {
		t.Error("self-update must have TargetAgnostic: true")
	}

	prev := ""
	if idx > 0 {
		prev = actions[idx-1].Command
	}
	if prev != "restore" {
		t.Errorf("self-update must come immediately after restore, got previous entry %q", prev)
	}

	next := ""
	if idx+1 < len(actions) {
		next = actions[idx+1].Command
	}
	if next != "install-hooks" {
		t.Errorf("self-update must come immediately before install-hooks, got next entry %q", next)
	}
}

// TestSelfUpdateActionChainsApply verifies the self-update entry merges
// "apply" into it via Also (menos-pasos change): running "Actualizar
// repositorio" once both fast-forwards main AND deploys it to the selected
// targets, collapsing what used to be two separate manual steps (self-update,
// then remembering to run apply) into one. Mirrors the existing skills/status
// Also-composition pattern (TestSkillsActionsRegistered) rather than a
// hand-rolled shape.
func TestSelfUpdateActionChainsApply(t *testing.T) {
	var selfUpdate Action
	found := false
	for _, a := range Actions() {
		if a.Command == "self-update" {
			selfUpdate = a
			found = true
			break
		}
	}
	if !found {
		t.Fatal(`Actions() must contain a top-level self-update action`)
	}

	if len(selfUpdate.Also) != 1 {
		t.Fatalf("self-update action must have exactly 1 Also entry (apply), got %d: %+v", len(selfUpdate.Also), selfUpdate.Also)
	}
	apply := selfUpdate.Also[0]
	if apply.Command != "apply" {
		t.Errorf("self-update's Also entry Command = %q, want %q", apply.Command, "apply")
	}
	if !apply.SupportsAll {
		t.Error("chained apply must have SupportsAll: true, so every selected target deploys in one invocation")
	}
	if apply.TargetAgnostic {
		t.Error("chained apply must NOT be TargetAgnostic -- it deploys to the selected targets")
	}

	if !selfUpdate.UsesTargets() {
		t.Error("self-update must report usesTargets() == true once apply is chained onto it via Also")
	}
}

// TestRestoreActionRegistered verifies the "Restaurar respaldo" entry:
// Mutating, per-target (never SupportsAll, mirroring capture's --target-all
// refusal), and carrying a non-empty overwrite-warning ConfirmMessage.
func TestRestoreActionRegistered(t *testing.T) {
	var restore Action
	found := false
	for _, a := range Actions() {
		if a.Command == "restore" {
			restore = a
			found = true
			break
		}
	}
	if !found {
		t.Fatal(`Actions() must contain a top-level "restore" action`)
	}
	if !restore.Mutating {
		t.Error("restore must have Mutating: true")
	}
	if restore.SupportsAll {
		t.Error("restore must have SupportsAll: false (the backend refuses --target all for restore)")
	}
	if restore.TargetAgnostic {
		t.Error("restore must NOT be TargetAgnostic -- it always operates on a specific target")
	}
	if restore.ConfirmMessage == "" {
		t.Error("restore must have a non-empty ConfirmMessage (R-003: must state that restore overwrites deployed files)")
	}
	if !strings.Contains(restore.ConfirmMessage, "sobrescribiendo") {
		t.Errorf("restore ConfirmMessage must warn about overwriting deployed files, got %q", restore.ConfirmMessage)
	}
}

// TestVersionFoldedIntoEstadoAlso verifies the "version" subcommand (R-002)
// is merged into Estado's Also list, TargetAgnostic and read-only, rather
// than appearing as a separate top-level menu entry.
func TestVersionFoldedIntoEstadoAlso(t *testing.T) {
	var estado Action
	found := false
	for _, a := range Actions() {
		if a.Command == "status" {
			estado = a
			found = true
			break
		}
		if a.Command == "version" {
			t.Fatal(`"version" must NOT appear as a separate top-level action`)
		}
	}
	if !found {
		t.Fatal(`Actions() must contain the top-level "status" (Estado) action`)
	}

	var version Action
	versionFound := false
	for _, sub := range estado.Also {
		if sub.Command == "version" {
			version = sub
			versionFound = true
		}
	}
	if !versionFound {
		t.Fatal(`Estado's Also must contain a "version" entry`)
	}
	if !version.TargetAgnostic {
		t.Error(`version (Also) must have TargetAgnostic: true`)
	}
	if version.Mutating {
		t.Error(`version (Also) must have Mutating: false (read-only)`)
	}
}

// The three longterm-mem entries share one Command and differ only in Args, so
// the helpers below key by Command+Args. A map keyed on Command alone would
// silently assert against whichever of the three landed last.
//
// findActionByCommandArgs returns the top-level action whose Command and Args
// both match, failing the test when no such entry exists.
func findActionByCommandArgs(t *testing.T, command string, args ...string) Action {
	t.Helper()
	want := strings.Join(append([]string{command}, args...), " ")
	for _, a := range Actions() {
		if strings.Join(append([]string{a.Command}, a.Args...), " ") == want {
			return a
		}
	}
	t.Fatalf("Actions() must contain an entry %q", want)
	return Action{}
}

// TestLongtermMemActionsRegistered pins the three longterm-mem entries and,
// above all, the two fields the backend actually validates: cmd_longterm_mem
// resolves --target for all three subcommands (so none of them is
// TargetAgnostic) and defaults it to "all" (so all three accept --target all).
func TestLongtermMemActionsRegistered(t *testing.T) {
	cases := []struct {
		verb     string
		mutating bool
		confirm  bool
	}{
		{verb: "status", mutating: false, confirm: false},
		{verb: "install", mutating: true, confirm: true},
		{verb: "uninstall", mutating: true, confirm: true},
	}

	for _, tc := range cases {
		t.Run(tc.verb, func(t *testing.T) {
			a := findActionByCommandArgs(t, "longterm-mem", tc.verb)
			if a.TargetAgnostic {
				t.Errorf("longterm-mem %s must have TargetAgnostic: false (the backend resolves --target for it)", tc.verb)
			}
			if !a.SupportsAll {
				t.Errorf("longterm-mem %s must have SupportsAll: true (the backend defaults --target to all)", tc.verb)
			}
			if a.Mutating != tc.mutating {
				t.Errorf("longterm-mem %s Mutating = %v, want %v", tc.verb, a.Mutating, tc.mutating)
			}
			if tc.confirm && a.ConfirmMessage == "" {
				t.Errorf("longterm-mem %s must carry a non-empty ConfirmMessage", tc.verb)
			}
			if a.Name == "" {
				t.Errorf("longterm-mem %s must have a non-empty Name", tc.verb)
			}
			if a.Hint == "" {
				t.Errorf("longterm-mem %s must have a non-empty Hint", tc.verb)
			}
		})
	}
}

// TestLongtermMemStatusHintNamesRegistration keeps the read-only entry's hint
// from degenerating into a bare "estado": the whole point of this row is that
// the operator learns the per-runtime MCP registration state.
func TestLongtermMemStatusHintNamesRegistration(t *testing.T) {
	a := findActionByCommandArgs(t, "longterm-mem", "status")
	hint := strings.ToLower(a.Hint)
	for _, want := range []string{"mcp", "runtime"} {
		if !strings.Contains(hint, want) {
			t.Errorf("longterm-mem status Hint must mention %q, got %q", want, a.Hint)
		}
	}
}

// TestLongtermMemConfirmMessagesNameConsequences asserts on substance, not on
// prose: a confirm screen that says nothing specific is the same as no confirm
// screen. install must name that each runtime's own configuration is written
// and that a backup precedes it; uninstall must name that the shared binary
// survives a partial uninstall, and the one flag that does remove it.
// The first version of this test required only the WORDS "binario" and
// "--purge" on the uninstall message. An adversarial re-verification proved
// that admits a message asserting the OPPOSITE: "Elimina el binario
// compartido y todas las entradas. Ver --purge." contains both keywords and
// passed. That is the dangerous direction of the error -- the operator would
// be told the shared binary is deleted when it survives -- so the guard was
// certifying vocabulary while claiming to certify consequences.
//
// These assertions pin CLAIMS instead: a phrase an inverted message cannot
// carry while meaning its opposite, plus an explicit rejection of the
// inversion's own wording. A keyword list can be satisfied by prose that
// means anything; a claim cannot.
func TestLongtermMemConfirmMessagesNameConsequences(t *testing.T) {
	t.Run("install", func(t *testing.T) {
		msg := strings.ToLower(findActionByCommandArgs(t, "longterm-mem", "install").ConfirmMessage)
		// The consequence is WHOSE files change and that they are backed up
		// first -- not that the word "configuración" appears somewhere.
		for _, want := range []string{
			"configuración propia de cada",
			"respaldo .bak",
		} {
			if !strings.Contains(msg, want) {
				t.Errorf("install ConfirmMessage must claim %q, got:\n%s", want, msg)
			}
		}
	})

	t.Run("uninstall", func(t *testing.T) {
		msg := strings.ToLower(findActionByCommandArgs(t, "longterm-mem", "uninstall").ConfirmMessage)
		// The whole point of this screen is the non-obvious semantics: only
		// the owned entry goes, and the shared binary stays.
		for _, want := range []string{
			"únicamente la entrada",
			"binario compartido sobrevive",
			"--purge",
		} {
			if !strings.Contains(msg, want) {
				t.Errorf("uninstall ConfirmMessage must claim %q, got:\n%s", want, msg)
			}
		}
		// And it must not say the opposite. This is the assertion the
		// keyword-only version lacked: it is what makes the inverted message
		// the re-verifier wrote fail instead of pass.
		for _, forbidden := range []string{
			"elimina el binario",
			"borra el binario",
			"no sobrevive",
		} {
			if strings.Contains(msg, forbidden) {
				t.Errorf("uninstall ConfirmMessage must not claim %q -- the shared binary survives a partial uninstall, and telling the operator otherwise is the more dangerous error; got:\n%s", forbidden, msg)
			}
		}
	})
}

// TestNoActionPassesPurge keeps the deliberate omission deliberate: --purge
// deletes the shared binary other runtimes may still be registered against,
// and the TUI has no second-level destructive confirmation. It stays a CLI
// act, and no later editor gets to "complete" the menu with it.
func TestNoActionPassesPurge(t *testing.T) {
	check := func(a Action, where string) {
		for _, arg := range a.Args {
			if arg == "--purge" {
				t.Errorf("%s must not pass --purge (Args = %v)", where, a.Args)
			}
		}
	}
	for _, a := range Actions() {
		check(a, fmt.Sprintf("top-level action %q", a.Name))
		for _, sub := range a.Also {
			check(sub, fmt.Sprintf("Also entry under %q", a.Name))
		}
	}
}

// TestCaptureAndRestoreAreCopyTargetActions: the backend refuses both for a
// package target, so the TUI must know it before offering them.
func TestCaptureAndRestoreAreCopyTargetActions(t *testing.T) {
	for _, command := range []string{"capture", "restore"} {
		var found bool
		for _, a := range Actions() {
			if a.Command == command {
				found = true
				if !a.CopyTargetsOnly {
					t.Errorf("%s must be CopyTargetsOnly: the backend refuses it for a package target", command)
				}
			}
		}
		if !found {
			t.Errorf("Actions() has no %q entry", command)
		}
	}
	for _, a := range Actions() {
		if a.CopyTargetsOnly && a.Command != "capture" && a.Command != "restore" {
			t.Errorf("%s is marked CopyTargetsOnly but the backend accepts it for package targets", a.Command)
		}
	}
}
