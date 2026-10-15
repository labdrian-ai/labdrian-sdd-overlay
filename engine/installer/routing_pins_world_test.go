package installer_test

// The world the routing pins run in (routing_pins_test.go): a throwaway overlay repository with
// the branches the script merges and compares, a home of its own, and a runner that keeps the two
// output streams and the exit code apart.
//
// The pins record what bin/labdrian-overlay does with the rows of overlay.manifest -- which row is
// read, which route it takes, where it is copied from and to, what each command says and writes --
// so that the way the script learns it can change without the program changing. They were
// recorded from the script as it was before its routing moved into the engine (`manifest routes`),
// and they hold what that program did as it did it.

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/enginetest"
)

// pinWorld is one overlay repository and the home its script runs in.
type pinWorld struct {
	t       *testing.T
	script  string
	home    string
	overlay string
	env     []string
	// gitBin is the git the world runs for itself: the clock (startClock) when the world has one,
	// and whatever the test process finds as "git" when it has none.
	gitBin string
}

// pinRun is what one run of the script left: what it wrote on each stream and how it ended.
type pinRun struct {
	Stdout string
	Stderr string
	Exit   int
}

// pinEnv is the environment the script runs in: the home and the overlay it is told of, an identity
// for git, the PATH of the machine, and the C locale, so that the words git and the tools say are
// the ones of the record whatever the machine's own locale is.
func pinEnv(home, overlay string) []string {
	return []string{
		"HOME=" + home,
		"OVERLAY_DIR=" + overlay,
		"GIT_AUTHOR_NAME=test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=test",
		"GIT_COMMITTER_EMAIL=test@test.com",
		"LC_ALL=C",
		"PATH=" + os.Getenv("PATH"),
	}
}

// The clock of a world. `git log` lists commits by their dates, and commits made in the same
// second tie: git then lists them in the order it happens to meet them, which for a few branches of
// one history is not the order they were made in and varies from one run to the next. A pin that
// holds the list as the script printed it needs a history whose commits have dates of their own,
// so the world puts a `git` first on the PATH that dates each call one second after the newest
// commit of the repository it runs in (and at pinClockEpoch + 1 in one that has none). It changes
// nothing else: the real git does the work, with the dates it was told.
const pinClockEpoch = 1_700_000_000

const pinClockScript = `#!/bin/sh
newest=$("$PIN_CLOCK_GIT" log -1 --all --format=%ct 2>/dev/null)
tick=$(( ${newest:-$PIN_CLOCK_EPOCH} + 1 ))
GIT_AUTHOR_DATE="$tick +0000"
GIT_COMMITTER_DATE="$tick +0000"
export GIT_AUTHOR_DATE GIT_COMMITTER_DATE
exec "$PIN_CLOCK_GIT" "$@"
`

// startClock puts the clock first on the PATH of the world. It is called once the world has an
// environment and before the first commit.
func (w *pinWorld) startClock() {
	w.t.Helper()
	realGit, err := exec.LookPath("git")
	if err != nil {
		w.t.Fatalf("the clock of the world needs git: %v", err)
	}
	dir := w.t.TempDir()
	w.gitBin = filepath.Join(dir, "git")
	if err := os.WriteFile(w.gitBin, []byte(pinClockScript), 0o755); err != nil {
		w.t.Fatalf("write the clock: %v", err)
	}
	w.env = append(w.env, "PIN_CLOCK_GIT="+realGit, "PIN_CLOCK_EPOCH="+strconv.Itoa(pinClockEpoch))
	w.changePath(func(old string) string { return dir + string(os.PathListSeparator) + old })
}

// commitTimes are the committer times of every commit of the repository of the world, newest first
// as `git log --all` lists them.
func (w *pinWorld) commitTimes() []int {
	w.t.Helper()
	var times []int
	for _, field := range strings.Fields(w.git("log", "--all", "--format=%ct")) {
		n, err := strconv.Atoi(field)
		if err != nil {
			w.t.Fatalf("commit time %q: %v", field, err)
		}
		times = append(times, n)
	}
	return times
}

func TestPinClockDatesEveryCommitOneSecondAfterTheNewestOfTheRepository(t *testing.T) {
	w := newPinWorld(t, "skill-a/SKILL.md managed\n", map[string]string{"skills/skill-a/SKILL.md": "# a\n"})
	w.commitOnMain("main: one", map[string]string{"one.txt": "1\n"})
	// A branch cut from the oldest commit: its commit is still the newest of the repository.
	w.git("checkout", "-b", "side", "upstream")
	w.commitOnMain("side: two", map[string]string{"two.txt": "2\n"})
	w.git("checkout", "main")
	w.git("merge", "--no-ff", "-m", "main: merge side", "side")

	want := []int{pinClockEpoch + 4, pinClockEpoch + 3, pinClockEpoch + 2, pinClockEpoch + 1}
	if got := w.commitTimes(); !reflect.DeepEqual(got, want) {
		t.Errorf("committer times, newest first = %v, want %v: the clock dates a commit one second after the newest, in the order they were made", got, want)
	}
}

func TestTheHomeOfAPinWorldHasANameOfItsOwn(t *testing.T) {
	// What a command says of the directory it runs in can carry its name (a registry is named after
	// its project), and a pin must not hold a name that depends on how many temporary directories
	// the test made before: the worlds of the pins keep their home under the name "home".
	for name, home := range map[string]string{
		"a world of the pins":     newPinWorld(t, "x managed\n", map[string]string{"skills/x/SKILL.md": "# x\n"}).home,
		"a world that bootstraps": newBootstrapWorld(t).home,
	} {
		if got := filepath.Base(home); got != "home" {
			t.Errorf("%s keeps its home in a directory called %q, want \"home\"", name, got)
		}
	}
}

// pinHome makes the home of a world: a directory called "home" in a temporary one.
func pinHome(t *testing.T) string {
	t.Helper()
	home := filepath.Join(t.TempDir(), "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("make the home of the world: %v", err)
	}
	return home
}

// newPinWorld builds an overlay repository holding the manifest and the files, with the
// 'upstream' and 'main' branches at the same commit, which is what apply, capture and the checks
// expect to find.
func newPinWorld(t *testing.T, manifest string, files map[string]string) *pinWorld {
	t.Helper()
	w := &pinWorld{t: t, script: overlayScript(t), home: pinHome(t), overlay: t.TempDir()}
	all := map[string]string{"overlay.manifest": manifest}
	for rel, content := range files {
		all[rel] = content
	}
	for rel, content := range all {
		w.write(w.overlay, rel, content)
	}
	w.env = pinEnv(w.home, w.overlay)
	w.startClock()
	w.git("init")
	w.git("config", "user.email", "test@test.com")
	w.git("config", "user.name", "test")
	w.git("checkout", "-b", "upstream")
	w.git("add", ".")
	w.git("commit", "-m", "upstream: baseline")
	w.git("checkout", "-b", "main")
	enginetest.Provision(t, w.home)
	return w
}

// withPath returns a copy of env whose PATH entry is what change makes of it. An environment with no
// PATH entry is an error: a test that edits the PATH of a world must not go on, as if it had, when
// the script would run on whatever PATH the shell defaults to.
func withPath(env []string, change func(old string) string) ([]string, error) {
	out := append([]string(nil), env...)
	for i, kv := range out {
		if old, ok := strings.CutPrefix(kv, "PATH="); ok {
			out[i] = "PATH=" + change(old)
			return out, nil
		}
	}
	return nil, errors.New("the environment of the world has no PATH entry to change")
}

func TestWithPathChangesThePathEntryAndRefusesAnEnvironmentWithNone(t *testing.T) {
	env := []string{"HOME=/h", "PATH=/usr/bin", "LC_ALL=C"}
	got, err := withPath(env, func(old string) string { return "/fake:" + old })
	if err != nil || !reflect.DeepEqual(got, []string{"HOME=/h", "PATH=/fake:/usr/bin", "LC_ALL=C"}) {
		t.Errorf("withPath = %q, %v, want the PATH entry changed and the rest as it was", got, err)
	}
	if env[1] != "PATH=/usr/bin" {
		t.Errorf("withPath changed the environment it was given: %q", env)
	}
	if got, err := withPath([]string{"HOME=/h"}, func(old string) string { return "/fake" }); err == nil {
		t.Errorf("withPath of an environment with no PATH = %q, nil, want an error: the test would go on as if the PATH had been changed", got)
	}
}

// changePath edits the PATH of the world, and fails the test when it has none.
func (w *pinWorld) changePath(change func(old string) string) {
	w.t.Helper()
	env, err := withPath(w.env, change)
	if err != nil {
		w.t.Fatal(err)
	}
	w.env = env
}

// write puts content at rel under root, making the directories it needs.
func (w *pinWorld) write(root, rel, content string) {
	w.t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		w.t.Fatalf("mkdir %s: %v", filepath.Dir(p), err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		w.t.Fatalf("write %s: %v", rel, err)
	}
}

// git runs git in the overlay repository and fails the test when it fails.
func (w *pinWorld) git(args ...string) string {
	w.t.Helper()
	bin := w.gitBin
	if bin == "" {
		bin = "git"
	}
	cmd := exec.Command(bin, args...)
	cmd.Dir = w.overlay
	cmd.Env = w.env
	out, err := cmd.CombinedOutput()
	if err != nil {
		w.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// run runs the script with the arguments, from the overlay repository, and keeps its streams apart.
func (w *pinWorld) run(args ...string) pinRun {
	w.t.Helper()
	return w.runIn(w.overlay, args...)
}

// runIn runs the script with the arguments from the directory dir, and keeps its streams apart.
func (w *pinWorld) runIn(dir string, args ...string) pinRun {
	w.t.Helper()
	cmd := exec.Command("bash", append([]string{w.script}, args...)...)
	cmd.Dir = dir
	cmd.Env = w.env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	run := pinRun{Stdout: w.normalize(stdout.String()), Stderr: gitChatter.ReplaceAllString(w.normalize(stderr.String()), "")}
	var exit *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exit):
		run.Exit = exit.ExitCode()
	default:
		w.t.Fatalf("run %v: %v", args, err)
	}
	return run
}

// The parts of an output that a run makes up and that a pin must not carry: the places the world
// lives in, the moment a backup or a deploy happened, the commit a branch is at, and the day; and
// the words git says on its own account, which its version decides: the advice it gives about a
// branch name, the way it spells that a merge has nothing to do, and the notice of every checkout
// it makes on stderr ("Already on", "Switched to"). What the script and the engine say is the pin;
// what git says of the checkouts they make is not.
var (
	backupStamp = regexp.MustCompile(`[0-9]{8}T[0-9]{6}Z(-[0-9]+)?`)
	commitLine  = regexp.MustCompile(`\[([^\]]+?) [0-9a-f]{7,}\]`)
	oneLineLog  = regexp.MustCompile(`(?m)^[0-9a-f]{7} `)
	moment      = regexp.MustCompile(`[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z`)
	calendarDay = regexp.MustCompile(`[0-9]{4}-[0-9]{2}-[0-9]{2}`)
	gitAdvice   = regexp.MustCompile(`(?m)^hint:.*\n`)
	upToDate    = regexp.MustCompile(`Already up-to-date\.`)
	gitChatter  = regexp.MustCompile(`(?m)^(Already on '[^']*'|Switched to (a new |and reset )?branch '[^']*')\n`)
)

// normalize replaces what varies from one run to the next by a name.
func (w *pinWorld) normalize(s string) string {
	s = strings.ReplaceAll(s, w.home, "$HOME")
	s = strings.ReplaceAll(s, w.overlay, "$OVERLAY")
	s = backupStamp.ReplaceAllString(s, "<STAMP>")
	s = commitLine.ReplaceAllString(s, "[$1 <SHA>]")
	s = oneLineLog.ReplaceAllString(s, "<SHA> ")
	s = moment.ReplaceAllString(s, "<TIME>")
	s = gitAdvice.ReplaceAllString(s, "")
	s = upToDate.ReplaceAllString(s, "Already up to date.")
	return calendarDay.ReplaceAllString(s, "<DAY>")
}

// recordedState shows state.json by what it records, a target at a time and sorted: the file
// lists the targets in the order bash happens to hold them, which is not what a pin is about.
func recordedState(body string) string {
	var lines []string
	for _, m := range stateEntry.FindAllStringSubmatch(body, -1) {
		lines = append(lines, m[1]+" "+m[2]+" "+m[3])
	}
	sort.Strings(lines)
	return strings.Join(lines, "; ")
}

var stateEntry = regexp.MustCompile(`"([a-z]+)": \{"version": "([^"]*)", "digest": "([0-9a-f]*)"`)

// tree lists every file under root, as "relative path: content", sorted, one per line: what a
// command left behind. A root that is not there lists as nothing.
func (w *pinWorld) tree(root string) string {
	w.t.Helper()
	var lines []string
	walk := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || enginetest.IsProvisioned(path) {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		content := quoteBody(string(body))
		if filepath.Base(path) == "state.json" {
			content = recordedState(string(body))
		}
		lines = append(lines, w.normalize(rel)+": "+w.normalize(content))
		return nil
	}
	if _, err := os.Stat(root); err != nil {
		return pinText("")
	}
	if err := filepath.Walk(root, walk); err != nil {
		w.t.Fatalf("walk %s: %v", root, err)
	}
	sort.Strings(lines)
	if len(lines) == 0 {
		return pinText("")
	}
	return strings.Join(lines, "\n") + "\n"
}

// quoteBody shows a file's content on one line.
func quoteBody(s string) string {
	return strings.ReplaceAll(strings.TrimSuffix(s, "\n"), "\n", `\n`)
}

// commitOnMain adds files to the branch the world is on and commits them, so that they exist on
// 'main' and not on 'upstream'.
func (w *pinWorld) commitOnMain(message string, files map[string]string) {
	w.t.Helper()
	for rel, content := range files {
		w.write(w.overlay, rel, content)
	}
	w.git("add", ".")
	w.git("commit", "-m", message)
}
