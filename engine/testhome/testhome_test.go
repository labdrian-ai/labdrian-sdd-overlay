package testhome

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// recorder is a testing.TB that records what Check complains about instead of failing the test that runs it.
type recorder struct {
	testing.TB
	complaints []string
}

func (r *recorder) Helper() {}

func (r *recorder) Errorf(format string, args ...any) {
	r.complaints = append(r.complaints, fmt.Sprintf(format, args...))
}

// scrub sets, by t.Setenv, every variable Enter touches, so that what a test does to them is undone when it ends
// and the tests do not depend on the environment they run in.
func scrub(t *testing.T) {
	t.Helper()
	names := append([]string{"HOME", "GIT_ALLOW_PROTOCOL", "GOCACHE", "GOMODCACHE", "GOPATH", "GOENV"}, xdgRoots...)
	for _, name := range append(names, overridden...) {
		t.Setenv(name, "kept-by-the-test")
	}
}

func TestEnter_PutsHomeAndTheXDGRootsInADirectoryMadeForTheRun(t *testing.T) {
	scrub(t)

	cleanup, err := Enter()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)

	home := os.Getenv("HOME")
	if filepath.Dir(home) != filepath.Clean(os.TempDir()) || !strings.HasPrefix(filepath.Base(home), Prefix) {
		t.Errorf("HOME = %q, want a directory named %s* directly under %s", home, Prefix, os.TempDir())
	}
	for name, want := range map[string]string{
		"XDG_CONFIG_HOME":    filepath.Join(home, ".config"),
		"XDG_STATE_HOME":     filepath.Join(home, ".local", "state"),
		"XDG_DATA_HOME":      filepath.Join(home, ".local", "share"),
		"XDG_CACHE_HOME":     filepath.Join(home, ".cache"),
		"GIT_ALLOW_PROTOCOL": "file",
	} {
		if got := os.Getenv(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}

func TestEnter_UnsetsTheVariablesThatNameAPathOfTheMachine(t *testing.T) {
	scrub(t)

	cleanup, err := Enter()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)

	for _, name := range []string{"STATE_DIR", "OVERLAY_DIR", "LABDRIAN_OVERLAY_DIR", "CODEX_HOME"} {
		if value, set := os.LookupEnv(name); set {
			t.Errorf("%s is still set (%q) in the home of the tests", name, value)
		}
		found := false
		for _, listed := range overridden {
			found = found || listed == name
		}
		if !found {
			t.Errorf("%s is not in the list of overrides the run unsets", name)
		}
	}
}

func TestEnter_CleanupRemovesTheHome(t *testing.T) {
	scrub(t)
	cleanup, err := Enter()
	if err != nil {
		t.Fatal(err)
	}
	home := os.Getenv("HOME")
	if err := os.WriteFile(filepath.Join(home, "left-behind"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	cleanup()

	if _, err := os.Stat(home); !os.IsNotExist(err) {
		t.Errorf("the home %s is still there after the cleanup (%v)", home, err)
	}
}

// writeFakeGo puts a `go` first on the PATH of the test that answers `go env` with the four values it is given,
// one per line, as the real command does.
func writeFakeGo(t *testing.T, gocache, gomodcache, gopath, goenv string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake go is a shell script")
	}
	dir := t.TempDir()
	script := fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' '%s' '%s' '%s' '%s'\n", gocache, gomodcache, gopath, goenv)
	if err := os.WriteFile(filepath.Join(dir, "go"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
}

// Replacing the home moves where the go command looks for its caches and its configuration, because their
// defaults are taken from it: a test that builds anything would start a cold build and, with no network, not find
// the modules it needs. The run therefore asks the go command where they are before the home is replaced, and
// keeps them there.
func TestEnter_KeepsTheGoToolchainWhereItWas(t *testing.T) {
	scrub(t)
	for _, name := range []string{"GOCACHE", "GOMODCACHE", "GOPATH", "GOENV"} {
		t.Setenv(name, "")
	}
	writeFakeGo(t, "/dev/cache", "/dev/mod", "/dev/path", "/dev/env")

	cleanup, err := Enter()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)

	for name, want := range map[string]string{
		"GOCACHE": "/dev/cache", "GOMODCACHE": "/dev/mod", "GOPATH": "/dev/path", "GOENV": "/dev/env",
	} {
		if got := os.Getenv(name); got != want {
			t.Errorf("%s = %q, want %q (what go env answered before the home changed)", name, got, want)
		}
	}
}

func TestEnter_KeepsWhatWasExportedForTheToolchain(t *testing.T) {
	scrub(t)
	t.Setenv("GOCACHE", "/exported/cache")
	t.Setenv("GOMODCACHE", "")
	t.Setenv("GOPATH", "")
	t.Setenv("GOENV", "")
	writeFakeGo(t, "/dev/cache", "/dev/mod", "/dev/path", "/dev/env")

	cleanup, err := Enter()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)

	if got := os.Getenv("GOCACHE"); got != "/exported/cache" {
		t.Errorf("GOCACHE = %q, want the exported value to win over what go env answers", got)
	}
	if got := os.Getenv("GOPATH"); got != "/dev/path" {
		t.Errorf("GOPATH = %q, want what go env answered, as it was not exported", got)
	}
}

// An answer that is empty says the command has nothing to keep, so no variable is made out of it; and an answer of
// another shape than one line per variable is not one to trust.
func TestEnter_PinsNothingFromAnEmptyOrMalformedAnswer(t *testing.T) {
	for name, script := range map[string]string{
		"an empty line for a variable": "#!/bin/sh\nprintf '%s\\n' '/dev/cache' '/dev/mod' '/dev/path' ''\n",
		"fewer lines than variables":   "#!/bin/sh\nprintf '%s\\n' '/dev/cache' 'go: some warning'\n",
		"more lines than variables":    "#!/bin/sh\nprintf '%s\\n' '/dev/cache' '/dev/mod' '/dev/path' '/dev/env' 'one more'\n",
	} {
		t.Run(name, func(t *testing.T) {
			if runtime.GOOS == "windows" {
				t.Skip("the fake go is a shell script")
			}
			scrub(t)
			for _, variable := range toolchain {
				t.Setenv(variable, "")
				os.Unsetenv(variable)
			}
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "go"), []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", dir)

			cleanup, err := Enter()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(cleanup)

			switch name {
			case "an empty line for a variable":
				if value, set := os.LookupEnv("GOENV"); set {
					t.Errorf("GOENV = %q, want it left unset: go env had nothing to say for it", value)
				}
				if got := os.Getenv("GOCACHE"); got != "/dev/cache" {
					t.Errorf("GOCACHE = %q, want the other answers kept", got)
				}
			default:
				for _, variable := range toolchain {
					if value, set := os.LookupEnv(variable); set {
						t.Errorf("%s = %q, want nothing pinned from an answer of the wrong shape", variable, value)
					}
				}
			}
		})
	}
}

// A machine with no go on its PATH is not a reason to fail every package: the tests that need the toolchain say
// so in their own words.
func TestEnter_WithNoGoCommandPinsNothingAndSucceeds(t *testing.T) {
	scrub(t)
	for _, name := range []string{"GOCACHE", "GOMODCACHE", "GOPATH", "GOENV"} {
		t.Setenv(name, "")
	}
	t.Setenv("PATH", t.TempDir())

	cleanup, err := Enter()
	if err != nil {
		t.Fatalf("Enter failed with no go command: %v", err)
	}
	t.Cleanup(cleanup)

	if got := os.Getenv("GOCACHE"); got != "" {
		t.Errorf("GOCACHE = %q, want nothing pinned", got)
	}
}

func TestCheck_AcceptsTheHomeOfTheTests(t *testing.T) {
	scrub(t)
	cleanup, err := Enter()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)

	r := &recorder{TB: t}
	Check(r)

	if len(r.complaints) != 0 {
		t.Errorf("Check complained about the home of the tests: %v", r.complaints)
	}
}

func TestCheck_ComplainsOfEachWayTheRunIsNotInForce(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T, home string)
		// want begins one of the complaints: each is told apart by its own opening words, because the name of
		// the home is a part of the name of every XDG root.
		want string
	}{
		{"a home that is nowhere near the temporary directory", func(t *testing.T, _ string) { t.Setenv("HOME", t.TempDir()) }, "HOME = "},
		{"a home directly under the temporary directory with another name", func(t *testing.T, _ string) {
			other, err := os.MkdirTemp("", "other-home-")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { os.RemoveAll(other) })
			t.Setenv("HOME", other)
		}, "HOME = "},
		{"a home with the right name below another directory", func(t *testing.T, _ string) {
			t.Setenv("HOME", filepath.Join(t.TempDir(), Prefix+"nested"))
		}, "HOME = "},
		{"a config root outside the home", func(t *testing.T, _ string) { t.Setenv("XDG_CONFIG_HOME", t.TempDir()) }, "XDG_CONFIG_HOME = "},
		{"a state root outside the home", func(t *testing.T, _ string) { t.Setenv("XDG_STATE_HOME", t.TempDir()) }, "XDG_STATE_HOME = "},
		{"a data root outside the home", func(t *testing.T, _ string) { t.Setenv("XDG_DATA_HOME", t.TempDir()) }, "XDG_DATA_HOME = "},
		{"a cache root outside the home", func(t *testing.T, _ string) { t.Setenv("XDG_CACHE_HOME", t.TempDir()) }, "XDG_CACHE_HOME = "},
		{"the overlay state directory set", func(t *testing.T, _ string) { t.Setenv("STATE_DIR", "/x/.labdrian-overlay") }, "STATE_DIR is set"},
		{"an override that is set", func(t *testing.T, _ string) { t.Setenv("CODEX_HOME", "/x/.codex") }, "CODEX_HOME is set"},
		{"git allowed to use a protocol other than file", func(t *testing.T, _ string) { t.Setenv("GIT_ALLOW_PROTOCOL", "file:ssh") }, "GIT_ALLOW_PROTOCOL = "},
		{"git left free to use any protocol", func(t *testing.T, _ string) { os.Unsetenv("GIT_ALLOW_PROTOCOL") }, "GIT_ALLOW_PROTOCOL = "},
		{"a pi installation in the home", func(t *testing.T, home string) {
			dir := filepath.Join(home, ".pi", "agent")
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte("{}"), 0o600); err != nil {
				t.Fatal(err)
			}
		}, "the home of the tests holds a Pi installation"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			scrub(t)
			cleanup, err := Enter()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(cleanup)
			tc.mutate(t, os.Getenv("HOME"))

			r := &recorder{TB: t}
			Check(r)

			for _, complaint := range r.complaints {
				if strings.HasPrefix(complaint, tc.want) {
					return
				}
			}
			t.Errorf("Check's complaints %q include none that begins %q", r.complaints, tc.want)
		})
	}
}
