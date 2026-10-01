package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeScriptBackend installs a fake bin/labdrian-overlay under root whose
// body is the given shell script, for adapter tests that need a backend that
// fails, is silent, or prints something the contract does not allow.
func writeScriptBackend(t *testing.T, root, body string) {
	t.Helper()
	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "labdrian-overlay"), []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatalf("write fake backend: %v", err)
	}
}

// TestParseTargets pins what the TUI assumes about `labdrian-overlay targets`:
// one "<name><TAB><kind>" line per target, in the backend's order. Anything
// else is an error, never a guess, because a misread catalog is exactly how
// the TUI would come to act on a target nobody chose.
func TestParseTargets(t *testing.T) {
	t.Run("parses name and kind per line, preserving the backend order", func(t *testing.T) {
		got, err := parseTargets("claude\tcopy\nopencode\tcopy\ncodex\tcopy\npi\tpackage\n")
		if err != nil {
			t.Fatalf("parseTargets: unexpected error: %v", err)
		}
		want := []Target{
			{Name: "claude", Kind: KindCopy},
			{Name: "opencode", Kind: KindCopy},
			{Name: "codex", Kind: KindCopy},
			{Name: "pi", Kind: KindPackage},
		}
		if len(got) != len(want) {
			t.Fatalf("parseTargets = %+v, want %+v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("target %d = %+v, want %+v", i, got[i], want[i])
			}
		}
	})

	t.Run("a missing final newline is fine", func(t *testing.T) {
		got, err := parseTargets("claude\tcopy")
		if err != nil || len(got) != 1 || got[0].Name != "claude" {
			t.Fatalf("parseTargets = %+v, %v; want one claude target", got, err)
		}
	})

	t.Run("an unknown kind is kept and is not a copy target", func(t *testing.T) {
		// A newer backend may add a kind. Keeping it verbatim means the
		// file-by-file actions (which require KindCopy) never apply to it.
		got, err := parseTargets("zed\tcontainer\n")
		if err != nil || len(got) != 1 {
			t.Fatalf("parseTargets = %+v, %v; want one target", got, err)
		}
		if got[0].Kind != TargetKind("container") || got[0].Kind == KindCopy {
			t.Errorf("kind = %q, want it kept verbatim and distinct from %q", got[0].Kind, KindCopy)
		}
	})

	rejected := []struct {
		name    string
		output  string
		mention string
	}{
		{"empty output", "", "no targets"},
		{"only a newline", "\n", "no targets"},
		{"a line without a kind", "claude\n", "claude"},
		{"a line with an empty kind", "claude\t\n", "claude"},
		{"a line with an empty name", "\tcopy\n", "name"},
		{"a line with a third column", "claude\tcopy\textra\n", "claude"},
		{"a blank line between targets", "claude\tcopy\n\npi\tpackage\n", "line 2"},
		{"the same target twice", "claude\tcopy\nclaude\tcopy\n", "claude"},
		{"spaces instead of a tab", "claude copy\n", "claude copy"},
	}
	for _, tc := range rejected {
		t.Run("rejects "+tc.name, func(t *testing.T) {
			got, err := parseTargets(tc.output)
			if err == nil {
				t.Fatalf("parseTargets(%q) = %+v, want an error", tc.output, got)
			}
			if got != nil {
				t.Errorf("a rejected catalog must return no targets, got %+v", got)
			}
			if !strings.Contains(err.Error(), tc.mention) {
				t.Errorf("error %q should mention %q", err, tc.mention)
			}
		})
	}
}

// TestOverlayCLI_Targets_AsksTheBackendFromTheRepoRoot proves the adapter's
// whole contract with the process: it runs `bin/labdrian-overlay targets`
// with no other argument, from the repo root, and returns what it parsed.
func TestOverlayCLI_Targets_AsksTheBackendFromTheRepoRoot(t *testing.T) {
	root := t.TempDir()
	recorder := filepath.Join(root, "invoked.log")
	writeStubBackend(t, root, recorder, "claude\tcopy\npi\tpackage", 0)

	got, err := newOverlayCLI(root).Targets()
	if err != nil {
		t.Fatalf("Targets: unexpected error: %v", err)
	}
	if len(got) != 2 || got[0] != (Target{Name: "claude", Kind: KindCopy}) || got[1] != (Target{Name: "pi", Kind: KindPackage}) {
		t.Fatalf("Targets = %+v, want [claude copy, pi package]", got)
	}

	data, err := os.ReadFile(recorder)
	if err != nil {
		t.Fatalf("the backend was never invoked: %v", err)
	}
	pwdPart, args, _ := strings.Cut(strings.TrimSpace(string(data)), "|")
	if args != "targets" {
		t.Errorf("backend invoked with %q, want exactly %q", args, "targets")
	}
	gotDir, _ := filepath.EvalSymlinks(pwdPart)
	wantDir, _ := filepath.EvalSymlinks(root)
	if gotDir != wantDir {
		t.Errorf("backend ran in %q, want the repo root %q", pwdPart, root)
	}
}

// TestOverlayCLI_Targets_FailsClosed: whatever goes wrong, the adapter returns
// an error and no targets, so the TUI has nothing to offer.
func TestOverlayCLI_Targets_FailsClosed(t *testing.T) {
	t.Run("a backend that exits non-zero, reporting why", func(t *testing.T) {
		root := t.TempDir()
		writeScriptBackend(t, root, `echo "boom: catalog unreadable" >&2; exit 3`)

		got, err := newOverlayCLI(root).Targets()
		if err == nil {
			t.Fatalf("Targets = %+v, want an error", got)
		}
		if got != nil {
			t.Errorf("a failed lookup must return no targets, got %+v", got)
		}
		if !strings.Contains(err.Error(), "boom: catalog unreadable") {
			t.Errorf("error %q should carry the backend's own explanation", err)
		}
	})

	t.Run("a backend that exits zero but prints a broken catalog", func(t *testing.T) {
		root := t.TempDir()
		writeScriptBackend(t, root, `echo "not a catalog"`)

		if got, err := newOverlayCLI(root).Targets(); err == nil {
			t.Fatalf("Targets = %+v, want an error", got)
		}
	})

	t.Run("a backend that is silent", func(t *testing.T) {
		root := t.TempDir()
		writeScriptBackend(t, root, `exit 0`)

		if got, err := newOverlayCLI(root).Targets(); err == nil {
			t.Fatalf("Targets = %+v, want an error for an empty catalog", got)
		}
	})

	t.Run("a repo root without bin/labdrian-overlay", func(t *testing.T) {
		if got, err := newOverlayCLI(t.TempDir()).Targets(); err == nil {
			t.Fatalf("Targets = %+v, want an error", got)
		}
	})

	t.Run("no repo root at all", func(t *testing.T) {
		got, err := newOverlayCLI("").Targets()
		if err == nil {
			t.Fatalf("Targets = %+v, want an error", got)
		}
		if !strings.Contains(err.Error(), "locate") {
			t.Errorf("error %q should say the backend could not be located", err)
		}
	})
}
