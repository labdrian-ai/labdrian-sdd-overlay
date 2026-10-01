package main

// The invariant behind T1: the TUI never acts on a target the operator did
// not see. `--target all` is the one argument that can name more than the
// selection, so it is sent only when the selection is the whole catalog the
// operator was shown, and only while the backend still answers with that same
// catalog.

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// recordedInvocations returns the argument string of each backend call the
// stub recorded, in order; nil when it was never invoked.
func recordedInvocations(t *testing.T, recorder string) []string {
	t.Helper()
	data, err := os.ReadFile(recorder)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatalf("read recorder: %v", err)
	}
	var out []string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		_, args, _ := strings.Cut(line, "|")
		out = append(out, args)
	}
	return out
}

// runAgainstStub runs action over selected through runBackend against a
// recording stub backend, with scope as the catalog the operator saw.
func runAgainstStub(t *testing.T, action Action, selected []Target, scope targetScope) (commandResult, []string) {
	t.Helper()
	root := t.TempDir()
	recorder := filepath.Join(root, "invoked.log")
	writeStubBackend(t, root, recorder, "", 0)
	res := runBackend(root, action, selected, scope)
	return res, recordedInvocations(t, recorder)
}

func statusAction() Action {
	return Action{Name: "Estado", Command: "status", SupportsAll: true}
}

func TestRunBackend_AllStandsForTheSelectionOnlyWhenItIsTheWholeCatalog(t *testing.T) {
	t.Run("every target of the catalog selected sends all", func(t *testing.T) {
		shown := fourTargets()
		_, got := runAgainstStub(t, statusAction(), shown, targetScope{shown: shown, catalog: &fakeCatalog{targets: shown}})
		if len(got) != 1 || got[0] != "status --target all" {
			t.Errorf("invocations = %q, want exactly [status --target all]", got)
		}
	})

	t.Run("the three targets the TUI used to call all no longer do", func(t *testing.T) {
		// This is the bug: a four-target catalog, three of them selected,
		// used to be sent as `--target all`, which also acts on pi.
		shown := fourTargets()
		catalog := &fakeCatalog{targets: shown}
		_, got := runAgainstStub(t, statusAction(), threeCopyTargets(), targetScope{shown: shown, catalog: catalog})
		want := []string{"status --target claude", "status --target opencode", "status --target codex"}
		if strings.Join(got, "|") != strings.Join(want, "|") {
			t.Errorf("invocations = %q, want one explicit call per selected target %q", got, want)
		}
		if catalog.asked != 0 {
			t.Errorf("explicit per-target calls asked the catalog %d times, want 0: only `all` needs re-checking", catalog.asked)
		}
	})

	t.Run("a one-target catalog fully selected is all", func(t *testing.T) {
		shown := []Target{{Name: "claude", Kind: KindCopy}}
		_, got := runAgainstStub(t, statusAction(), shown, targetScope{shown: shown, catalog: &fakeCatalog{targets: shown}})
		if len(got) != 1 || got[0] != "status --target all" {
			t.Errorf("invocations = %q, want [status --target all]", got)
		}
	})

	t.Run("an empty selection of an empty catalog is not all", func(t *testing.T) {
		// Two empty sets are equal, and `all` would then act on every target
		// the backend has. Nothing selected means nothing runs.
		_, got := runAgainstStub(t, statusAction(), nil, targetScope{catalog: &fakeCatalog{}})
		if len(got) != 0 {
			t.Errorf("invocations = %q, want none", got)
		}
	})

	t.Run("a non-all action never sends all, even for the whole catalog", func(t *testing.T) {
		shown := fourTargets()
		capture := Action{Name: "Capturar", Command: "capture", SupportsAll: false}
		_, got := runAgainstStub(t, capture, shown, targetScope{shown: shown, catalog: &fakeCatalog{targets: shown}})
		for _, args := range got {
			if strings.Contains(args, "--target all") {
				t.Errorf("capture was sent %q; the backend refuses --target all for it", args)
			}
		}
		if len(got) != 4 {
			t.Errorf("invocations = %q, want one per target", got)
		}
	})
}

func TestRunBackend_AllIsRefusedUnlessTheBackendStillListsTheCatalogShown(t *testing.T) {
	shown := threeCopyTargets()

	t.Run("the backend now lists another target", func(t *testing.T) {
		// What an update that adds a target looks like from the TUI: it showed
		// three, the backend now has four, and `all` would reach the fourth.
		catalog := &fakeCatalog{targets: fourTargets()}
		res, got := runAgainstStub(t, statusAction(), shown, targetScope{shown: shown, catalog: catalog})

		if len(got) != 0 {
			t.Errorf("the backend was invoked with %q; `--target all` must not be sent", got)
		}
		if res.err == nil {
			t.Error("a refused invocation must fail the result, not pass as a success")
		}
		if !strings.Contains(res.output, "catálogo") {
			t.Errorf("the transcript should say the catalog changed, got:\n%s", res.output)
		}
		if catalog.asked != 1 {
			t.Errorf("the catalog was asked %d times, want once before the all invocation", catalog.asked)
		}
	})

	t.Run("the backend cannot be asked", func(t *testing.T) {
		catalog := &fakeCatalog{err: errors.New("catalog unreadable")}
		res, got := runAgainstStub(t, statusAction(), shown, targetScope{shown: shown, catalog: catalog})

		if len(got) != 0 {
			t.Errorf("the backend was invoked with %q; an unverified `all` must not be sent", got)
		}
		if res.err == nil || !strings.Contains(res.output, "catalog unreadable") {
			t.Errorf("want the failure and its cause in the result, got err=%v output:\n%s", res.err, res.output)
		}
	})

	t.Run("the backend lists one target fewer", func(t *testing.T) {
		catalog := &fakeCatalog{targets: shown[:2]}
		res, got := runAgainstStub(t, statusAction(), shown, targetScope{shown: shown, catalog: catalog})
		if len(got) != 0 || res.err == nil {
			t.Errorf("invocations = %q, err = %v; a changed catalog must refuse `all`", got, res.err)
		}
	})

	t.Run("the same set in another order is still the catalog shown", func(t *testing.T) {
		reordered := []Target{shown[2], shown[0], shown[1]}
		res, got := runAgainstStub(t, statusAction(), shown, targetScope{shown: shown, catalog: &fakeCatalog{targets: reordered}})
		if res.err != nil || len(got) != 1 || got[0] != "status --target all" {
			t.Errorf("invocations = %q, err = %v; want [status --target all]", got, res.err)
		}
	})

	t.Run("a refused all does not stop the invocations around it", func(t *testing.T) {
		// "Actualizar repositorio": self-update, then the chained apply. The
		// first runs; only the target-using second one is refused.
		chain := Action{
			Name: "Actualizar repositorio", Command: "self-update", TargetAgnostic: true,
			Also: []Action{{Command: "apply", SupportsAll: true}},
		}
		catalog := &fakeCatalog{targets: fourTargets()}
		res, got := runAgainstStub(t, chain, shown, targetScope{shown: shown, catalog: catalog})

		if len(got) != 1 || got[0] != "self-update" {
			t.Errorf("invocations = %q, want only [self-update]", got)
		}
		if res.err == nil {
			t.Error("the refused apply must fail the result")
		}
	})
}

// TestRunActionCmd_DeselectingATargetNeverSendsAll drives the same bug from
// the model: the operator starts with everything selected, turns pi off, and
// runs an action. Nothing in that path may say `all`.
func TestRunActionCmd_DeselectingATargetNeverSendsAll(t *testing.T) {
	root := t.TempDir()
	recorder := filepath.Join(root, "invoked.log")
	writeStubBackend(t, root, recorder, "", 0)

	catalog := &fakeCatalog{targets: fourTargets()}
	m := newModel(deps{repoRoot: root, catalog: catalog})
	updated, _ := m.Update(loadTargetsCmd(catalog)())
	m = updated.(model)
	m.selected[3] = false // pi

	done := m.runActionCmd(statusAction(), m.selectedTargets())().(runDoneMsg)

	got := recordedInvocations(t, recorder)
	want := "status --target claude|status --target opencode|status --target codex"
	if strings.Join(got, "|") != want {
		t.Errorf("invocations = %q, want %q", got, want)
	}
	if done.result.err != nil {
		t.Errorf("unexpected error: %v", done.result.err)
	}
}
