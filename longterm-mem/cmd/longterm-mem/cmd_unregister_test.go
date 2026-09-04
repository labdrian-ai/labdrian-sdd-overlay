package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRun_DispatchesUnregisterSubcommand proves "unregister" is registered
// in run's switch, not falling through to the unknown-subcommand default
// (exit 2): pointing HOME at a fresh temp dir with no ~/.claude.json means
// there is nothing to remove, which is UnregisterNoop -- exit 0, a
// distinct outcome only reachable from inside cmdUnregister.
func TestRun_DispatchesUnregisterSubcommand(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	got := run([]string{"unregister", "--target", "claude", "--state-dir", t.TempDir()})
	if got != 0 {
		t.Fatalf("run([unregister --target claude ...]) = %d, want 0, proving unregister dispatches into cmdUnregister rather than the unknown-subcommand fallback", got)
	}
}

// TestCmdUnregister_UnknownTargetExitsTwo (usage error, R-019): an
// unrecognized --target value is rejected before any file I/O, exit 2.
func TestCmdUnregister_UnknownTargetExitsTwo(t *testing.T) {
	got := run([]string{"unregister", "--target", "bogus"})
	if got != 2 {
		t.Fatalf("run([unregister --target bogus]) = %d, want 2", got)
	}
}

// TestCmdUnregister_MissingTargetExitsTwo (usage error): --target is
// required, mirroring cmdRegister's own guard.
func TestCmdUnregister_MissingTargetExitsTwo(t *testing.T) {
	got := run([]string{"unregister"})
	if got != 2 {
		t.Fatalf("run([unregister]) with no --target = %d, want 2", got)
	}
}

// TestCmdUnregister_RemovesInstalledEntry (12b.5, R-019): a real end-to-end
// register-then-unregister through the built command -- register installs
// the ownership-tagged entry for real, unregister then removes exactly it,
// exit 0.
func TestCmdUnregister_RemovesInstalledEntry(t *testing.T) {
	configRoot := t.TempDir()
	configPath := filepath.Join(configRoot, ".claude.json")
	if err := os.WriteFile(configPath, []byte(`{"mcpServers":{}}`), 0o600); err != nil {
		t.Fatalf("seed .claude.json: %v", err)
	}
	stateDir := t.TempDir()

	if exit := run([]string{"register", "--target", "claude", "--config-root", configRoot, "--state-dir", stateDir, "--binary", "/opt/bin/longterm-mem"}); exit != 0 {
		t.Fatalf("register: exit = %d, want 0", exit)
	}

	var stdout string
	var exit int
	stdout = captureStdout(t, func() {
		exit = run([]string{"unregister", "--target", "claude", "--config-root", configRoot, "--state-dir", stateDir})
	})
	if exit != 0 {
		t.Fatalf("run([unregister --target claude ...]) = %d, want 0; stdout:\n%s", exit, stdout)
	}
	if !strings.Contains(stdout, "claude: removed") {
		t.Fatalf("unregister stdout = %q, want it to report claude: removed", stdout)
	}

	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read result config: %v", err)
	}
	if strings.Contains(string(got), "longterm-mem") {
		t.Fatalf("result config still mentions longterm-mem after unregister:\n%s", got)
	}
}

// TestCmdUnregister_UnmanagedExitsSix (12b.5, R-019 "Untagged entry is
// preserved and reported, not removed"): install-state has no record for
// claude, but ~/.claude.json already has an untagged longterm-mem entry --
// unregister must report it unmanaged (exit 6) and leave the file
// byte-identical.
func TestCmdUnregister_UnmanagedExitsSix(t *testing.T) {
	configRoot := t.TempDir()
	original := []byte(`{"mcpServers":{"longterm-mem":{"type":"stdio","command":"/someone/elses/binary"}}}`)
	configPath := filepath.Join(configRoot, ".claude.json")
	if err := os.WriteFile(configPath, original, 0o600); err != nil {
		t.Fatalf("seed .claude.json: %v", err)
	}
	stateDir := t.TempDir()

	stderrPath := filepath.Join(t.TempDir(), "stderr.txt")
	captured, err := os.Create(stderrPath)
	if err != nil {
		t.Fatalf("create stderr capture: %v", err)
	}
	realStderr := os.Stderr
	os.Stderr = captured
	var exit int
	stdout := captureStdout(t, func() {
		exit = run([]string{"unregister", "--target", "claude", "--config-root", configRoot, "--state-dir", stateDir})
	})
	os.Stderr = realStderr
	if err := captured.Close(); err != nil {
		t.Fatalf("close stderr capture: %v", err)
	}

	if exit != 6 {
		t.Fatalf("run([unregister --target claude ...]) with an untagged entry = %d, want 6; stdout:\n%s", exit, stdout)
	}
	if !strings.Contains(stdout, "unmanaged") {
		t.Fatalf("stdout = %q, want it to report the target as unmanaged", stdout)
	}

	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read result config: %v", err)
	}
	if string(got) != string(original) {
		t.Fatalf("config file was modified despite being unmanaged:\nbefore = %s\nafter = %s", original, got)
	}
}

// TestCmdUnregister_AllSkipsRuntimesThatAreNotInstalled (12b.5): unlike
// cmdRegister's --target all (which needs its own "skip a missing config"
// special case, 12a.6), a runtime that was never installed is exactly
// UnregisterNoop -- exit 0, no special-casing required, proving
// cmd_unregister.go's own doc comment about why it needs no such guard.
func TestCmdUnregister_AllSkipsRuntimesThatAreNotInstalled(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	xdgConfig := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdgConfig)
	t.Setenv("CODEX_HOME", "")

	if err := os.WriteFile(filepath.Join(home, ".claude.json"), []byte(`{"mcpServers":{}}`), 0o600); err != nil {
		t.Fatalf("seed .claude.json: %v", err)
	}
	// No opencode.json, no codex config.toml: neither is installed here.

	exit := run([]string{"unregister", "--target", "all", "--state-dir", t.TempDir()})
	if exit != 0 {
		t.Fatalf("run([unregister --target all]) with only claude present = %d, want 0", exit)
	}
}

// TestCmdUnregister_UnresolvableStateDirExitsEight mirrors cmdRegister's
// own exit-code discipline: a state directory that could not be resolved
// from the environment is a precondition failure, not a target that was
// attempted and failed, and it must not share exit 1 with one.
func TestCmdUnregister_UnresolvableStateDirExitsEight(t *testing.T) {
	t.Setenv("HOME", "")
	xdgConfig := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdgConfig)

	var exit int
	_ = captureStderr(t, func() {
		exit = run([]string{"unregister", "--target", "opencode"})
	})
	if exit != 8 {
		t.Fatalf("run([unregister --target opencode]) with an unresolvable state dir = %d, want 8", exit)
	}
}

// TestCmdUnregister_UnresolvableConfigRootExitsEight: the same for the
// per-target config root, so exit 8 means one thing -- "a path could not be
// resolved" -- wherever it is raised.
func TestCmdUnregister_UnresolvableConfigRootExitsEight(t *testing.T) {
	t.Setenv("HOME", "")

	var exit int
	_ = captureStderr(t, func() {
		exit = run([]string{"unregister", "--target", "claude", "--state-dir", t.TempDir()})
	})
	if exit != 8 {
		t.Fatalf("run([unregister --target claude --state-dir ...]) with an unresolvable config root = %d, want 8", exit)
	}
}
