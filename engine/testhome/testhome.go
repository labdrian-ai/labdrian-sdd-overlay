// Package testhome is test support: it gives a test binary a home directory of its own.
//
// A program that is told nothing looks for its files where the environment points: the settings of Claude, Codex
// and Pi, the state of the overlay and the shaper's clearance store, the global configuration of git. They sit
// under HOME or under a variable derived from it, and a few have a variable of their own. A test that forgets to
// name one of them would read, and in a few cases write, the machine of whoever runs the tests. Every test binary
// that can reach such a path therefore starts with Run in its TestMain, and proves it did with Check:
//
//	func TestMain(m *testing.M) { os.Exit(testhome.Run(m)) }
//
//	func TestTheTestsRunInAHomeOfTheirOwn(t *testing.T) { testhome.Check(t) }
//
// A test that needs a home of its own sets it again (t.Setenv); the run only makes sure that a test that does not
// is never found in the machine's. The older TestMains of this module (live_guard_test.go) do the same by hand
// and add the guard for the `pi` CLI (piguard).
package testhome

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Prefix begins the name of the home directory a run makes, which is how Check knows it.
const Prefix = "engine-test-home-"

// xdgRoots are the XDG base directories a run moves under the home it makes: the places a program keeps its
// configuration, state, data and cache when it is not told otherwise.
var xdgRoots = []string{"XDG_CONFIG_HOME", "XDG_STATE_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME"}

// overridden are the variables that name a path of the machine and that the programs of this module read: the
// overlay's state directory, the overlay directory (the engine's and the script's spelling) and Codex's
// configuration directory. A run unsets them, so that a test which does not name its own finds the default under
// the home of the run (the state directory is $HOME/.labdrian-overlay) and not the path whoever runs the tests
// exported. It is unset and not pinned under the home because a test that hands the script a home of its own
// (HOME=...) expects the state directory to follow it.
var overridden = []string{"STATE_DIR", "OVERLAY_DIR", "LABDRIAN_OVERLAY_DIR", "CODEX_HOME"}

// gitProtocols are the transports git may use in a test: files on this machine. The overlay script fetches tags
// and branches from `origin` (version, update, self-update, apply, sync-check --check-origin), and a test that
// runs it in a repository whose origin is a network address would otherwise reach that network with the
// credentials of whoever runs the tests (ssh keys, an agent, a credential helper). With this git refuses any other
// transport before it connects, saying "transport 'ssh' not allowed".
const gitProtocols = "file"

// toolchain are the variables through which the go command finds its caches and its configuration. Each has a
// default taken from the home directory, so replacing the home would move it.
var toolchain = []string{"GOCACHE", "GOMODCACHE", "GOPATH", "GOENV"}

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

// Enter makes the home of the run and points the process at it: HOME and the four XDG roots go to it, the
// overrides of the machine's paths are unset, and the go command is kept where it was (a test that builds
// anything would otherwise start a cold build under the new home and, with no network, not find the modules).
// The cleanup removes the home.
func Enter() (cleanup func(), err error) {
	pins := toolchainPins()

	dir, err := os.MkdirTemp("", Prefix)
	if err != nil {
		return nil, fmt.Errorf("create the home of the tests: %w", err)
	}
	set := map[string]string{
		"HOME":               dir,
		"GIT_ALLOW_PROTOCOL": gitProtocols,
		"XDG_CONFIG_HOME":    filepath.Join(dir, ".config"),
		"XDG_STATE_HOME":     filepath.Join(dir, ".local", "state"),
		"XDG_DATA_HOME":      filepath.Join(dir, ".local", "share"),
		"XDG_CACHE_HOME":     filepath.Join(dir, ".cache"),
	}
	for name, value := range pins {
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
// are under it, no override names a path of the machine, and the home holds no Pi
// installation. It is what a package's guard test calls; without the TestMain it fails.
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
	if got := os.Getenv("GIT_ALLOW_PROTOCOL"); got != gitProtocols {
		t.Errorf("GIT_ALLOW_PROTOCOL = %q, want %q: git could reach a remote with the credentials of whoever runs the tests", got, gitProtocols)
	}
	if _, err := os.Stat(filepath.Join(home, ".pi", "agent", "settings.json")); err == nil {
		t.Errorf("the home of the tests holds a Pi installation: it is not one made for them")
	}
}

// toolchainPins asks the go command where its caches and configuration are, once, before the home is replaced,
// and returns those of the variables that are not already exported. A machine with no go command, or one that
// does not answer, pins nothing: the tests that need the toolchain say so in their own words.
func toolchainPins() map[string]string {
	pins := map[string]string{}
	out, err := exec.Command("go", append([]string{"env"}, toolchain...)...).Output()
	if err != nil {
		return pins
	}
	// One line per variable, so only the line break that ends the last one goes: an empty last answer is a line.
	answers := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
	if len(answers) != len(toolchain) {
		return pins
	}
	for i, name := range toolchain {
		if os.Getenv(name) == "" && answers[i] != "" {
			pins[name] = answers[i]
		}
	}
	return pins
}
