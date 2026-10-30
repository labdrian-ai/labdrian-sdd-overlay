// Package piguard is the test support that keeps a test run away from the real `pi`. An earlier
// version of the Pi tests ran a real `pi remove` against the setup of the person who ran `go
// test`; since the Pi adapter takes its commands through a port, no test of it starts a process,
// and this guard is the net under every test that might.
//
// Install puts a `pi` and a `codex` first on the PATH of the process. Each is the same shell script, which
// records its arguments and exits 97, so whatever looks up `pi` or `codex` finds the guard before any real one, and the
// run can ask afterwards whether anything started it. A TestMain installs it and fails the run
// when Started reports true.
//
// One test may still run the real `pi`: the live test of the Pi global tier, when the person
// running the tests exports LABDRIAN_PI_LIVE_TEST=1. RequestLive then hands it the PATH of the run
// without the guard, and RefuseRealHome keeps it to a HOME of its own that is not the person's
// (live.go). The guard stays installed for every other test.
package piguard

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// guarded are the programs the guard refuses: the Pi CLI, and the Codex CLI that the review launcher starts.
var guarded = []string{"pi", "codex"}

// Guard is an installed guard.
type Guard struct {
	dir      string
	marker   string
	previous string
}

// Install makes the guard and puts it first on the PATH. On Windows, which has no shell script to
// put there, it installs nothing and the guard never reports a start.
func Install() (*Guard, error) {
	g := &Guard{previous: os.Getenv("PATH")}
	if runtime.GOOS == "windows" {
		return g, nil
	}
	dir, err := os.MkdirTemp("", GuardDirPrefix+"*")
	if err != nil {
		return nil, err
	}
	g.dir = dir
	g.marker = filepath.Join(dir, "started")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> '" + g.marker + "'\nexit 97\n"
	for _, name := range guarded {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
			os.RemoveAll(dir)
			return nil, err
		}
	}
	path := dir
	if g.previous != "" {
		path += string(os.PathListSeparator) + g.previous
	}
	if err := os.Setenv("PATH", path); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	return g, nil
}

// Started reports whether anything ran the guard.
func (g *Guard) Started() bool {
	if g.marker == "" {
		return false
	}
	_, err := os.Stat(g.marker)
	return err == nil
}

// Report is what the guard was started with, one line per start, or empty when it never was.
func (g *Guard) Report() string {
	if g.marker == "" {
		return ""
	}
	data, err := os.ReadFile(g.marker)
	if err != nil {
		return ""
	}
	return string(data)
}

// Close puts the PATH back and removes the guard.
func (g *Guard) Close() {
	if g.dir == "" {
		return
	}
	os.Setenv("PATH", g.previous)
	os.RemoveAll(g.dir)
	g.dir = ""
}

// Verdict is the message a TestMain prints when the run started the guard, which then fails the
// run; ok is true, and the message empty, when nothing did.
func (g *Guard) Verdict() (message string, ok bool) {
	if !g.Started() {
		return "", true
	}
	return fmt.Sprintf("a test started `pi` or `codex`; the guard refused it. It was started with:\n%s", g.Report()), false
}
