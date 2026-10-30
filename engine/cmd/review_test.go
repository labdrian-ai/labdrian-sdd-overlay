package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	reviewapp "github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/fsstore"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/gitexec"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewverdict"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/roles"
)

// The tests of `review bind` and `review record` run the command over a repository they make in a
// temporary directory, with the git of the machine reading no configuration of the machine
// (goldenEnvironment). The worktrees of a review are made beside the repository, so they are made inside the
// same temporary directory and removed with it.

// reviewWorld is a repository of two commits, base and head, and the temporary directory it sits in.
type reviewWorld struct {
	t      *testing.T
	parent string   // the temporary directory: the repository and, beside it, its worktrees
	root   string   // the repository
	cwd    string   // the working directory the command runs in
	env    []string // the environment of the git that makes the repository
	base   string
	head   string
}

func newReviewWorld(t *testing.T) *reviewWorld {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git unavailable: %v", err)
	}
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	root := filepath.Join(parent, "repo")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	w := &reviewWorld{t: t, parent: parent, root: root, cwd: root, env: append(goldenEnvironment(), overlaytest.GitSettings()...)}
	w.git("init", "-q", "-b", "main")
	overlaytest.WriteFile(t, root, "a.txt", "one\n")
	overlaytest.WriteFile(t, root, "dir/b.txt", "b\n")
	w.git("add", "-A")
	w.git("commit", "-q", "-m", "base")
	w.base = w.name("HEAD")
	overlaytest.WriteFile(t, root, "a.txt", "one\ntwo\n")
	overlaytest.WriteFile(t, root, "c.txt", "c\n")
	w.git("add", "-A")
	w.git("commit", "-q", "-m", "head")
	w.head = w.name("HEAD")
	return w
}

func (w *reviewWorld) git(args ...string) string {
	w.t.Helper()
	return overlaytest.Git(w.t, w.root, w.env, args...)
}

func (w *reviewWorld) name(rev string) string { return strings.TrimSpace(w.git("rev-parse", rev)) }

func (w *reviewWorld) worktree(head string) string {
	return filepath.Join(w.parent, "repo-worktrees", "review-"+head[:12])
}

func (w *reviewWorld) results(feature string) string {
	return filepath.Join(w.root, "odd", "review-results", feature)
}

// run runs `review <args>` in-process, with the working directory and the environment of the world, and
// returns what it wrote on each stream and the exit codes it called.
func (w *reviewWorld) run(stdin string, args ...string) (stdout, stderr string, exits []int) {
	w.t.Helper()
	p := newCapturedProcess()
	p.stdin = strings.NewReader(stdin)
	w.runOn(p, args...)
	return p.out.String(), p.err.String(), p.exits
}

// runOn runs `review <args>` in-process on the process p, which a test builds with the streams it wants.
func (w *reviewWorld) runOn(p *capturedProcess, args ...string) {
	w.t.Helper()
	d := testDeps()
	d.getwd = func() (string, error) { return w.cwd, nil }
	d.environ = goldenEnvironment
	run(p.process, d, append([]string{"review"}, args...))
}

// brokenStdout is a standard output that refuses every write, as a pipe whose reader has gone away does.
type brokenStdout struct{ err error }

func (b brokenStdout) Write([]byte) (int, error) { return 0, b.err }

// reviewBinding is the JSON object `review bind` prints.
type reviewBinding struct {
	Base            string   `json:"base"`
	Head            string   `json:"head"`
	DiffSHA256      string   `json:"diff_sha256"`
	Paths           []string `json:"paths"`
	Worktree        string   `json:"worktree"`
	WorktreeCreated bool     `json:"worktree_created"`
	DiffBytes       int      `json:"diff_bytes"`
	DiffLimitBytes  int      `json:"diff_limit_bytes"`
	OverLimit       bool     `json:"over_limit"`
	Note            string   `json:"note"`
}

// bind runs `review bind` for the two commits of the world and returns the binding it printed.
func (w *reviewWorld) bind() reviewBinding {
	w.t.Helper()
	stdout, stderr, exits := w.run("", "bind", "--base", w.base, "--head", w.head)
	if !equalInts(exits, []int{0}) || stderr != "" {
		w.t.Fatalf("setup: review bind exit calls = %v, stderr = %q; want [0] and nothing", exits, stderr)
	}
	return decodeBinding(w.t, stdout)
}

func decodeBinding(t *testing.T, stdout string) reviewBinding {
	t.Helper()
	var b reviewBinding
	if err := json.Unmarshal([]byte(stdout), &b); err != nil {
		t.Fatalf("stdout is not the JSON binding: %v\n%s", err, stdout)
	}
	return b
}

// reply is the JSON object of a reviewer that inspected every path of the binding and found nothing.
func reply(t *testing.T, b reviewBinding, edit func(map[string]any)) string {
	t.Helper()
	paths := make([]any, len(b.Paths))
	for i, p := range b.Paths {
		paths[i] = p
	}
	o := map[string]any{
		"candidate":  map[string]any{"base": b.Base, "head": b.Head, "diff_sha256": b.DiffSHA256},
		"inspection": map[string]any{"status": "completed", "paths": paths, "reason": ""},
		"findings":   []any{},
		"evidence":   []any{"read every changed path"},
	}
	if edit != nil {
		edit(o)
	}
	data, err := json.Marshal(o)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	return string(data)
}

func sum(data []byte) string {
	s := sha256.Sum256(data)
	return hex.EncodeToString(s[:])
}

func listNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("read %s: %v", dir, err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}

// The three verbs are routed, and a command line the verbs cannot read is a usage error: exit 1 with one line
// on standard error and nothing made, before git is asked anything.
func TestReviewRefusesACommandLineItCannotRead(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"no verb", nil, "error: review requires a verb: bind, record, verify, check, exempt, sweep, anchor\n"},
		{"an unknown verb", []string{"bogus"}, "error: review: unknown verb \"bogus\" (expected bind, record, verify, check, exempt, sweep or anchor)\n"},
		{"verify with no feature", []string{"verify", "--stored", "s.json"}, "error: review verify: --feature is required\n"},
		{"verify with no stored result", []string{"verify", "--feature", "f"}, "error: review verify: --stored is required\n"},
		{"verify with a flag of record", []string{"verify", "--feature", "f", "--stored", "s.json", "--head", "h"}, "error: review verify: unknown flag \"--head\"\n"},
		{"the verdicts twice", []string{"verify", "--feature", "f", "--stored", "s.json", "--verdicts", "a", "--verdicts", "b"}, "error: review verify: --verdicts given twice\n"},
		{"the stored result given to record", []string{"record", "--feature", "f", "--base", "b", "--head", "h", "--stored", "s.json"}, "error: review record: unknown flag \"--stored\"\n"},
		{"an unknown flag", []string{"bind", "--nope"}, "error: review bind: unknown flag \"--nope\"\n"},
		{"a flag with no value", []string{"bind", "--base"}, "error: review bind: --base requires a value\n"},
		{"a positional argument", []string{"bind", "extra"}, "error: review bind: unexpected argument \"extra\"\n"},
		{"no base", []string{"bind", "--head", "h"}, "error: review bind: --base is required\n"},
		{"no head", []string{"bind", "--base", "b"}, "error: review bind: --head is required\n"},
		{"an empty base", []string{"bind", "--base", "", "--head", "h"}, "error: review bind: --base is required\n"},
		{"a flag twice", []string{"bind", "--base", "b", "--base", "c", "--head", "h"}, "error: review bind: --base given twice\n"},
		{"a flag of record given to bind", []string{"bind", "--feature", "f", "--base", "b", "--head", "h"}, "error: review bind: unknown flag \"--feature\"\n"},
		{"the result given to bind", []string{"bind", "--result", "x", "--base", "b", "--head", "h"}, "error: review bind: unknown flag \"--result\"\n"},
		{"record with no feature", []string{"record", "--base", "b", "--head", "h"}, "error: review record: --feature is required\n"},
		{"record with no base", []string{"record", "--feature", "f", "--head", "h"}, "error: review record: --base is required\n"},
		{"record with a result and no value", []string{"record", "--feature", "f", "--base", "b", "--head", "h", "--result"}, "error: review record: --result requires a value\n"},
		{"the result twice", []string{"record", "--feature", "f", "--base", "b", "--head", "h", "--result", "a", "--result", "b"}, "error: review record: --result given twice\n"},
		{"the working directory twice", []string{"record", "--feature", "f", "--base", "b", "--head", "h", "--cwd", "a", "--cwd", "b"}, "error: review record: --cwd given twice\n"},
		{"record with no rows", []string{"record", "--feature", "f", "--base", "b", "--head", "h"}, "error: review record: --rows is required\n"},
		{"record with empty rows", []string{"record", "--feature", "f", "--base", "b", "--head", "h", "--rows", ""}, "error: review record: --rows is required\n"},
		{"the rows given to bind", []string{"bind", "--rows", "default", "--base", "b", "--head", "h"}, "error: review bind: unknown flag \"--rows\"\n"},
		{"the rows given to verify", []string{"verify", "--feature", "f", "--stored", "s.json", "--rows", "default"}, "error: review verify: unknown flag \"--rows\"\n"},
		{"the rows given to check", []string{"check", "--feature", "f", "--rows", "default"}, "error: review check: unknown flag \"--rows\"\n"},
		{"the rows given to exempt", []string{"exempt", "--feature", "f", "--base", "b", "--head", "h", "--risk", "none", "--rows", "default"}, "error: review exempt: unknown flag \"--rows\"\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newReviewWorld(t)

			stdout, stderr, exits := w.run("", tc.args...)

			if stdout != "" || stderr != tc.want || !equalInts(exits, []int{1}) {
				t.Errorf("stdout = %q, stderr = %q, exits = %v; want nothing, %q, [1]", stdout, stderr, exits, tc.want)
			}
			if _, err := os.Stat(filepath.Join(w.parent, "repo-worktrees")); err == nil {
				t.Error("a worktree directory was made for a command line that was refused")
			}
		})
	}
}

func TestBindPrintsTheBindingOfTheCandidateAndMakesTheDetachedWorktree(t *testing.T) {
	w := newReviewWorld(t)

	stdout, stderr, exits := w.run("", "bind", "--base", "HEAD~1", "--head", "HEAD")

	if !equalInts(exits, []int{0}) || stderr != "" {
		t.Fatalf("exit calls = %v, stderr = %q; want [0] and nothing", exits, stderr)
	}
	if !strings.HasSuffix(stdout, "}\n") || strings.Count(stdout, "\n}") != 1 {
		t.Errorf("stdout is not one JSON object followed by a newline:\n%s", stdout)
	}
	b := decodeBinding(t, stdout)
	if b.Base != w.base || b.Head != w.head {
		t.Errorf("base = %s and head = %s, want the full names %s and %s", b.Base, b.Head, w.base, w.head)
	}
	diff, err := exec.Command("git", append([]string{"-C", w.root}, gitexec.DiffArgs(w.base, w.head)...)...).Output()
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	if b.DiffSHA256 != sum(diff) || b.DiffBytes != len(diff) {
		t.Errorf("diff_sha256 = %s over %d bytes, want %s over %d: the SHA-256 of the bytes of the canonical diff", b.DiffSHA256, b.DiffBytes, sum(diff), len(diff))
	}
	if strings.Join(b.Paths, ",") != "a.txt,c.txt" {
		t.Errorf("paths = %q, want [a.txt c.txt]", b.Paths)
	}
	if b.Worktree != w.worktree(w.head) || !b.WorktreeCreated {
		t.Errorf("worktree = %s (created %v), want %s created", b.Worktree, b.WorktreeCreated, w.worktree(w.head))
	}
	if b.DiffLimitBytes != review.DiffLimitBytes || b.OverLimit || b.Note != "" {
		t.Errorf("limit = %d, over_limit = %v, note = %q; want %d, false and no note", b.DiffLimitBytes, b.OverLimit, b.Note, review.DiffLimitBytes)
	}
	if content, err := os.ReadFile(filepath.Join(b.Worktree, "c.txt")); err != nil || string(content) != "c\n" {
		t.Errorf("c.txt in the worktree = %q, %v; want the file of the head", content, err)
	}
	if got := strings.TrimSpace(overlaytest.Git(t, b.Worktree, w.env, "rev-parse", "HEAD")); got != w.head {
		t.Errorf("the worktree is at %s, want the head %s", got, w.head)
	}
}

// Binding the same candidate again finds the worktree where it was put and changes nothing.
func TestBindAgainReusesTheWorktreeAndPrintsTheSameBinding(t *testing.T) {
	w := newReviewWorld(t)
	first := w.bind()
	if err := os.WriteFile(filepath.Join(first.Worktree, "marker"), []byte("kept"), 0o644); err != nil {
		t.Fatal(err)
	}

	second := w.bind()

	if second.WorktreeCreated {
		t.Error("worktree_created = true the second time, want false: the worktree was there at the head")
	}
	second.WorktreeCreated = first.WorktreeCreated
	if second.Base != first.Base || second.DiffSHA256 != first.DiffSHA256 || second.Worktree != first.Worktree || strings.Join(second.Paths, ",") != strings.Join(first.Paths, ",") {
		t.Errorf("second binding %+v differs from the first %+v", second, first)
	}
	if _, err := os.Stat(filepath.Join(first.Worktree, "marker")); err != nil {
		t.Errorf("the worktree was touched by the second bind: %v", err)
	}
}

func TestBindSaysSoWhenTheDiffIsOverTheLimitAndDoesNotFail(t *testing.T) {
	w := newReviewWorld(t)
	overlaytest.WriteFile(t, w.root, "big.txt", strings.Repeat("0123456789012345678901234567890123456789012345678\n", 3200))
	w.git("add", "-A")
	w.git("commit", "-q", "-m", "big")
	w.head = w.name("HEAD")

	stdout, stderr, exits := w.run("", "bind", "--base", w.base, "--head", w.head)

	if !equalInts(exits, []int{0}) || stderr != "" {
		t.Fatalf("exit calls = %v, stderr = %q; want [0] and nothing for a diff over the limit", exits, stderr)
	}
	b := decodeBinding(t, stdout)
	if !b.OverLimit || b.DiffBytes <= review.DiffLimitBytes {
		t.Errorf("over_limit = %v with %d bytes, want true with more than %d", b.OverLimit, b.DiffBytes, review.DiffLimitBytes)
	}
	if !strings.Contains(b.Note, strconv.Itoa(review.DiffLimitBytes)) || !strings.Contains(b.Note, "one launch per commit") {
		t.Errorf("note = %q, want it to give the limit and say the candidate is split into one launch per commit", b.Note)
	}
	if !b.WorktreeCreated {
		t.Error("worktree_created = false, want the worktree made for a diff over the limit too")
	}
}

// Standard output is where the verbs hand over what they made: the binding the parent of the chain reads, the
// record it appends. When it cannot be written (a pipe whose reader has gone, a full disk) the verb has not
// done what it is for, and exit 0 would tell the parent that it holds a binding it never received. The work
// that was done stays done, and the message says where it is.
func TestBindFailsWithExit1WhenTheBindingCannotBeWritten(t *testing.T) {
	w := newReviewWorld(t)
	p := newCapturedProcess()
	p.stdout = brokenStdout{errors.New("broken pipe")}

	w.runOn(p, "bind", "--base", w.base, "--head", w.head)

	want := "error: review bind: the worktree is at " + w.worktree(w.head) + ", but the binding could not be written: write to stdout: broken pipe\n"
	if !equalInts(p.exits, []int{1}) || p.err.String() != want || p.out.Len() != 0 {
		t.Errorf("exits = %v, stderr = %q, stdout = %q; want [1], %q and nothing", p.exits, p.err.String(), p.out.String(), want)
	}
	if info, err := os.Stat(w.worktree(w.head)); err != nil || !info.IsDir() {
		t.Errorf("the worktree the message names is not there: %v", err)
	}
}

func TestBindRefusesAPathThatIsTakenAndLeavesItAsItIs(t *testing.T) {
	w := newReviewWorld(t)
	taken := w.worktree(w.head)
	if err := os.MkdirAll(taken, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(taken, "mine"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, exits := w.run("", "bind", "--base", w.base, "--head", w.head)

	if stdout != "" || !equalInts(exits, []int{1}) {
		t.Errorf("stdout = %q, exits = %v; want nothing and [1]", stdout, exits)
	}
	if !strings.HasPrefix(stderr, "error: review bind: ") || !strings.Contains(stderr, taken) || !strings.Contains(stderr, "not a worktree of this repository") {
		t.Errorf("stderr = %q, want the path and why it is refused", stderr)
	}
	if content, err := os.ReadFile(filepath.Join(taken, "mine")); err != nil || string(content) != "mine" {
		t.Errorf("the file that was there = %q, %v; want it untouched", content, err)
	}
}

// The worktrees are beside the main checkout, wherever the command runs: from a linked worktree, which is
// where a person works on a feature branch, the review worktree is still next to the main checkout.
func TestBindFromALinkedWorktreePutsTheReviewWorktreeBesideTheMainCheckout(t *testing.T) {
	w := newReviewWorld(t)
	linked := filepath.Join(w.parent, "feature-checkout")
	w.git("worktree", "add", "-q", "--detach", linked, w.head)
	w.cwd = linked

	stdout, stderr, exits := w.run("", "bind", "--base", w.base, "--head", w.head)

	if !equalInts(exits, []int{0}) || stderr != "" {
		t.Fatalf("exit calls = %v, stderr = %q; want [0] and nothing", exits, stderr)
	}
	if got := decodeBinding(t, stdout).Worktree; got != w.worktree(w.head) {
		t.Errorf("worktree = %s, want %s: beside the main checkout %s", got, w.worktree(w.head), w.root)
	}
}

func TestBindReadsTheRepositoryFromCwdAndResolvesARelativeOneAgainstTheWorkingDirectory(t *testing.T) {
	w := newReviewWorld(t)
	w.cwd = w.parent

	stdout, stderr, exits := w.run("", "bind", "--cwd", "repo", "--base", w.base, "--head", w.head)

	if !equalInts(exits, []int{0}) || stderr != "" {
		t.Fatalf("exit calls = %v, stderr = %q; want [0] and nothing", exits, stderr)
	}
	if got := decodeBinding(t, stdout).Worktree; got != w.worktree(w.head) {
		t.Errorf("worktree = %s, want %s", got, w.worktree(w.head))
	}
	other := t.TempDir()
	_, stderr, exits = w.run("", "bind", "--cwd", other, "--base", w.base, "--head", w.head)
	if !equalInts(exits, []int{1}) || !strings.HasPrefix(stderr, "error: review bind: ") {
		t.Errorf("outside a repository: exits = %v, stderr = %q; want [1] and an error of review bind", exits, stderr)
	}
}

func TestBindRefusesWhatIsNotACommitAndWhatGitWouldReadAsAnOption(t *testing.T) {
	w := newReviewWorld(t)
	marker := filepath.Join(w.parent, "written-by-an-option")

	for _, tc := range []struct{ name, base, want string }{
		{"a name that is nothing", "nonesuch", "resolve base \"nonesuch\""},
		{"an option", "--output=" + marker, "starts with '-' and would be read as an option"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, exits := w.run("", "bind", "--base", tc.base, "--head", w.head)

			if stdout != "" || !equalInts(exits, []int{1}) || !strings.Contains(stderr, tc.want) {
				t.Errorf("stdout = %q, exits = %v, stderr = %q; want nothing, [1] and %q", stdout, exits, stderr, tc.want)
			}
			if _, err := os.Stat(marker); err == nil {
				t.Error("a file was written by a revision that is an option")
			}
			if _, err := os.Stat(filepath.Join(w.parent, "repo-worktrees")); err == nil {
				t.Error("a worktree was made for a candidate that could not be bound")
			}
		})
	}
}

func TestRecordStoresTheReplyUnderTheNextFreeNumberAndPrintsTheSkeletonOfTheRecord(t *testing.T) {
	w := newReviewWorld(t)
	b := w.bind()
	ok := reply(t, b, nil)

	stdout, stderr, exits := w.run(ok, "record", "--feature", "own-rdd", "--base", w.base, "--head", w.head, "--rows", "default")

	if !equalInts(exits, []int{0}) {
		t.Fatalf("exit calls = %v, stderr = %q; want [0]", exits, stderr)
	}
	short := w.head[:12]
	storedPath := filepath.Join(w.results("own-rdd"), short+"-1.json")
	stored, err := os.ReadFile(storedPath)
	if err != nil {
		t.Fatalf("the result is not stored at %s: %v", storedPath, err)
	}
	if info, _ := os.Stat(storedPath); info == nil || info.Mode().Perm() != 0o644 {
		t.Errorf("the stored file has mode %v, want 0644", info)
	}
	var file struct {
		Version int `json:"version"`
		Binding struct {
			Base       string   `json:"base"`
			Head       string   `json:"head"`
			DiffSHA256 string   `json:"diff_sha256"`
			Paths      []string `json:"paths"`
		} `json:"binding"`
		Reply json.RawMessage `json:"reply"`
	}
	if err := json.Unmarshal(stored, &file); err != nil {
		t.Fatalf("the stored file is not JSON: %v", err)
	}
	if file.Version != 1 || file.Binding.Base != b.Base || file.Binding.Head != b.Head || file.Binding.DiffSHA256 != b.DiffSHA256 ||
		strings.Join(file.Binding.Paths, ",") != strings.Join(b.Paths, ",") {
		t.Errorf("stored binding %+v, want version 1 and the binding %+v", file, b)
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, file.Reply); err != nil || compact.String() != ok {
		t.Errorf("stored reply = %s, want the reply that was sent: %s", compact.String(), ok)
	}

	var skeleton roles.RoleHandoff
	if err := json.Unmarshal([]byte(stdout), &skeleton); err != nil {
		t.Fatalf("stdout is not the skeleton of a RoleHandoff: %v\n%s", err, stdout)
	}
	ref := "odd/review-results/own-rdd/" + short + "-1.json"
	if skeleton.Version != 1 || skeleton.FromRole != roles.RoleReviewer || skeleton.PayloadKind != "review-report" ||
		skeleton.PayloadSHA256 != sum(stored) || skeleton.Status != roles.StatusCompleted {
		t.Errorf("skeleton = %+v, want version 1, from the reviewer, a review-report with the digest of the stored file, completed", skeleton)
	}
	wantEvidence := []roles.Evidence{
		{Kind: "review-diff", Ref: w.base + ".." + w.head, SHA256: b.DiffSHA256},
		{Kind: "review-result", Ref: ref, SHA256: sum(stored)},
	}
	if len(skeleton.Evidence) != 2 || skeleton.Evidence[0] != wantEvidence[0] || skeleton.Evidence[1] != wantEvidence[1] {
		t.Errorf("evidence = %+v, want %+v", skeleton.Evidence, wantEvidence)
	}
	for _, want := range []string{ref, sum(stored), "project_id", "goal_id", "chain_id", "seq", "prev_sha256", "to_role", "context.summary"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr = %q, want it to name %q", stderr, want)
		}
	}
}

func TestRecordNeverReplacesAResultAndTakesTheNextNumberFromAnyReader(t *testing.T) {
	w := newReviewWorld(t)
	b := w.bind()
	ok := reply(t, b, nil)
	file := filepath.Join(w.parent, "reply.json")
	if err := os.WriteFile(file, []byte(ok), 0o644); err != nil {
		t.Fatal(err)
	}
	short := w.head[:12]

	for i, tc := range []struct {
		name  string
		stdin string
		args  []string
	}{
		{"stdin by default", ok, nil},
		{"stdin by name", ok, []string{"--result", "-"}},
		{"a file", "", []string{"--result", file}},
		{"stdin once more", ok, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := listNames(t, w.results("own-rdd"))
			args := append([]string{"record", "--feature", "own-rdd", "--base", w.base, "--head", w.head, "--rows", "default"}, tc.args...)

			_, stderr, exits := w.run(tc.stdin, args...)

			if !equalInts(exits, []int{0}) {
				t.Fatalf("exit calls = %v, stderr = %q; want [0]", exits, stderr)
			}
			after := listNames(t, w.results("own-rdd"))
			want := append(append([]string{}, before...), short+"-"+string(rune('1'+i))+".json")
			if strings.Join(after, ",") != strings.Join(want, ",") {
				t.Errorf("results = %q, want %q: one new file with the next number, and no temporary file", after, want)
			}
		})
	}
}

// --cwd names the repository, as it does for bind, and a relative --result is a file of the working directory of
// the process (as the --file of roles validate is), not of the repository --cwd names.
func TestRecordReadsTheRepositoryFromCwdAndARelativeResultFromTheWorkingDirectory(t *testing.T) {
	w := newReviewWorld(t)
	b := w.bind()
	if err := os.WriteFile(filepath.Join(w.parent, "reply.json"), []byte(reply(t, b, nil)), 0o644); err != nil {
		t.Fatal(err)
	}
	w.cwd = w.parent

	_, stderr, exits := w.run("", "record", "--cwd", "repo", "--feature", "own-rdd", "--base", w.base, "--head", w.head, "--rows", "default", "--result", "reply.json")

	if !equalInts(exits, []int{0}) {
		t.Fatalf("exit calls = %v, stderr = %q; want [0]", exits, stderr)
	}
	if got, want := listNames(t, w.results("own-rdd")), []string{w.head[:12] + "-1.json"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("the repository --cwd names holds %q, want %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(w.parent, "odd")); err == nil {
		t.Error("the results were stored in the working directory, not in the repository --cwd names")
	}

	other := t.TempDir()
	_, stderr, exits = w.run(reply(t, b, nil), "record", "--cwd", other, "--feature", "own-rdd", "--base", w.base, "--head", w.head, "--rows", "default")
	if !equalInts(exits, []int{1}) || !strings.HasPrefix(stderr, "error: review record: ") {
		t.Errorf("outside a repository: exits = %v, stderr = %q; want [1] and an error of review record", exits, stderr)
	}
	if names := listNames(t, other); len(names) != 0 {
		t.Errorf("the directory outside a repository holds %q, want nothing", names)
	}
}

// A git that hangs must not hang the command, and the git of the verbs sees the environment of the process: the
// adapter is built with the environment as the deps give it (it removes the variables of git itself) and a
// deadline for every start of git.
func TestReviewGitIsToldTheEnvironmentOfTheProcessAndADeadline(t *testing.T) {
	environ := []string{"HOME=/home/someone", "GIT_DIR=/elsewhere/.git"}
	d := testDeps()
	d.environ = func() []string { return environ }

	got := reviewGitOptions(d)

	if !reflect.DeepEqual(got.Environ, environ) {
		t.Errorf("Environ = %q, want the environment of the deps %q", got.Environ, environ)
	}
	if got.Timeout != reviewGitTimeout || got.Timeout <= 0 || got.Timeout > time.Hour {
		t.Errorf("Timeout = %v, want reviewGitTimeout (%v), a deadline of minutes: zero is no limit at all", got.Timeout, reviewGitTimeout)
	}
}

// A reply that does not hold is refused with the named reason, exit 2, and writes nothing: not a file, not a
// directory. The binding it is held to is recomputed from git, so the reviewer's echo can only agree with it.
func TestRecordRefusesAReplyThatDoesNotHoldAndWritesNothing(t *testing.T) {
	w := newReviewWorld(t)
	b := w.bind()
	flip := func(s string) string {
		if s[0] == '0' {
			return "1" + s[1:]
		}
		return "0" + s[1:]
	}

	for _, tc := range []struct {
		name  string
		reply string
		want  string
	}{
		{"another digest", reply(t, b, func(o map[string]any) { o["candidate"].(map[string]any)["diff_sha256"] = flip(b.DiffSHA256) }), "reviewresult: candidate echo differs from the binding"},
		{"another head", reply(t, b, func(o map[string]any) { o["candidate"].(map[string]any)["head"] = w.base }), "reviewresult: candidate echo differs from the binding"},
		{"an inspection that was not completed", reply(t, b, func(o map[string]any) {
			o["inspection"].(map[string]any)["status"] = "partial"
			o["inspection"].(map[string]any)["reason"] = "ran out of room"
		}), "reviewresult: inspection not completed"},
		{"paths that are not the paths of the diff", reply(t, b, func(o map[string]any) { o["inspection"].(map[string]any)["paths"] = []any{"a.txt"} }), "reviewresult: inspected paths differ from the paths of the diff"},
		{"a severe finding without proof", reply(t, b, func(o map[string]any) {
			o["findings"] = []any{map[string]any{
				"id": "F1", "severity": "BLOCKER", "causal_disposition": "introduced", "evidence_class": "deterministic",
				"location": "a.txt:1", "claim": "it breaks", "proof_refs": []any{},
			}}
		}), "reviewresult: severe finding without a proof reference"},
		{"prose", "I looked at it and it is fine.", "reviewresult: unparseable reply"},
		{"nothing", "", "reviewresult: unparseable reply"},
		{"a reply that is too large", strings.Repeat(" ", review.MaxReplyBytes+1), "review: reply too large"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, exits := w.run(tc.reply, "record", "--feature", "own-rdd", "--base", w.base, "--head", w.head, "--rows", "default")

			if stdout != "" || !equalInts(exits, []int{2}) {
				t.Errorf("stdout = %q, exits = %v; want nothing and [2]", stdout, exits)
			}
			if want := "error: review record: the reply is refused and nothing was stored: "; !strings.HasPrefix(stderr, want) || !strings.Contains(stderr, tc.want) {
				t.Errorf("stderr = %q, want it to begin %q and name %q", stderr, want, tc.want)
			}
			if _, err := os.Stat(filepath.Join(w.root, "odd")); err == nil {
				t.Error("a refused reply made the results directory")
			}
		})
	}
}

// A refused reply leaves what is stored as it is.
func TestRecordLeavesTheStoredResultsAsTheyAreWhenAReplyIsRefused(t *testing.T) {
	w := newReviewWorld(t)
	b := w.bind()
	w.run(reply(t, b, nil), "record", "--feature", "own-rdd", "--base", w.base, "--head", w.head, "--rows", "default")
	stored := filepath.Join(w.results("own-rdd"), w.head[:12]+"-1.json")
	before, err := os.ReadFile(stored)
	if err != nil {
		t.Fatal(err)
	}

	_, _, exits := w.run("{}", "record", "--feature", "own-rdd", "--base", w.base, "--head", w.head, "--rows", "default")

	after, _ := os.ReadFile(stored)
	if !equalInts(exits, []int{2}) || !bytes.Equal(before, after) || len(listNames(t, w.results("own-rdd"))) != 1 {
		t.Errorf("exits = %v, results = %q; want [2] and the one file as it was", exits, listNames(t, w.results("own-rdd")))
	}
}

// The feature is one safe segment of a path and nothing else: whatever else a command line gives cannot
// lead outside odd/review-results/.
func TestRecordRefusesAFeatureThatIsNotOneSafePathSegment(t *testing.T) {
	w := newReviewWorld(t)
	b := w.bind()
	ok := reply(t, b, nil)
	before := listNames(t, w.parent)

	for _, feature := range []string{"..", "../escape", "a/b", "/etc", "/tmp/escape", ".hidden", "-x", "a b", "a\\b", "a\x00b", "a\nb", strings.Repeat("x", 101), "..a", "a..b"} {
		t.Run(feature, func(t *testing.T) {
			stdout, stderr, exits := w.run(ok, "record", "--feature", feature, "--base", w.base, "--head", w.head, "--rows", "default")

			if stdout != "" || !equalInts(exits, []int{1}) || !strings.Contains(stderr, "not a single safe path segment") {
				t.Errorf("stdout = %q, exits = %v, stderr = %q; want nothing, [1] and a feature refused as unsafe", stdout, exits, stderr)
			}
			if _, err := os.Stat(filepath.Join(w.root, "odd")); err == nil {
				t.Error("a feature that was refused made the results directory")
			}
			if after := listNames(t, w.parent); strings.Join(after, ",") != strings.Join(before, ",") {
				t.Errorf("the directory beside the repository holds %q, was %q", after, before)
			}
		})
	}
}

// What the repository itself puts on the way to the results directory cannot lead the write out of it.
func TestRecordWritesNothingThroughASymbolicLink(t *testing.T) {
	for _, tc := range []struct {
		name string
		link string // the path inside the repository that becomes a link
		make string // the directory that must exist before the link is made
	}{
		{"odd", "odd", ""},
		{"the results directory", "odd/review-results", "odd"},
		{"the directory of the feature", "odd/review-results/own-rdd", "odd/review-results"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newReviewWorld(t)
			b := w.bind()
			outside := t.TempDir()
			if tc.make != "" {
				if err := os.MkdirAll(filepath.Join(w.root, tc.make), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Symlink(outside, filepath.Join(w.root, tc.link)); err != nil {
				t.Fatal(err)
			}

			stdout, stderr, exits := w.run(reply(t, b, nil), "record", "--feature", "own-rdd", "--base", w.base, "--head", w.head, "--rows", "default")

			if stdout != "" || !equalInts(exits, []int{1}) || !strings.Contains(stderr, "symbolic link") {
				t.Errorf("stdout = %q, exits = %v, stderr = %q; want nothing, [1] and the link named", stdout, exits, stderr)
			}
			if names := listNames(t, outside); len(names) != 0 {
				t.Errorf("the target of the link holds %q, want nothing written through it", names)
			}
		})
	}
}

func TestRecordFailsWithExit1WhenTheRepositoryOrTheReplyCannotBeRead(t *testing.T) {
	w := newReviewWorld(t)
	b := w.bind()
	ok := reply(t, b, nil)
	outside := t.TempDir()

	for _, tc := range []struct {
		name string
		cwd  string
		base string // "" is the base of the world
		args []string
		want string
	}{
		{"a directory that is in no repository", outside, "", nil, "error: review record: "},
		{"a result file that is not there", w.root, "", []string{"--result", filepath.Join(w.parent, "missing.json")}, "error: review record: "},
		{"a commit that is not there", w.root, "nonesuch", nil, "resolve base \"nonesuch\""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w.cwd = tc.cwd
			defer func() { w.cwd = w.root }()
			base := tc.base
			if base == "" {
				base = w.base
			}
			args := []string{"record", "--feature", "own-rdd", "--base", base, "--head", w.head, "--rows", "default"}
			args = append(args, tc.args...)

			stdout, stderr, exits := w.run(ok, args...)

			if stdout != "" || !equalInts(exits, []int{1}) || !strings.Contains(stderr, tc.want) {
				t.Errorf("stdout = %q, exits = %v, stderr = %q; want nothing, [1] and %q", stdout, exits, stderr, tc.want)
			}
			if _, err := os.Stat(filepath.Join(w.root, "odd")); err == nil {
				t.Error("a record that failed made the results directory")
			}
		})
	}
}

// The result is stored before the handoff is printed, and a handoff that cannot be printed leaves the result
// where it is. The exit is 1, which is not the 2 of a reply that was refused: nothing was refused, and the
// message says where the result is so that the handoff can be made again from it.
func TestRecordFailsWithExit1AndSaysWhereTheResultIsWhenTheHandoffCannotBeWritten(t *testing.T) {
	w := newReviewWorld(t)
	b := w.bind()
	p := newCapturedProcess()
	p.stdin = strings.NewReader(reply(t, b, nil))
	p.stdout = brokenStdout{errors.New("broken pipe")}

	w.runOn(p, "record", "--feature", "own-rdd", "--base", w.base, "--head", w.head, "--rows", "default")

	ref := "odd/review-results/own-rdd/" + w.head[:12] + "-1.json"
	want := "error: review record: the result is stored at " + ref + ", but the handoff could not be written: write to stdout: broken pipe\n"
	if !equalInts(p.exits, []int{1}) || !strings.HasSuffix(p.err.String(), want) || p.out.Len() != 0 {
		t.Errorf("exits = %v, stderr = %q, stdout = %q; want [1], an error that ends with %q, and nothing on stdout", p.exits, p.err.String(), p.out.String(), want)
	}
	if _, err := os.Stat(filepath.Join(w.root, filepath.FromSlash(ref))); err != nil {
		t.Errorf("the result the message names is not stored: %v", err)
	}
}

// racingResults is the results directory of the working tree with other writers in it: each of its first races
// Creates finds the name already taken, by a file another process wrote just before, and the store answers that
// it is taken. It notes how long after each of those answers the verb tried again.
type racingResults struct {
	reviewapp.Results
	races    int
	answered time.Time
	waits    []time.Duration
}

func (r *racingResults) Create(root, feature, name string, data []byte) error {
	if !r.answered.IsZero() {
		r.waits = append(r.waits, time.Since(r.answered))
		r.answered = time.Time{}
	}
	if r.races == 0 {
		return r.Results.Create(root, feature, name, data)
	}
	r.races--
	if err := r.Results.Create(root, feature, name, []byte("{\"written\":\"by another process\"}\n")); err != nil {
		return err
	}
	err := r.Results.Create(root, feature, name, data)
	r.answered = time.Now()
	return err
}

// review/app does not read the clock (it is in the application ring), so the wait between the tries for a
// number is wired here, on the clock of the machine. The shortest wait of the backoff is a millisecond, and a
// verb that was wired without a clock tries again at once, which is a few microseconds after the answer.
func TestRecordWaitsOnTheClockOfTheMachineBetweenTriesForANumber(t *testing.T) {
	w := newReviewWorld(t)
	b := w.bind()
	d := testDeps()
	d.environ = goldenEnvironment
	results := &racingResults{Results: fsstore.New(), races: 3}
	service := reviewServiceOver(newReviewGit(d), results)

	recorded, err := service.Record(w.root, "own-rdd", w.base, w.head, "default", []byte(reply(t, b, nil)))

	if want := "odd/review-results/own-rdd/" + w.head[:12] + "-4.json"; err != nil || recorded.Path != want {
		t.Fatalf("Record() = %q, %v; want the result stored under the fourth number, %q", recorded.Path, err, want)
	}
	if len(results.waits) != 3 {
		t.Fatalf("the verb tried again %d times, want 3: one for each number another writer took", len(results.waits))
	}
	for i, waited := range results.waits {
		if waited < time.Millisecond {
			t.Errorf("the verb tried again %v after the answer of try %d, want at least a millisecond: it waits on the clock of the machine", waited, i+1)
		}
	}
}

func TestUsageDocumentsTheReviewVerbs(t *testing.T) {
	var out bytes.Buffer
	usage(&out)
	text := out.String()

	for _, want := range []string{
		"engine review bind --base <rev> --head <rev> [--cwd <repo>]",
		"engine review record --feature <name> --base <rev> --head <rev> --rows <id,id> [--result <file>|-] [--cwd <repo>]",
		"diff_sha256", "over_limit", "-worktrees/review-<head12>",
		"odd/review-results/<feature>/<head12>-<n>.json", "never over an existing file",
		"exit 0 stored, 2 reply refused", "--rows is required", "roles/reviewer/rows.json",
		"exit 0 bound, 1 usage error, refusal or a binding that could not be written",
		fmt.Sprintf("A diff over %d bytes is bound all the same", review.DiffLimitBytes),
	} {
		if !strings.Contains(text, want) {
			t.Errorf("usage lacks %q", want)
		}
	}
}

// The built program: a whole review of a candidate, with the streams and the exit codes the parent of the role
// chain reads. The skeleton `record` prints is the record of the chain once the parent fills the members only
// it knows, and `roles validate` agrees.
func TestReviewVerbsOfTheBuiltProgramBindStoreAndHandOffARecordTheChainAccepts(t *testing.T) {
	w := newReviewWorld(t)
	bin := engineBinary(t)
	run := func(stdin string, args ...string) (stdout, stderr string, code int) {
		t.Helper()
		cmd := exec.Command(bin, args...)
		cmd.Dir = w.root
		cmd.Env = goldenEnvironment()
		cmd.Stdin = strings.NewReader(stdin)
		var o, e bytes.Buffer
		cmd.Stdout, cmd.Stderr = &o, &e
		err := cmd.Run()
		code = 0
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else if err != nil {
			t.Fatalf("run %v: %v", args, err)
		}
		return o.String(), e.String(), code
	}

	out, errText, code := run("", "review", "bind", "--base", "HEAD~1", "--head", "HEAD")
	if code != 0 || errText != "" {
		t.Fatalf("review bind: exit %d, stderr %q; want 0 and nothing", code, errText)
	}
	b := decodeBinding(t, out)
	ok := reply(t, b, nil)

	out, errText, code = run(ok, "review", "record", "--feature", "own-rdd", "--base", "HEAD~1", "--head", "HEAD", "--rows", "default")
	if code != 0 || !strings.Contains(errText, "odd/review-results/own-rdd/"+w.head[:12]+"-1.json") {
		t.Fatalf("review record: exit %d, stderr %q; want 0 and the stored path named", code, errText)
	}
	var skeleton roles.RoleHandoff
	if err := json.Unmarshal([]byte(out), &skeleton); err != nil {
		t.Fatalf("stdout is not the skeleton: %v\n%s", err, out)
	}
	write := func(h roles.RoleHandoff) string {
		data, err := json.Marshal(h)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(t.TempDir(), "record.json")
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}

	if _, _, code := run("", "roles", "validate", "--file", write(skeleton)); code == 0 {
		t.Error("roles validate accepted the skeleton as it is printed, want it refused until the parent fills its members")
	}
	filled := skeleton
	filled.ProjectID, filled.GoalID, filled.ChainID, filled.Seq = "p", "g", "c", 1
	filled.PrevSHA256, filled.ToRole = strings.Repeat("0", 64), roles.RoleDelivery
	filled.Context.Summary = "reviewed, nothing found"
	if out, errText, code := run("", "roles", "validate", "--file", write(filled)); code != 0 {
		t.Errorf("roles validate refused the skeleton with only the members of the parent filled: exit %d\n%s%s", code, out, errText)
	}

	_, _, code = run(ok, "review", "record", "--feature", "own-rdd", "--base", "HEAD~1", "--head", "HEAD", "--rows", "default")
	if got := listNames(t, w.results("own-rdd")); code != 0 || strings.Join(got, ",") != w.head[:12]+"-1.json,"+w.head[:12]+"-2.json" {
		t.Errorf("second record: exit %d, results %q; want 0 and numbers 1 and 2", code, got)
	}
	tampered := reply(t, b, func(o map[string]any) { o["candidate"].(map[string]any)["diff_sha256"] = strings.Repeat("f", 64) })
	out, errText, code = run(tampered, "review", "record", "--feature", "own-rdd", "--base", "HEAD~1", "--head", "HEAD", "--rows", "default")
	if code != 2 || out != "" || !strings.Contains(errText, "candidate echo differs from the binding") {
		t.Errorf("tampered record: exit %d, stdout %q, stderr %q; want 2, nothing and the named reason", code, out, errText)
	}
	if got := listNames(t, w.results("own-rdd")); len(got) != 2 {
		t.Errorf("results = %q after a refused reply, want the two that were there", got)
	}

	// A reviewer finds a blocker and the verifier reproduces it: a reply that is not a verdict is refused and
	// stores nothing, the verdict is stored beside the result, and the skeleton it prints, with the members of the
	// parent filled, is a record the chain accepts.
	third := w.head[:12] + "-3.json"
	if _, errText, code := run(reply(t, b, withBlocker), "review", "record", "--feature", "own-rdd", "--base", "HEAD~1", "--head", "HEAD", "--rows", "default"); code != 0 {
		t.Fatalf("record of a blocker: exit %d, stderr %q; want 0", code, errText)
	}
	out, errText, code = run("The blocker reproduces.", "review", "verify", "--feature", "own-rdd", "--stored", third)
	if code != 2 || out != "" || !strings.Contains(errText, "the reply is refused and nothing was stored") {
		t.Errorf("verify of prose: exit %d, stdout %q, stderr %q; want 2, nothing and the refusal", code, out, errText)
	}
	out, errText, code = run(verdictOn("reproduced"), "review", "verify", "--feature", "own-rdd", "--stored", third)
	if code != 0 || !strings.Contains(errText, "the review blocks") {
		t.Fatalf("verify: exit %d, stderr %q; want 0 and the decision", code, errText)
	}
	if got := listNames(t, w.results("own-rdd")); !slices.Contains(got, w.head[:12]+"-3.verifier.json") || len(got) != 4 {
		t.Errorf("results = %q, want the three results and the verdict beside the third", got)
	}
	if err := json.Unmarshal([]byte(out), &skeleton); err != nil {
		t.Fatalf("stdout is not the skeleton: %v\n%s", err, out)
	}
	filled = skeleton
	filled.ProjectID, filled.GoalID, filled.ChainID, filled.Seq = "p", "g", "c", 1
	filled.PrevSHA256 = strings.Repeat("0", 64)
	filled.Context.Summary = "reviewed, a blocker reproduced"
	if out, errText, code := run("", "roles", "validate", "--file", write(filled)); code != 0 || filled.ToRole != roles.RoleBuilder {
		t.Errorf("roles validate refused the skeleton of a blocked review with only the members of the parent filled (to_role %q): exit %d\n%s%s", filled.ToRole, code, out, errText)
	}
	if _, errText, code := run("", "review", "bind", "--base", "HEAD~1"); code != 1 || !strings.Contains(errText, "--head is required") {
		t.Errorf("bind with no head: exit %d, stderr %q; want 1 and the flag named", code, errText)
	}
}

// A result stored in a directory that could not be flushed is stored: the verb exits 0 with the skeleton, and
// says on stderr that the result may not survive a crash, so that nobody records it a second time.
func TestRecordSaysAStoredResultWasNotFlushedAndStillHandsTheSkeleton(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := -1
	p := process{stdout: &stdout, stderr: &stderr, exit: func(c int) { code = c }}
	recorded := reviewapp.Recorded{Path: "odd/review-results/own-rdd/222222222222-1.json", SHA256: "abc",
		NotFlushed: fmt.Errorf("%w: sync: input/output error", reviewapp.ErrNotFlushed)}

	reportRecorded(p, recorded)

	if code != 0 {
		t.Errorf("exit code = %d, want 0: the result is stored", code)
	}
	if stdout.Len() == 0 {
		t.Errorf("stdout is empty, want the skeleton of the record")
	}
	for _, want := range []string{"stored " + recorded.Path, "could not be flushed", "do not record it again"} {
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("stderr = %q, want it to say %q", stderr.String(), want)
		}
	}
}

// withBlocker puts one BLOCKER in the reply of a reviewer: F1, which the diff introduced.
func withBlocker(o map[string]any) {
	o["findings"] = []any{map[string]any{
		"id": "F1", "location": "a.txt:2", "severity": "BLOCKER", "claim": "the second line breaks the first",
		"evidence_class": "deterministic", "causal_disposition": "introduced", "proof_refs": []any{"a.txt:2"},
	}}
}

// verdictOn is the reply of a verifier that answers F1 with result on head.
func verdictOn(result string) string {
	return `{"verdicts":[{"id":"F1","result":"` + result + `","command":"go test ./...","output":"--- FAIL: TestFirst","base_result":"not-reproduced"}]}`
}

// recordBlocker records the reply of a reviewer who found the BLOCKER F1 in the commits from base to head, and
// returns the binding of the candidate and the name of the stored result.
func (w *reviewWorld) recordBlocker(base, head string) (reviewBinding, string) {
	w.t.Helper()
	stdout, stderr, exits := w.run("", "bind", "--base", base, "--head", head)
	if !equalInts(exits, []int{0}) {
		w.t.Fatalf("setup: review bind exit calls = %v, stderr = %q; want [0]", exits, stderr)
	}
	b := decodeBinding(w.t, stdout)
	_, stderr, exits = w.run(reply(w.t, b, withBlocker), "record", "--feature", "own-rdd", "--base", base, "--head", head, "--rows", "default")
	if !equalInts(exits, []int{0}) {
		w.t.Fatalf("setup: review record exit calls = %v, stderr = %q; want [0]", exits, stderr)
	}
	return b, storedName(w.t, stderr)
}

// storedName is the base name of the file that `review record` says it stored: the line `review record: stored <path>
// (sha256 <sum>)` of its standard error. It is the name wherever a sweep took the number a result would have, which a
// name built from the head and the number 1 is not.
func storedName(t *testing.T, stderr string) string {
	t.Helper()
	for _, line := range strings.Split(stderr, "\n") {
		if rest, ok := strings.CutPrefix(line, "review record: stored "); ok {
			if path, _, found := strings.Cut(rest, " (sha256 "); found {
				return filepath.Base(path)
			}
		}
	}
	t.Fatalf("setup: review record stderr = %q; want a line `review record: stored <path> (sha256 <sum>)`", stderr)
	return ""
}

// verify holds the reply of a verifier to the stored result, stores it beside the result, says what it decided
// and hands the skeleton of the record with the verdict bound as evidence and the builder as the next role.
func TestVerifyStoresTheVerdictBesideTheResultAndPrintsTheDecisionAndTheSkeleton(t *testing.T) {
	w := newReviewWorld(t)
	b, stored := w.recordBlocker(w.base, w.head)
	result, err := os.ReadFile(filepath.Join(w.results("own-rdd"), stored))
	if err != nil {
		t.Fatal(err)
	}
	verdicts := verdictOn("reproduced")

	stdout, stderr, exits := w.run(verdicts, "verify", "--feature", "own-rdd", "--stored", stored)

	if !equalInts(exits, []int{0}) {
		t.Fatalf("exit calls = %v, stderr = %q; want [0]", exits, stderr)
	}
	short := w.head[:12]
	verdictPath := filepath.Join(w.results("own-rdd"), short+"-1.verifier.json")
	file, err := os.ReadFile(verdictPath)
	if err != nil {
		t.Fatalf("the verdict is not stored at %s: %v", verdictPath, err)
	}
	if info, _ := os.Stat(verdictPath); info == nil || info.Mode().Perm() != 0o644 {
		t.Errorf("the stored verdict has mode %v, want 0644", info)
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, file); err != nil || compact.String() != verdicts {
		t.Errorf("stored verdict = %s, want the reply that was sent: %s", compact.String(), verdicts)
	}
	var skeleton roles.RoleHandoff
	if err := json.Unmarshal([]byte(stdout), &skeleton); err != nil {
		t.Fatalf("stdout is not the skeleton of a RoleHandoff: %v\n%s", err, stdout)
	}
	dir := "odd/review-results/own-rdd/"
	wantEvidence := []roles.Evidence{
		{Kind: "review-diff", Ref: w.base + ".." + w.head, SHA256: b.DiffSHA256},
		{Kind: "review-result", Ref: dir + stored, SHA256: sum(result)},
		{Kind: "verifier-report", Ref: dir + short + "-1.verifier.json", SHA256: sum(file)},
	}
	if !reflect.DeepEqual(skeleton.Evidence, wantEvidence) || skeleton.PayloadSHA256 != sum(result) || skeleton.ToRole != roles.RoleBuilder || skeleton.FromRole != roles.RoleReviewer {
		t.Errorf("skeleton = %+v, want the reviewer's review-report of the result, the verdict as evidence %+v, and the builder next", skeleton, wantEvidence)
	}
	for _, want := range []string{"stored " + dir + short + "-1.verifier.json", sum(file), "the review blocks", "blocking: F1", "fills project_id, goal_id, chain_id, seq, prev_sha256, context.summary"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr = %q, want it to say %q", stderr, want)
		}
	}
}

// What the verb says follows the decision: who is blocking, what is left unchecked, what is fixed before delivery
// and what is carried, and the members the parent fills, which include to_role only when nobody is next yet. A
// verdict stored in a directory that could not be flushed is stored all the same.
func TestVerifyReportsTheDecisionAndNamesWhatTheParentFills(t *testing.T) {
	notFlushed := fmt.Errorf("%w: sync: input/output error", reviewapp.ErrNotFlushed)
	for _, tc := range []struct {
		name string
		v    reviewapp.Verified
		want []string
	}{
		{"it blocks", reviewapp.Verified{
			Decision: reviewverdict.Decision{Outcome: reviewverdict.Blocks, Blocking: []string{"F1", "F3"}, Fix: []string{"F2"}, Carry: []string{"F4"}},
			Handoff:  roles.RoleHandoff{ToRole: roles.RoleBuilder},
		}, []string{"the review blocks", "blocking: F1, F3", "fix before delivery: F2", "carry: F4", "fills project_id, goal_id, chain_id, seq, prev_sha256, context.summary"}},
		{"a check did not run", reviewapp.Verified{
			Decision: reviewverdict.Decision{Outcome: reviewverdict.Unverified, Unverified: []string{"F1"}},
		}, []string{"the review is not clear", "unverified: F1", "fills project_id, goal_id, chain_id, seq, prev_sha256, to_role, context.summary"}},
		{"it is clear", reviewapp.Verified{
			Decision: reviewverdict.Decision{Outcome: reviewverdict.Clear},
			Handoff:  roles.RoleHandoff{ToRole: roles.RoleDelivery},
		}, []string{"the review is clear"}},
		{"its directory was not flushed", reviewapp.Verified{
			Decision:   reviewverdict.Decision{Outcome: reviewverdict.Clear},
			NotFlushed: notFlushed,
		}, []string{"could not be flushed", "do not verify it again"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := -1
			p := process{stdout: &stdout, stderr: &stderr, exit: func(c int) { code = c }}
			tc.v.Path, tc.v.SHA256 = "odd/review-results/own-rdd/222222222222-1.verifier.json", "abc"

			reportVerified(p, tc.v)

			if code != 0 || stdout.Len() == 0 {
				t.Errorf("exit code = %d with %d bytes on stdout, want 0 and the skeleton of the record: the verdict is stored", code, stdout.Len())
			}
			for _, want := range append([]string{"stored " + tc.v.Path}, tc.want...) {
				if !strings.Contains(stderr.String(), want) {
					t.Errorf("stderr = %q, want it to say %q", stderr.String(), want)
				}
			}
		})
	}
}

// A reply of the verifier that does not hold to the stored result is exit 2, as a refused reply of the reviewer is,
// and nothing is stored.
func TestVerifyRefusesAReplyThatDoesNotHoldAndStoresNothing(t *testing.T) {
	w := newReviewWorld(t)
	_, stored := w.recordBlocker(w.base, w.head)
	before := listNames(t, w.results("own-rdd"))

	for name, verdicts := range map[string]string{
		"prose":                      "The blocker reproduces.",
		"an id no finding has":       strings.Replace(verdictOn("reproduced"), `"F1"`, `"F9"`, 1),
		"a value outside enum":       verdictOn("maybe"),
		"a verdict without evidence": strings.Replace(verdictOn("reproduced"), `"output":"--- FAIL: TestFirst"`, `"output":""`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			stdout, stderr, exits := w.run(verdicts, "verify", "--feature", "own-rdd", "--stored", stored)

			if stdout != "" || !equalInts(exits, []int{2}) || !strings.HasPrefix(stderr, "error: review verify: the reply is refused and nothing was stored: reviewverdict: ") {
				t.Errorf("stdout = %q, stderr = %q, exits = %v; want nothing, the refusal of the reply by a named rule, [2]", stdout, stderr, exits)
			}
			if got := listNames(t, w.results("own-rdd")); !reflect.DeepEqual(got, before) {
				t.Errorf("results = %q after a refused reply, want %q", got, before)
			}
		})
	}
}

// A candidate is corrected once. The second head of a base whose review was blocked, when it is blocked too, is
// refused with exit 1 and nothing is stored: the owner decides.
func TestVerifyRefusesASecondBlockingVerdictForTheSameBaseAndStoresNothing(t *testing.T) {
	w := newReviewWorld(t)
	_, first := w.recordBlocker(w.base, w.head)
	if _, stderr, exits := w.run(verdictOn("reproduced"), "verify", "--feature", "own-rdd", "--stored", first); !equalInts(exits, []int{0}) {
		t.Fatalf("setup: the first verify exit calls = %v, stderr = %q; want [0]", exits, stderr)
	}
	overlaytest.WriteFile(t, w.root, "a.txt", "one\ntwo\nthree\n")
	w.git("add", "-A")
	w.git("commit", "-q", "-m", "fix")
	fix := w.name("HEAD")
	_, second := w.recordBlocker(w.base, fix)

	stdout, stderr, exits := w.run(verdictOn("reproduced"), "verify", "--feature", "own-rdd", "--stored", second)

	if stdout != "" || !equalInts(exits, []int{1}) || !strings.Contains(stderr, "the correction round of the candidate was spent") || !strings.Contains(stderr, w.head[:12]+"-1.verifier.json") {
		t.Errorf("stdout = %q, stderr = %q, exits = %v; want nothing, an error that names the verdict that spent the round, [1]", stdout, stderr, exits)
	}
	if _, err := os.Stat(filepath.Join(w.results("own-rdd"), fix[:12]+"-1.verifier.json")); err == nil {
		t.Error("the verdict of the second head is stored, want nothing stored")
	}
}

// The reply of the verifier is read from the file --verdicts names, a relative one from the working directory
// of the process, or from stdin when the flag is "-" or absent.
func TestVerifyReadsTheReplyFromTheFileOfTheFlagOrFromStdin(t *testing.T) {
	for _, tc := range []struct {
		name, flag, stdin, file string
	}{
		{"a relative file", "verdicts.json", "this is not the reply", verdictOn("reproduced")},
		{"a dash", "-", verdictOn("reproduced"), "this is not the reply"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newReviewWorld(t)
			_, stored := w.recordBlocker(w.base, w.head)
			overlaytest.WriteFile(t, w.root, "verdicts.json", tc.file)

			_, stderr, exits := w.run(tc.stdin, "verify", "--feature", "own-rdd", "--stored", stored, "--verdicts", tc.flag)

			if !equalInts(exits, []int{0}) {
				t.Fatalf("exit calls = %v, stderr = %q; want [0]", exits, stderr)
			}
			file, err := os.ReadFile(filepath.Join(w.results("own-rdd"), w.head[:12]+"-1.verifier.json"))
			var compact bytes.Buffer
			if err != nil || json.Compact(&compact, file) != nil || compact.String() != verdictOn("reproduced") {
				t.Errorf("stored verdict = %q (%v), want the reply that was read: %s", file, err, verdictOn("reproduced"))
			}
		})
	}
}

// The help of the labdrian wrapper (bin/lib/usage.sh) is shorter than the engine's, and written by hand: it says
// the same signature, in the wrapper's spelling, and the same facts a caller acts on, so the two cannot drift.
func TestWrapperUsageOfReviewVerifySaysWhatTheEngineUsageSays(t *testing.T) {
	wrapper, err := os.ReadFile("../../bin/lib/usage.sh")
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	usage(&out)
	// The wrapper's text wraps at its own width, so its words are compared with the line breaks taken out.
	flat := strings.Join(strings.Fields(string(wrapper)), " ")
	for _, pair := range []struct{ engine, inWrapper string }{
		{"engine review verify --feature <name> --stored <result> [--verdicts <file>|-] [--cwd <repo>]", "verify --feature NAME --stored RESULT [--verdicts FILE|-] [--cwd REPO]"},
		{"<head12>-<n>.verifier.json", "HEAD12-N.verifier.json"},
		{"never over an existing file", "never over a file"},
		{"verifier-report", "verifier-report"},
		{"to_role filled (builder when", "to_role filled (builder when"},
		{"delivery when it is clear", "delivery when it is clear"},
		{"one correction round per candidate", "one correction round per candidate"},
		{"exit 0 stored, 2 reply refused, 1 usage error", "exit 0 stored, 2 reply refused, 1 error"},
		{"engine review check --feature <name> [--head <rev>] [--base <rev> | --onto <rev>] [--cwd <repo>]", "check --feature NAME [--head REV] [--base REV | --onto REV] [--cwd REPO]"},
		{"--onto <rev> names the integration branch of the delivery", "--onto REV names the integration branch of the delivery"},
		{"each range starting where the one before ended", "each range starting where the one before ended"},
		{"at least one result binds the head and every one is clear", "one result binds the head and every one is clear"},
		{"a check that did not run never passes", "a check that did not run never passes"},
		{"exit 0 cleared, 1 usage error, refusal or any other failure", "exit 0 cleared, 1 refused or error"},
	} {
		engine, inWrapper := pair.engine, pair.inWrapper
		if !strings.Contains(out.String(), engine) {
			t.Errorf("the engine usage lacks %q", engine)
		}
		if !strings.Contains(flat, inWrapper) {
			t.Errorf("bin/lib/usage.sh lacks %q, which the engine usage says as %q", inWrapper, engine)
		}
	}
}

func TestUsageDocumentsReviewVerify(t *testing.T) {
	var out bytes.Buffer
	usage(&out)
	text := out.String()

	for _, want := range []string{
		"engine review verify --feature <name> --stored <result> [--verdicts <file>|-] [--cwd <repo>]",
		"<head12>-<n>.verifier.json", "evidence review-diff, review-result and verifier-report",
		"at most one correction round per candidate", "exit 0 stored, 2 reply refused, 1 usage error, a spent correction round or any other failure",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("usage lacks %q", want)
		}
	}
}

// require names the first flag that has no value, and refuses a name it does not know instead of reading it
// as a flag that was left out.
func TestReviewFlagsRequireRefusesANameItDoesNotKnow(t *testing.T) {
	f := reviewFlags{feature: "own-rdd"}
	if err := f.require("--feature", "--stored"); err == nil || err.Error() != "--stored is required" {
		t.Errorf("require(--feature, --stored) error = %v, want \"--stored is required\"", err)
	}
	if err := f.require("--feature", "--sotred"); err == nil || !strings.Contains(err.Error(), "--sotred is not a flag that can be required") {
		t.Errorf("require(--feature, --sotred) error = %v, want one that says --sotred is not a flag that can be required", err)
	}
}
