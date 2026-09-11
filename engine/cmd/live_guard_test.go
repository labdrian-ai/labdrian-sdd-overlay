package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMain isolates every test in this package from the developer's live
// machine: the Pi adapter's default constructor reads HOME, STATE_DIR and
// OVERLAY_DIR from the environment and its Install/Uninstall shell out to
// a real `pi` CLI, which once removed a freshly installed labdrian-pi
// package during `go test ./...`. Individual tests may still override
// these with t.Setenv.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "engine-test-home-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(home)
	os.Setenv("HOME", home)
	os.Setenv("STATE_DIR", filepath.Join(home, ".labdrian-overlay"))
	os.Unsetenv("OVERLAY_DIR")
	os.Setenv("LABDRIAN_PI_BIN", filepath.Join(home, "pi-must-not-run"))
	os.Exit(m.Run())
}
