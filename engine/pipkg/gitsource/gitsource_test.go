package gitsource_test

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/execrunner"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/pipkg"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/pipkg/gitsource"
)

// The adapter satisfies the port the package builder owns.
var _ pipkg.SourceRepo = gitsource.Repo{}

// call is one call the fake runner saw.
type call struct {
	env         []string
	bin         string
	args        []string
	hadDeadline bool
	remaining   time.Duration
}

// fakeRunner answers from a script and records what it was asked.
type fakeRunner struct {
	calls  []call
	stdout string
	stderr string
	err    error
}

func (f *fakeRunner) Output(ctx context.Context, env []string, bin string, args ...string) ([]byte, []byte, error) {
	c := call{env: env, bin: bin, args: args}
	if deadline, ok := ctx.Deadline(); ok {
		c.hadDeadline, c.remaining = true, time.Until(deadline)
	}
	f.calls = append(f.calls, c)
	return []byte(f.stdout), []byte(f.stderr), f.err
}

func TestEveryCallAsksGitAboutTheDirectoryItWasGiven(t *testing.T) {
	cases := map[string]struct {
		run  func(r gitsource.Repo)
		want string
	}{
		"IsWorkTree":                          {func(r gitsource.Repo) { _, _ = r.IsWorkTree("/o") }, "-C /o rev-parse --is-inside-work-tree"},
		"HasCommit":                           {func(r gitsource.Repo) { _, _ = r.HasCommit("/o", "main") }, "-C /o cat-file -e --end-of-options main^{commit}"},
		"HasPath":                             {func(r gitsource.Repo) { _, _ = r.HasPath("/o", "abc", "pi") }, "-C /o cat-file -e --end-of-options abc:pi"},
		"IsAncestor":                          {func(r gitsource.Repo) { _, _ = r.IsAncestor("/o", "a", "b") }, "-C /o merge-base --is-ancestor --end-of-options a b"},
		"Resolve":                             {func(r gitsource.Repo) { _, _ = r.Resolve("/o", "HEAD") }, "-C /o rev-parse --verify --end-of-options HEAD"},
		"HasChanges":                          {func(r gitsource.Repo) { _, _ = r.HasChanges("/o", "skills", "agents") }, "-C /o status --porcelain --untracked-files=all -- skills agents"},
		"LatestTag":                           {func(r gitsource.Repo) { _, _ = r.LatestTag("/o", "abc", "v*") }, "-C /o describe --tags --abbrev=0 --match v* --end-of-options abc"},
		"LatestTag at the checked-out commit": {func(r gitsource.Repo) { _, _ = r.LatestTag("/o", "", "v*") }, "-C /o describe --tags --abbrev=0 --match v*"},
		"Export":                              {func(r gitsource.Repo) { _, _ = r.Export("/o", "abc", []string{"skills", "pi"}) }, "-C /o archive --format=tar --end-of-options abc skills pi"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			runner := &fakeRunner{}
			c.run(gitsource.New(runner, gitsource.Options{}))
			if len(runner.calls) != 1 {
				t.Fatalf("%d calls, want 1", len(runner.calls))
			}
			if got := runner.calls[0].bin + " " + strings.Join(runner.calls[0].args, " "); got != "git "+c.want {
				t.Errorf("ran %q, want %q", got, "git "+c.want)
			}
		})
	}
}

func TestTheEnvironmentOfTheOptionsIsTheOneGitIsHanded(t *testing.T) {
	runner := &fakeRunner{}
	repo := gitsource.New(runner, gitsource.Options{Env: []string{"PATH=/bin", "HOME=/h"}})
	_, _ = repo.IsWorkTree("/o")
	if got := strings.Join(runner.calls[0].env, ","); got != "PATH=/bin,HOME=/h" {
		t.Errorf("env = %q, want the one of the options", got)
	}

	runner = &fakeRunner{}
	_, _ = gitsource.New(runner, gitsource.Options{}).IsWorkTree("/o")
	if runner.calls[0].env != nil {
		t.Errorf("env = %v, want nil (the environment of the process) when the options name none", runner.calls[0].env)
	}
}

func TestNoTimeoutMeansNoDeadlineAndATimeoutIsOne(t *testing.T) {
	runner := &fakeRunner{}
	_, _ = gitsource.New(runner, gitsource.Options{}).IsWorkTree("/o")
	if runner.calls[0].hadDeadline {
		t.Error("a call had a deadline although the options set none")
	}

	runner = &fakeRunner{}
	_, _ = gitsource.New(runner, gitsource.Options{Timeout: time.Minute}).IsWorkTree("/o")
	if !runner.calls[0].hadDeadline || runner.calls[0].remaining > time.Minute || runner.calls[0].remaining < 50*time.Second {
		t.Errorf("deadline in %v (set: %v), want about a minute", runner.calls[0].remaining, runner.calls[0].hadDeadline)
	}
}

func TestResolveAndLatestTagAndHasChangesTrimTheirOutput(t *testing.T) {
	runner := &fakeRunner{stdout: "  abc123\n"}
	repo := gitsource.New(runner, gitsource.Options{})
	if got, _ := repo.Resolve("/o", "HEAD"); got != "abc123" {
		t.Errorf("Resolve = %q, want abc123", got)
	}
	if got, _ := repo.LatestTag("/o", "", "v*"); got != "abc123" {
		t.Errorf("LatestTag = %q, want abc123", got)
	}
	if changed, _ := repo.HasChanges("/o"); !changed {
		t.Error("HasChanges said clean for a status that printed a line")
	}
	runner.stdout = " \n"
	if changed, _ := repo.HasChanges("/o"); changed {
		t.Error("HasChanges said changed for a status that printed only space")
	}
}

func TestExportTellsAGitFailureFromGitNotStarting(t *testing.T) {
	_, err := gitsource.New(&fakeRunner{err: &exec.ExitError{}, stderr: "fatal: not a valid object name\n"}, gitsource.Options{}).Export("/o", "abc", []string{"skills"})
	if err == nil || !strings.HasPrefix(err.Error(), "git archive abc: ") || !strings.HasSuffix(err.Error(), "(fatal: not a valid object name)") {
		t.Errorf("err = %v, want the failure of git archive with what git said", err)
	}

	_, err = gitsource.New(&fakeRunner{err: &exec.Error{Name: "git", Err: exec.ErrNotFound}}, gitsource.Options{}).Export("/o", "abc", []string{"skills"})
	if err == nil || !strings.HasPrefix(err.Error(), "starting git archive: ") {
		t.Errorf("err = %v, want it to say git could not be started", err)
	}

	_, err = gitsource.New(&fakeRunner{err: context.DeadlineExceeded}, gitsource.Options{Timeout: time.Second}).Export("/o", "abc", []string{"skills"})
	if err == nil || !strings.HasPrefix(err.Error(), "git archive abc: ") || !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want a git archive failure that wraps the deadline", err)
	}
}

// ---- against a real git, in a temporary repository, under an environment the test owns ----

// gitEnv is the environment of every git a test starts: no variable of the developer's, their
// git configuration out of reach, and a committer, so nothing outside the temporary directory is
// read or written.
func gitEnv(t *testing.T) []string {
	t.Helper()
	home := t.TempDir()
	env := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + home,
		"GIT_CONFIG_GLOBAL=" + os.DevNull,
		"GIT_CONFIG_SYSTEM=" + os.DevNull,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test.com",
		// A temporary directory that sits inside some repository must not be taken for part of it.
		"GIT_CEILING_DIRECTORIES=" + filepath.Dir(home),
	}
	return env
}

func realRepo(t *testing.T) (repo gitsource.Repo, dir string, git func(args ...string) string) {
	t.Helper()
	if testing.Short() {
		t.Skip("spawns real git")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	env := gitEnv(t)
	dir = t.TempDir()
	git = func(args ...string) string {
		t.Helper()
		out, _, err := execrunner.New().Output(context.Background(), env, "git", append([]string{"-C", dir}, args...)...)
		if err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "-q")
	return gitsource.New(execrunner.New(), gitsource.Options{Env: env}), dir, git
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAgainstARealRepository(t *testing.T) {
	repo, dir, git := realRepo(t)
	write(t, filepath.Join(dir, "skills", "a", "SKILL.md"), "one\n")
	write(t, filepath.Join(dir, "agents", "GADU.md"), "gadu\n")
	git("add", "-A")
	git("commit", "-q", "-m", "first")
	first := git("rev-parse", "HEAD")
	git("tag", "v1.2.3")
	git("tag", "other-9")
	write(t, filepath.Join(dir, "skills", "a", "SKILL.md"), "two\n")
	git("commit", "-q", "-am", "second")
	second := git("rev-parse", "HEAD")

	yes := func(ok bool, err error) bool {
		t.Helper()
		if err != nil {
			t.Fatalf("a question git can answer returned an error: %v", err)
		}
		return ok
	}
	if !yes(repo.IsWorkTree(dir)) || !yes(repo.IsWorkTree(filepath.Join(dir, "skills"))) {
		t.Error("IsWorkTree said no for a repository and for a directory inside it")
	}
	if yes(repo.IsWorkTree(t.TempDir())) {
		t.Error("IsWorkTree said yes for a directory that is not in a repository")
	}
	if !yes(repo.HasCommit(dir, "HEAD")) || !yes(repo.HasCommit(dir, first)) || yes(repo.HasCommit(dir, "no-such-ref")) {
		t.Error("HasCommit does not tell a commit from a name that is none")
	}
	if !yes(repo.HasPath(dir, first, "agents")) || yes(repo.HasPath(dir, first, "pi")) {
		t.Error("HasPath does not tell a path the commit holds from one it does not")
	}
	if !yes(repo.IsAncestor(dir, first, second)) || yes(repo.IsAncestor(dir, second, first)) || !yes(repo.IsAncestor(dir, second, second)) {
		t.Error("IsAncestor is wrong for a parent, a child and a commit with itself")
	}
	if got, err := repo.Resolve(dir, "HEAD"); err != nil || got != second {
		t.Errorf("Resolve(HEAD) = %q, %v, want %q", got, err, second)
	}
	if _, err := repo.Resolve(dir, "no-such-ref"); err == nil {
		t.Error("Resolve accepted a ref that names nothing")
	}
	if got, err := repo.LatestTag(dir, "", "v*"); err != nil || got != "v1.2.3" {
		t.Errorf("LatestTag = %q, %v, want v1.2.3 (the tag that matches, not the other)", got, err)
	}
	if got, err := repo.LatestTag(dir, first, "v*"); err != nil || got != "v1.2.3" {
		t.Errorf("LatestTag at the first commit = %q, %v", got, err)
	}
	if _, err := repo.LatestTag(dir, "", "nothing-*"); err == nil {
		t.Error("LatestTag found a tag for a pattern nothing matches")
	}

	if changed, err := repo.HasChanges(dir, "skills", "agents"); err != nil || changed {
		t.Errorf("HasChanges on a clean tree = %v, %v", changed, err)
	}
	write(t, filepath.Join(dir, "skills", "a", "new.md"), "untracked\n")
	if changed, _ := repo.HasChanges(dir, "skills", "agents"); !changed {
		t.Error("HasChanges missed an untracked file under a path")
	}
	if changed, _ := repo.HasChanges(dir, "agents"); changed {
		t.Error("HasChanges reported a change outside the paths asked")
	}

	archive, err := repo.Export(dir, first, []string{"skills", "agents"})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if got := readTar(t, archive); got["skills/a/SKILL.md"] != "one\n" || got["agents/GADU.md"] != "gadu\n" {
		t.Errorf("the archive holds %v, want the files as they were at the first commit", got)
	}
	if _, err := repo.Export(dir, first, []string{"pi"}); err == nil || !strings.HasPrefix(err.Error(), "git archive "+first+": ") {
		t.Errorf("Export of a path the commit lacks = %v, want a git archive failure", err)
	}
}

func readTar(t *testing.T, archive []byte) map[string]string {
	t.Helper()
	files := map[string]string{}
	r := tar.NewReader(bytes.NewReader(archive))
	for {
		h, err := r.Next()
		if err == io.EOF {
			return files
		}
		if err != nil {
			t.Fatalf("reading the archive: %v", err)
		}
		if h.Typeflag == tar.TypeReg {
			body, _ := io.ReadAll(r)
			files[h.Name] = string(body)
		}
	}
}

// The environment of the options is the one git sees: a variable that points git at another
// repository, set in the process, does not reach it.
func TestARepositoryVariableOfTheProcessDoesNotReachGit(t *testing.T) {
	repo, dir, git := realRepo(t)
	write(t, filepath.Join(dir, "f"), "x\n")
	git("add", "-A")
	git("commit", "-q", "-m", "one")
	want := git("rev-parse", "HEAD")

	elsewhere := t.TempDir()
	t.Setenv("GIT_DIR", filepath.Join(elsewhere, "not-a-repository"))
	got, err := repo.Resolve(dir, "HEAD")
	if err != nil || got != want {
		t.Errorf("Resolve = %q, %v, want %q: a GIT_DIR of the process reached git through the options' environment", got, err, want)
	}
}

// ---- a ref is never an option ----

var refCalls = map[string]func(r gitsource.Repo, ref string) error{
	"HasCommit":         func(r gitsource.Repo, ref string) error { _, err := r.HasCommit("/o", ref); return err },
	"HasPath":           func(r gitsource.Repo, ref string) error { _, err := r.HasPath("/o", ref, "pi"); return err },
	"IsAncestor first":  func(r gitsource.Repo, ref string) error { _, err := r.IsAncestor("/o", ref, "main"); return err },
	"IsAncestor second": func(r gitsource.Repo, ref string) error { _, err := r.IsAncestor("/o", "main", ref); return err },
	"Resolve":           func(r gitsource.Repo, ref string) error { _, err := r.Resolve("/o", ref); return err },
	"LatestTag":         func(r gitsource.Repo, ref string) error { _, err := r.LatestTag("/o", ref, "v*"); return err },
	"Export": func(r gitsource.Repo, ref string) error {
		_, err := r.Export("/o", ref, []string{"skills"})
		return err
	},
}

func TestARefThatStartsWithADashIsRefusedAndGitIsNotRun(t *testing.T) {
	for name, call := range refCalls {
		t.Run(name, func(t *testing.T) {
			runner := &fakeRunner{}
			err := call(gitsource.New(runner, gitsource.Options{}), "--output=/tmp/pwned")
			var invalid *gitsource.InvalidRefError
			if !errors.As(err, &invalid) || invalid.Ref != "--output=/tmp/pwned" {
				t.Fatalf("err = %v, want an *InvalidRefError naming the ref", err)
			}
			if !strings.Contains(err.Error(), `"--output=/tmp/pwned"`) {
				t.Errorf("err = %q, want it to name the ref", err)
			}
			if len(runner.calls) != 0 {
				t.Errorf("git was run %d time(s) for a ref it would read as an option: %v", len(runner.calls), runner.calls)
			}
		})
	}
}

// An empty rev is how LatestTag says "the checked-out commit", not a ref to refuse.
func TestAnEmptyRevForLatestTagIsNotARef(t *testing.T) {
	runner := &fakeRunner{stdout: "v1\n"}
	if _, err := gitsource.New(runner, gitsource.Options{}).LatestTag("/o", "", "v*"); err != nil || len(runner.calls) != 1 {
		t.Fatalf("err = %v, calls = %d, want the call to be made", err, len(runner.calls))
	}
}

// Every ref reaches git after --end-of-options, so a ref the check above missed still could not
// be read as an option. git 2.24 (2019) is the first that knows it; the git of this machine and
// of the CI image are newer.
func TestEveryRefFollowsTheOptionTerminator(t *testing.T) {
	for name, call := range refCalls {
		t.Run(name, func(t *testing.T) {
			runner := &fakeRunner{}
			_ = call(gitsource.New(runner, gitsource.Options{}), "feature")
			args := runner.calls[0].args
			terminator, ref := -1, -1
			for i, a := range args {
				if a == "--end-of-options" {
					terminator = i
				}
				if strings.Contains(a, "feature") {
					ref = i
				}
			}
			if terminator < 0 || ref < terminator {
				t.Errorf("argv %v: the ref must come after --end-of-options", args)
			}
		})
	}
}

// The hostile case end to end, against a real git: the archive of a ref that reads as --output
// writes nothing.
func TestAnOutputOptionDisguisedAsARefWritesNoFile(t *testing.T) {
	repo, dir, git := realRepo(t)
	write(t, filepath.Join(dir, "skills", "a"), "x\n")
	git("add", "-A")
	git("commit", "-q", "-m", "one")
	target := filepath.Join(t.TempDir(), "pwned")

	if _, err := repo.Export(dir, "--output="+target, []string{"skills"}); err == nil {
		t.Fatal("Export accepted a ref that is an option")
	}
	if _, err := os.Stat(target); err == nil {
		t.Fatal("git wrote the file the option named")
	}
}

// ---- "git answered no" is not "git could not answer" ----

// shellRunner runs a shell script in place of git, through the real process adapter, so the
// errors the adapter classifies are the ones the operating system produces.
type shellRunner struct{ script string }

func (s shellRunner) Output(ctx context.Context, env []string, _ string, _ ...string) ([]byte, []byte, error) {
	return execrunner.New().Output(ctx, env, "/bin/sh", "-c", s.script)
}

// missingRunner asks for a git that is not on the PATH.
type missingRunner struct{}

func (missingRunner) Output(ctx context.Context, env []string, _ string, _ ...string) ([]byte, []byte, error) {
	return execrunner.New().Output(ctx, env, "git-that-does-not-exist-9f3a", "x")
}

type unavailable interface{ Unavailable() bool }

func isUnavailable(err error) bool {
	var u unavailable
	return errors.As(err, &u) && u.Unavailable()
}

func predicates(repo gitsource.Repo) map[string]func() (bool, error) {
	return map[string]func() (bool, error){
		"IsWorkTree": func() (bool, error) { return repo.IsWorkTree("/o") },
		"HasCommit":  func() (bool, error) { return repo.HasCommit("/o", "main") },
		"HasPath":    func() (bool, error) { return repo.HasPath("/o", "main", "pi") },
		"IsAncestor": func() (bool, error) { return repo.IsAncestor("/o", "a", "b") },
	}
}

func TestANonZeroExitOfGitIsAnAnswerOfNo(t *testing.T) {
	for name, ask := range predicates(gitsource.New(shellRunner{"exit 1"}, gitsource.Options{})) {
		t.Run(name, func(t *testing.T) {
			if ok, err := ask(); ok || err != nil {
				t.Fatalf("= %v, %v, want no answer of yes and no error", ok, err)
			}
		})
	}
}

func TestAGitThatIsNotInstalledIsAnAnswerOfNo(t *testing.T) {
	for name, ask := range predicates(gitsource.New(missingRunner{}, gitsource.Options{})) {
		t.Run(name, func(t *testing.T) {
			if ok, err := ask(); ok || err != nil {
				t.Fatalf("= %v, %v, want no and no error: a machine without git builds unversioned", ok, err)
			}
		})
	}
	if _, err := gitsource.New(missingRunner{}, gitsource.Options{}).Resolve("/o", "HEAD"); err == nil || isUnavailable(err) {
		t.Errorf("Resolve err = %v, want a plain failure that is not marked unavailable", err)
	}
}

func TestAGitThatCouldNotAnswerIsAnError(t *testing.T) {
	cases := map[string]struct {
		runner  gitsource.Runner
		options gitsource.Options
	}{
		"killed by a signal":  {shellRunner{"kill -9 $$"}, gitsource.Options{}},
		"stopped at deadline": {shellRunner{"exec /bin/sleep 30"}, gitsource.Options{Timeout: 100 * time.Millisecond}},
	}
	for name, c := range cases {
		for pname, ask := range predicates(gitsource.New(c.runner, c.options)) {
			t.Run(name+"/"+pname, func(t *testing.T) {
				ok, err := ask()
				if ok || err == nil || !isUnavailable(err) {
					t.Fatalf("= %v, %v, want an error marked unavailable", ok, err)
				}
			})
		}
	}
	// The same failures from the methods that return values carry the mark too.
	repo := gitsource.New(shellRunner{"kill -9 $$"}, gitsource.Options{})
	if _, err := repo.Resolve("/o", "HEAD"); !isUnavailable(err) {
		t.Errorf("Resolve err = %v, want it marked unavailable", err)
	}
	if _, err := repo.HasChanges("/o", "skills"); !isUnavailable(err) {
		t.Errorf("HasChanges err = %v, want it marked unavailable", err)
	}
	if _, err := repo.LatestTag("/o", "", "v*"); !isUnavailable(err) {
		t.Errorf("LatestTag err = %v, want it marked unavailable", err)
	}
	if _, err := repo.Export("/o", "main", []string{"skills"}); !isUnavailable(err) {
		t.Errorf("Export err = %v, want it marked unavailable", err)
	}
	// And an exit status of git is not marked.
	repo = gitsource.New(shellRunner{"exit 128"}, gitsource.Options{})
	if _, err := repo.Resolve("/o", "nope"); err == nil || isUnavailable(err) {
		t.Errorf("Resolve err = %v, want a plain failure for an exit status", err)
	}
}

// A program that prints more than the bound of the process adapter is not an answer of git: the
// adapter reports it as one that could not answer, with the reason in the chain.
func TestAnOutputOverTheBoundIsAGitThatCouldNotAnswer(t *testing.T) {
	runner := execrunnerWithBound{script: `i=0; while [ $i -lt 500 ]; do printf '0123456789'; i=$((i+1)); done`, max: 100}
	repo := gitsource.New(runner, gitsource.Options{})

	_, err := repo.Export("/o", "main", []string{"skills"})
	if !errors.Is(err, execrunner.ErrOutputTooLarge) || !isUnavailable(err) {
		t.Fatalf("Export err = %v, want ErrOutputTooLarge in the chain and marked unavailable", err)
	}
	if _, err := repo.Resolve("/o", "main"); !errors.Is(err, execrunner.ErrOutputTooLarge) || !isUnavailable(err) {
		t.Errorf("Resolve err = %v, want the same", err)
	}
	if ok, err := repo.HasCommit("/o", "main"); ok || !errors.Is(err, execrunner.ErrOutputTooLarge) {
		t.Errorf("HasCommit = %v, %v, want an error, not a no", ok, err)
	}
}

// execrunnerWithBound runs a shell script through the real process adapter under a bound.
type execrunnerWithBound struct {
	script string
	max    int64
}

func (e execrunnerWithBound) Output(ctx context.Context, env []string, _ string, _ ...string) ([]byte, []byte, error) {
	return execrunner.New().WithMaxOutput(e.max).Output(ctx, env, "/bin/sh", "-c", e.script)
}
