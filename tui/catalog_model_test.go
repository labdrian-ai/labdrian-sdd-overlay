package main

// Model behavior that follows from reading the targets from the backend's
// catalog (D5): what the TUI shows, what it does when the catalog cannot be
// read, and which targets each action may run against.

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// collectMsgs runs cmd and returns every message it produces, flattening the
// batches tea.Batch wraps its commands in.
func collectMsgs(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return []tea.Msg{msg}
	}
	var out []tea.Msg
	for _, sub := range batch {
		out = append(out, collectMsgs(sub)...)
	}
	return out
}

// TestInit_AsksTheCatalogPortForTheTargets: the targets arrive through the
// port, from the command Init issues, and are not built into the model.
func TestInit_AsksTheCatalogPortForTheTargets(t *testing.T) {
	catalog := &fakeCatalog{targets: fourTargets()}
	m := unloadedTestModel(t, catalog)

	var loaded *targetsLoadedMsg
	for _, msg := range collectMsgs(m.Init()) {
		if l, ok := msg.(targetsLoadedMsg); ok {
			loaded = &l
		}
	}
	if loaded == nil {
		t.Fatal("Init() must issue a command that delivers a targetsLoadedMsg")
	}
	if len(loaded.targets) != 4 || loaded.err != nil {
		t.Errorf("targetsLoadedMsg = %+v, want the catalog's four targets and no error", loaded)
	}
	if catalog.asked != 1 {
		t.Errorf("the catalog was asked %d times at launch, want exactly once", catalog.asked)
	}
}

// TestModel_ShowsNoTargetsUntilTheCatalogArrives: before the answer there is
// nothing to select, so there is nothing to continue with.
func TestModel_ShowsNoTargetsUntilTheCatalogArrives(t *testing.T) {
	m := unloadedTestModel(t, &fakeCatalog{targets: threeCopyTargets()})

	rendered := stripANSI(m.View())
	if !strings.Contains(rendered, "Cargando destinos") {
		t.Errorf("before the catalog arrives the targets screen should say it is loading, got:\n%s", rendered)
	}
	for _, name := range []string{"claude", "opencode", "codex"} {
		if strings.Contains(rendered, name) {
			t.Errorf("target %q is shown before any catalog arrived:\n%s", name, rendered)
		}
	}
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.scr != screenTargets {
		t.Errorf("enter with no targets moved to screen %v, want to stay on the targets screen", m.scr)
	}
}

// TestCatalogLoaded_ShowsEveryBackendTargetSelected: whatever the backend
// lists is what the operator sees, selected by default, pi included when the
// backend lists it.
func TestCatalogLoaded_ShowsEveryBackendTargetSelected(t *testing.T) {
	m := newTestModelWith(t, &fakeCatalog{targets: fourTargets()})

	if len(m.targets) != 4 {
		t.Fatalf("model holds %d targets, want the catalog's 4", len(m.targets))
	}
	if !m.allSelected() {
		t.Error("every target of a freshly loaded catalog must start selected")
	}
	rendered := stripANSI(m.View())
	for _, tgt := range fourTargets() {
		if !strings.Contains(rendered, tgt.Name) {
			t.Errorf("target %q is missing from the targets screen:\n%s", tgt.Name, rendered)
		}
	}
	if got := strings.Count(rendered, "[✓]"); got != 4 {
		t.Errorf("%d targets are shown selected, want 4", got)
	}
}

// TestCatalogFailure_FailsClosed: when the backend's catalog cannot be read
// the TUI says so and offers no target action. Falling back to a guessed list
// is the drift this whole change removes.
func TestCatalogFailure_FailsClosed(t *testing.T) {
	boom := errors.New("labdrian-overlay targets: exit status 3: catalog unreadable")
	m := newTestModelWith(t, &fakeCatalog{err: boom})

	t.Run("the error is shown and no target is", func(t *testing.T) {
		rendered := stripANSI(m.View())
		if !strings.Contains(rendered, "catalog unreadable") {
			t.Errorf("the targets screen must show why the catalog could not be read, got:\n%s", rendered)
		}
		for _, name := range []string{"claude", "opencode", "codex", "[✓]", "[ ]"} {
			if strings.Contains(rendered, name) {
				t.Errorf("%q is shown although no catalog was read:\n%s", name, rendered)
			}
		}
	})

	t.Run("enter does not reach the actions", func(t *testing.T) {
		after := pressKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
		if after.scr != screenTargets {
			t.Errorf("enter moved to screen %v, want to stay on the targets screen", after.scr)
		}
	})

	t.Run("select-all selects nothing", func(t *testing.T) {
		after := pressKey(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
		if after.anySelected() {
			t.Error("select-all selected something although there are no targets")
		}
	})

	t.Run("a target action cannot start even from the actions screen", func(t *testing.T) {
		forced := m
		forced.scr = screenActions
		forced.aCursor = findAction(t, forced, "apply")
		updated, cmd := forced.updateActions(tea.KeyMsg{Type: tea.KeyEnter})
		after := updated.(model)
		if after.scr != screenActions || cmd != nil {
			t.Errorf("apply with no targets must be a no-op, got screen %v and cmd %v", after.scr, cmd != nil)
		}
	})

	t.Run("the update shortcut is withheld too", func(t *testing.T) {
		// "u" jumps to self-update, whose chained apply is a target action.
		behind := m
		behind.behindOrigin = 3
		after := pressKey(t, behind, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")})
		if after.scr == screenConfirm {
			t.Error("the update shortcut opened a confirm screen although the target catalog is unknown")
		}
	})
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

func targetNames(targets []Target) string {
	var names []string
	for _, tgt := range targets {
		names = append(names, tgt.Name)
	}
	return strings.Join(names, ", ")
}

// TestCapture_NeverRunsAgainstAPackageTarget: with pi shown and selected by
// default, capture runs for the copy targets only, and the confirm screen
// names exactly those.
func TestCapture_NeverRunsAgainstAPackageTarget(t *testing.T) {
	m := newTestModelWith(t, &fakeCatalog{targets: fourTargets()})
	m.scr = screenActions
	m.aCursor = findAction(t, m, "capture")

	updated, _ := m.updateActions(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)

	if m.scr != screenConfirm {
		t.Fatalf("capture with copy targets selected should reach the confirm screen, got %v", m.scr)
	}
	if got := targetNames(m.pendingTargets); got != "claude, opencode, codex" {
		t.Errorf("capture will run against %q, want only the copy targets", got)
	}
	rendered := stripANSI(m.View())
	if !strings.Contains(rendered, "en: claude, opencode, codex") || strings.Contains(rendered, "codex, pi") {
		t.Errorf("the confirm screen must name exactly the targets capture runs against, got:\n%s", rendered)
	}
}

// TestCapture_WithOnlyAPackageTargetSelectedIsANoOp: nothing applicable is
// left, so there is nothing to confirm or run.
func TestCapture_WithOnlyAPackageTargetSelectedIsANoOp(t *testing.T) {
	m := newTestModelWith(t, &fakeCatalog{targets: fourTargets()})
	m.selected = map[int]bool{3: true} // pi alone
	m.scr = screenActions
	m.aCursor = findAction(t, m, "capture")

	updated, cmd := m.updateActions(tea.KeyMsg{Type: tea.KeyEnter})
	after := updated.(model)

	if after.scr != screenActions || cmd != nil {
		t.Errorf("capture with only pi selected must do nothing, got screen %v and cmd %v", after.scr, cmd != nil)
	}
	if after.pendingAction.Command == "capture" {
		t.Error("pendingAction was set to capture although no selected target can be captured")
	}
}

// TestRestore_NeverRunsAgainstAPackageTarget: even a package target that has
// a backup directory is not offered a restore, because the backend refuses it.
func TestRestore_NeverRunsAgainstAPackageTarget(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeBackupFixture(t, home, "claude", "20260301T093000Z", "v1.5.0\tdigest123\t2026-03-01T09:30:00Z")
	writeBackupFixture(t, home, "pi", "20260301T093000Z", "v1.5.0\tdigest123\t2026-03-01T09:30:00Z")

	m := newTestModelWith(t, &fakeCatalog{targets: fourTargets()})
	m.selected = map[int]bool{0: true, 3: true} // claude and pi
	m.scr = screenActions
	m.aCursor = findAction(t, m, "restore")

	updated, _ := m.updateActions(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)

	if m.scr != screenConfirm {
		t.Fatalf("restore with claude's backup available should reach the confirm screen, got %v", m.scr)
	}
	if got := targetNames(m.pendingTargets); got != "claude" {
		t.Errorf("restore will run against %q, want only claude", got)
	}
}
