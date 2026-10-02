package fsadapter

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMain isolates every test in this package from the developer's live machine. The
// adapter reads no environment variable (the state home is handed to it), but the rule
// for every package that keeps files is isolation regardless: an earlier engine test ran
// a real `pi remove` during `go test ./...`, and a new package does not inherit the guard
// that the packages around it install for themselves. Individual tests may still
// override these with t.Setenv.
func TestMain(m *testing.M) {
	os.Exit(runIsolated(m))
}

// runIsolated runs the package's tests under a fresh HOME and refuses to run them at all
// when the isolation cannot be set up: a failed Setenv would leave a test running against
// whatever environment was already there, which is what the guard exists to prevent. The
// temporary HOME is removed before the exit code is returned, so it never outlives the
// test binary.
func runIsolated(m *testing.M) int {
	home, err := os.MkdirTemp("", "engine-test-home-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create isolated test home: %v\n", err)
		return 1
	}
	defer os.RemoveAll(home)
	for key, value := range map[string]string{
		"HOME":            home,
		"XDG_STATE_HOME":  filepath.Join(home, ".local", "state"),
		"XDG_CONFIG_HOME": filepath.Join(home, ".config"),
	} {
		if err := os.Setenv(key, value); err != nil {
			fmt.Fprintf(os.Stderr, "set %s: %v\n", key, err)
			return 1
		}
	}
	return m.Run()
}

// TestLiveGuard_IsolatesHomeAndXDGDirectories proves the TestMain guard is in force for
// this package: HOME is a fresh temporary directory and both XDG roots live under it, so
// no test can reach the developer's real state.
func TestLiveGuard_IsolatesHomeAndXDGDirectories(t *testing.T) {
	sep := string(os.PathSeparator)
	home := os.Getenv("HOME")
	if home == "" || !strings.HasPrefix(filepath.Clean(home), filepath.Clean(os.TempDir())+sep) {
		t.Fatalf("HOME = %q, want a fresh directory under the temporary directory", home)
	}
	for _, name := range []string{"XDG_STATE_HOME", "XDG_CONFIG_HOME"} {
		dir := os.Getenv(name)
		if !strings.HasPrefix(filepath.Clean(dir), filepath.Clean(home)+sep) {
			t.Errorf("%s = %q, want a directory under the isolated HOME %q", name, dir, home)
		}
	}
}
