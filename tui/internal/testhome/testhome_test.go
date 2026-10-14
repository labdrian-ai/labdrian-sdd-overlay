package testhome

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

// scrub gives the test a process environment in which every variable the package touches is set, by t.Setenv, so
// that the test's own changes are undone when it ends and the tests do not depend on the environment they run in.
func scrub(t *testing.T) {
	t.Helper()
	for _, name := range append(append([]string{"HOME", "GIT_ALLOW_PROTOCOL", "GIT_CONFIG_NOSYSTEM"}, xdgRoots...), overridden...) {
		t.Setenv(name, "kept-by-the-test")
	}
}

// enter runs Enter in a scrubbed environment and removes the home when the test ends.
func enter(t *testing.T) {
	t.Helper()
	scrub(t)
	cleanup, err := Enter()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)
}

// Enter replaces the home directory with one made for the test binary, and the XDG roots with directories under
// it, so that nothing a program derives a default path from points at the machine's own.
func TestEnter_PutsHomeAndTheXDGRootsInADirectoryMadeForTheRun(t *testing.T) {
	enter(t)

	home := os.Getenv("HOME")
	if filepath.Dir(home) != filepath.Clean(os.TempDir()) || !strings.HasPrefix(filepath.Base(home), Prefix) {
		t.Errorf("HOME = %q, want a directory named %s* directly under %s", home, Prefix, os.TempDir())
	}
	for name, want := range map[string]string{
		"XDG_CONFIG_HOME": filepath.Join(home, ".config"),
		"XDG_STATE_HOME":  filepath.Join(home, ".local", "state"),
		"XDG_DATA_HOME":   filepath.Join(home, ".local", "share"),
		"XDG_CACHE_HOME":  filepath.Join(home, ".cache"),
	} {
		if got := os.Getenv(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}

// A program that is told nothing looks where its overrides point; the run keeps the overrides of whoever runs the
// tests out of the way, so a test that does not name a path never finds the machine's own through one. The state
// directory is the one that mattered: the backend script follows an exported STATE_DIR, and the tests that read
// back what it wrote under the default failed on a machine that exported one.
func TestEnter_UnsetsTheVariablesThatNameAPathOfTheMachine(t *testing.T) {
	enter(t)

	want := []string{
		"STATE_DIR", "OVERLAY_DIR", "LABDRIAN_OVERLAY_DIR", "CODEX_HOME",
		"GIT_CONFIG_GLOBAL", "GIT_CONFIG_SYSTEM", "GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE",
	}
	for _, name := range want {
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
	if len(overridden) != len(want) {
		t.Errorf("the run unsets %d variables %v, this test names %d: name the new one here", len(overridden), overridden, len(want))
	}
}

// Git is pinned to the transports and the configuration of the test: files on this machine only, and no system
// configuration (the one file git reads that does not move with HOME).
func TestEnter_PinsGitToLocalFilesAndNoSystemConfiguration(t *testing.T) {
	enter(t)

	if got := os.Getenv("GIT_ALLOW_PROTOCOL"); got != "file" {
		t.Errorf("GIT_ALLOW_PROTOCOL = %q, want file", got)
	}
	if got := os.Getenv("GIT_CONFIG_NOSYSTEM"); got != "1" {
		t.Errorf("GIT_CONFIG_NOSYSTEM = %q, want 1", got)
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

	if _, err := os.Stat(home); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the home %s is still there after the cleanup (%v)", home, err)
	}
}

// The reason for it all: a developer's own git settings (a user name, a signing key, a default branch, a url
// rewrite) must not reach a test. Git finds them through HOME, through XDG_CONFIG_HOME and, when it is exported,
// through GIT_CONFIG_GLOBAL, and the run leaves it none of the three. The real git is asked, in a world where each
// way of finding the machine's configuration is open, before and after the run enters its home.
func TestEnter_GitReadsNoConfigurationOfTheMachine(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("there is no git on the PATH to ask")
	}
	askGit := func(t *testing.T, key string) string {
		t.Helper()
		cmd := exec.Command("git", "config", "--get", key)
		cmd.Dir = t.TempDir()
		out, _ := cmd.Output() // exit 1 is "no such key", which is the answer wanted
		return strings.TrimSpace(string(out))
	}
	tests := []struct {
		name  string
		world func(t *testing.T, machine string)
	}{
		{"the configuration in HOME", func(t *testing.T, machine string) {
			t.Setenv("HOME", machine)
		}},
		{"the configuration in XDG_CONFIG_HOME", func(t *testing.T, machine string) {
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(machine, "xdg"))
		}},
		{"a file exported as GIT_CONFIG_GLOBAL", func(t *testing.T, machine string) {
			t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(machine, ".gitconfig"))
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			machine := t.TempDir()
			config := "[user]\n\tname = the-machines-user\n"
			for _, path := range []string{filepath.Join(machine, ".gitconfig"), filepath.Join(machine, "xdg", "git", "config")} {
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(config), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			scrub(t)
			// Neither the environment of a developer nor the scrub's placeholders: the world is the one that is named.
			for _, name := range append(append([]string{}, xdgRoots...), overridden...) {
				os.Unsetenv(name)
			}
			os.Unsetenv("GIT_CONFIG_NOSYSTEM")
			tc.world(t, machine)
			if got := askGit(t, "user.name"); got != "the-machines-user" {
				t.Fatalf("the world is not the one it says: git reads user.name = %q, want the machine's", got)
			}

			cleanup, err := Enter()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(cleanup)

			if got := askGit(t, "user.name"); got != "" {
				t.Errorf("git reads user.name = %q once the home is replaced, want nothing: the configuration of the machine still reaches the tests", got)
			}
		})
	}
}

// Check is what the tests of a package call to prove the run is in force there.
func TestCheck_AcceptsTheHomeOfTheTests(t *testing.T) {
	enter(t)

	r := &recorder{TB: t}
	Check(r)

	if len(r.complaints) != 0 {
		t.Errorf("Check complained about the home of the tests: %v", r.complaints)
	}
}

func TestCheck_ComplainsOfEachWayTheRunIsNotInForce(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T)
		// want begins one of the complaints: each is told apart by its own opening words, because the name of
		// the home is a part of the name of every XDG root.
		want string
	}{
		{"a home that is nowhere near the temporary directory", func(t *testing.T) { t.Setenv("HOME", t.TempDir()) }, "HOME = "},
		{"a home directly under the temporary directory with another name", func(t *testing.T) {
			other, err := os.MkdirTemp("", "other-home-")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { os.RemoveAll(other) })
			t.Setenv("HOME", other)
		}, "HOME = "},
		{"a home with the right name below another directory", func(t *testing.T) {
			t.Setenv("HOME", filepath.Join(t.TempDir(), Prefix+"nested"))
		}, "HOME = "},
		{"a config root outside the home", func(t *testing.T) { t.Setenv("XDG_CONFIG_HOME", t.TempDir()) }, "XDG_CONFIG_HOME = "},
		{"a state root outside the home", func(t *testing.T) { t.Setenv("XDG_STATE_HOME", t.TempDir()) }, "XDG_STATE_HOME = "},
		{"a data root outside the home", func(t *testing.T) { t.Setenv("XDG_DATA_HOME", t.TempDir()) }, "XDG_DATA_HOME = "},
		{"a cache root outside the home", func(t *testing.T) { t.Setenv("XDG_CACHE_HOME", t.TempDir()) }, "XDG_CACHE_HOME = "},
		{"an exported state directory", func(t *testing.T) { t.Setenv("STATE_DIR", "/x/state") }, "STATE_DIR is set"},
		{"an exported overlay directory", func(t *testing.T) { t.Setenv("OVERLAY_DIR", "/x/overlay") }, "OVERLAY_DIR is set"},
		{"an exported global git configuration", func(t *testing.T) { t.Setenv("GIT_CONFIG_GLOBAL", "/x/gitconfig") }, "GIT_CONFIG_GLOBAL is set"},
		{"git free to read the system configuration", func(t *testing.T) { t.Setenv("GIT_CONFIG_NOSYSTEM", "0") }, "GIT_CONFIG_NOSYSTEM = "},
		{"git free to reach any transport", func(t *testing.T) { t.Setenv("GIT_ALLOW_PROTOCOL", "ssh:https") }, "GIT_ALLOW_PROTOCOL = "},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			enter(t)
			tc.mutate(t)

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
