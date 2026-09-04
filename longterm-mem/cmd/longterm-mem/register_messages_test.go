package main

import (
	"os"
	"path/filepath"
	"testing"
)

// unreadableDir creates a directory at path so every os.ReadFile of it
// fails, and returns the exact error the command will hit. Deriving the
// expected message from the same OS error keeps the assertions below
// byte-exact about OUR formatting -- the command word, the target, the
// "read <path>: " shape -- without hardcoding an errno string.
func unreadableDir(t *testing.T, path string) error {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	_, err := os.ReadFile(path)
	if err == nil {
		t.Fatalf("reading the directory %s unexpectedly succeeded", path)
	}
	return err
}

// TestCmdUnregister_FailuresNameUnregisterNotRegister pins the exact
// failure line, byte for byte, for every runtime and for both of the two
// files unregister reads.
//
// De-duplicating the stacked prefix on the register side left unregister
// printing each error exactly as the writers produced it, and every one of
// those writers says "register:" -- so `unregister` reported its own
// failures under the other subcommand's name:
//
//	longterm-mem: register: claude: read /.../.claude.json: ...
//
// A user reading that goes looking at a register run that never happened,
// and a script grepping for the command it invoked finds nothing. The
// install-state case is the same defect one layer deeper: it printed
// "register: claude: register: read ...", naming the wrong command twice,
// because the shared state-file helper restated a command word its caller
// had already chosen. Only an exact-string assertion catches either --
// every "contains" check in this suite passed throughout.
func TestCmdUnregister_FailuresNameUnregisterNotRegister(t *testing.T) {
	for _, tc := range []struct {
		target     string
		configName string
	}{
		{"claude", ".claude.json"},
		{"opencode", "opencode.json"},
		{"codex", "config.toml"},
	} {
		t.Run(tc.target+"/unreadable config", func(t *testing.T) {
			configRoot, stateDir := t.TempDir(), t.TempDir()
			configPath := filepath.Join(configRoot, tc.configName)
			readErr := unreadableDir(t, configPath)

			var exit int
			stderr := captureStderr(t, func() {
				exit = run([]string{"unregister", "--target", tc.target, "--config-root", configRoot, "--state-dir", stateDir})
			})
			if exit != 1 {
				t.Fatalf("run([unregister --target %s ...]) = %d, want 1; stderr:\n%s", tc.target, exit, stderr)
			}

			want := "longterm-mem: unregister: " + tc.target + ": read " + configPath + ": " + readErr.Error() + "\n"
			if stderr != want {
				t.Fatalf("unregister stderr =\n%q\nwant =\n%q", stderr, want)
			}
		})
	}

	t.Run("claude/unreadable install-state", func(t *testing.T) {
		configRoot, stateDir := t.TempDir(), t.TempDir()
		if err := os.WriteFile(filepath.Join(configRoot, ".claude.json"), []byte(`{"mcpServers":{}}`), 0o600); err != nil {
			t.Fatalf("seed .claude.json: %v", err)
		}
		statePath := filepath.Join(stateDir, "install-state.json")
		readErr := unreadableDir(t, statePath)

		var exit int
		stderr := captureStderr(t, func() {
			exit = run([]string{"unregister", "--target", "claude", "--config-root", configRoot, "--state-dir", stateDir})
		})
		if exit != 1 {
			t.Fatalf("run([unregister --target claude ...]) = %d, want 1; stderr:\n%s", exit, stderr)
		}

		want := "longterm-mem: unregister: claude: read " + statePath + ": " + readErr.Error() + "\n"
		if stderr != want {
			t.Fatalf("unregister stderr =\n%q\nwant =\n%q", stderr, want)
		}
	})
}

// TestCmdRegister_FailuresStillNameRegister is the other half: naming
// unregister's failures correctly must not rename register's. Both
// commands read the same two files through the same helpers, so a fix
// applied inside a shared helper rather than at each direction's own wrap
// would make BOTH say "unregister:", trading one wrong command name for
// another.
func TestCmdRegister_FailuresStillNameRegister(t *testing.T) {
	configRoot, stateDir := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(configRoot, ".claude.json"), []byte(`{"mcpServers":{}}`), 0o600); err != nil {
		t.Fatalf("seed .claude.json: %v", err)
	}
	statePath := filepath.Join(stateDir, "install-state.json")
	readErr := unreadableDir(t, statePath)

	var exit int
	stderr := captureStderr(t, func() {
		exit = run([]string{"register", "--target", "claude", "--config-root", configRoot, "--state-dir", stateDir, "--binary", "/opt/bin/longterm-mem"})
	})
	if exit != 1 {
		t.Fatalf("run([register --target claude ...]) = %d, want 1; stderr:\n%s", exit, stderr)
	}

	want := "longterm-mem: register: claude: read " + statePath + ": " + readErr.Error() + "\n"
	if stderr != want {
		t.Fatalf("register stderr =\n%q\nwant =\n%q", stderr, want)
	}
}
