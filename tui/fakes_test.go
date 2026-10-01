package main

// In-memory stand-ins for the TUI's ports, and the constructors that give
// model tests a ready model without a process or a file behind it. Anything
// that needs a real answer from the backend belongs in a *_backend_test.go
// file instead.

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// fakeCatalog is a TargetCatalog that answers from memory and counts how many
// times it was asked.
type fakeCatalog struct {
	targets []Target
	err     error
	asked   int
}

func (f *fakeCatalog) Targets() ([]Target, error) {
	f.asked++
	if f.err != nil {
		return nil, f.err
	}
	return append([]Target(nil), f.targets...), nil
}

// threeCopyTargets is the catalog most model tests run against: the three
// per-file targets, as the backend lists them.
func threeCopyTargets() []Target {
	return []Target{
		{Name: "claude", Kind: KindCopy},
		{Name: "opencode", Kind: KindCopy},
		{Name: "codex", Kind: KindCopy},
	}
}

// fourTargets adds the package target, as the backend's own catalog does.
func fourTargets() []Target {
	return append(threeCopyTargets(), Target{Name: "pi", Kind: KindPackage})
}

// newTestModel returns a model whose catalog has already loaded the three
// copy targets, every one selected -- the state the TUI is in a moment after
// launch. Its repo root is an empty scratch directory, so the launch probe
// finds no backend there and never reaches real state.
func newTestModel(t *testing.T) model {
	t.Helper()
	return newTestModelWith(t, &fakeCatalog{targets: threeCopyTargets()})
}

// newTestModelWith is newTestModel over a catalog the caller controls. It
// delivers the catalog the same way the program does: by running the command
// Init issues and handing its message to Update.
func newTestModelWith(t *testing.T, catalog TargetCatalog) model {
	t.Helper()
	m := newModel(deps{repoRoot: t.TempDir(), catalog: catalog})
	updated, _ := m.Update(loadTargetsCmd(catalog)())
	return updated.(model)
}

// unloadedTestModel is a model that has not yet received its catalog.
func unloadedTestModel(t *testing.T, catalog TargetCatalog) model {
	t.Helper()
	return newModel(deps{repoRoot: t.TempDir(), catalog: catalog})
}

// pressKey feeds one key to the model's Update and returns the new model.
func pressKey(t *testing.T, m model, key tea.KeyMsg) model {
	t.Helper()
	updated, _ := m.Update(key)
	return updated.(model)
}
