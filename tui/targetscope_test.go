package main

// The invariant behind T1, end to end: the TUI never acts on a target the
// operator did not see. `--target all` is the one argument that can name more
// than the selection, so it is sent only when the selection is the whole catalog
// the operator was shown, and only while the backend still answers with that
// same catalog.
//
// The rule has three homes: the scope in the domain (CoversAll, ConfirmUnchanged),
// the runner that applies it (package app, over a recording backend), and the
// model that hands the runner the selection and the catalog shown. These tests
// are the fourth, and the one that proves the three are joined: the program as
// main composes it, driven key by key against the stub backend, judged by what
// the backend received. They are the cases the first version of the rule was
// tested with, when it was one function run against the script.

import (
	"strings"
	"testing"
)

// afterLaunch is what the backend received after the two invocations the
// program makes on its own at launch: the catalog and the probe.
func afterLaunch(s *session) []string {
	got := s.stub.invoked()
	if len(got) < 2 {
		return nil
	}
	return got[2:]
}

// receivedAfterLaunch is afterLaunch, joined for a comparison.
func receivedAfterLaunch(s *session) string { return strings.Join(afterLaunch(s), "|") }

func TestTargetScope_AllStandsForTheSelectionOnlyWhenItIsTheWholeCatalog(t *testing.T) {
	t.Run("every target of the catalog selected sends all, after checking the catalog", func(t *testing.T) {
		s := newSession(t).start().toActions().onRow(rowSyncCheck).press("enter")
		if got, want := receivedAfterLaunch(s), "targets|sync-check --target all"; got != want {
			t.Errorf("the backend received %q, want %q", got, want)
		}
	})

	t.Run("the three targets the TUI used to call all no longer do", func(t *testing.T) {
		// This is the bug: a four-target catalog, three of them selected, used
		// to be sent as `--target all`, which also acts on pi.
		s := newSession(t).start().press("down", "down", "down", "space") // pi off
		s.toActions().onRow(rowSyncCheck).press("enter")
		want := "sync-check --target claude|sync-check --target opencode|sync-check --target codex"
		if got := receivedAfterLaunch(s); got != want {
			t.Errorf("the backend received %q, want one explicit call per selected target, and no question to the catalog: %q", got, want)
		}
	})

	t.Run("a one-target catalog fully selected is all", func(t *testing.T) {
		s := newSession(t)
		s.stub.say("targets", "claude\tcopy\n")
		s.start().toActions().onRow(rowSyncCheck).press("enter")
		if got, want := receivedAfterLaunch(s), "targets|sync-check --target all"; got != want {
			t.Errorf("the backend received %q, want %q", got, want)
		}
	})

	t.Run("a catalog with no targets offers nothing to run", func(t *testing.T) {
		// Two empty sets are equal, and `all` would then act on every target the
		// backend has. An empty catalog is an error, so there is nothing to
		// select and no action to start.
		s := newSession(t)
		s.stub.say("targets", "")
		s.start().press("enter")
		if got := receivedAfterLaunch(s); got != "" {
			t.Errorf("the backend received %q after launch, want nothing", got)
		}
		if view := s.view(); !strings.Contains(view, "No se pudo leer el catálogo de destinos") || strings.Contains(view, "Elegir una acción") {
			t.Errorf("an empty catalog should leave the operator on the targets screen, saying so; got:\n%s", view)
		}
	})

	t.Run("nothing selected runs nothing", func(t *testing.T) {
		s := newSession(t).start().press("a", "enter") // select-all toggled off; enter has nowhere to go
		if got := receivedAfterLaunch(s); got != "" {
			t.Errorf("the backend received %q with nothing selected, want nothing", got)
		}
		if strings.Contains(s.view(), "Elegir una acción") {
			t.Error("enter with nothing selected reached the actions")
		}
	})

	t.Run("an action the backend refuses all for never sends all, even for the whole catalog", func(t *testing.T) {
		s := newSession(t)
		s.stub.say("targets", "claude\tcopy\nopencode\tcopy\ncodex\tcopy\n")
		s.start().toActions().onRow(rowCapture).press("enter", "y")
		want := "capture --target claude|capture --target opencode|capture --target codex"
		if got := receivedAfterLaunch(s); got != want {
			t.Errorf("the backend received %q, want one capture per target and no all: %q", got, want)
		}
	})

	t.Run("capture on a catalog that holds a package target skips it", func(t *testing.T) {
		s := newSession(t).start().toActions().onRow(rowCapture).press("enter", "y")
		want := "capture --target claude|capture --target opencode|capture --target codex"
		if got := receivedAfterLaunch(s); got != want {
			t.Errorf("the backend received %q, want the three copy targets only: %q", got, want)
		}
	})
}

func TestTargetScope_AllIsRefusedUnlessTheBackendStillListsTheCatalogShown(t *testing.T) {
	const shown = "claude\tcopy\nopencode\tcopy\ncodex\tcopy\n"
	// confirming returns the program on the confirm screen of apply with the
	// three targets shown, which is the moment an update elsewhere can change
	// what `all` means.
	confirming := func(t *testing.T) *session {
		t.Helper()
		s := newSession(t)
		s.resize(250, 40) // wide enough that no line of the transcript wraps
		s.stub.say("targets", shown)
		return s.start().toActions().onRow(rowApply).press("enter")
	}
	refused := func(t *testing.T, s *session) {
		t.Helper()
		if got, want := receivedAfterLaunch(s), "targets"; got != want {
			t.Errorf("the backend received %q after the confirmation, want only the question to the catalog: %q", got, want)
		}
		view := s.view()
		for _, want := range []string{"Comando falló", "[omitido:", "$ bin/labdrian-overlay apply --target all"} {
			if !strings.Contains(view, want) {
				t.Errorf("the result of a refused all should show %q, got:\n%s", want, view)
			}
		}
	}

	t.Run("the backend now lists another target", func(t *testing.T) {
		// What an update that adds a target looks like from the TUI: it showed
		// three, the backend now has four, and `all` would reach the fourth.
		s := confirming(t)
		s.stub.say("targets", defaultCatalog)
		s.press("y")
		refused(t, s)
		if view := s.view(); !strings.Contains(view, "catálogo") {
			t.Errorf("the transcript should say the catalog changed, got:\n%s", view)
		}
	})

	t.Run("the backend lists one target fewer", func(t *testing.T) {
		s := confirming(t)
		s.stub.say("targets", "claude\tcopy\nopencode\tcopy\n")
		s.press("y")
		refused(t, s)
	})

	t.Run("the backend cannot be asked", func(t *testing.T) {
		s := confirming(t)
		s.stub.exit("targets", 3)
		s.stub.complain("targets", "catalog unreadable\n")
		s.press("y")
		refused(t, s)
		if view := s.view(); !strings.Contains(view, "catalog unreadable") {
			t.Errorf("the transcript should carry the cause, got:\n%s", view)
		}
	})

	t.Run("the backend answers in a form that is not a catalog", func(t *testing.T) {
		s := confirming(t)
		s.stub.say("targets", "not a catalog\n")
		s.press("y")
		refused(t, s)
	})

	t.Run("the same set in another order is still the catalog shown", func(t *testing.T) {
		s := confirming(t)
		s.stub.say("targets", "codex\tcopy\nclaude\tcopy\nopencode\tcopy\n")
		s.press("y")
		if got, want := receivedAfterLaunch(s), "targets|apply --target all"; got != want {
			t.Errorf("the backend received %q, want the check and then all: %q", got, want)
		}
		if view := s.view(); !strings.Contains(view, "Completado") {
			t.Errorf("an unchanged catalog should let the run complete, got:\n%s", view)
		}
	})

	t.Run("a refused all does not stop the invocations around it", func(t *testing.T) {
		// "Actualizar repositorio": self-update, then the chained apply. The
		// first runs; only the target-using second one is refused.
		s := newSession(t)
		s.resize(250, 40)
		s.stub.say("targets", shown)
		s.stub.say("self-update", "main is now at origin/main\n")
		s.start().toActions().onRow(rowSelfUpdate).press("enter")
		s.stub.say("targets", defaultCatalog)
		s.press("y")

		// self-update ran, then the check that refused the apply; the apply never
		// started, and the run failed, so the launch probe was not asked again.
		if got, want := receivedAfterLaunch(s), "self-update|targets"; got != want {
			t.Errorf("the backend received %q, want %q", got, want)
		}
		view := s.view()
		for _, want := range []string{"Comando falló", "$ bin/labdrian-overlay self-update", "main is now at origin/main", "$ bin/labdrian-overlay apply --target all", "[omitido:"} {
			if !strings.Contains(view, want) {
				t.Errorf("the result should show %q, got:\n%s", want, view)
			}
		}
	})
}
