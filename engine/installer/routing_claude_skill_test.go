package installer_test

import (
	"os"
	"path/filepath"
	"testing"
)

// A row routed claude-skill is copied for Claude Code alone (S4, T20). The world keeps its home and
// its overlay in temporary directories, so the apply below never touches the real home.
func TestApplyCopiesAClaudeSkillRowForClaudeAlone(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	w := newPinWorld(t, "plain/SKILL.md custom\n_shared/only/run.sh custom claude-skill\n", map[string]string{
		"skills/plain/SKILL.md":      "# plain\n",
		"skills/_shared/only/run.sh": "echo only\n",
	})
	for _, target := range []string{"claude", "codex"} {
		if run := w.run("apply", "--target", target); run.Exit != 0 {
			t.Fatalf("apply --target %s: %s", target, pinRunText(run))
		}
	}
	got, err := os.ReadFile(filepath.Join(w.home, ".claude", "skills", "_shared", "only", "run.sh"))
	if err != nil || string(got) != "echo only\n" {
		t.Errorf("claude copy = %q, %v, want the source", got, err)
	}
	if _, err := os.Stat(filepath.Join(w.home, ".codex", "skills", "_shared", "only", "run.sh")); !os.IsNotExist(err) {
		t.Errorf("codex copy of a claude-skill row: stat error = %v, want it absent", err)
	}
	if _, err := os.Stat(filepath.Join(w.home, ".codex", "skills", "plain", "SKILL.md")); err != nil {
		t.Errorf("control: a plain skill row is missing for codex: %v", err)
	}
}
