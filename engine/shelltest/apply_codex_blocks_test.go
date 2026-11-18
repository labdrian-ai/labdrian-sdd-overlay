package shelltest

// The whole command: `apply --target codex` over a sandbox overlay repository and a temporary home,
// with the engine the tests build. It writes the owner rules block in the three AGENTS files, leaves
// every other byte of them alone, writes nothing the second time, and no other target does it.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
)

func TestApplyTargetCodexWritesTheOwnerRulesBlockInTheThreeFilesOnly(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	overlay := overlaytest.Script(t)
	home := t.TempDir()
	const rules = "## Rules\n\n- placeholder rule\n"
	const foreign = "<!-- gentle-ai:persona -->\nplaceholder\n<!-- /gentle-ai:persona -->\n\nunmarked placeholder line\n"
	_, env := overlaytest.NewOverlay(t, home, map[string]string{
		"overlay.manifest":            "test-skill/SKILL.md   managed\n",
		"skills/test-skill/SKILL.md":  "# overlay skill\n",
		"instructions/owner-rules.md": rules,
	})
	env = append(env, "CODEX_HOME="+filepath.Join(home, ".codex"))
	files := []string{filepath.Join(home, "AGENTS.md"), filepath.Join(home, ".codex", "AGENTS.md"), filepath.Join(home, ".codex", "agents.md")}
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files[1], []byte(foreign), 0o644); err != nil {
		t.Fatal(err)
	}

	if out, err := overlaytest.Combined(t, overlay, env, "", "apply", "--target", "claude"); err != nil {
		t.Fatalf("apply --target claude: %v\n%s", err, out)
	}
	for _, path := range files {
		if _, err := os.Stat(path); err == nil && path != files[1] {
			t.Errorf("apply --target claude wrote %s", path)
		}
	}
	if got, _ := os.ReadFile(files[1]); string(got) != foreign {
		t.Errorf("apply --target claude changed %s: %q", files[1], got)
	}

	out, err := overlaytest.Combined(t, overlay, env, "", "apply", "--target", "codex")
	if err != nil {
		t.Fatalf("apply --target codex: %v\n%s", err, out)
	}
	block := "<!-- labdrian:owner-rules -->\n" + strings.TrimRight(rules, "\n") + "\n<!-- /labdrian:owner-rules -->\n"
	for i, path := range files {
		want := block
		if i == 1 {
			want = foreign + "\n" + block
		}
		if got, err := os.ReadFile(path); err != nil || string(got) != want {
			t.Errorf("%s = %q (%v), want %q", path, got, err, want)
		}
	}
	if !strings.Contains(out, "owner rules written to "+files[2]) {
		t.Errorf("apply did not report the files it wrote:\n%s", out)
	}

	again, err := overlaytest.Combined(t, overlay, env, "", "apply", "--target", "codex")
	if err != nil {
		t.Fatalf("second apply --target codex: %v\n%s", err, again)
	}
	if !strings.Contains(again, "owner rules already current in "+files[2]) {
		t.Errorf("the second apply did not report the blocks as current:\n%s", again)
	}
}
