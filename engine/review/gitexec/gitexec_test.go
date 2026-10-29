package gitexec_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/gitexec"
)

// The adapter is the Git port of the use cases.
var _ app.Git = gitexec.Git{}

const (
	baseSHA  = "1111111111111111111111111111111111111111"
	headSHA  = "2222222222222222222222222222222222222222"
	otherSHA = "3333333333333333333333333333333333333333"
	root     = "/work/repo"
)

var repo = app.Repository{Root: root, Main: root, CommonDir: root + "/.git"}

// call is one start of a program as the fake runner saw it.
type call struct {
	env      []string
	bin      string
	args     []string
	deadline time.Time
	hasLimit bool
}

// fakeRunner answers every start with the same bytes and records it.
type fakeRunner struct {
	stdout, stderr []byte
	err            error
	calls          []call
}

func (f *fakeRunner) Output(ctx context.Context, env []string, bin string, args ...string) ([]byte, []byte, error) {
	deadline, ok := ctx.Deadline()
	f.calls = append(f.calls, call{env: env, bin: bin, args: args, deadline: deadline, hasLimit: ok})
	return f.stdout, f.stderr, f.err
}

// OutputStdin answers as Output does and records the start; the input is not kept.
func (f *fakeRunner) OutputStdin(ctx context.Context, env []string, _ []byte, bin string, args ...string) ([]byte, []byte, error) {
	return f.Output(ctx, env, bin, args...)
}

func (f *fakeRunner) only(t *testing.T) call {
	t.Helper()
	if len(f.calls) != 1 {
		t.Fatalf("git was started %d times, want once: %v", len(f.calls), f.calls)
	}
	return f.calls[0]
}

func newGit(r *fakeRunner, environ ...string) gitexec.Git {
	return gitexec.New(r, gitexec.Options{Environ: environ})
}

// The canonical diff is a definition, not a habit: the exact command, spelled out in the README of
// odd/review-results, whose every part is there because something in the repository or on the machine could
// otherwise change the bytes. The user's configuration cannot, nor the commit that is checked out.
func TestDiffArgsIsTheCanonicalDiff(t *testing.T) {
	want := []string{
		"-c", "core.quotePath=true",
		"-c", "diff.suppressBlankEmpty=false",
		"-c", "core.attributesFile=" + os.DevNull,
		"-c", "core.compression=0",
		"-c", "core.looseCompression=0",
		"--attr-source=4b825dc642cb6eb9a060e54bf8d69288fbee4904",
		"diff-tree", "-p", "-r", "--full-index", "--binary", "--no-renames", "--no-color", "--no-ext-diff", "--no-textconv",
		"--ignore-submodules=none", "--src-prefix=a/", "--dst-prefix=b/", "-U3",
		baseSHA, headSHA,
	}
	if got := gitexec.DiffArgs(baseSHA, headSHA); !reflect.DeepEqual(got, want) {
		t.Errorf("DiffArgs() = %q, want %q", got, want)
	}
}

// The attributes of a diff come from the empty tree and not from a commit, so that no .gitattributes of the
// candidate can name a driver whose configuration is the user's. The empty tree is not the same object in
// a SHA-256 repository, which the length of the names says.
func TestDiffArgsTakesTheAttributesFromTheEmptyTreeOfTheHashFunctionOfTheCommits(t *testing.T) {
	sha256Base, sha256Head := strings.Repeat("a", 64), strings.Repeat("b", 64)
	for _, tc := range []struct{ name, base, head, want string }{
		{"SHA-1", baseSHA, headSHA, "--attr-source=4b825dc642cb6eb9a060e54bf8d69288fbee4904"},
		{"SHA-256", sha256Base, sha256Head, "--attr-source=6ef19b41225c5369f1c104d45d8d85efa9b057b53b14b4b9b939dd74decc5321"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := gitexec.DiffArgs(tc.base, tc.head)

			found := 0
			for _, a := range args {
				if strings.HasPrefix(a, "--attr-source=") {
					found++
					if a != tc.want {
						t.Errorf("attribute source = %q, want %q", a, tc.want)
					}
				}
			}
			if found != 1 {
				t.Errorf("DiffArgs() has %d --attr-source options, want exactly one: %q", found, args)
			}
			if n := len(args); n < 2 || args[n-2] != tc.base || args[n-1] != tc.head {
				t.Errorf("DiffArgs() = %q, want it to end with the base and the head", args)
			}
		})
	}
}

// The two empty trees are the ones git knows, which the tests that use a real git check.
func TestTheEmptyTreeConstantsAreTheNamesTheDiffUses(t *testing.T) {
	if gitexec.EmptyTreeSHA1 != "4b825dc642cb6eb9a060e54bf8d69288fbee4904" {
		t.Errorf("EmptyTreeSHA1 = %q", gitexec.EmptyTreeSHA1)
	}
	if gitexec.EmptyTreeSHA256 != "6ef19b41225c5369f1c104d45d8d85efa9b057b53b14b4b9b939dd74decc5321" {
		t.Errorf("EmptyTreeSHA256 = %q", gitexec.EmptyTreeSHA256)
	}
}

func TestPathsArgsListsTheNamesOfTheSameDiffWithNULTerminators(t *testing.T) {
	want := []string{"diff-tree", "-r", "--name-only", "-z", "--no-renames", "--ignore-submodules=none", baseSHA, headSHA}
	if got := gitexec.PathsArgs(baseSHA, headSHA); !reflect.DeepEqual(got, want) {
		t.Errorf("PathsArgs() = %q, want %q", got, want)
	}
}

// Diff starts git in the root of the working tree with exactly the arguments of DiffArgs, and returns
// what git printed to the byte: the digest is of those bytes, so not a character is trimmed or added.
func TestDiffStartsGitInTheRootAndReturnsItsBytesUntouched(t *testing.T) {
	printed := "diff --git a/a b/a\n+x\n\n"
	r := &fakeRunner{stdout: []byte(printed)}

	got, err := newGit(r).Diff(repo, baseSHA, headSHA)
	if err != nil {
		t.Fatalf("Diff() error = %v, want nil", err)
	}

	if string(got) != printed {
		t.Errorf("Diff() = %q, want %q", got, printed)
	}
	c := r.only(t)
	if want := append([]string{"-C", root}, gitexec.DiffArgs(baseSHA, headSHA)...); c.bin != "git" || !reflect.DeepEqual(c.args, want) {
		t.Errorf("started %q %q, want git %q", c.bin, c.args, want)
	}
}

// Paths asks for the names of the same diff, and reads them: NUL-terminated, so that a name with a space, a
// quote, a newline or a byte that is not text comes back as git holds it.
func TestPathsReadsTheNULTerminatedNames(t *testing.T) {
	for _, tc := range []struct {
		name   string
		stdout string
		want   []string
	}{
		{"none", "", []string{}},
		{"one", "a.go\x00", []string{"a.go"}},
		{"in the order git printed them", "b.go\x00a.go\x00dir/c.go\x00", []string{"b.go", "a.go", "dir/c.go"}},
		{"awkward names", "with space.txt\x00new\nline.txt\x00caf\xc3\xa9.txt\x00\"quoted\".txt\x00bad\xffbyte.txt\x00", []string{"with space.txt", "new\nline.txt", "café.txt", `"quoted".txt`, "bad\xffbyte.txt"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &fakeRunner{stdout: []byte(tc.stdout)}

			got, err := newGit(r).Paths(repo, baseSHA, headSHA)
			if err != nil {
				t.Fatalf("Paths() error = %v, want nil", err)
			}

			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Paths() = %q, want %q", got, tc.want)
			}
			if want := append([]string{"-C", root}, gitexec.PathsArgs(baseSHA, headSHA)...); !reflect.DeepEqual(r.only(t).args, want) {
				t.Errorf("started git with %q, want %q", r.only(t).args, want)
			}
		})
	}
}

// Output that is not whole is not a list: a name with no terminator was cut, and an empty name is none.
func TestPathsRefusesOutputThatIsNotWhole(t *testing.T) {
	for _, tc := range []struct{ name, stdout string }{
		{"a name that was cut", "a.go\x00b.g"},
		{"an empty name", "a.go\x00\x00"},
		{"only a terminator", "\x00"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := newGit(&fakeRunner{stdout: []byte(tc.stdout)}).Paths(repo, baseSHA, headSHA)
			if err == nil {
				t.Errorf("Paths() = %q, nil; want an error", got)
			}
		})
	}
}

// Commit resolves what a person typed to the commit it names, peeling a tag, and takes nothing but a full name
// from git.
func TestCommitAsksForOneCommitAndAcceptsOnlyAFullName(t *testing.T) {
	r := &fakeRunner{stdout: []byte(headSHA + "\n")}

	got, err := newGit(r).Commit(repo, "HEAD~1")
	if err != nil {
		t.Fatalf("Commit() error = %v, want nil", err)
	}

	if got != headSHA {
		t.Errorf("Commit() = %q, want %q", got, headSHA)
	}
	want := []string{"-C", root, "rev-parse", "--verify", "--end-of-options", "HEAD~1^{commit}"}
	if c := r.only(t); c.bin != "git" || !reflect.DeepEqual(c.args, want) {
		t.Errorf("started %q %q, want git %q", c.bin, c.args, want)
	}
}

func TestCommitRefusesAnAnswerThatIsNotOneFullName(t *testing.T) {
	for _, tc := range []struct{ name, stdout string }{
		{"nothing", ""},
		{"an abbreviation", "2222222\n"},
		{"two lines", headSHA + "\n" + baseSHA + "\n"},
		{"upper case", strings.ToUpper("abcdef") + strings.Repeat("A", 34) + "\n"},
		{"text", "fatal\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := newGit(&fakeRunner{stdout: []byte(tc.stdout)}).Commit(repo, "HEAD")
			if err == nil {
				t.Errorf("Commit() = %q, nil; want an error", got)
			}
		})
	}
}

// Parent asks git for the parents of one commit, given by its full name after --end-of-options, and reads the first one:
// git prints the commit and then its parents on one line, and a commit that has none is a line of the commit alone.
func TestParentAsksForTheParentsOfOneCommitAndReadsTheFirst(t *testing.T) {
	for _, tc := range []struct{ name, stdout, want string }{
		{"one parent", headSHA + " " + baseSHA + "\n", baseSHA},
		{"a merge, whose first parent is the one it was made on", headSHA + " " + baseSHA + " " + otherSHA + "\n", baseSHA},
		{"no parent", headSHA + "\n", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &fakeRunner{stdout: []byte(tc.stdout)}

			got, err := newGit(r).Parent(repo, headSHA)

			if err != nil || got != tc.want {
				t.Errorf("Parent() = %q, %v; want %q and no error", got, err, tc.want)
			}
			want := []string{"-C", root, "rev-list", "--parents", "-n", "1", "--end-of-options", headSHA}
			if c := r.only(t); c.bin != "git" || !reflect.DeepEqual(c.args, want) {
				t.Errorf("started %q %q, want git %q", c.bin, c.args, want)
			}
		})
	}
}

func TestParentRefusesAnAnswerThatIsNotTheCommitAndItsParents(t *testing.T) {
	for _, tc := range []struct{ name, stdout string }{
		{"nothing", ""},
		{"another commit", baseSHA + " " + otherSHA + "\n"},
		{"a parent that is an abbreviation", headSHA + " 1111111\n"},
		{"two lines", headSHA + " " + baseSHA + "\n" + headSHA + "\n"},
		{"text", "fatal\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := newGit(&fakeRunner{stdout: []byte(tc.stdout)}).Parent(repo, headSHA)
			if err == nil {
				t.Errorf("Parent() = %q, nil; want an error", got)
			}
		})
	}
}

func TestAddWorktreeMakesADetachedWorktreeWithNoHooks(t *testing.T) {
	r := &fakeRunner{}

	err := newGit(r).AddWorktree(repo, "/work/repo-worktrees/review-222222222222", headSHA)
	if err != nil {
		t.Fatalf("AddWorktree() error = %v, want nil", err)
	}

	want := []string{"-C", root, "-c", "core.hooksPath=" + os.DevNull, "worktree", "add", "--detach", "/work/repo-worktrees/review-222222222222", headSHA}
	if c := r.only(t); c.bin != "git" || !reflect.DeepEqual(c.args, want) {
		t.Errorf("started %q %q, want git %q", c.bin, c.args, want)
	}
}

// Git sees the environment of the process without a single GIT_ variable, which would steer it to another
// repository, another configuration or another diff, with the two it is given in their place: the system
// attributes are not read, and the messages are not translated.
func TestEveryGitIsStartedUnderTheEnvironmentOfTheProcessWithoutItsGITVariables(t *testing.T) {
	environ := []string{
		"HOME=/home/someone", "PATH=/usr/bin", "LANG=de_DE.UTF-8", "LC_ALL=de_DE.UTF-8", "git_lowercase=kept",
		"GIT_DIR=/elsewhere/.git", "GIT_WORK_TREE=/elsewhere", "GIT_INDEX_FILE=/elsewhere/index",
		"GIT_DIFF_OPTS=-u9", "GIT_EXTERNAL_DIFF=/bin/false", "GIT_CONFIG_GLOBAL=/elsewhere/config", "GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=diff.noprefix", "GIT_CONFIG_VALUE_0=true", "GIT_ATTR_NOSYSTEM=0",
	}
	want := []string{
		"HOME=/home/someone", "PATH=/usr/bin", "LANG=de_DE.UTF-8", "git_lowercase=kept",
		"GIT_ATTR_NOSYSTEM=1", "LC_ALL=C",
	}
	for name, run := range map[string]func(g gitexec.Git){
		"Commit":      func(g gitexec.Git) { _, _ = g.Commit(repo, "HEAD") },
		"Diff":        func(g gitexec.Git) { _, _ = g.Diff(repo, baseSHA, headSHA) },
		"Paths":       func(g gitexec.Git) { _, _ = g.Paths(repo, baseSHA, headSHA) },
		"AddWorktree": func(g gitexec.Git) { _ = g.AddWorktree(repo, "/w/review-x", headSHA) },
	} {
		t.Run(name, func(t *testing.T) {
			r := &fakeRunner{stdout: []byte(headSHA + "\n")}
			run(newGit(r, environ...))

			c := r.only(t)
			if c.env == nil {
				t.Fatal("git was started with a nil environment, which the runner reads as the environment of the process")
			}
			if !reflect.DeepEqual(c.env, want) {
				t.Errorf("environment = %q, want %q", c.env, want)
			}
		})
	}
}

// Even with no environment at all, git is started with an empty one and not the process's.
func TestGitIsStartedWithAnEmptyEnvironmentAndNotTheProcessesWhenThereIsNone(t *testing.T) {
	r := &fakeRunner{stdout: []byte(headSHA + "\n")}
	_, _ = newGit(r).Commit(repo, "HEAD")

	c := r.only(t)
	if c.env == nil {
		t.Fatal("environment is nil, want the adapter's own two variables")
	}
	if want := []string{"GIT_ATTR_NOSYSTEM=1", "LC_ALL=C"}; !reflect.DeepEqual(c.env, want) {
		t.Errorf("environment = %q, want %q", c.env, want)
	}
}

// The questions Locate and Worktree put go through gitprov, which scrubs the environment itself and hands the
// runner the result: that is the environment those starts of git have, and no variable of git of the process
// (a configuration, an external diff, a pager) reaches them either, while the rest of the environment does.
func TestTheStartsOfGitThatLocateAndWorktreeMakeCarryNoGITVariableEither(t *testing.T) {
	environ := []string{
		"HOME=/home/someone", "PATH=/usr/bin",
		"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=core.fsmonitor", "GIT_CONFIG_VALUE_0=/bin/false",
		"GIT_EXTERNAL_DIFF=/bin/false", "GIT_PAGER=cat",
	}
	dir := t.TempDir()
	for name, run := range map[string]func(g gitexec.Git){
		"Locate":   func(g gitexec.Git) { _, _ = g.Locate(dir) },
		"Worktree": func(g gitexec.Git) { _, _ = g.Worktree(repo, dir) },
	} {
		t.Run(name, func(t *testing.T) {
			r := &fakeRunner{stdout: []byte(dir + "\n")}
			run(newGit(r, environ...))

			if len(r.calls) == 0 {
				t.Fatal("git was never started")
			}
			for _, c := range r.calls {
				if c.env == nil {
					t.Errorf("git %q was started with a nil environment, which the runner reads as the environment of the process", c.args)
				}
				have := map[string]bool{}
				for _, kv := range c.env {
					have[kv] = true
					if strings.HasPrefix(kv, "GIT_") {
						t.Errorf("git %q was started with %q, want no GIT_ variable", c.args, kv)
					}
				}
				for _, kv := range []string{"HOME=/home/someone", "PATH=/usr/bin"} {
					if !have[kv] {
						t.Errorf("git %q was started without %q, want the rest of the environment kept", c.args, kv)
					}
				}
			}
		})
	}
}

// What is at a path is told from what git answered. A git that gave no answer (it could not be started, it was
// killed, or it ran out of time) and a question that could not even be put to it say nothing about the path,
// and that is an error, not a path that is taken: a person told that a worktree "is taken" when git timed
// out would move away the very directory they need.
func TestWorktreeReportsAGitThatGaveNoAnswerAsAnErrorAndNotAsATakenPath(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name    string
		runner  *fakeRunner
		environ []string
		path    string
		is      error  // what the error must wrap, when it is a value
		says    string // what the error must say, when it is text
		asked   bool   // whether git was started
	}{
		{"the time ran out", &fakeRunner{err: fmt.Errorf("%w (%v)", context.DeadlineExceeded, "signal: killed")}, nil, dir, context.DeadlineExceeded, "", true},
		{"git could not be started", &fakeRunner{err: &exec.Error{Name: "git", Err: exec.ErrNotFound}}, nil, dir, exec.ErrNotFound, "", true},
		{"git was killed by a signal", &fakeRunner{err: &exec.ExitError{}, stderr: []byte("Segmentation fault")}, nil, dir, nil, "Segmentation fault", true},
		{"the path is not absolute", &fakeRunner{}, nil, ".", nil, "is not an absolute path", false},
		{"the environment redirects git", &fakeRunner{}, []string{"GIT_DIR=/elsewhere/.git"}, dir, nil, "GIT_DIR", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := newGit(tc.runner, tc.environ...).Worktree(repo, tc.path)

			if err == nil {
				t.Fatalf("Worktree(%s) = %+v, nil; want an error", tc.path, got)
			}
			if got != (app.WorktreeState{}) {
				t.Errorf("Worktree(%s) = %+v with an error, want the zero state", tc.path, got)
			}
			if tc.is != nil && !errors.Is(err, tc.is) {
				t.Errorf("Worktree(%s) error = %v, want one that wraps %v", tc.path, err, tc.is)
			}
			if !strings.Contains(err.Error(), tc.says) {
				t.Errorf("Worktree(%s) error = %q, want it to say %q", tc.path, err, tc.says)
			}
			if started := len(tc.runner.calls) > 0; started != tc.asked {
				t.Errorf("git was started = %v, want %v", started, tc.asked)
			}
		})
	}
}

// lateRunner is a Runner that returns only when the time it was given is up, and then reports the exit status
// of a process: what a Runner that stops a process at its deadline may hand back as it is.
type lateRunner struct{ err error }

func (l lateRunner) Output(ctx context.Context, env []string, bin string, args ...string) ([]byte, []byte, error) {
	<-ctx.Done()
	return nil, nil, l.err
}

func (l lateRunner) OutputStdin(ctx context.Context, env []string, _ []byte, bin string, args ...string) ([]byte, []byte, error) {
	return l.Output(ctx, env, bin, args...)
}

// exitStatus is the error of a real process that ended with a status of 2: the test program, run with a flag
// it does not know.
func exitStatus(t *testing.T) error {
	t.Helper()
	err := exec.Command(os.Args[0], "-no-such-flag").Run()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 2 {
		t.Fatalf("setup: the test program run with an unknown flag = %v, want an exit status of 2", err)
	}
	return err
}

// A git that ended with an exit status after its time was up did not answer in time: what it said before it
// was stopped is not an answer. The adapter judges by the clock too, and not only by the kind of the error.
func TestWorktreeIsAnErrorWhenGitEndedWithAnExitStatusAfterItsTimeWasUp(t *testing.T) {
	status := exitStatus(t)
	g := gitexec.New(lateRunner{err: status}, gitexec.Options{Timeout: time.Nanosecond})

	got, err := g.Worktree(repo, t.TempDir())

	var exit *exec.ExitError
	if err == nil || got != (app.WorktreeState{}) || !errors.As(err, &exit) {
		t.Errorf("Worktree() = %+v, %v; want the zero state and an error that holds the exit status git ended with", got, err)
	}
}

// An answer of git that says the directory is not a working tree of the repository is an answer: the path is
// taken, and no error is made of it. Here git answers a directory with what a bare repository gives.
func TestWorktreeCountsAsTakenWhatGitAnsweredIsNotAWorkingTreeOfTheRepository(t *testing.T) {
	dir := t.TempDir()
	r := &fakeRunner{stdout: []byte(dir + "\n")}

	got, err := newGit(r).Worktree(repo, dir)

	if err != nil || got != (app.WorktreeState{Present: true}) {
		t.Errorf("Worktree(%s) = %+v, %v; want a path that is there and is not a worktree of the repository, no error", dir, got, err)
	}
	if len(r.calls) == 0 {
		t.Error("git was never started, want the answer to come from it")
	}
}

// What git said is part of the error: a person told "exit status 128" cannot do anything with it.
func TestAFailureOfGitCarriesWhatGitSaid(t *testing.T) {
	boom := errors.New("exit status 128")
	r := &fakeRunner{stderr: []byte("fatal: bad object 1111\n"), err: boom}

	_, err := newGit(r).Diff(repo, baseSHA, headSHA)

	if !errors.Is(err, boom) {
		t.Errorf("Diff() error = %v, want one that wraps %v", err, boom)
	}
	for _, want := range []string{"diff-tree", "exit status 128", "fatal: bad object 1111"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Diff() error = %v, want it to say %q", err, want)
		}
	}
}

// A git that hangs is stopped: every call has the deadline the options give, and there is no way to ask for
// none. Options that give no limit (zero, or a duration that is not positive) get the default, a deadline of
// minutes: a git that never answers must not hang the command that waits for it.
func TestEveryGitCallHasADeadlineAndTheOptionsThatGiveNoneGetTheDefault(t *testing.T) {
	if gitexec.DefaultTimeout < time.Minute || gitexec.DefaultTimeout > time.Hour {
		t.Fatalf("DefaultTimeout = %v, want a deadline of minutes", gitexec.DefaultTimeout)
	}
	for _, tc := range []struct {
		name    string
		timeout time.Duration
		want    time.Duration
	}{
		{"a timeout", 90 * time.Second, 90 * time.Second},
		{"no timeout given", 0, gitexec.DefaultTimeout},
		{"a negative timeout", -time.Second, gitexec.DefaultTimeout},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &fakeRunner{stdout: []byte(headSHA + "\n")}
			before := time.Now()

			_, _ = gitexec.New(r, gitexec.Options{Timeout: tc.timeout}).Commit(repo, "HEAD")

			c := r.only(t)
			if !c.hasLimit {
				t.Fatal("the context has no deadline, want every start of git to have one")
			}
			if left := c.deadline.Sub(before); left < tc.want || left > tc.want+5*time.Second {
				t.Errorf("deadline is %v after the call began, want %v", left, tc.want)
			}
		})
	}
}

// The questions Locate and Worktree put go through gitprov and have the deadline too.
func TestTheStartsOfGitThatLocateAndWorktreeMakeHaveTheDefaultDeadlineToo(t *testing.T) {
	dir := t.TempDir()
	for name, run := range map[string]func(g gitexec.Git){
		"Locate":   func(g gitexec.Git) { _, _ = g.Locate(dir) },
		"Worktree": func(g gitexec.Git) { _, _ = g.Worktree(repo, dir) },
	} {
		t.Run(name, func(t *testing.T) {
			r := &fakeRunner{stdout: []byte(dir + "\n")}
			run(newGit(r))

			if len(r.calls) == 0 {
				t.Fatal("git was never started")
			}
			for _, c := range r.calls {
				if !c.hasLimit {
					t.Errorf("git %q was started with no deadline, want about %v", c.args, gitexec.DefaultTimeout)
				} else if left := time.Until(c.deadline); left < gitexec.DefaultTimeout-time.Minute || left > gitexec.DefaultTimeout {
					t.Errorf("git %q was started with a deadline in %v, want about %v", c.args, left, gitexec.DefaultTimeout)
				}
			}
		})
	}
}
