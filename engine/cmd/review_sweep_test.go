package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
	reviewapp "github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/fsstore"
)

// The tests of `review sweep` (own-rdd T6b4, S6 b) run the verb over the repository of a reviewWorld with the real
// adapters, a state home in a temporary directory and, where a program of the sweep matters, stubs of the analyzers
// on a PATH that holds nothing else. analyzers.ExecRunner looks the program up on the PATH of the process, not on the
// environment it is handed, so the tests set the PATH of the process.

// sweepRun runs `review <args>` with the state home stateHome.
func (w *reviewWorld) sweepRun(stateHome, stdin string, args ...string) (stdout, stderr string, exits []int) {
	w.t.Helper()
	p := newCapturedProcess()
	p.stdin = strings.NewReader(stdin)
	d := testDeps()
	d.getwd = func() (string, error) { return w.cwd, nil }
	d.environ = goldenEnvironment
	d.getenv = environmentOf(map[string]string{"XDG_STATE_HOME": stateHome})
	run(p.process, d, append([]string{"review"}, args...))
	return p.out.String(), p.err.String(), p.exits
}

// onlyThesePrograms sets the PATH of the process to a directory that holds git and the stubs, and nothing else. A stub
// is the body of a shell script.
func onlyThesePrograms(t *testing.T, stubs map[string]string) {
	t.Helper()
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skipf("git unavailable: %v", err)
	}
	bin := t.TempDir()
	if err := os.Symlink(git, filepath.Join(bin, "git")); err != nil {
		t.Fatalf("setup: %v", err)
	}
	for name, body := range stubs {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	t.Setenv("PATH", bin)
}

// swept is what `review sweep` prints on stdout.
type swept struct {
	Outcome    string   `json:"outcome"`
	Path       string   `json:"path"`
	SHA256     string   `json:"sha256"`
	Candidates []string `json:"candidates"`
}

func decodeSwept(t *testing.T, stdout string) swept {
	t.Helper()
	var s swept
	if err := json.Unmarshal([]byte(stdout), &s); err != nil {
		t.Fatalf("stdout is not the JSON of a sweep: %v\n%s", err, stdout)
	}
	return s
}

func TestReviewSweepStoresACleanSweepAndPrintsItsOutcomePathAndDigest(t *testing.T) {
	w := newReviewWorld(t)
	state := t.TempDir()

	stdout, stderr, exits := w.sweepRun(state, "", "sweep", "--feature", "own-rdd", "--base", w.base, "--head", w.head)

	if !equalInts(exits, []int{0}) {
		t.Fatalf("exit calls = %v, stderr = %q; want [0]", exits, stderr)
	}
	storedPath := filepath.Join(w.results("own-rdd"), w.head[:12]+"-1.sweep.json")
	stored, err := os.ReadFile(storedPath)
	if err != nil {
		t.Fatalf("the sweep is not stored at %s: %v", storedPath, err)
	}
	got := decodeSwept(t, stdout)
	if got.Outcome != "clean" || got.Path != "odd/review-results/own-rdd/"+w.head[:12]+"-1.sweep.json" || got.SHA256 != sum(stored) || len(got.Candidates) != 0 {
		t.Errorf("stdout = %+v, want clean, the path of the record, its digest and no candidates", got)
	}
	if !strings.Contains(stderr, "review sweep: stored "+got.Path+" (sha256 "+got.SHA256+")") {
		t.Errorf("stderr = %q, want it to say where the sweep is stored", stderr)
	}
	if !strings.Contains(string(stored), `"outcome": "clean"`) {
		t.Errorf("the stored sweep = %s, want a clean outcome", stored)
	}
}

func TestReviewSweepFiresOnAFileLeftInTheWorkingTreeAndPrintsTheCandidates(t *testing.T) {
	w := newReviewWorld(t)
	overlaytest.WriteFile(t, w.root, "stray.txt", "left behind\n")

	stdout, stderr, exits := w.sweepRun(t.TempDir(), "", "sweep", "--feature", "own-rdd", "--base", w.base, "--head", w.head)

	if !equalInts(exits, []int{0}) {
		t.Fatalf("exit calls = %v, stderr = %q; want [0]: a sweep that fired is stored", exits, stderr)
	}
	got := decodeSwept(t, stdout)
	if got.Outcome != "fired" || len(got.Candidates) != 1 || got.Candidates[0] != "T3: ?? stray.txt" {
		t.Errorf("stdout = %+v, want fired with the candidate %q", got, "T3: ?? stray.txt")
	}
}

func TestReviewSweepRunsTheAnalyzersOnCopiesKeptUnderTheStateHome(t *testing.T) {
	w := newReviewWorld(t)
	overlaytest.WriteFile(t, w.root, "run.sh", "echo hi\n")
	overlaytest.WriteFile(t, w.root, "bad.sh", "echo bad\n")
	w.git("add", "-A")
	w.git("commit", "-q", "-m", "shell")
	// The base copy has no bad.sh, so only the head copy gets a finding from the stub.
	onlyThesePrograms(t, map[string]string{"shellcheck": `if [ -f bad.sh ]; then echo "bad.sh:1:1: warning: bad"; exit 1; fi; exit 0`})
	state := t.TempDir()
	head := w.name("HEAD")

	stdout, stderr, exits := w.sweepRun(state, "", "sweep", "--feature", "own-rdd", "--base", "HEAD~1", "--head", "HEAD")

	if !equalInts(exits, []int{0}) {
		t.Fatalf("exit calls = %v, stderr = %q; want [0]", exits, stderr)
	}
	got := decodeSwept(t, stdout)
	if got.Outcome != "fired" || len(got.Candidates) != 1 || !strings.HasPrefix(got.Candidates[0], "T2 shellcheck .: ") {
		t.Errorf("stdout = %+v, want fired with one T2 candidate of shellcheck", got)
	}
	if !strings.HasPrefix(got.Path, "odd/review-results/own-rdd/"+head[:12]+"-1.sweep") {
		t.Errorf("path = %q, want the sweep of the head", got.Path)
	}
	headCopies, _ := filepath.Glob(filepath.Join(state, "labdrian", "review-sweep", "*", "head", "bad.sh"))
	if len(headCopies) != 1 {
		t.Errorf("copies of the head under the state home with bad.sh = %v, want one", headCopies)
	}
}

func TestReviewSweepExitsTwoNamingTheToolThatCannotRunAndStoresNothing(t *testing.T) {
	w := newReviewWorld(t)
	overlaytest.WriteFile(t, w.root, "run.sh", "echo hi\n")
	w.git("add", "-A")
	w.git("commit", "-q", "-m", "shell")
	onlyThesePrograms(t, nil)

	stdout, stderr, exits := w.sweepRun(t.TempDir(), "", "sweep", "--feature", "own-rdd", "--base", "HEAD~1", "--head", "HEAD")

	if stdout != "" || !equalInts(exits, []int{2}) || !strings.HasPrefix(stderr, "error: review sweep: ") || !strings.Contains(stderr, "shellcheck") {
		t.Errorf("stdout = %q, stderr = %q, exits = %v; want nothing, an error that names shellcheck, [2]", stdout, stderr, exits)
	}
	if names := listNames(t, w.results("own-rdd")); len(names) != 0 {
		t.Errorf("stored %v, want nothing: a check that did not run stores nothing", names)
	}
}

func TestReviewSweepExitsTwoWhileAnotherSweepOfTheRepositoryRuns(t *testing.T) {
	w := newReviewWorld(t)
	state := t.TempDir()
	copies, err := fsstore.NewCopies(state)
	if err != nil {
		t.Skipf("the copies of a sweep are unsupported here: %v", err)
	}
	unlock, err := copies.Lock(reviewapp.Repository{Main: w.root})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer unlock()

	stdout, stderr, exits := w.sweepRun(state, "", "sweep", "--feature", "own-rdd", "--base", w.base, "--head", w.head)

	want := "error: review sweep: another review sweep of this repository is running\n"
	if stdout != "" || stderr != want || !equalInts(exits, []int{2}) {
		t.Errorf("stdout = %q, stderr = %q, exits = %v; want nothing, %q, [2]", stdout, stderr, exits, want)
	}
	if names := listNames(t, w.results("own-rdd")); len(names) != 0 {
		t.Errorf("stored %v, want nothing", names)
	}
}

func TestReviewSweepExitsTwoWhenTheBindingIsRefused(t *testing.T) {
	w := newReviewWorld(t)

	for name, args := range map[string][]string{
		"a range of no commit":   {"--feature", "own-rdd", "--base", w.head, "--head", w.head},
		"a revision git rejects": {"--feature", "own-rdd", "--base", "no-such-revision", "--head", w.head},
		"an unsafe feature":      {"--feature", "../x", "--base", w.base, "--head", w.head},
	} {
		t.Run(name, func(t *testing.T) {
			stdout, stderr, exits := w.sweepRun(t.TempDir(), "", append([]string{"sweep"}, args...)...)

			if stdout != "" || !equalInts(exits, []int{2}) || !strings.HasPrefix(stderr, "error: review sweep: ") {
				t.Errorf("stdout = %q, stderr = %q, exits = %v; want nothing, an error of review sweep, [2]", stdout, stderr, exits)
			}
		})
	}
}

func TestReviewSweepExitsTwoWhenTheStateHomeCannotBeFound(t *testing.T) {
	w := newReviewWorld(t)
	p := newCapturedProcess()
	d := testDeps()
	d.getwd = func() (string, error) { return w.cwd, nil }
	d.environ = goldenEnvironment
	d.getenv = environmentOf(nil)

	run(p.process, d, []string{"review", "sweep", "--feature", "own-rdd", "--base", w.base, "--head", w.head})

	if p.out.String() != "" || !equalInts(p.exits, []int{2}) || !strings.HasPrefix(p.err.String(), "error: review sweep: ") {
		t.Errorf("stdout = %q, stderr = %q, exits = %v; want nothing, an error of review sweep, [2]", p.out.String(), p.err.String(), p.exits)
	}
}

func TestReviewSweepRefusesACommandLineItCannotReadWithExitOne(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"no feature", []string{"sweep", "--base", "b", "--head", "h"}, "error: review sweep: --feature is required\n"},
		{"no base", []string{"sweep", "--feature", "f", "--head", "h"}, "error: review sweep: --base is required\n"},
		{"no head", []string{"sweep", "--feature", "f", "--base", "b"}, "error: review sweep: --head is required\n"},
		{"an unknown flag", []string{"sweep", "--feature", "f", "--base", "b", "--head", "h", "--rows", "default"}, "error: review sweep: unknown flag \"--rows\"\n"},
		{"the role", []string{"sweep", "--feature", "f", "--base", "b", "--head", "h", "--role", "sweeper"}, "error: review sweep: unknown flag \"--role\"\n"},
		{"a flag twice", []string{"sweep", "--feature", "f", "--feature", "g", "--base", "b", "--head", "h"}, "error: review sweep: --feature given twice\n"},
		{"a positional argument", []string{"sweep", "extra"}, "error: review sweep: unexpected argument \"extra\"\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newReviewWorld(t)

			stdout, stderr, exits := w.sweepRun(t.TempDir(), "", tc.args...)

			if stdout != "" || stderr != tc.want || !equalInts(exits, []int{1}) {
				t.Errorf("stdout = %q, stderr = %q, exits = %v; want nothing, %q, [1]", stdout, stderr, exits, tc.want)
			}
		})
	}
}

func TestUsageDocumentsReviewSweepAndTheRolesOfReviewRecord(t *testing.T) {
	var out strings.Builder
	usage(&out)
	text := out.String()

	for _, want := range []string{
		"engine review sweep --feature <name> --base <rev> --head <rev> [--cwd <repo>]",
		"another review sweep of this repository is running",
		"<head12>-<n>.sweep.json",
		"exit 0 stored (clean or fired), 2 refused or a check that did not run, 1 usage error",
		"engine review record --role sweeper|polisher --feature <name> --base <rev> --head <rev> [--result <file>|-] [--cwd <repo>]",
		"<head12>-<n>.sweeper.json", "<head12>-<n>.polisher.json",
		"--rows with another role is a usage error",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("usage lacks %q", want)
		}
	}
}

// The three places that define the checks of `review sweep` (own-rdd T6b7c, S6 g) say what T1 to T3
// are, and none of them sends the reader to the role-chain skill, which does not define them.
func TestUsageAndReadmeDefineTheChecksOfReviewSweep(t *testing.T) {
	var out bytes.Buffer
	usage(&out)
	engine := strings.Join(strings.Fields(out.String()), " ")
	flatFile := func(path string) string {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return strings.Join(strings.Fields(string(raw)), " ")
	}
	for _, place := range []struct {
		name, text string
		holds      []string
	}{
		{"the engine usage", engine, []string{
			"runs the checks T1 to T3 of the sweeper once on the one commit",
			"T1: the paths the commit deleted or renamed",
			"T2: deadcode and staticcheck over each Go module the commit changes",
			"T3: git status of the working tree as it is",
		}},
		{"bin/lib/usage.sh", flatFile("../../bin/lib/usage.sh"), []string{
			"run the checks T1 to T3 of the sweeper once on the one commit",
			"T1 the paths the commit deleted or renamed",
			"T2 the new lines of deadcode, staticcheck and shellcheck",
			"T3 the working tree's git status",
		}},
		{"README.md", flatFile("../../README.md"), []string{
			"Run the checks T1 to T3 of the sweeper once on the one commit",
			"T1: the paths the commit deleted or renamed",
			"T2: `deadcode -test ./...` over each Go module",
			"T3: `git status` of the working tree",
		}},
	} {
		for _, want := range place.holds {
			if !strings.Contains(place.text, want) {
				t.Errorf("%s lacks %q", place.name, want)
			}
		}
		if strings.Contains(place.text, "of the sweeper (skills/role-chain/SKILL.md)") {
			t.Errorf("%s sends the reader to the skill for the definition of T1 to T3", place.name)
		}
	}
}
