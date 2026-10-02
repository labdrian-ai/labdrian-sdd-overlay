package fsstore

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMain isolates every test in this package from the developer's live machine. These
// tests run a real git in temporary repositories, and git reads the user's global
// configuration from HOME and XDG_CONFIG_HOME; the rule for every package that keeps files
// is isolation regardless (an earlier engine test ran a real `pi remove` during
// `go test ./...`), and a new package does not inherit the guard that the packages around it
// install for themselves. The review-transaction stores the tests use are always the ones
// they create inside their own temporary repositories, never the developer's.
func TestMain(m *testing.M) {
	os.Exit(runIsolated(m))
}

// runIsolated runs the package's tests under a fresh HOME and refuses to run them at all
// when the isolation cannot be set up: a failed Setenv would leave a test running against
// whatever environment was already there, which is what the guard exists to prevent. Every
// ambient GIT_* variable is removed too, as gitprov's tests do for themselves: a harness that
// runs the tests with GIT_DIR, GIT_WORK_TREE or GIT_INDEX_FILE set (a git hook does) would
// otherwise point both the setup git commands and the code under test at its own
// repository. The temporary HOME is removed before the exit code is returned.
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
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if !strings.HasPrefix(name, "GIT_") {
			continue
		}
		if err := os.Unsetenv(name); err != nil {
			fmt.Fprintf(os.Stderr, "unset %s: %v\n", name, err)
			return 1
		}
	}
	return m.Run()
}

// TestLiveGuardClearsTheGitEnvironment proves the TestMain guard removed every GIT_*
// variable, which makes the real-git fixtures of this package independent of the repository
// the test run was started from. It is only load-bearing when the run starts with one set
// (GIT_DIR=/nonexistent go test ./reviewreceipt/fsstore).
func TestLiveGuardClearsTheGitEnvironment(t *testing.T) {
	for _, kv := range os.Environ() {
		if name, _, _ := strings.Cut(kv, "="); strings.HasPrefix(name, "GIT_") {
			t.Errorf("%s is set in the test environment, want every GIT_* variable cleared", name)
		}
	}
}

// TestLiveGuardIsolatesHomeAndXDGDirectories proves the TestMain guard is in force for this
// package: HOME is a fresh temporary directory and both XDG roots live under it.
func TestLiveGuardIsolatesHomeAndXDGDirectories(t *testing.T) {
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
