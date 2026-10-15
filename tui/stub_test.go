package main

// A stand-in for bin/labdrian-overlay that answers like the backend does for the questions the TUI asks and
// records every invocation, so a whole session of the program (the catalog, the lookups, the actions, the probe)
// can be driven against it without a repository, a state directory or a deployed target behind it.

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// stubScript is the backend. It appends its arguments to invoked.log, then answers from files under .stub/:
// out/<key> is what it prints, err/<key> what it prints on stderr, exit/<key> its exit status, and on/<key> a
// script it runs first (how a test makes one invocation change the answer of another). The key is the
// subcommand, except for `restore --target <name> --list`, whose key is restore-list-<name>.
const stubScript = `#!/bin/sh
d="$(dirname "$0")/../.stub"
printf '%s\n' "$*" >> "$d/invoked.log"
key="$1"
if [ "$1" = restore ] && [ "$2" = --target ] && [ "$4" = --list ]; then key="restore-list-$3"; fi
if [ -f "$d/on/$key" ]; then sh "$d/on/$key"; fi
if [ -f "$d/out/$key" ]; then cat "$d/out/$key"; fi
if [ -f "$d/err/$key" ]; then cat "$d/err/$key" >&2; fi
code=0
if [ -f "$d/exit/$key" ]; then code="$(cat "$d/exit/$key")"; fi
exit "$code"
`

// defaultCatalog is what the backend's `targets` lists: the three per-file targets and the package target.
const defaultCatalog = "claude\tcopy\nopencode\tcopy\ncodex\tcopy\npi\tpackage\n"

// stubBackend is a backend installed under a scratch repo root.
type stubBackend struct {
	t    *testing.T
	root string
	dir  string
}

// newStubBackend installs the stub under a fresh root and gives `targets` the default catalog.
func newStubBackend(t *testing.T) *stubBackend {
	t.Helper()
	root := t.TempDir()
	s := &stubBackend{t: t, root: root, dir: filepath.Join(root, ".stub")}
	if err := os.MkdirAll(filepath.Join(root, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "bin", "labdrian-overlay"), []byte(stubScript), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, sub := range []string{"out", "err", "exit", "on"} {
		if err := os.MkdirAll(filepath.Join(s.dir, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	s.say("targets", defaultCatalog)
	return s
}

func (s *stubBackend) write(kind, key, content string) {
	s.t.Helper()
	if err := os.WriteFile(filepath.Join(s.dir, kind, key), []byte(content), 0o644); err != nil {
		s.t.Fatal(err)
	}
}

// say sets what the backend prints for key.
func (s *stubBackend) say(key, stdout string) { s.t.Helper(); s.write("out", key, stdout) }

// complain sets what the backend prints on stderr for key.
func (s *stubBackend) complain(key, stderr string) { s.t.Helper(); s.write("err", key, stderr) }

// exit sets the exit status of key.
func (s *stubBackend) exit(key string, code int) {
	s.t.Helper()
	s.write("exit", key, strconv.Itoa(code))
}

// onRun sets a shell command the backend runs when key is invoked, before it answers.
func (s *stubBackend) onRun(key, script string) { s.t.Helper(); s.write("on", key, script) }

// backups gives target the listing `restore --target <target> --list` prints for those backups
// ("<timestamp> (<version>)", newest last).
func (s *stubBackend) backups(target string, entries ...string) {
	s.t.Helper()
	var b strings.Builder
	b.WriteString("Backups for " + target + " (newest last):\n")
	for _, e := range entries {
		b.WriteString("  " + e + "\n")
	}
	s.say("restore-list-"+target, b.String())
}

// noBackups makes target have none, which is what the backend says for a target nothing was saved for.
func (s *stubBackend) noBackups(target string) {
	s.t.Helper()
	s.say("restore-list-"+target, "No backups available for target '"+target+"'.\n")
}

// invoked returns the arguments of each invocation the backend received, in order.
func (s *stubBackend) invoked() []string {
	s.t.Helper()
	data, err := os.ReadFile(filepath.Join(s.dir, "invoked.log"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		s.t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}
