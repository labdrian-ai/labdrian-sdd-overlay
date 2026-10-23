package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func agentgenRun(t *testing.T, env map[string]string, args ...string) *capturedProcess {
	t.Helper()
	p := newCapturedProcess()
	d := testDeps()
	d.getenv = environmentOf(env)
	runAgentgenGenerate(p.process, d, args)
	return p
}

// seedRoleSources puts the role sources of this repository under root, which is all the generator
// reads from the repository.
func seedRoleSources(t *testing.T, root string) {
	t.Helper()
	source := filepath.Join("..", "..", "roles")
	err := filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(root, "roles", rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatalf("copying the role sources: %v", err)
	}
}

func TestGaduGenerateNeedsTheOverlayDirectory(t *testing.T) {
	p := agentgenRun(t, nil)

	want := "gadu-generate: OVERLAY_DIR is not set\n  Run: OVERLAY_DIR=<overlay-repo-root> gentle-ai-overlay gadu-generate\n"
	if p.err.String() != want || len(p.exits) != 1 || p.exits[0] != 1 || p.out.Len() != 0 {
		t.Errorf("stderr %q, exits %v, stdout %q; want %q, [1], nothing", p.err.String(), p.exits, p.out.String(), want)
	}
}

func TestGaduGenerateWritesTheArtifactsTheCheckThenAcceptsAndTheCheckRefusesTheirAbsence(t *testing.T) {
	root := t.TempDir()
	seedRoleSources(t, root)
	env := map[string]string{"OVERLAY_DIR": root}

	if p := agentgenRun(t, env, "--check"); len(p.exits) != 1 || p.exits[0] != 1 || !strings.HasPrefix(p.err.String(), "gadu-generate --check: generated agent artifacts are stale or missing:\n") {
		t.Errorf("check of an empty root: stderr %q, exits %v; want the stale refusal and [1]", p.err.String(), p.exits)
	}

	generated := agentgenRun(t, env)
	if want := "gadu-generate: agents/<role>.md, pi/agents/<role>.md, and codex/agents/<role>.toml written\n"; generated.out.String() != want || len(generated.exits) != 0 || generated.err.Len() != 0 {
		t.Errorf("generate: stdout %q, stderr %q, exits %v; want %q and no exit", generated.out.String(), generated.err.String(), generated.exits, want)
	}

	checked := agentgenRun(t, env, "--check")
	if want := "gadu-generate --check: OK (committed artifacts match generator output)\n"; checked.out.String() != want || len(checked.exits) != 0 || checked.err.Len() != 0 {
		t.Errorf("check after generate: stdout %q, stderr %q, exits %v; want %q and no exit", checked.out.String(), checked.err.String(), checked.exits, want)
	}
}

// The role files are written by the same command, and the check that follows names none of them as
// missing or stale: the one verb keeps all the generated agents in step (S1: Check in CI).
func TestGaduGenerateWritesTheFilesOfTheRolesToo(t *testing.T) {
	root := t.TempDir()
	seedRoleSources(t, root)

	if p := agentgenRun(t, map[string]string{"OVERLAY_DIR": root}); len(p.exits) != 0 {
		t.Fatalf("generate: stderr %q, exits %v", p.err.String(), p.exits)
	}

	for _, path := range []string{"agents/reviewer.md", "pi/agents/reviewer.md", "codex/agents/reviewer.toml", "codex/agents/delivery.toml"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(path))); err != nil {
			t.Errorf("the command did not write %s: %v", path, err)
		}
	}
}
