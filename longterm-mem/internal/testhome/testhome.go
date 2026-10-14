// Package testhome is test support: it gives a test binary a home directory of its own.
//
// A program that is told nothing looks for its files where the environment points: Engram's database, the vault
// registry, the embedding index, the configuration of the runtimes and the global configuration of git all sit
// under HOME or under a variable derived from it, and a few have a variable of their own. A test that forgets to
// name one of them would read, and in a few cases write, the machine of whoever runs the tests. Every test binary
// that can reach such a path therefore starts with Run in its TestMain, and proves it did with Check:
//
//	func TestMain(m *testing.M) { os.Exit(testhome.Run(m)) }
//
//	func TestTheTestsRunInAHomeOfTheirOwn(t *testing.T) { testhome.Check(t) }
//
// A test that needs a home of its own sets it again (t.Setenv); the run only makes sure that a test that does not
// is never found in the machine's.
package testhome

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Prefix begins the name of the home directory a run makes, which is how Check knows it.
const Prefix = "longterm-mem-test-home-"

// xdgRoots are the XDG base directories a run moves under the home it makes: the places a program keeps its
// configuration, state, data and cache when it is not told otherwise.
var xdgRoots = []string{"XDG_CONFIG_HOME", "XDG_STATE_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME"}

// overridden are the variables that name a path of the machine and that the programs of this module read: the
// directory of a runtime's configuration, the state directory and the overrides of the Engram database, the vault
// registry, the state directory and the vault. A run unsets them, so that a test which does not name its own finds
// the default under the home of the run and not the path whoever runs the tests exported.
var overridden = []string{
	"CODEX_HOME",
	"STATE_DIR",
	"LONGTERM_MEM_ENGRAM_DB",
	"LONGTERM_MEM_STATE_DIR",
	"LONGTERM_MEM_VAULTS_FILE",
	"LONGTERM_MEM_VAULT",
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

// Enter makes the home of the run and points the process at it: HOME and the four XDG roots go to it, the
// overrides of the machine's paths are unset, and the go command is kept on the caches it had (a test that builds
// the module would otherwise start a cold build under the new home and, with no network, not find the modules).
// The cleanup removes the home.
func Enter() (cleanup func(), err error) {
	home, _ := os.UserHomeDir()
	cache, _ := os.UserCacheDir()
	config, _ := os.UserConfigDir()
	pins := toolchainPins(os.Getenv, os.ReadFile, home, cache, config)

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
// are under it, no override names a path of the machine, and the home holds no Engram directory. It is what a
// package's guard test calls; without the TestMain it fails.
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
	if _, err := os.Stat(filepath.Join(home, ".engram")); !os.IsNotExist(err) {
		t.Errorf("the home of the tests holds a .engram directory (%v): it is not one made for them", err)
	}
}

// toolchainPins are the variables to set so that the go command keeps finding what it finds now, once the home is
// replaced. The go command resolves each of its paths the same way: an exported variable first, then the file
// GOENV names (go env -w writes it), then a default under the home. So the file's own location is pinned when
// it is the default one (the default moves with the config root), and a path is pinned to the default of the
// current home when neither the environment nor that file sets it. GOMODCACHE is not pinned: its default is under
// GOPATH, which is.
//
// These are the go command's rules written out here, and not its answer asked for (`go env`, as engine/testhome
// does), because R-021 forbids os/exec in non-test code of this module and this is such code. They are kept to the
// real command by TestEnter_LeavesTheRealGoCommandAnsweringAsItDid, which starts it, before and after Enter, in
// worlds that export, write in a go env file or leave to the defaults each of these variables, and requires the
// same answers: a change in the go command's rules fails that test instead of drifting unseen.
//
// home, cache and config are the user's home, cache and config directories as they are now; getenv and readFile
// read the environment and the file system, so that a table can stand in for both.
func toolchainPins(getenv func(string) string, readFile func(string) ([]byte, error), home, cache, config string) map[string]string {
	pins := map[string]string{}

	goenv := getenv("GOENV")
	if goenv == "" && config != "" {
		goenv = filepath.Join(config, "go", "env")
		pins["GOENV"] = goenv
	}
	fromFile := parseGoEnv(readFile, goenv)
	isSet := func(name string) bool { return getenv(name) != "" || fromFile[name] != "" }

	if !isSet("GOPATH") && home != "" {
		pins["GOPATH"] = filepath.Join(home, "go")
	}
	if !isSet("GOCACHE") && cache != "" {
		pins["GOCACHE"] = filepath.Join(cache, "go-build")
	}
	return pins
}

// parseGoEnv reads the NAME=value lines of the file go env -w writes. A file that cannot be read has no settings.
func parseGoEnv(readFile func(string) ([]byte, error), path string) map[string]string {
	settings := map[string]string{}
	data, _ := readFile(path)
	for _, line := range strings.Split(string(data), "\n") {
		// A commented line is a name that begins with "# ", which no variable is called.
		if name, value, found := strings.Cut(line, "="); found {
			settings[name] = value
		}
	}
	return settings
}
