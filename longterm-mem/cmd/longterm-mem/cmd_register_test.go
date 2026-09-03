package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captureStderr redirects os.Stderr for the duration of fn and returns
// everything written to it, mirroring captureStdout's own convention
// (main_test.go). register/unregister report every failure on stderr, and
// the exact text is part of the contract: it is the only instruction a user
// staring at exit 6 ever gets.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stderr.txt")
	captured, err := os.Create(path)
	if err != nil {
		t.Fatalf("create stderr capture: %v", err)
	}
	real := os.Stderr
	os.Stderr = captured
	fn()
	os.Stderr = real
	if err := captured.Close(); err != nil {
		t.Fatalf("close stderr capture: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read stderr capture: %v", err)
	}
	return string(data)
}

// TestCmdRegister_ConflictMessageIsNotPrefixedThreeTimes pins the exact
// exit-6 line, byte for byte.
//
// It used to read:
//
//	longterm-mem: register: claude: register: claude: register: an entry
//	with this name already exists and is not owned by longterm-mem
//
// -- three stacked "register:" prefixes and two "claude:" ones, because
// each of the three layers (the sentinel's own text, jsonInstall's wrap,
// and this command's per-target Fprintf) re-stated what the layer below it
// had already said. An exact-string assertion is the only kind that catches
// that: every "contains" check in the suite passed while the message was
// unreadable.
//
// The line also has to earn its place: it names the file to edit and says
// what to do, because "not owned by longterm-mem" alone leaves a user with
// no next step.
func TestCmdRegister_ConflictMessageIsNotPrefixedThreeTimes(t *testing.T) {
	configRoot := t.TempDir()
	configPath := filepath.Join(configRoot, ".claude.json")
	if err := os.WriteFile(configPath, []byte(`{"mcpServers":{"longterm-mem":{"type":"stdio","command":"/someone/elses/binary"}}}`), 0o600); err != nil {
		t.Fatalf("seed .claude.json: %v", err)
	}

	var exit int
	stderr := captureStderr(t, func() {
		exit = run([]string{"register", "--target", "claude", "--config-root", configRoot, "--state-dir", t.TempDir(), "--binary", "/opt/bin/longterm-mem"})
	})
	if exit != 6 {
		t.Fatalf("run([register --target claude ...]) with an untagged conflict = %d, want 6", exit)
	}

	want := "longterm-mem: register: claude: " + configPath + ": an entry with this name already exists and is not owned by longterm-mem; to hand it to longterm-mem, remove that entry by hand and run register again\n"
	if stderr != want {
		t.Fatalf("register stderr =\n%q\nwant =\n%q", stderr, want)
	}
	if strings.Count(stderr, "register:") != 1 {
		t.Fatalf("stderr repeats the %q prefix %d times, want exactly 1:\n%s", "register:", strings.Count(stderr, "register:"), stderr)
	}
}

// TestCmdRegister_UnresolvablePathsExitEight (exit-code discipline): a
// precondition that could not be resolved from the environment at all --
// no HOME, so no state directory, no binary path, no config root -- is not
// the same event as a target longterm-mem tried to write and could not.
// Nothing was attempted, nothing was touched, and the fix is the caller's
// environment rather than the runtime's config. Sharing exit 1 with a real
// per-target write failure made a script unable to tell "you forgot
// --state-dir" from "your ~/.claude.json is broken".
func TestCmdRegister_UnresolvablePathsExitEight(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{
			// --binary is supplied, so only the state directory is
			// unresolvable.
			name: "unresolvable state dir",
			args: []string{"register", "--target", "opencode", "--binary", "/opt/bin/longterm-mem"},
		},
		{
			// --state-dir is supplied, so only the binary path is.
			name: "unresolvable binary path",
			args: []string{"register", "--target", "opencode", "--state-dir", "PLACEHOLDER"},
		},
		{
			// Both are supplied, so only the config root is left
			// unresolvable -- and claude's root is HOME itself, which
			// XDG_CONFIG_HOME cannot stand in for.
			name: "unresolvable config root",
			args: []string{"register", "--target", "claude", "--state-dir", "PLACEHOLDER", "--binary", "/opt/bin/longterm-mem"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", "")
			xdgConfig := t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", xdgConfig)
			opencodeDir := filepath.Join(xdgConfig, "opencode")
			if err := os.MkdirAll(opencodeDir, 0o755); err != nil {
				t.Fatalf("mkdir opencode config dir: %v", err)
			}
			if err := os.WriteFile(filepath.Join(opencodeDir, "opencode.json"), []byte(`{"mcp":{}}`), 0o600); err != nil {
				t.Fatalf("seed opencode.json: %v", err)
			}

			args := append([]string(nil), tc.args...)
			for i, a := range args {
				if a == "PLACEHOLDER" {
					args[i] = t.TempDir()
				}
			}

			var exit int
			_ = captureStderr(t, func() { exit = run(args) })
			if exit != 8 {
				t.Fatalf("run(%v) with an unresolvable path = %d, want 8", args, exit)
			}
		})
	}
}

// TestCmdRegister_WriteFailureStillExitsOne is the other side of the
// assertion above, and the reason it is worth making: a target that WAS
// attempted and failed keeps exit 1. Without this, moving the pre-flight
// guards to 8 could have been "fixed" by moving everything to 8.
func TestCmdRegister_WriteFailureStillExitsOne(t *testing.T) {
	configRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(configRoot, ".claude.json"), []byte(`{"mcpServers":`), 0o600); err != nil {
		t.Fatalf("seed unparseable .claude.json: %v", err)
	}

	var exit int
	_ = captureStderr(t, func() {
		exit = run([]string{"register", "--target", "claude", "--config-root", configRoot, "--state-dir", t.TempDir(), "--binary", "/opt/bin/longterm-mem"})
	})
	if exit != 1 {
		t.Fatalf("run([register --target claude ...]) over an unparseable config = %d, want 1", exit)
	}
}

// TestCmdRegister_RecoversFromALostInstallState (D9, "install-state
// lockout") is the command-level proof of the package-level adoption
// scenario: the recovery a user actually performs is re-running register,
// and it has to exit 0, not 6.
func TestCmdRegister_RecoversFromALostInstallState(t *testing.T) {
	configRoot := t.TempDir()
	configPath := filepath.Join(configRoot, ".claude.json")
	if err := os.WriteFile(configPath, []byte(`{"mcpServers":{}}`), 0o600); err != nil {
		t.Fatalf("seed .claude.json: %v", err)
	}
	stateDir := t.TempDir()
	args := []string{"register", "--target", "claude", "--config-root", configRoot, "--state-dir", stateDir, "--binary", "/opt/bin/longterm-mem"}

	if exit := captureExit(t, args); exit != 0 {
		t.Fatalf("first register = %d, want 0", exit)
	}
	installed, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config after the first register: %v", err)
	}

	if err := os.Remove(filepath.Join(stateDir, "install-state.json")); err != nil {
		t.Fatalf("remove install-state.json: %v", err)
	}

	if exit := captureExit(t, args); exit != 0 {
		t.Fatalf("register after losing install-state.json = %d, want 0 — a lost ownership record must not read as someone else's entry", exit)
	}
	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config after the second register: %v", err)
	}
	if string(got) != string(installed) {
		t.Fatalf("config changed while adopting its own entry:\nbefore =\n%s\nafter =\n%s", installed, got)
	}
}

// captureExit runs args with both standard streams captured, returning only
// the exit code -- the tests above care about the code, not the report.
func captureExit(t *testing.T, args []string) int {
	t.Helper()
	var exit int
	captureStdout(t, func() {
		_ = captureStderr(t, func() { exit = run(args) })
	})
	return exit
}
