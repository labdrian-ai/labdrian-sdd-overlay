package shelltest

// `labdrian apply --target claude` runs `engine runtime install --target claude --component owner-rules`,
// the one step codex and opencode share, so that the owner rules block of ~/.claude/CLAUDE.md is written and
// nothing else of the Claude adapter (settings.json stays install-hooks' alone). The step cases use the
// recording engine of apply_codex_runtime_test.go; the last test runs the whole command with the engine the
// tests build, on a sandbox home.

import (
	"crypto/sha256"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
)

const claudeStepSnippet = `OVERLAY_DIR="$1"; ENGINE_BINARY="$2"; cd "$OVERLAY_DIR"
rc=0; runtime_install_and_report claude || rc=$?; echo "exit: $rc"
`

func runClaudeStep(t *testing.T, engine, answer, exit string, withRules bool) (out, record string) {
	t.Helper()
	overlaytest.RequireBash(t)
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	checkout := filepath.Join(dir, "checkout")
	for _, d := range []string{home, filepath.Join(checkout, "instructions")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if withRules {
		if err := os.WriteFile(filepath.Join(checkout, "instructions", "owner-rules.md"), []byte("placeholder rules\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	engineBinary := filepath.Join(dir, "missing-engine")
	if engine != "" {
		engineBinary = filepath.Join(dir, "engine")
		overlaytest.WriteTool(t, engineBinary, recordingEngine)
	}
	recordFile := filepath.Join(dir, "record")
	shell := overlaytest.Shell{
		Env: []string{"HOME=" + home, "PATH=" + os.Getenv("PATH"), "RECORD=" + recordFile, "ANSWER=" + answer, "EXIT=" + exit},
		Dir: dir,
	}
	out, err := overlaytest.Source(t, overlaytest.Script(t), shell, claudeStepSnippet, checkout, engineBinary)
	if err != nil {
		t.Fatalf("the step failed to run: %v\n%s", err, out)
	}
	data, _ := os.ReadFile(recordFile)
	return out, strings.ReplaceAll(strings.ReplaceAll(string(data), checkout, "CHECKOUT"), home, "HOME")
}

func TestApplyClaudeRunsOnlyTheOwnerRulesComponentAgainstTheCheckout(t *testing.T) {
	out, record := runClaudeStep(t, "engine", "[claude] install: restart_required", "0", true)
	if want := "runtime install --target claude --component owner-rules|LABDRIAN_OVERLAY_DIR=CHECKOUT|HOME=HOME|CWD=CHECKOUT\n"; record != want {
		t.Errorf("the engine was called %q, want exactly %q", record, want)
	}
	if !strings.Contains(out, "  [claude] install: restart_required") || !strings.Contains(out, "exit: 0") {
		t.Errorf("want the engine's answer shown and the step to succeed, got:\n%s", out)
	}
}

func TestApplyClaudeReportsAFailedOwnerRulesInstall(t *testing.T) {
	out, record := runClaudeStep(t, "engine", "line one\nline two", "1", true)
	if !strings.Contains(record, "runtime install --target claude --component owner-rules") {
		t.Errorf("the engine was not called: %q", record)
	}
	for _, line := range []string{"  line one", "  line two"} {
		if !strings.Contains("\n"+out, "\n"+line+"\n") {
			t.Errorf("want the line %q indented by two spaces, got:\n%q", line, out)
		}
	}
	if !strings.Contains(out, "exit: 1") {
		t.Errorf("a failed install must make the step fail, got:\n%s", out)
	}
}

func TestApplyClaudeWithoutAnEngineBinaryNamesTheRecoveryAndDoesNotFail(t *testing.T) {
	out, record := runClaudeStep(t, "", "", "0", true)
	if record != "" {
		t.Errorf("an engine that is not there was called: %q", record)
	}
	if !strings.Contains(out, "claude: engine binary not found") || !strings.Contains(out, "owner rules not installed") ||
		!strings.Contains(out, "labdrian install-hooks") || !strings.Contains(out, "exit: 0") {
		t.Errorf("want a note naming the recovery and a step that does not fail, got:\n%s", out)
	}
}

func TestApplyClaudeWithACheckoutThatHasNoOwnerRulesIsSilentAndDoesNotStartTheEngine(t *testing.T) {
	out, record := runClaudeStep(t, "engine", "[claude] install: restart_required", "0", false)
	if record != "" {
		t.Errorf("the engine was called for a checkout with no owner rules: %q", record)
	}
	if out != "exit: 0\n" {
		t.Errorf("want no output and a step that does not fail, got:\n%q", out)
	}
}

func sumOf(t *testing.T, path string) [32]byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(data)
}

func TestApplyTargetClaudeWritesTheOwnerRulesBlockAndLeavesSettingsAlone(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	overlay := overlaytest.Script(t)
	home := t.TempDir()
	const rules = "## Rules\n\n- placeholder rule\n"
	const foreign = "<!-- gentle-ai:persona -->\nplaceholder\n<!-- /gentle-ai:persona -->\n\nunmarked placeholder line\n"
	const settings = "{\n  \"outputStyle\": \"placeholder\",\n  \"hooks\": {}\n}\n"
	checkout, env := overlaytest.NewOverlay(t, home, map[string]string{
		"overlay.manifest":            "test-skill/SKILL.md   managed\n",
		"skills/test-skill/SKILL.md":  "# overlay skill\n",
		"instructions/owner-rules.md": rules,
	})
	root := filepath.Join(home, ".claude")
	claudeMd, settingsJSON := filepath.Join(root, "CLAUDE.md"), filepath.Join(root, "settings.json")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	for path, body := range map[string]string{claudeMd: foreign, settingsJSON: settings} {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	settingsSum := sumOf(t, settingsJSON)

	out, err := overlaytest.Combined(t, overlay, env, "", "apply", "--target", "claude")
	if err != nil {
		t.Fatalf("apply --target claude: %v\n%s", err, out)
	}
	block := "<!-- labdrian:owner-rules -->\n" + strings.TrimRight(rules, "\n") + "\n<!-- /labdrian:owner-rules -->\n"
	if got, err := os.ReadFile(claudeMd); err != nil || string(got) != foreign+"\n"+block {
		t.Errorf("%s = %q (%v), want %q", claudeMd, got, err, foreign+"\n"+block)
	}
	if !strings.Contains(out, "owner rules written to "+claudeMd) {
		t.Errorf("apply did not report the file it wrote:\n%s", out)
	}
	if sumOf(t, settingsJSON) != settingsSum {
		t.Errorf("apply --target claude changed %s", settingsJSON)
	}
	if _, err := os.Stat(settingsJSON + ".bak"); err == nil {
		t.Errorf("apply --target claude left %s.bak", settingsJSON)
	}

	before := sumOf(t, claudeMd)
	if again, err := overlaytest.Combined(t, overlay, env, "", "apply", "--target", "claude"); err != nil {
		t.Fatalf("second apply --target claude: %v\n%s", err, again)
	}
	if sumOf(t, claudeMd) != before {
		t.Errorf("the second apply changed %s", claudeMd)
	}
	engine := exec.Command(filepath.Join(root, "bin", "gentle-ai-overlay"), "runtime", "status", "--component", "owner-rules", "--target", "claude")
	engine.Env = []string{"HOME=" + home, "PATH=" + os.Getenv("PATH"), "LABDRIAN_OVERLAY_DIR=" + checkout}
	if status, err := engine.CombinedOutput(); err != nil {
		t.Errorf("runtime status --component owner-rules --target claude: %v\n%s", err, status)
	}
}
