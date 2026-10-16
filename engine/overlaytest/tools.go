package overlaytest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/enginetest"
)

// WriteTool writes a program of the test's own at path, making the directories above it. content is
// the whole file, its first line (#!/bin/sh) included.
func WriteTool(t testing.TB, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// StubEngine puts a program of the test's own where the script looks for the engine in home, instead
// of the engine: the script finds a binary there and does not build one. content is the whole file.
func StubEngine(t testing.TB, home, content string) {
	t.Helper()
	WriteTool(t, enginetest.Path(home), content)
}

// ToolchainEnv is where this host's go command keeps its caches and its path (GOCACHE, GOMODCACHE and
// GOPATH), as settings for a run whose HOME is a directory of the test's: a `go build` the script
// starts there reuses the warm build and module caches, and does not start a cold build under the new
// home which, with no network, would not find the modules.
func ToolchainEnv(t testing.TB) []string {
	t.Helper()
	out, err := exec.Command("go", "env", "GOCACHE", "GOMODCACHE", "GOPATH").Output()
	if err != nil {
		t.Fatalf("go env: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 go env lines, got %d: %q", len(lines), out)
	}
	return []string{
		"GOCACHE=" + lines[0],
		"GOMODCACHE=" + lines[1],
		"GOPATH=" + lines[2],
	}
}
