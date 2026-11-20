package main

import (
	"bytes"
	"crypto/sha256"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ownerRulesRun runs `runtime <args>` in the sandbox of s, home and overlay of its own, and answers
// stdout, stderr and the exit code.
func ownerRulesRun(t *testing.T, s openCodeSandbox, args ...string) (stdout, stderr string, exit int) {
	t.Helper()
	d := testDeps()
	d.getenv = environmentOf(map[string]string{"HOME": s.home, "PATH": s.bin, "LABDRIAN_OVERLAY_DIR": s.overlay})
	var out, errBuf bytes.Buffer
	exit = -1
	runRuntimeCore(d, noPi(), noGit(), args, &out, &errBuf, func(code int) { exit = code })
	return out.String(), errBuf.String(), exit
}

func TestRuntimeOwnerRulesComponentRunsTheFourActionsOnClaudeMDAlone(t *testing.T) {
	s := newOpenCodeSandbox(t)
	claudeDir := filepath.Join(s.home, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	settingsJSON := filepath.Join(claudeDir, "settings.json")
	fixture := "{\"hooks\": {}}\n"
	if err := os.WriteFile(settingsJSON, []byte(fixture), 0o600); err != nil {
		t.Fatal(err)
	}
	wantSum := sha256.Sum256([]byte(fixture))
	claudeMD := filepath.Join(claudeDir, "CLAUDE.md")
	block := "<!-- labdrian:owner-rules -->\n## Rules\n\n- a rule\n<!-- /labdrian:owner-rules -->\n"

	run := func(action string, wantExit int, wantOut string) {
		t.Helper()
		out, errOut, exit := ownerRulesRun(t, s, action, "--component", "owner-rules", "--target", "claude")
		if exit != wantExit || errOut != "" || out != wantOut {
			t.Errorf("%s: exit %d stderr %q stdout %q, want exit %d stdout %q", action, exit, errOut, out, wantExit, wantOut)
		}
	}
	run("status", 1, "[claude] status: partial — Claude owner rules block is missing in "+claudeMD+"; run 'engine runtime install --target claude'\n")
	run("install", 0, "[claude] install: restart_required — owner rules written to "+claudeMD+"\n")
	if got := readFileText(t, claudeMD); got != block {
		t.Errorf("CLAUDE.md = %q", got)
	}
	run("install", 0, "[claude] install: restart_required — owner rules already current in "+claudeMD+"\n")
	run("update", 0, "[claude] update: restart_required — owner rules already current in "+claudeMD+"\n")
	run("status", 0, "[claude] status: supported — owner rules block is current in "+claudeMD+"\n")
	run("uninstall", 0, "[claude] uninstall: restart_required — owner rules removed from "+claudeMD+"\n")
	data, err := os.ReadFile(settingsJSON)
	if err != nil || sha256.Sum256(data) != wantSum {
		t.Errorf("settings.json = %q, err %v", data, err)
	}
	if _, err := os.Stat(settingsJSON + ".bak"); err == nil {
		t.Error("a settings.json.bak appeared")
	}
}

func TestRuntimeOwnerRulesComponentHonoursConfigRoot(t *testing.T) {
	s := newOpenCodeSandbox(t)
	root := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	_, errOut, exit := ownerRulesRun(t, s, "install", "--component", "owner-rules", "--target", "claude", "--config-root", root)
	if exit != 0 || errOut != "" {
		t.Fatalf("exit %d stderr %q", exit, errOut)
	}
	if !strings.Contains(readFileText(t, filepath.Join(root, "CLAUDE.md")), "labdrian:owner-rules") {
		t.Error("the block is not in the --config-root CLAUDE.md")
	}
	if _, err := os.Stat(filepath.Join(s.home, ".claude", "CLAUDE.md")); err == nil {
		t.Error("the home CLAUDE.md was written")
	}
}

func TestRuntimeOwnerRulesComponentRefusesEveryTargetButClaude(t *testing.T) {
	const want = "error: --component owner-rules supports --target claude only\n"
	for name, targetArgs := range map[string][]string{
		"codex":    {"--target", "codex"},
		"opencode": {"--target", "opencode"},
		"pi":       {"--target", "pi"},
		"all":      {"--target", "all"},
		"none":     nil,
	} {
		for _, action := range []string{"status", "install", "update", "uninstall"} {
			s := newOpenCodeSandbox(t)
			args := append([]string{action, "--component", "owner-rules"}, targetArgs...)
			out, errOut, exit := ownerRulesRun(t, s, args...)
			if exit != 1 || errOut != want || out != "" {
				t.Errorf("%s %s: exit %d stdout %q stderr %q", action, name, exit, out, errOut)
			}
			if _, err := os.Stat(filepath.Join(s.home, ".claude")); err == nil {
				t.Errorf("%s %s: wrote under the home", action, name)
			}
		}
	}
}
