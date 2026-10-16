package installer_test

// The world the longterm-mem pins run in. The pins are the longterm_pins_*_test.go files beside this one,
// one for each command: what `longterm-mem install` records of the targets it registered, and what
// `longterm-mem uninstall` removes from that record and when it removes the shared binary and the
// engine's registration record. They record what bin/labdrian-overlay did, so that where the
// script keeps the record of the installed targets can change without the program changing; they were
// recorded from the script as it was before that record moved into the engine (`installed-targets`),
// and they hold what that program did as it did it, but for the lines a later commit changed on
// purpose, whose diffs are in the history of testdata/longterm_pins.golden.
//
// The world is the routing pins' (routing_pins_world_test.go): an overlay repository and a home that
// has the engine. Around it are two programs that stand in for the machine's: a `go` that builds
// nothing (it puts the longterm-mem below where the script asked for it) and the longterm-mem itself,
// which only records what it is asked and ends with the code the test gave it for that verb and
// target. Neither reaches the real toolchain, the real longterm-mem or any real runtime.

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/filelock"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
)

// fakeGoScript is the `go` of the world: `go build -o <path> ...` copies the fake longterm-mem to
// <path>, or ends with 1 when FAKE_GO_FAIL is set. The script builds the engine with the real go only
// when it has none, and the world has one.
const fakeGoScript = `#!/bin/sh
out=""
while [ $# -gt 0 ]; do
  if [ "$1" = "-o" ]; then shift; out="$1"; fi
  shift
done
[ -n "$out" ] || exit 1
[ -z "$FAKE_GO_FAIL" ] || exit 1
cp "$FAKE_LM_SOURCE" "$out"
chmod +x "$out"
`

// fakeLongtermMemScript is the longterm-mem of the world. It writes its arguments, one call a line, in
// FAKE_LM_LOG; for register and unregister it ends with FAKE_REGISTER_EXIT_<target> (or
// FAKE_UNREGISTER_EXIT_<target>), else FAKE_REGISTER_EXIT (FAKE_UNREGISTER_EXIT), else 0; any other
// verb ends with 0.
const fakeLongtermMemScript = `#!/bin/sh
echo "$*" >> "$FAKE_LM_LOG"
verb="$1"; shift
target=""
while [ $# -gt 0 ]; do
  if [ "$1" = "--target" ]; then target="$2"; fi
  shift
done
case "$verb" in
  register) var="FAKE_REGISTER_EXIT_$target"; dflt="${FAKE_REGISTER_EXIT:-0}" ;;
  unregister) var="FAKE_UNREGISTER_EXIT_$target"; dflt="${FAKE_UNREGISTER_EXIT:-0}" ;;
  *) exit 0 ;;
esac
eval "code=\${$var:-$dflt}"
exit "$code"
`

// longtermWorld is a world whose PATH has the fake go first, whose overlay has the longterm-mem module
// directory the script wants to find, and whose longterm-mem records what it is asked.
func longtermWorld(t *testing.T) *pinWorld {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	w := newPinWorld(t, "skill-a/SKILL.md managed\n", map[string]string{"skills/skill-a/SKILL.md": "# skill-a\n"})
	overlaytest.WriteFile(w.t, w.overlay, "longterm-mem/README.md", "# the module the script looks for\n")
	dir := t.TempDir()
	for name, body := range map[string]string{"go": fakeGoScript, "fake-longterm-mem": fakeLongtermMemScript} {
		overlaytest.WriteTool(t, filepath.Join(dir, name), body)
	}
	w.setEnv("FAKE_LM_SOURCE", filepath.Join(dir, "fake-longterm-mem"))
	w.setEnv("FAKE_LM_LOG", filepath.Join(dir, "longterm-mem.log"))
	w.putFirst(dir)
	return w
}

// setEnv sets one variable of the environment of the world, replacing an earlier value.
func (w *pinWorld) setEnv(name, value string) {
	w.t.Helper()
	out := make([]string, 0, len(w.env)+1)
	for _, kv := range w.env {
		if !strings.HasPrefix(kv, name+"=") {
			out = append(out, kv)
		}
	}
	w.env = append(out, name+"="+value)
}

// stateDir is the state directory of the script: the one it is told in STATE_DIR, cleaned as the script
// cleans it, or the default.
func (w *pinWorld) stateDir() string {
	for _, kv := range w.env {
		if dir, ok := strings.CutPrefix(kv, "STATE_DIR="); ok {
			return filepath.Clean(dir)
		}
	}
	return filepath.Join(w.home, ".labdrian-overlay")
}

// trackingFile is the file the script keeps the targets it installed in.
func (w *pinWorld) trackingFile() string {
	return filepath.Join(w.stateDir(), "longterm-mem", "installed-targets")
}

// sharedBinary is where the script deploys longterm-mem to, and looks for it.
func (w *pinWorld) sharedBinary() string { return filepath.Join(w.stateDir(), "bin", "longterm-mem") }

// placeBinary puts the fake longterm-mem where an earlier install would have left it.
func (w *pinWorld) placeBinary() {
	w.t.Helper()
	body, err := os.ReadFile(w.env0("FAKE_LM_SOURCE"))
	if err != nil {
		w.t.Fatal(err)
	}
	overlaytest.WriteTool(w.t, w.sharedBinary(), string(body))
	// The mode a person's umask would leave is not what the pins are about.
	if err := os.Chmod(w.sharedBinary(), 0o755); err != nil {
		w.t.Fatal(err)
	}
}

// env0 is the value of a variable of the environment of the world.
func (w *pinWorld) env0(name string) string {
	w.t.Helper()
	for _, kv := range w.env {
		if value, ok := strings.CutPrefix(kv, name+"="); ok {
			return value
		}
	}
	w.t.Fatalf("the environment of the world has no %s", name)
	return ""
}

// seedTracking writes the bytes the script finds in its record of the installed targets, with the
// mode a person's editor would leave.
func (w *pinWorld) seedTracking(content string) {
	w.t.Helper()
	if err := os.MkdirAll(filepath.Dir(w.trackingFile()), 0o755); err != nil {
		w.t.Fatal(err)
	}
	if err := os.WriteFile(w.trackingFile(), []byte(content), 0o644); err != nil {
		w.t.Fatal(err)
	}
	if err := os.Chmod(w.trackingFile(), 0o644); err != nil {
		w.t.Fatal(err)
	}
}

// toolChatter is a line a tool of the machine (rm, mkdir, mktemp) says on its own account: its words,
// the way it quotes a path and the template it was given are the tool's and the platform's, not the
// script's, so a pin holds that there was one and not what it said.
var toolChatter = regexp.MustCompile(`(?m)^(rm|mv|mkdir|mktemp|tr|sort|grep|find|touch|chmod): .*$`)

// runLongterm runs the script with the arguments and keeps both streams apart, with what the tools of
// the machine said on their own account named and not quoted.
//
// The script runs under the umask 022: the mode of a file it creates (the record, by appending to it)
// is the mode the umask leaves, and a pin must not depend on the umask of whoever runs the tests.
func (w *pinWorld) runLongterm(args ...string) pinRun {
	w.t.Helper()
	cmd := exec.Command("bash", append([]string{"-c", `umask 022; exec bash "$@"`, "bash", w.script}, args...)...)
	cmd.Dir = w.overlay
	cmd.Env = w.env
	got := overlaytest.Capture(w.t, cmd)
	return pinRun{
		Stdout: toolChatter.ReplaceAllString(w.normalize(got.Stdout), "<a tool of the machine says something>"),
		Stderr: toolChatter.ReplaceAllString(w.normalize(got.Stderr), "<a tool of the machine says something>"),
		Exit:   got.Exit,
	}
}

// longtermState shows what a run left under the state directory: every file with its mode and its
// content, quoted so that a byte that is not a letter can be seen, the binary by what it is, and the
// calls the fake longterm-mem recorded. A lock the script takes, and the temporary file it writes
// through, are the script's own and are shown by what they leave behind: only a temporary file is
// shown, and none should be left.
func (w *pinWorld) longtermState() string {
	w.t.Helper()
	var lines []string
	root := w.stateDir()
	walk := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if isLockArtifact(rel) {
			return nil
		}
		body, err := os.ReadFile(path)
		if os.IsPermission(err) {
			lines = append(lines, rel+" (mode "+modeText(info)+"): <cannot be read>")
			return nil
		}
		if err != nil {
			return err
		}
		text := w.normalize(string(body))
		if path == w.sharedBinary() {
			if want, rerr := os.ReadFile(w.env0("FAKE_LM_SOURCE")); rerr == nil && bytes.Equal(want, body) {
				text = "<the fake longterm-mem>"
			}
		}
		lines = append(lines, rel+" (mode "+modeText(info)+"): "+strconv.Quote(text))
		return nil
	}
	if _, err := os.Stat(root); err == nil {
		if err := filepath.Walk(root, walk); err != nil {
			w.t.Fatalf("walk %s: %v", root, err)
		}
	}
	sort.Strings(lines)
	state := "(nothing)\n"
	if len(lines) > 0 {
		state = strings.Join(lines, "\n") + "\n"
	}
	return "--- state directory\n" + state + "--- calls of longterm-mem\n" + w.loggedCalls(w.env0("FAKE_LM_LOG"))
}

// isLockArtifact says whether a path under the state directory is the lock the script takes around
// its record of the installed targets: the lock itself, and the directory the break-in of a stale one
// takes. They are not what a run is pinned by; that the lock is let go is a test of its own.
func isLockArtifact(rel string) bool {
	return strings.HasPrefix(rel, ".installed-targets.lock") || strings.HasPrefix(rel, ".installed-targets.flock")
}

// modeText is the permission bits of a file as the pins show them.
func modeText(info os.FileInfo) string {
	return strconv.FormatUint(uint64(info.Mode().Perm()), 8)
}

// pinLongterm pins a run: how it ended and what it said, then what it left in the state directory.
func pinLongterm(t *testing.T, w *pinWorld, name string, run pinRun) {
	t.Helper()
	pin(t, name, pinRunText(run)+w.longtermState())
}

// pinSkipped marks the pins a test could not make as compared, so that the record is not reported as
// holding entries nobody looks at, and skips the test, saying which pins were not compared and why.
//
// The pins that need a record nobody can read, or a directory nobody can write in, can only be made by a
// user who is not root: root reads a file whose mode is 0 and writes where it should not. A run as root
// (a container) skips them, and the skip is the only trace: `go test -v` shows it with the pins named.
// The paths they pin are held for every user by tests that inject the failure (a store that answers
// Unreadable, a rewrite that fails at a stage) in engine/runtime/ownership/app, engine/runtime/ownership/fsstore
// and engine/cmd, and the way the script reacts to every exit code of the engine by
// longterm_engine_calls_test.go, which needs no privilege.
func pinSkipped(t *testing.T, reason string, names ...string) {
	t.Helper()
	reason += ": pins not compared: " + strings.Join(names, ", ")
	consumed.Lock()
	for _, name := range names {
		consumed.names[name] = true
	}
	consumed.Unlock()
	t.Skip(reason)
}

// holdLock takes the lock around the record of the installed targets, as another run of the overlay
// that is going on would: an advisory file lock on a file of the state directory, which the engine
// waits for and then refuses. It returns the function that lets it go. (Before the record moved into
// the engine the lock was a directory called .installed-targets.lock, which the script waited for
// about a second and then refused; see leftLockDirectory.)
func (w *pinWorld) holdLock() func() {
	w.t.Helper()
	if err := os.MkdirAll(w.stateDir(), 0o755); err != nil {
		w.t.Fatal(err)
	}
	release, err := filelock.Acquire(filepath.Join(w.stateDir(), ".installed-targets.flock"), filelock.Options{})
	if err != nil {
		w.t.Fatal(err)
	}
	return release
}

// leftLockDirectory leaves the lock the script used to take, a directory called
// .installed-targets.lock, in the state directory, as old as the age says, as a run of the script that
// was killed would have left it. The engine's lock is another file, so the directory is no lock of
// its own and is not waited for, not broken into and not warned about.
func (w *pinWorld) leftLockDirectory(age time.Duration) {
	w.t.Helper()
	lock := filepath.Join(w.stateDir(), ".installed-targets.lock")
	if err := os.MkdirAll(lock, 0o755); err != nil {
		w.t.Fatal(err)
	}
	when := time.Now().Add(-age)
	if err := os.Chtimes(lock, when, when); err != nil {
		w.t.Fatal(err)
	}
}

// canReadWhatIsMode0 says whether this user can read a file whose mode is 0, as root can: the
// pins about a record that cannot be read are made for a user who cannot.
func canReadWhatIsMode0(t *testing.T, dir string) bool {
	t.Helper()
	probe := filepath.Join(dir, "probe-mode-0")
	if err := os.WriteFile(probe, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(probe, 0); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(probe)
	f, err := os.Open(probe)
	if err == nil {
		f.Close()
	}
	return err == nil
}
