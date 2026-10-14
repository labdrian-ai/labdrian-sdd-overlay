package testhome

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
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

// scrub gives the test a process environment in which every variable the package touches is set, by t.Setenv, so
// that the test's own changes are undone when it ends and the tests do not depend on the environment they run in.
func scrub(t *testing.T) {
	t.Helper()
	for _, name := range append(append([]string{"HOME", "GOENV", "GOPATH", "GOCACHE", "GOMODCACHE"}, xdgRoots...), overridden...) {
		t.Setenv(name, "kept-by-the-test")
	}
}

// Enter replaces the home directory with one made for the test binary, and the XDG roots with directories under
// it, so that nothing a program derives a default path from points at the machine's own.
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
// tests out of the way, so a test that does not name a path never finds the machine's own through one.
func TestEnter_UnsetsTheVariablesThatNameAPathOfTheMachine(t *testing.T) {
	scrub(t)

	cleanup, err := Enter()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)

	want := []string{
		"CODEX_HOME", "STATE_DIR", "LONGTERM_MEM_ENGRAM_DB", "LONGTERM_MEM_STATE_DIR",
		"LONGTERM_MEM_VAULTS_FILE", "LONGTERM_MEM_VAULT",
	}
	for _, name := range want {
		if value, set := os.LookupEnv(name); set {
			t.Errorf("%s is still set (%q) in the home of the tests", name, value)
		}
	}
	for _, name := range want {
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

	if _, err := os.Stat(home); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the home %s is still there after the cleanup (%v)", home, err)
	}
}

// Replacing the home moves where the go command looks for its caches and its configuration, because their defaults
// are taken from it: a test that builds the module would start a cold build and, with no network, fail to find
// the modules. The run therefore keeps the toolchain where it was, by the rules the go command applies itself:
// an exported variable first, then the file GOENV names, then the default under the home.
func TestToolchainPins(t *testing.T) {
	const (
		home   = "/home/dev"
		cache  = "/home/dev/.cache"
		config = "/home/dev/.config"
	)
	tests := []struct {
		name string
		env  map[string]string
		file map[string]string // the contents of the file GOENV names, by path; absent means unreadable
		want map[string]string
	}{
		{
			name: "nothing exported and no go env file: the defaults, under the home that is about to go",
			want: map[string]string{
				"GOENV":   "/home/dev/.config/go/env",
				"GOPATH":  "/home/dev/go",
				"GOCACHE": "/home/dev/.cache/go-build",
			},
		},
		{
			name: "everything exported: nothing to keep, the exports survive the change of home",
			env: map[string]string{
				"GOENV": "/etc/goenv", "GOPATH": "/srv/go", "GOCACHE": "/srv/cache", "GOMODCACHE": "/srv/mod",
			},
			want: map[string]string{},
		},
		{
			name: "the go env file sets the paths: it is kept reachable and nothing else is pinned",
			file: map[string]string{
				"/home/dev/.config/go/env": "# written by go env -w\nGOPATH=/srv/go\nGOCACHE=/srv/cache\n",
			},
			want: map[string]string{"GOENV": "/home/dev/.config/go/env"},
		},
		{
			name: "the go env file sets one path: the other is pinned to its default, and a commented line sets nothing",
			file: map[string]string{
				"/home/dev/.config/go/env": "# GOCACHE=/commented-out\nGOPATH=/srv/go\n",
			},
			want: map[string]string{
				"GOENV":   "/home/dev/.config/go/env",
				"GOCACHE": "/home/dev/.cache/go-build",
			},
		},
		{
			name: "an exported value beats the file: the file's value for the same name is not a reason to pin",
			env:  map[string]string{"GOPATH": "/srv/go"},
			file: map[string]string{
				"/home/dev/.config/go/env": "GOPATH=/elsewhere\n",
			},
			want: map[string]string{
				"GOENV":   "/home/dev/.config/go/env",
				"GOCACHE": "/home/dev/.cache/go-build",
			},
		},
		{
			name: "GOENV exported to another file: that file is read, and GOENV itself is not pinned",
			env:  map[string]string{"GOENV": "/etc/goenv"},
			file: map[string]string{"/etc/goenv": "GOCACHE=/srv/cache\n"},
			want: map[string]string{"GOPATH": "/home/dev/go"},
		},
		{
			name: "a value with an equals sign keeps everything after the first",
			file: map[string]string{
				"/home/dev/.config/go/env": "GOPATH=/srv/a=b\nGOCACHE=/srv/cache\n",
			},
			want: map[string]string{"GOENV": "/home/dev/.config/go/env"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			getenv := func(name string) string { return tc.env[name] }
			readFile := func(path string) ([]byte, error) {
				text, ok := tc.file[path]
				if !ok {
					return nil, os.ErrNotExist
				}
				return []byte(text), nil
			}

			got := toolchainPins(getenv, readFile, home, cache, config)

			if len(got) != len(tc.want) {
				t.Errorf("pins = %v, want %v", got, tc.want)
			}
			for name, want := range tc.want {
				if got[name] != want {
					t.Errorf("pin %s = %q, want %q (all: %v)", name, got[name], want, got)
				}
			}
		})
	}
}

// The pins are applied before the home is replaced, so a go command started by a test finds the caches the run
// started with.
func TestEnter_KeepsTheToolchainOnItsOwnCaches(t *testing.T) {
	scrub(t)
	t.Setenv("GOPATH", "")
	t.Setenv("GOCACHE", "")
	t.Setenv("GOENV", "")
	realHome := filepath.Join(t.TempDir(), "real-home")
	t.Setenv("HOME", realHome)
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	cleanup, err := Enter()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)

	if got, want := os.Getenv("GOPATH"), filepath.Join(realHome, "go"); got != want {
		t.Errorf("GOPATH = %q, want %q (the default of the home the run started with)", got, want)
	}
	if got, want := os.Getenv("GOCACHE"), filepath.Join(realHome, ".cache", "go-build"); got != want {
		t.Errorf("GOCACHE = %q, want %q", got, want)
	}
	if got, want := os.Getenv("GOENV"), filepath.Join(realHome, ".config", "go", "env"); got != want {
		t.Errorf("GOENV = %q, want %q", got, want)
	}
	if home := os.Getenv("HOME"); home == realHome {
		t.Errorf("HOME is still %q", home)
	}
}

// toolchainVariables are the variables through which the go command finds its caches and its configuration.
var toolchainVariables = []string{"GOCACHE", "GOMODCACHE", "GOPATH", "GOENV"}

// goEnv asks the real go command, from the environment this process has now, where it keeps its caches and its
// configuration.
func goEnv(t *testing.T) map[string]string {
	t.Helper()
	out, err := exec.Command("go", append([]string{"env", "-json"}, toolchainVariables...)...).Output()
	if err != nil {
		t.Fatalf("go env: %v", err)
	}
	answers := map[string]string{}
	if err := json.Unmarshal(out, &answers); err != nil {
		t.Fatalf("go env -json printed %q: %v", out, err)
	}
	return answers
}

// The rules of toolchainPins are the go command's own, written out here and not asked of it, because R-021 lets
// two files of the module import os/exec and this is not one of them. This test is what keeps them from drifting
// away from the real command unseen: in each world it asks the real go command where its caches and its
// configuration are, enters the home of a run, and asks again from the environment the run leaves. The answers must
// be the same, for whichever of the variables the world exports, writes in a go env file or leaves to the defaults.
// $DIR in the table below is the directory of the world.
func TestEnter_LeavesTheRealGoCommandAnsweringAsItDid(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the go command on windows does not take its defaults from HOME")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("there is no go command on the PATH to compare the rules with")
	}
	tests := []struct {
		name string
		// exports are the toolchain variables the world exports.
		exports map[string]string
		// file is the go env file of the world, at the GOENV it exports or else at the place the go command looks.
		file string
		// answers are what the real go command must say before the run enters its home for the world to be the one
		// the name describes; underHome names the variables whose default it must say, which is a place under the
		// home that is about to go.
		answers   map[string]string
		underHome []string
	}{
		{
			name:      "nothing exported and no go env file: every default moves with the home",
			underHome: toolchainVariables,
		},
		{
			name:      "the go env file sets two paths: the others are the defaults",
			file:      "GOPATH=$DIR/from-file/go\nGOCACHE=$DIR/from-file/cache\n",
			answers:   map[string]string{"GOPATH": "$DIR/from-file/go", "GOCACHE": "$DIR/from-file/cache"},
			underHome: []string{"GOENV"},
		},
		{
			name:      "the go env file sets one path and a commented line sets nothing",
			file:      "# GOCACHE=$DIR/commented-out\nGOPATH=$DIR/from-file/go\n",
			answers:   map[string]string{"GOPATH": "$DIR/from-file/go"},
			underHome: []string{"GOCACHE", "GOENV"},
		},
		{
			name:      "an exported value beats the file",
			exports:   map[string]string{"GOPATH": "$DIR/exported/go"},
			file:      "GOPATH=$DIR/from-file/go\n",
			answers:   map[string]string{"GOPATH": "$DIR/exported/go"},
			underHome: []string{"GOCACHE", "GOENV"},
		},
		{
			name:      "GOENV exported to another file: that file is read and the default place is not",
			exports:   map[string]string{"GOENV": "$DIR/other/env"},
			file:      "GOCACHE=$DIR/from-file/cache\n",
			answers:   map[string]string{"GOENV": "$DIR/other/env", "GOCACHE": "$DIR/from-file/cache"},
			underHome: []string{"GOPATH"},
		},
		{
			name: "everything exported: nothing moves with the home",
			exports: map[string]string{
				"GOCACHE": "$DIR/exported/cache", "GOMODCACHE": "$DIR/exported/mod", "GOPATH": "$DIR/exported/go", "GOENV": "$DIR/exported/env",
			},
			answers: map[string]string{
				"GOCACHE": "$DIR/exported/cache", "GOMODCACHE": "$DIR/exported/mod", "GOPATH": "$DIR/exported/go", "GOENV": "$DIR/exported/env",
			},
		},
		{
			name:      "a value with an equals sign in the go env file",
			file:      "GOPATH=$DIR/a=b\nGOCACHE=$DIR/from-file/cache\n",
			answers:   map[string]string{"GOPATH": "$DIR/a=b", "GOCACHE": "$DIR/from-file/cache"},
			underHome: []string{"GOENV"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			expand := func(text string) string { return strings.ReplaceAll(text, "$DIR", dir) }
			realHome := filepath.Join(dir, "home")
			if err := os.MkdirAll(realHome, 0o700); err != nil {
				t.Fatal(err)
			}

			// The environment of a developer who exports none of what the run touches, but for what the world says.
			scrub(t)
			for _, name := range append(append([]string{}, toolchainVariables...), xdgRoots...) {
				t.Setenv(name, "")
				os.Unsetenv(name)
			}
			t.Setenv("HOME", realHome)
			for name, value := range tc.exports {
				t.Setenv(name, expand(value))
			}
			if tc.file != "" {
				path := os.Getenv("GOENV")
				if path == "" {
					path = goEnv(t)["GOENV"]
				}
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(expand(tc.file)), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			before := goEnv(t)
			for name, want := range tc.answers {
				if before[name] != expand(want) {
					t.Fatalf("the world is not the one it says: go env %s = %q, want %q", name, before[name], expand(want))
				}
			}
			for _, name := range tc.underHome {
				if !strings.HasPrefix(before[name], realHome+string(os.PathSeparator)) {
					t.Fatalf("the world is not the one it says: go env %s = %q, want a default under the home %q", name, before[name], realHome)
				}
			}

			cleanup, err := Enter()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(cleanup)
			after := goEnv(t)

			for _, name := range toolchainVariables {
				if after[name] != before[name] {
					t.Errorf("go env %s = %q once the home is replaced, was %q: the rules of toolchainPins no longer keep the go command where it was", name, after[name], before[name])
				}
			}
		})
	}
}

// Check is what the tests of a package call to prove the run is in force there.
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
		{"an override that is set", func(t *testing.T, _ string) { t.Setenv("LONGTERM_MEM_ENGRAM_DB", "/x/engram.db") }, "LONGTERM_MEM_ENGRAM_DB is set"},
		{"an Engram directory in the home", func(t *testing.T, home string) {
			if err := os.Mkdir(filepath.Join(home, ".engram"), 0o700); err != nil {
				t.Fatal(err)
			}
		}, "the home of the tests holds a .engram directory"},
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
