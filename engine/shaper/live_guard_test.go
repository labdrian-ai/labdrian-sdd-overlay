package shaper

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestMain isolates every state and config location this package could
// resolve, so no test can reach the real user store even if it forgets its
// own overrides. (The package starts no `pi`; the Pi adapter's own tests carry
// the guard for the CLI.)
func TestMain(m *testing.M) {
	os.Exit(runIsolated(m))
}

func runIsolated(m *testing.M) int {
	dir, err := os.MkdirTemp("", "shaper-test-env-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create isolated test env: %v\n", err)
		return 1
	}
	defer os.RemoveAll(dir)
	for key, value := range map[string]string{
		"HOME":             filepath.Join(dir, "home"),
		"XDG_STATE_HOME":   filepath.Join(dir, "state"),
		"XDG_CONFIG_HOME":  filepath.Join(dir, "config"),
		"LABDRIAN_TESTING": "1",
	} {
		if err := os.Setenv(key, value); err != nil {
			fmt.Fprintf(os.Stderr, "set %s: %v\n", key, err)
			return 1
		}
	}
	return m.Run()
}
