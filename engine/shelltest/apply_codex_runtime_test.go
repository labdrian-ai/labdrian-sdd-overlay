package shelltest

// `labdrian apply --target codex` runs `engine runtime install --target codex`, as `apply --target pi`
// runs it for pi, so that the Codex adapter keeps the owner rules block in the three AGENTS files.
// Every case runs the step in a temporary home with a stand-in for the engine that records its call.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
)

// recordingEngine appends its arguments and the checkout it was told about to "$RECORD" and answers
// with the text and exit code of the case.
const recordingEngine = `#!/bin/sh
printf '%s|LABDRIAN_OVERLAY_DIR=%s|HOME=%s|CWD=%s\n' "$*" "$LABDRIAN_OVERLAY_DIR" "$HOME" "$(pwd)" >> "$RECORD"
echo "$ANSWER"
exit "$EXIT"
`

// stepSnippet runs the step the way cmd_apply does, for an overlay checkout in "$1" and the engine in "$2".
const stepSnippet = `OVERLAY_DIR="$1"; ENGINE_BINARY="$2"; cd "$OVERLAY_DIR"
rc=0; codex_runtime_install_and_report || rc=$?; echo "exit: $rc"
`

func runCodexStep(t *testing.T, engine, answer, exit string, withRules bool) (out, record string) {
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
	engineBinary := engine
	if engineBinary == "" {
		engineBinary = filepath.Join(dir, "missing-engine")
	} else {
		engineBinary = filepath.Join(dir, "engine")
		overlaytest.WriteTool(t, engineBinary, recordingEngine)
	}
	recordFile := filepath.Join(dir, "record")
	shell := overlaytest.Shell{
		Env: []string{"HOME=" + home, "PATH=" + os.Getenv("PATH"), "RECORD=" + recordFile, "ANSWER=" + answer, "EXIT=" + exit},
		Dir: dir,
	}
	out, err := overlaytest.Source(t, overlaytest.Script(t), shell, stepSnippet, checkout, engineBinary)
	if err != nil {
		t.Fatalf("the step failed to run: %v\n%s", err, out)
	}
	data, _ := os.ReadFile(recordFile)
	return out, strings.ReplaceAll(strings.ReplaceAll(string(data), checkout, "CHECKOUT"), home, "HOME")
}

func TestApplyCodexRunsTheCodexRuntimeInstallAgainstTheCheckout(t *testing.T) {
	out, record := runCodexStep(t, "engine", "codex install restart_required", "0", true)
	if want := "runtime install --target codex|LABDRIAN_OVERLAY_DIR=CHECKOUT|HOME=HOME|CWD=CHECKOUT\n"; record != want {
		t.Errorf("the engine was called %q, want exactly %q", record, want)
	}
	if !strings.Contains(out, "  codex install restart_required") || !strings.Contains(out, "exit: 0") {
		t.Errorf("want the engine's answer shown and the step to succeed, got:\n%s", out)
	}
}

func TestApplyCodexReportsAFailedCodexRuntimeInstall(t *testing.T) {
	out, record := runCodexStep(t, "engine", "codex install partial: owner rules could not be read", "1", true)
	if !strings.Contains(record, "runtime install --target codex") {
		t.Errorf("the engine was not called: %q", record)
	}
	if !strings.Contains(out, "codex install partial: owner rules could not be read") || !strings.Contains(out, "exit: 1") {
		t.Errorf("a failed install must show its answer and make the step fail, got:\n%s", out)
	}
}

func TestApplyCodexWithoutAnEngineBinaryNamesTheRecoveryAndDoesNotFail(t *testing.T) {
	out, record := runCodexStep(t, "", "", "0", true)
	if record != "" {
		t.Errorf("an engine that is not there was called: %q", record)
	}
	if !strings.Contains(out, "codex: engine binary not found") || !strings.Contains(out, "labdrian install-hooks") || !strings.Contains(out, "exit: 0") {
		t.Errorf("want a note naming the recovery and a step that does not fail, got:\n%s", out)
	}
}

// A checkout that holds no owner rules (an older main) has nothing to install: the step prints nothing
// and does not start the engine, so apply's output for such a checkout is what it was.
func TestApplyCodexWithACheckoutThatHasNoOwnerRulesIsSilentAndDoesNotStartTheEngine(t *testing.T) {
	out, record := runCodexStep(t, "engine", "codex install restart_required", "0", false)
	if record != "" {
		t.Errorf("the engine was called for a checkout with no owner rules: %q", record)
	}
	if out != "exit: 0\n" {
		t.Errorf("want no output and a step that does not fail, got:\n%q", out)
	}
}
