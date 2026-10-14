package ui

// What the model does differently for one action than for another is read from
// the fields the action declares (RequiresBackup, RefreshProbe, ...), never from
// the name of its command. These tests give the model menus whose commands are
// not the ones the program ships, so a model that looks at a name fails.

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/labdrian-ai/labdrian-sdd-overlay/tui/internal/domain"
)

// modelWithMenu returns a loaded model whose action menu is exactly menu, over
// the three copy targets, with restore vouching for the backups of claude.
func modelWithMenu(t *testing.T, menu ...domain.Action) (model, *fakeRestore) {
	t.Helper()
	catalog := &fakeCatalog{targets: threeCopyTargets()}
	restore := &fakeRestore{backed: []string{"claude"}}
	d := testDeps(catalog)
	d.Actions = menu
	d.Restore = restore
	updated, _ := newModel(d).Update(loadTargetsCmd(catalog)())
	m := updated.(model)
	m.scr = screenActions
	return m, restore
}

// TestUpdateActions_RequiresBackupDecidesTheLookup: an action that declares
// RequiresBackup is looked up before its confirm screen, whatever its command
// is called, and one called restore that declares none is not.
func TestUpdateActions_RequiresBackupDecidesTheLookup(t *testing.T) {
	t.Run("an action that requires a backup is looked up first", func(t *testing.T) {
		rollback := domain.Action{Name: "Revertir", Command: "rollback", Mutating: true, CopyTargetsOnly: true, RequiresBackup: true}
		m, restore := modelWithMenu(t, rollback)

		updated, cmd := m.updateActions(tea.KeyMsg{Type: tea.KeyEnter})
		m = updated.(model)

		if m.scr != screenLookup || cmd == nil {
			t.Fatalf("enter on an action that requires a backup reached screen %v (command %t), want the lookup", m.scr, cmd != nil)
		}
		if got := lookupResult(t, cmd); got.action.Command != "rollback" || targetNames(got.targets) != "claude" {
			t.Errorf("the lookup answered %+v, want the rollback offered for claude, the only target with a backup", got)
		}
		if len(restore.asked) != 1 {
			t.Errorf("the planner was asked %d times, want once", len(restore.asked))
		}
	})

	t.Run("an action called restore that requires none goes straight to its confirm screen", func(t *testing.T) {
		plain := domain.Action{Name: "Restaurar", Command: "restore", Mutating: true, CopyTargetsOnly: true}
		m, restore := modelWithMenu(t, plain)

		updated, _ := m.updateActions(tea.KeyMsg{Type: tea.KeyEnter})
		m = updated.(model)

		if m.scr != screenConfirm {
			t.Errorf("enter on an action that requires no backup reached screen %v, want its confirm screen: the command string is being read", m.scr)
		}
		if len(restore.asked) != 0 {
			t.Errorf("the planner was asked about %q for an action that requires no backup", restore.asked)
		}
	})
}

// TestGlobalUKey_OffersTheActionThatRefreshesTheProbe: the banner reports how
// far the repository is behind, and its shortcut offers the first action that
// declares RefreshProbe -- the one that moves the repository -- not an action
// that merely has a familiar name.
func TestGlobalUKey_OffersTheActionThatRefreshesTheProbe(t *testing.T) {
	decoy := domain.Action{Name: "Actualizar", Command: "self-update", Mutating: true}
	remedy := domain.Action{Name: "Poner al día", Command: "catch-up", Mutating: true, RefreshProbe: true}

	t.Run("the action that declares it", func(t *testing.T) {
		m, _ := modelWithMenu(t, decoy, remedy)
		m.behindOrigin = 3

		m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")})

		if m.scr != screenConfirm || m.pendingAction.Command != "catch-up" {
			t.Errorf("u reached screen %v with %q pending, want the confirm screen of catch-up, the action that refreshes the probe", m.scr, m.pendingAction.Command)
		}
	})

	t.Run("two declare it: the first in the menu", func(t *testing.T) {
		later := domain.Action{Name: "Traer tags", Command: "fetch-tags", Mutating: true, RefreshProbe: true}
		m, _ := modelWithMenu(t, decoy, remedy, later)
		m.behindOrigin = 3

		m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")})

		if m.pendingAction.Command != "catch-up" {
			t.Errorf("u offered %q, want catch-up: the first action that declares RefreshProbe", m.pendingAction.Command)
		}
	})

	t.Run("none declares it: nothing to offer", func(t *testing.T) {
		m, _ := modelWithMenu(t, decoy)
		m.behindOrigin = 3

		m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")})

		if m.scr != screenActions || m.pendingAction.Command != "" {
			t.Errorf("u reached screen %v with %q pending although no action refreshes the probe: the command string is being read", m.scr, m.pendingAction.Command)
		}
	})
}
