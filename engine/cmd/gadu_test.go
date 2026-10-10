package main

import (
	"strings"
	"testing"
)

func gaduRun(t *testing.T, env map[string]string, args ...string) *capturedProcess {
	t.Helper()
	p := newCapturedProcess()
	d := testDeps()
	d.getenv = environmentOf(env)
	runGaduGenerate(p.process, d, args)
	return p
}

func TestGaduGenerateNeedsTheOverlayDirectory(t *testing.T) {
	p := gaduRun(t, nil)

	want := "gadu-generate: OVERLAY_DIR is not set\n  Run: OVERLAY_DIR=<overlay-repo-root> gentle-ai-overlay gadu-generate\n"
	if p.err.String() != want || len(p.exits) != 1 || p.exits[0] != 1 || p.out.Len() != 0 {
		t.Errorf("stderr %q, exits %v, stdout %q; want %q, [1], nothing", p.err.String(), p.exits, p.out.String(), want)
	}
}

func TestGaduGenerateWritesTheArtifactsTheCheckThenAcceptsAndTheCheckRefusesTheirAbsence(t *testing.T) {
	root := t.TempDir()
	env := map[string]string{"OVERLAY_DIR": root}

	if p := gaduRun(t, env, "--check"); len(p.exits) != 1 || p.exits[0] != 1 || !strings.HasPrefix(p.err.String(), "gadu-generate --check: GADU artifacts are stale or missing:\n") {
		t.Errorf("check of an empty root: stderr %q, exits %v; want the stale refusal and [1]", p.err.String(), p.exits)
	}

	generated := gaduRun(t, env)
	if want := "gadu-generate: agents/GADU.md, opencode/agents/GADU.md, and skills/gadu-operator/SKILL.md written\n"; generated.out.String() != want || len(generated.exits) != 0 || generated.err.Len() != 0 {
		t.Errorf("generate: stdout %q, stderr %q, exits %v; want %q and no exit", generated.out.String(), generated.err.String(), generated.exits, want)
	}

	checked := gaduRun(t, env, "--check")
	if want := "gadu-generate --check: OK (committed artifacts match generator output)\n"; checked.out.String() != want || len(checked.exits) != 0 || checked.err.Len() != 0 {
		t.Errorf("check after generate: stdout %q, stderr %q, exits %v; want %q and no exit", checked.out.String(), checked.err.String(), checked.exits, want)
	}
}
