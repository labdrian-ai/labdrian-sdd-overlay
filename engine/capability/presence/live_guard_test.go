package presence_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMain isolates every test in this package from the developer's live
// machine. The prober reads none of HOME, XDG_STATE_HOME, or XDG_CONFIG_HOME (the
// home and PATH it looks under are fields its caller hands it, and it only
// stats), but the rule for every Phase 7 package is isolation regardless: an
// earlier engine test ran a real `pi remove` during `go test ./...`, and a
// new package does not inherit the guard that engine/runtime and engine/cmd
// install for themselves. Individual tests may still override these with
// t.Setenv.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "engine-test-home-*")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", home)
	os.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	os.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	code := m.Run()
	// Unlike a deferred cleanup, this runs before os.Exit, so the temporary
	// HOME never outlives the test binary.
	os.RemoveAll(home)
	os.Exit(code)
}

// TestLiveGuard_IsolatesHomeAndXDGDirectories proves the TestMain guard is in
// force for this package: HOME is a fresh temporary directory and both XDG
// roots live under it, so no test can reach the developer's real state.
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
