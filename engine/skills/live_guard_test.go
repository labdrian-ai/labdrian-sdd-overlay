package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMain isolates every test in this package from the developer's live
// machine. The project-tier verbs and the approval verbs resolve paths from
// arguments, not from HOME, but the rule for every engine package is
// isolation regardless: an earlier engine test ran a real `pi remove` during
// `go test ./...`, and a package does not inherit the guard another package
// installs for itself. HOME, XDG_STATE_HOME and XDG_CONFIG_HOME are pinned
// under one temporary directory, so no test can write ~/.claude, ~/.pi,
// ~/.codex or $XDG_STATE_HOME/labdrian by accident. Individual tests may
// still override these with t.Setenv.
//
// The real-CLI tests in project_retire_rollback_e2e_test.go compile the
// engine with `go build`. A temporary HOME would silently move the Go build
// cache to a cold directory, so GOCACHE is pinned to the developer's cache
// before HOME is replaced: the guard changes where state would be written,
// never how fast the build runs.
//
// The shared engine binary those tests build is removed here, after the
// package's tests run.
func TestMain(m *testing.M) {
	if os.Getenv("GOCACHE") == "" {
		if cache, err := os.UserCacheDir(); err == nil {
			os.Setenv("GOCACHE", filepath.Join(cache, "go-build"))
		}
	}
	home, err := os.MkdirTemp("", "engine-test-home-*")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", home)
	os.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	os.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	code := m.Run()

	if retireE2EBinary.dir != "" {
		_ = os.RemoveAll(retireE2EBinary.dir)
	}
	// Unlike a deferred cleanup, this runs before os.Exit, so the temporary
	// HOME never outlives the test binary.
	_ = os.RemoveAll(home)
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
