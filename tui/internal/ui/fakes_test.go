package ui

// In-memory stand-ins for what the model talks to (the catalog, the action
// runner, the restore planner and the repo prober), and the constructors that
// give model tests a ready model without a process or a file behind it.
// Anything that needs a real answer from the backend belongs in a
// *_backend_test.go file instead.

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/labdrian-ai/labdrian-sdd-overlay/tui/internal/domain"
)

// fakeCatalog is a TargetCatalog that answers from memory and counts how many
// times it was asked.
type fakeCatalog struct {
	targets []domain.Target
	err     error
	asked   int
}

func (f *fakeCatalog) Targets() ([]domain.Target, error) {
	f.asked++
	if f.err != nil {
		return nil, f.err
	}
	return append([]domain.Target(nil), f.targets...), nil
}

// runCall is one run the model asked of its ActionRunner.
type runCall struct {
	action   domain.Action
	selected []domain.Target
	shown    []domain.Target
}

// fakeRunner is an ActionRunner that records what it was asked and answers
// with result, which names the action and the targets it was asked about.
type fakeRunner struct {
	calls  []runCall
	result domain.CommandResult
}

func (f *fakeRunner) Run(action domain.Action, selected, shown []domain.Target) domain.CommandResult {
	f.calls = append(f.calls, runCall{action: action, selected: selected, shown: shown})
	res := f.result
	res.Action, res.Targets = action, selected
	return res
}

// printing is the invocations of a run that printed output: a result with
// something on its screen, for the tests that are about anything but the
// transcript itself.
func printing(output string) []domain.Invocation {
	return []domain.Invocation{{Args: []string{"status"}, Run: domain.Execution{Output: output}}}
}

// fakeRestore is a RestorePlanner that vouches for the targets in backed. It
// records the candidates it was asked about, so a test can prove a target was
// never put to it, and it answers with a text of its own that names the
// targets it vouches for.
type fakeRestore struct {
	backed []string
	asked  [][]string
	gate   chan struct{} // when set, every question waits until it is closed
}

func (f *fakeRestore) Confirmation(action domain.Action, candidates []domain.Target) (string, []domain.Target, bool) {
	if f.gate != nil {
		<-f.gate
	}
	names := make([]string, len(candidates))
	for i, c := range candidates {
		names[i] = c.Name
	}
	f.asked = append(f.asked, names)
	var have []domain.Target
	for _, c := range candidates {
		for _, name := range f.backed {
			if c.Name == name {
				have = append(have, c)
			}
		}
	}
	if len(have) == 0 {
		return "", nil, false
	}
	return action.ConfirmMessage + "\n\nBACKUPS FOR: " + targetNames(have), have, true
}

// fakeProbe is a RepoProber that answers with fixed counts.
type fakeProbe struct {
	origin, release int
	asked           int
}

func (f *fakeProbe) Behind() domain.RepoBehind {
	f.asked++
	return domain.RepoBehind{Origin: f.origin, Release: f.release}
}

// threeCopyTargets is the catalog most model tests run against: the three
// per-file targets, as the backend lists them.
func threeCopyTargets() []domain.Target {
	return []domain.Target{
		{Name: "claude", Kind: domain.KindCopy},
		{Name: "opencode", Kind: domain.KindCopy},
		{Name: "codex", Kind: domain.KindCopy},
	}
}

// fourTargets adds the package target, as the backend's own catalog does.
func fourTargets() []domain.Target {
	return append(threeCopyTargets(), domain.Target{Name: "pi", Kind: domain.KindPackage})
}

// testDeps is a model's collaborators with nothing behind them: the menu as
// the program has it, the catalog given, a runner that runs nothing, a planner
// that vouches for no backup and a probe that knows nothing.
func testDeps(catalog domain.TargetCatalog) Deps {
	return Deps{
		Actions: domain.Actions(),
		Catalog: catalog,
		Runner:  &fakeRunner{},
		Restore: &fakeRestore{},
		Probe:   &fakeProbe{origin: domain.RepoBehindOriginNA, release: domain.RepoBehindOriginNA},
	}
}

// newTestModel returns a model whose catalog has already loaded the three
// copy targets, every one selected -- the state the TUI is in a moment after
// launch.
func newTestModel(t *testing.T) model {
	t.Helper()
	return newLoadedModel(t, &fakeCatalog{targets: threeCopyTargets()}, &fakeRestore{})
}

// newTestModelWith is newTestModel over a catalog the caller controls.
func newTestModelWith(t *testing.T, catalog domain.TargetCatalog) model {
	t.Helper()
	return newLoadedModel(t, catalog, &fakeRestore{})
}

// newLoadedModel builds a model over the given catalog and restore planner and
// delivers the catalog the same way the program does: by running the command
// Init issues and handing its message to Update.
func newLoadedModel(t *testing.T, catalog domain.TargetCatalog, restore RestorePlanner) model {
	t.Helper()
	d := testDeps(catalog)
	d.Restore = restore
	updated, _ := newModel(d).Update(loadTargetsCmd(catalog)())
	return updated.(model)
}

// unloadedTestModel is a model that has not yet received its catalog.
func unloadedTestModel(t *testing.T, catalog domain.TargetCatalog) model {
	t.Helper()
	return newModel(testDeps(catalog))
}

// pressKey feeds one key to the model's Update and returns the new model.
func pressKey(t *testing.T, m model, key tea.KeyMsg) model {
	t.Helper()
	updated, _ := m.Update(key)
	return updated.(model)
}
