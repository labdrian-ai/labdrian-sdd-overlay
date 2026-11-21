package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const codegraphEntryJSON = `{"hooks":{"UserPromptSubmit":[{"hooks":[{"type":"command","command":"codegraph prompt-hook","timeout":30}]}]}}`

func TestMergeSettingsSaysWhenItReplacedTheBareCodegraphPromptHook(t *testing.T) {
	p, path := mergeMainAgent(t, codegraphEntryJSON)
	want := "notice: " + path + " ran \"codegraph prompt-hook\" on every prompt; install-hooks replaced it with a wrapper that skips task notifications and runs it for every other prompt (a codegraph reinstall puts the bare command back: run install-hooks again; uninstall-hooks restores it)\n"
	if p.err.String() != want {
		t.Errorf("stderr = %q, want %q", p.err.String(), want)
	}
	if p.out.String() != "merge-settings: hooks installed successfully\n" || len(p.exits) != 0 {
		t.Errorf("stdout = %q, exits = %v; the install still succeeds", p.out.String(), p.exits)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "exec "+mainAgentHookCommand+" codegraph-prompt-hook") {
		t.Errorf("settings do not hold the wrapper:\n%s", data)
	}
}

func TestMergeSettingsSaysNothingAboutCodegraphWhenItReplacedNothing(t *testing.T) {
	if second := mergeTwiceStderr(t, codegraphEntryJSON); second != "" {
		t.Errorf("the second install said %q, want nothing", second)
	}
	for _, before := range []string{"", `{"model":"opus"}`} {
		p, _ := mergeMainAgent(t, before)
		if strings.Contains(p.err.String(), "codegraph") {
			t.Errorf("before %q: stderr = %q, want no word on codegraph", before, p.err.String())
		}
	}
}

// mergeTwiceStderr installs twice over before and returns what the second install said on stderr.
func mergeTwiceStderr(t *testing.T, before string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(before), 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{"--settings", path, "--hook-command", mainAgentHookCommand}
	runMergeSettings(newCapturedProcess().process, args)
	p := newCapturedProcess()
	runMergeSettings(p.process, args)
	return p.err.String()
}

func TestUninstallHooksGivesTheBareCodegraphCommandBack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(codegraphEntryJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{"--settings", path, "--hook-command", mainAgentHookCommand}
	runMergeSettings(newCapturedProcess().process, args)
	p := newCapturedProcess()
	runUninstallHooks(p.process, args)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"command": "codegraph prompt-hook"`) || !strings.Contains(string(data), `"timeout": 30`) || strings.Contains(string(data), "codegraph-prompt-hook") {
		t.Errorf("after uninstall:\n%s", data)
	}
	if p.out.String() != "uninstall-hooks: hooks removed successfully\n" || p.err.String() != "" || len(p.exits) != 0 {
		t.Errorf("stdout = %q, stderr = %q, exits = %v", p.out.String(), p.err.String(), p.exits)
	}
}
