package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// longtermMemStateDirFixture points every runtime config root at a throwaway
// HOME and returns an overridden state dir that is deliberately NOT under
// that HOME — the exact shape a `STATE_DIR=... bin/labdrian-overlay` run
// produces, and the only shape that can tell "derived from --state-dir"
// apart from "derived from HOME".
func longtermMemStateDirFixture(t *testing.T) (home string, stateDir string, binaryPath string) {
	t.Helper()
	home = t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("CODEX_HOME", "")

	stateDir = t.TempDir()
	binDir := filepath.Join(stateDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", binDir, err)
	}
	binaryPath = filepath.Join(binDir, "longterm-mem")
	if err := os.WriteFile(binaryPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write %s: %v", binaryPath, err)
	}
	return home, stateDir, binaryPath
}

// TestRunRuntimeCore_LongtermMemStateDirDerivesBinaryPath is the A2 defect:
// `engine runtime --component longterm-mem --state-dir <dir>` used the
// HOME-derived default binary path regardless of --state-dir, so a binary
// the overlay entrypoint had genuinely deployed under an overridden
// STATE_DIR was reported as "missing binary".
func TestRunRuntimeCore_LongtermMemStateDirDerivesBinaryPath(t *testing.T) {
	_, stateDir, _ := longtermMemStateDirFixture(t)

	var outBuf, errBuf bytes.Buffer
	exitCode := -1
	runRuntimeCore(
		[]string{"status", "--component", "longterm-mem", "--state-dir", stateDir},
		&outBuf,
		&errBuf,
		func(code int) { exitCode = code },
	)

	if strings.Contains(outBuf.String(), "missing binary") {
		t.Fatalf("status must resolve the binary under the overridden --state-dir, got %q", outBuf.String())
	}
	if exitCode != 0 {
		t.Fatalf("status should exit 0 when the binary is present under --state-dir, got %d\nstdout=%q\nstderr=%q", exitCode, outBuf.String(), errBuf.String())
	}
}

// TestRunRuntimeCore_LongtermMemStateDirEntryIsOwned proves the other half:
// ownership is re-derived from the adapter's own binary path, so an entry
// naming the binary deployed under the overridden STATE_DIR must be
// recognised as this overlay's own and recorded — not reported as an
// unmanaged third-party entry.
func TestRunRuntimeCore_LongtermMemStateDirEntryIsOwned(t *testing.T) {
	home, stateDir, binaryPath := longtermMemStateDirFixture(t)

	entry := map[string]any{
		"mcpServers": map[string]any{
			"longterm-mem": map[string]any{
				"type":    "stdio",
				"command": binaryPath,
				"args":    []string{"mcp"},
			},
		},
	}
	encoded, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal claude config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".claude.json"), encoded, 0o644); err != nil {
		t.Fatalf("write claude config: %v", err)
	}

	var outBuf, errBuf bytes.Buffer
	exitCode := -1
	runRuntimeCore(
		[]string{"install", "--component", "longterm-mem", "--state-dir", stateDir},
		&outBuf,
		&errBuf,
		func(code int) { exitCode = code },
	)

	if exitCode != 0 {
		t.Fatalf("install should exit 0 for an owned entry under --state-dir, got %d\nstdout=%q\nstderr=%q", exitCode, outBuf.String(), errBuf.String())
	}
	if strings.Contains(outBuf.String(), "unmanaged") {
		t.Fatalf("an entry naming the --state-dir binary is owned, not unmanaged; got %q", outBuf.String())
	}

	recorded, err := os.ReadFile(filepath.Join(stateDir, "longterm-mem-registration.json"))
	if err != nil {
		t.Fatalf("registration should be written under --state-dir: %v", err)
	}
	var reg struct {
		Targets map[string]struct {
			EntryPresent bool `json:"entry_present"`
		} `json:"targets"`
	}
	if err := json.Unmarshal(recorded, &reg); err != nil {
		t.Fatalf("registration should be valid JSON: %v", err)
	}
	if !reg.Targets["claude"].EntryPresent {
		t.Fatalf("claude should be recorded from its owned entry, got %s", string(recorded))
	}
}
