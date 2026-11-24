package evidencesh_test

// E1: what the adapter of app.Classified answers and refuses (S6, T4w, L84). The real script is the repository's
// skills/_shared/evidence/evidence.sh with evidence-rules.md, copied into a temporary directory and run under sh and awk
// through execrunner; the stubs are files of that directory; a fake Runner hands in what no host makes on demand. Every
// test keeps its HOME and its temporary files in a directory of its own, never the real ones (L54).

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/execrunner"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/evidencesh"
)

var _ app.Classified = evidencesh.Script{}

const (
	kindsHead    = "# Evidence\n\n## Kinds\n\n| Kind | Level | Rule | Applies when | Source | Scope |\n|------|-------|------|--------------|--------|-------|\n"
	evidenceHead = "\n## Evidence\n\n| Kind | Level | Review | Date | Finding |\n|------|-------|--------|------|---------|\n"
)

func page(kinds, evidence string) string { return kindsHead + kinds + evidenceHead + evidence }

// world is a deployed copy of the evidence store, a repository root with no page, and the environment a run gets.
type world struct {
	t       *testing.T
	tmp     string
	root    string
	script  string
	environ []string
}

func newWorld(t *testing.T) world {
	t.Helper()
	overlaytest.RequireShAndAwk(t)
	tmp, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(overlaytest.RepoRoot(t), "skills", "_shared", "evidence")
	for _, name := range []string{"evidence.sh", "evidence-rules.md"} {
		data, err := os.ReadFile(filepath.Join(src, name))
		if err != nil {
			t.Fatal(err)
		}
		overlaytest.WriteFile(t, tmp, filepath.Join("store", name), string(data))
	}
	w := world{t: t, tmp: tmp, root: filepath.Join(tmp, "repo"), script: filepath.Join(tmp, "store", "evidence.sh")}
	for _, dir := range []string{w.root, filepath.Join(tmp, "tmpdir")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	w.environ = []string{"HOME=" + tmp, "PATH=" + os.Getenv("PATH"), "TMPDIR=" + filepath.Join(tmp, "tmpdir")}
	return w
}

func (w world) writePage(content string) string {
	w.t.Helper()
	path := filepath.Join(w.root, "docs", "wiki", "evidence.md")
	overlaytest.WriteFile(w.t, w.root, filepath.Join("docs", "wiki", "evidence.md"), content)
	return path
}

// stub makes a script file holding body and returns its path.
func (w world) stub(name, body string) string {
	w.t.Helper()
	overlaytest.WriteFile(w.t, w.tmp, filepath.Join("stubs", name), body)
	return filepath.Join(w.tmp, "stubs", name)
}

func (w world) adapter(script string, timeout time.Duration) evidencesh.Script {
	return evidencesh.New(execrunner.New(), evidencesh.Options{Script: script, Environ: w.environ, Timeout: timeout})
}

func (w world) classified(reviews ...string) ([]bool, error) {
	return w.adapter(w.script, 0).Classified(w.root, reviews)
}

func wantError(t *testing.T, err error, parts ...string) {
	t.Helper()
	if err == nil {
		t.Fatalf("no error, want one holding %q", parts)
	}
	for _, part := range parts {
		if !strings.Contains(err.Error(), part) {
			t.Errorf("the error is %q, want it to hold %q", err, part)
		}
	}
}

func TestClassified_AnswersEachReviewByItsEvidenceRow(t *testing.T) { // (a)
	w := newWorld(t)
	w.writePage(page("| alpha-kind | 1 | | | | project |\n",
		"| alpha-kind | 1 | f/aaaa-1 | 2026-01-01 | one |\n| alpha-kind | 1 | f/cccc-1 | 2026-01-02 | two |\n"))

	got, err := w.classified("f/aaaa-1", "f/bbbb-1", "f/cccc-1")

	if err != nil || !reflect.DeepEqual(got, []bool{true, false, true}) {
		t.Errorf("Classified = %v, %v; want [true false true] and no error", got, err)
	}
}

func TestClassified_AnswersNoWhenThereIsNoPageOrThePageFailsItsCheck(t *testing.T) { // (b), (c)
	t.Run("no docs/wiki under the root", func(t *testing.T) {
		w := newWorld(t)
		got, err := w.classified("f/aaaa-1", "f/bbbb-1")
		if err != nil || !reflect.DeepEqual(got, []bool{false, false}) {
			t.Errorf("Classified = %v, %v; want [false false] and no error", got, err)
		}
	})
	t.Run("a row whose kind has no Kinds row", func(t *testing.T) {
		w := newWorld(t)
		w.writePage(page("| alpha-kind | 1 | | | | project |\n", "| other-kind | 1 | f/aaaa-1 | 2026-01-01 | x |\n"))
		got, err := w.classified("f/aaaa-1")
		if err != nil || !reflect.DeepEqual(got, []bool{false}) {
			t.Errorf("Classified = %v, %v; want [false] and no error", got, err)
		}
	})
}

// fake is a Runner that records its call and answers as told.
type fake struct {
	calls    int
	env      []string
	bin      string
	args     []string
	deadline time.Time
	has      bool
	stdout   string
	stderr   string
	err      error
}

func (f *fake) Output(ctx context.Context, env []string, bin string, args ...string) ([]byte, []byte, error) {
	f.calls++
	f.env, f.bin, f.args = env, bin, args
	f.deadline, f.has = ctx.Deadline()
	return []byte(f.stdout), []byte(f.stderr), f.err
}

// presentScript is the absolute path of a file that exists, for a test whose Runner is a fake: the adapter asks that
// something is at the script's path before it runs it, and a fake needs no real script.
func presentScript(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "evidence.sh")
	if err := os.WriteFile(path, []byte("exit 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// countingRunner is a Runner that only counts its calls.
type countingRunner struct{ calls int }

func (c *countingRunner) Output(context.Context, []string, string, ...string) ([]byte, []byte, error) {
	c.calls++
	return nil, nil, nil
}

func TestClassified_NamesAnEvidenceStoreThatIsNotDeployed(t *testing.T) { // (h), (u)
	missing := filepath.Join(t.TempDir(), "nowhere", "evidence.sh")
	t.Run("a script path with nothing at it is named, not run (h)", func(t *testing.T) {
		w := newWorld(t)
		_, err := w.adapter(missing, 0).Classified(w.root, []string{"f/aaaa-1"})
		wantError(t, err, "the evidence store is not deployed", missing, "labdrian apply")
		if strings.Contains(err.Error(), "exited") {
			t.Errorf("the error %q reads as an exit of the script", err)
		}
		if !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("the error %v does not wrap fs.ErrNotExist", err)
		}
	})
	t.Run("the runner is not called (u)", func(t *testing.T) {
		c := &countingRunner{}
		got, err := evidencesh.New(c, evidencesh.Options{Script: missing}).Classified("/r", []string{"f/aaaa-1"})
		if err == nil || got != nil || c.calls != 0 {
			t.Errorf("Classified = %v, %v after %d calls; want no answer, an error and no call", got, err, c.calls)
		}
	})
}

func TestClassified_RunsNothingForNoReview(t *testing.T) { // (d)
	for name, reviews := range map[string][]string{"nil": nil, "empty": {}} {
		t.Run(name, func(t *testing.T) {
			f := &fake{}
			got, err := evidencesh.New(f, evidencesh.Options{Script: "/x/evidence.sh"}).Classified("/r", reviews)
			if err != nil || got == nil || len(got) != 0 || f.calls != 0 {
				t.Errorf("Classified = %#v, %v after %d calls; want an empty non-nil slice, no error and no call", got, err, f.calls)
			}
		})
	}
}

func TestClassified_RefusesWhatTheScriptCannotAnswer(t *testing.T) { // (e)-(i)
	t.Run("no awk on PATH (e)", func(t *testing.T) {
		w := newWorld(t)
		empty := filepath.Join(w.tmp, "empty")
		if err := os.MkdirAll(empty, 0o755); err != nil {
			t.Fatal(err)
		}
		w.environ = []string{"HOME=" + w.tmp, "PATH=" + empty}
		_, err := w.classified("f/aaaa-1")
		wantError(t, err, "exited 3", "awk is not on PATH")
	})
	t.Run("a page nobody can read (f)", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root reads a page of mode 000")
		}
		w := newWorld(t)
		path := w.writePage(page("| alpha-kind | 1 | | | | project |\n", "| alpha-kind | 1 | f/aaaa-1 | 2026-01-01 | x |\n"))
		if err := os.Chmod(path, 0); err != nil {
			t.Fatal(err)
		}
		_, err := w.classified("f/aaaa-1")
		wantError(t, err, "exited 4", "cannot read")
	})
	t.Run("a directory at the page path (g)", func(t *testing.T) {
		w := newWorld(t)
		if err := os.MkdirAll(filepath.Join(w.root, "docs", "wiki", "evidence.md"), 0o755); err != nil {
			t.Fatal(err)
		}
		_, err := w.classified("f/aaaa-1")
		wantError(t, err, "exited 4", "cannot read")
	})
	t.Run("an identifier the script refuses (i)", func(t *testing.T) {
		w := newWorld(t)
		_, err := w.classified("f/aaaa 1")
		wantError(t, err, "exited 2", "usage:")
	})
}

func TestClassified_RefusesAnAnswerThatIsNotOneLineForEachReview(t *testing.T) { // (j)-(n), (s)
	t.Run("past the timeout (j)", func(t *testing.T) {
		w := newWorld(t)
		start := time.Now()
		_, err := w.adapter(w.stub("slow.sh", "exec sleep 5\n"), 100*time.Millisecond).Classified(w.root, []string{"f/aaaa-1"})
		wantError(t, err, "gave no answer within 100ms")
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("the error %v does not wrap context.DeadlineExceeded", err)
		}
		if took := time.Since(start); took > 1500*time.Millisecond {
			t.Errorf("the call took %v, want it back within 1.5s of a 100ms timeout", took)
		}
	})
	for _, tc := range []struct{ name, body string }{
		{"too few lines (k)", "printf 'yes\\n'\n"},
		{"another word (l)", "printf 'yes\\nmaybe\\n'\n"},
		{"no final line break (m)", "printf 'yes\\nno'\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newWorld(t)
			_, err := w.adapter(w.stub("lines.sh", tc.body), 0).Classified(w.root, []string{"f/aaaa-1", "f/bbbb-1"})
			wantError(t, err, "without one line yes or no for each of the 2 results")
			if strings.Contains(err.Error(), "maybe") {
				t.Errorf("the error %q quotes the output of the script", err)
			}
		})
	}
	t.Run("an exit of its own and a first line of stderr (n)", func(t *testing.T) {
		w := newWorld(t)
		_, err := w.adapter(w.stub("boom.sh", "printf 'boom\\nsecond\\n' >&2\nexit 1\n"), 0).Classified(w.root, []string{"f/aaaa-1"})
		wantError(t, err, "exited 1: boom")
		if strings.Contains(err.Error(), "second") {
			t.Errorf("the error %q holds more than the first line of stderr", err)
		}
	})
	t.Run("killed by a signal (s)", func(t *testing.T) {
		w := newWorld(t)
		_, err := w.adapter(w.stub("kill.sh", "kill -9 $$\n"), 0).Classified(w.root, []string{"f/aaaa-1"})
		wantError(t, err, "stopped by a signal")
	})
}

func TestClassified_RefusesWhatItMustNotRun(t *testing.T) { // (o), (p)
	t.Run("a sh that cannot start (o)", func(t *testing.T) {
		f := &fake{err: &os.PathError{Op: "fork/exec", Path: "/bin/sh", Err: syscall.E2BIG}}
		_, err := evidencesh.New(f, evidencesh.Options{Script: presentScript(t)}).Classified("/r", []string{"f/aaaa-1"})
		wantError(t, err, "could not run")
		if !errors.Is(err, syscall.E2BIG) {
			t.Errorf("the error %v does not wrap syscall.E2BIG", err)
		}
	})
	t.Run("an exit that is not an ExitError is not an answer", func(t *testing.T) {
		f := &fake{err: errors.New("boom")}
		got, err := evidencesh.New(f, evidencesh.Options{Script: presentScript(t)}).Classified("/r", []string{"f/aaaa-1"})
		if err == nil || got != nil {
			t.Errorf("Classified = %v, %v; want no answer and an error", got, err)
		}
	})
	t.Run("a script path that is not absolute (p)", func(t *testing.T) {
		f := &fake{}
		_, err := evidencesh.New(f, evidencesh.Options{Script: ".claude/skills/evidence.sh"}).Classified("/r", []string{"f/aaaa-1"})
		wantError(t, err, `the evidence script ".claude/skills/evidence.sh" is not an absolute path`)
		if f.calls != 0 {
			t.Errorf("the runner was called %d times, want none", f.calls)
		}
	})
	t.Run("an exit 126 of sh is not an answer", func(t *testing.T) {
		var exit *exec.ExitError
		err := exec.Command("sh", "-c", "exit 126").Run()
		if !errors.As(err, &exit) {
			t.Skip("sh gave no exit error")
		}
		f := &fake{err: err}
		_, got := evidencesh.New(f, evidencesh.Options{Script: presentScript(t)}).Classified("/r", []string{"f/aaaa-1"})
		wantError(t, got, "exited 126")
	})
}

func TestClassified_StartsShWithTheScriptTheModeTheWikiAndTheReviews(t *testing.T) { // (q), (r)
	env := []string{"A=1", "B=2"}
	f := &fake{stdout: "no\nyes\n"}
	script := presentScript(t)
	s := evidencesh.New(f, evidencesh.Options{Script: script, Environ: env, Timeout: 700 * time.Millisecond})
	env[0] = "A=changed"

	before := time.Now()
	got, err := s.Classified("/r", []string{"f/aaaa-1", "g/bbbb-2"})
	after := time.Now()

	if err != nil || !reflect.DeepEqual(got, []bool{false, true}) {
		t.Fatalf("Classified = %v, %v; want [false true] and no error", got, err)
	}
	if f.calls != 1 || f.bin != "sh" {
		t.Errorf("%d calls of %q, want one of sh", f.calls, f.bin)
	}
	if want := []string{script, "reviewed-each", "/r/docs/wiki", "f/aaaa-1", "g/bbbb-2"}; !reflect.DeepEqual(f.args, want) {
		t.Errorf("args = %q, want %q", f.args, want)
	}
	if want := []string{"A=1", "B=2"}; !reflect.DeepEqual(f.env, want) {
		t.Errorf("env = %q, want the options' environment as given: %q", f.env, want)
	}
	if !f.has || f.deadline.Before(before.Add(700*time.Millisecond)) || f.deadline.After(after.Add(700*time.Millisecond)) {
		t.Errorf("deadline = %v (set %v), want 700ms after the call", f.deadline, f.has)
	}
}

func TestClassified_UsesTheDefaultTimeoutWhenNoneIsGiven(t *testing.T) { // (r)
	if evidencesh.DefaultTimeout != 2*time.Second {
		t.Errorf("DefaultTimeout = %v, want 2s", evidencesh.DefaultTimeout)
	}
	for _, timeout := range []time.Duration{0, -time.Second} {
		f := &fake{stdout: "yes\n"}
		before := time.Now()
		_, err := evidencesh.New(f, evidencesh.Options{Script: presentScript(t), Timeout: timeout}).Classified("/r", []string{"f/aaaa-1"})
		after := time.Now()
		if err != nil {
			t.Fatal(err)
		}
		if !f.has || f.deadline.Before(before.Add(evidencesh.DefaultTimeout)) || f.deadline.After(after.Add(evidencesh.DefaultTimeout)) {
			t.Errorf("timeout %v: deadline = %v (set %v), want DefaultTimeout after the call", timeout, f.deadline, f.has)
		}
	}
}
