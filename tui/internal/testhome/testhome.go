// Package testhome is test support: it gives a test binary a home directory of its own.
//
// A program that is told nothing looks for its files where the environment points. The backend script the TUI
// drives keeps its state under a directory derived from HOME (or named by STATE_DIR), reads the deployment
// targets of Claude, Codex and Pi from HOME, and runs git, which reads the global configuration of whoever runs
// it. A test that forgets to name one of those paths would read, and in a few cases write, the machine of whoever
// runs the tests, and a test that follows an exported STATE_DIR fails on a machine that exports one. Every test
// binary of the module therefore starts with Run in its TestMain, and proves it did with Check:
//
//	func TestMain(m *testing.M) { os.Exit(testhome.Run(m)) }
//
//	func TestTheTestsRunInAHomeOfTheirOwn(t *testing.T) { testhome.Check(t) }
//
// A test that needs a home of its own sets it again (t.Setenv, or the environment of the process it starts); the
// run only makes sure that a test that does not is never found in the machine's. A test in this package that
// walks the module (module_test.go) holds every package that has tests to both lines.
package testhome

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Prefix begins the name of the home directory a run makes, which is how Check knows it.
const Prefix = "tui-test-home-"

// xdgRoots are the XDG base directories a run moves under the home it makes: the places a program keeps its
// configuration, state, data and cache when it is not told otherwise. Git reads $XDG_CONFIG_HOME/git/config as
// part of its global configuration, so this is also where a developer's own settings would come from.
var xdgRoots = []string{"XDG_CONFIG_HOME", "XDG_STATE_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME"}

// overridden are the variables that name a path of the machine and that the backend script, the TUI or git read:
// the overlay's state directory and the overlay directory (the TUI's and the script's spelling), Codex's
// configuration directory, the files git takes its global and system configuration from, and the variables git
// exports to the hooks it runs (a test run from a hook would otherwise have every repository it makes redirected
// to the one being committed). A run unsets them, so that a test which does not name its own finds the default
// under the home of the run, and not the path whoever runs the tests exported.
var overridden = []string{
	"STATE_DIR",
	"OVERLAY_DIR",
	"LABDRIAN_OVERLAY_DIR",
	"CODEX_HOME",
	"GIT_CONFIG_GLOBAL",
	"GIT_CONFIG_SYSTEM",
	"GIT_DIR",
	"GIT_WORK_TREE",
	"GIT_INDEX_FILE",
}

// pinned are the variables a run sets to a value of its own. GIT_CONFIG_NOSYSTEM keeps git from reading
// /etc/gitconfig, the one place of its configuration that does not move with HOME. GIT_ALLOW_PROTOCOL is the set
// of transports git may use in a test: files on this machine. The backend fetches tags and branches from `origin`
// (version, update, self-update, sync-check --check-origin), and a test that runs it in a repository whose origin
// is a network address would otherwise reach that network with the credentials of whoever runs the tests (ssh
// keys, an agent, a credential helper); with this git refuses any other transport before it connects.
var pinned = map[string]string{
	"GIT_CONFIG_NOSYSTEM": "1",
	"GIT_ALLOW_PROTOCOL":  "file",
}

// Run enters a home of the run's own, runs the tests of the binary and removes the home. Its result is the exit
// code for os.Exit.
func Run(m *testing.M) int {
	cleanup, err := Enter()
	if err != nil {
		fmt.Fprintf(os.Stderr, "testhome: %v\n", err)
		return 1
	}
	defer cleanup()
	return m.Run()
}

// Enter makes the home of the run and points the process at it: HOME and the four XDG roots go to it, git is
// pinned to the files of this machine and to no system configuration, and the overrides of the machine's paths
// are unset. The cleanup removes the home.
func Enter() (cleanup func(), err error) {
	dir, err := os.MkdirTemp("", Prefix)
	if err != nil {
		return nil, fmt.Errorf("create the home of the tests: %w", err)
	}
	set := map[string]string{
		"HOME":            dir,
		"XDG_CONFIG_HOME": filepath.Join(dir, ".config"),
		"XDG_STATE_HOME":  filepath.Join(dir, ".local", "state"),
		"XDG_DATA_HOME":   filepath.Join(dir, ".local", "share"),
		"XDG_CACHE_HOME":  filepath.Join(dir, ".cache"),
	}
	for name, value := range pinned {
		set[name] = value
	}
	for name, value := range set {
		if err := os.Setenv(name, value); err != nil {
			os.RemoveAll(dir)
			return nil, fmt.Errorf("set %s for the tests: %w", name, err)
		}
	}
	for _, name := range overridden {
		if err := os.Unsetenv(name); err != nil {
			os.RemoveAll(dir)
			return nil, fmt.Errorf("unset %s for the tests: %w", name, err)
		}
	}
	return func() { os.RemoveAll(dir) }, nil
}

// Check proves, for the test binary it runs in, that Run is in force: the home is one a run made, the XDG roots
// are under it, no override names a path of the machine, and git is pinned. It is what a package's guard test
// calls; without the TestMain it fails.
func Check(t testing.TB) {
	t.Helper()
	home := os.Getenv("HOME")
	if filepath.Dir(home) != filepath.Clean(os.TempDir()) || !strings.HasPrefix(filepath.Base(home), Prefix) {
		t.Errorf("HOME = %q, want a directory named %s* directly under %s: the tests do not run in a home of their own", home, Prefix, os.TempDir())
	}
	for _, name := range xdgRoots {
		if dir := os.Getenv(name); !strings.HasPrefix(filepath.Clean(dir), filepath.Clean(home)+string(os.PathSeparator)) {
			t.Errorf("%s = %q, want a directory under the home of the tests %q", name, dir, home)
		}
	}
	for _, name := range overridden {
		if value, set := os.LookupEnv(name); set {
			t.Errorf("%s is set (%q): the tests would use a path of the machine where they name none", name, value)
		}
	}
	for name, want := range pinned {
		if got := os.Getenv(name); got != want {
			t.Errorf("%s = %q, want %q: git could read the configuration of the machine or reach a remote with the credentials of whoever runs the tests", name, got, want)
		}
	}
}
