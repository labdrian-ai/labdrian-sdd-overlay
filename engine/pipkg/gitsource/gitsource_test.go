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
		"IsWorkTree":                          {func(r gitsource.Repo) { r.IsWorkTree("/o") }, "-C /o rev-parse --is-inside-work-tree"},
		"HasCommit":                           {func(r gitsource.Repo) { r.HasCommit("/o", "main") }, "-C /o cat-file -e main^{commit}"},
		"HasPath":                             {func(r gitsource.Repo) { r.HasPath("/o", "abc", "pi") }, "-C /o cat-file -e abc:pi"},
		"IsAncestor":                          {func(r gitsource.Repo) { r.IsAncestor("/o", "a", "b") }, "-C /o merge-base --is-ancestor a b"},
		"Resolve":                             {func(r gitsource.Repo) { _, _ = r.Resolve("/o", "HEAD") }, "-C /o rev-parse HEAD"},
		"HasChanges":                          {func(r gitsource.Repo) { _, _ = r.HasChanges("/o", "skills", "agents") }, "-C /o status --porcelain --untracked-files=all -- skills agents"},
		"LatestTag":                           {func(r gitsource.Repo) { _, _ = r.LatestTag("/o", "abc", "v*") }, "-C /o describe --tags --abbrev=0 --match v* abc"},
		"LatestTag at the checked-out commit": {func(r gitsource.Repo) { _, _ = r.LatestTag("/o", "", "v*") }, "-C /o describe --tags --abbrev=0 --match v*"},
		"Export":                              {func(r gitsource.Repo) { _, _ = r.Export("/o", "abc", []string{"skills", "pi"}) }, "-C /o archive --format=tar abc -- skills pi"},
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
	repo.IsWorkTree("/o")
	if got := strings.Join(runner.calls[0].env, ","); got != "PATH=/bin,HOME=/h" {
		t.Errorf("env = %q, want the one of the options", got)
	}

	runner = &fakeRunner{}
	gitsource.New(runner, gitsource.Options{}).IsWorkTree("/o")
	if runner.calls[0].env != nil {
		t.Errorf("env = %v, want nil (the environment of the process) when the options name none", runner.calls[0].env)
	}
}

func TestNoTimeoutMeansNoDeadlineAndATimeoutIsOne(t *testing.T) {
	runner := &fakeRunner{}
	gitsource.New(runner, gitsource.Options{}).IsWorkTree("/o")
	if runner.calls[0].hadDeadline {
		t.Error("a call had a deadline although the options set none")
	}

	runner = &fakeRunner{}
	gitsource.New(runner, gitsource.Options{Timeout: time.Minute}).IsWorkTree("/o")
	if !runner.calls[0].hadDeadline || runner.calls[0].remaining > time.Minute || runner.calls[0].remaining < 50*time.Second {
		t.Errorf("deadline in %v (set: %v), want about a minute", runner.calls[0].remaining, runner.calls[0].hadDeadline)
	}
}

func TestAQuestionGitCannotAnswerIsNo(t *testing.T) {
	runner := &fakeRunner{err: errors.New("git is not installed")}
	repo := gitsource.New(runner, gitsource.Options{})
	if repo.IsWorkTree("/o") || repo.HasCommit("/o", "main") || repo.HasPath("/o", "abc", "pi") || repo.IsAncestor("/o", "a", "b") {
		t.Error("a predicate answered yes although git failed")
	}
	if _, err := repo.Resolve("/o", "HEAD"); err == nil {
		t.Error("Resolve hid the failure of git")
	}
	if _, err := repo.HasChanges("/o", "skills"); err == nil {
		t.Error("HasChanges hid the failure of git")
	}
	if _, err := repo.LatestTag("/o", "", "v*"); err == nil {
		t.Error("LatestTag hid the failure of git")
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

	if !repo.IsWorkTree(dir) || !repo.IsWorkTree(filepath.Join(dir, "skills")) {
		t.Error("IsWorkTree said no for a repository and for a directory inside it")
	}
	if repo.IsWorkTree(t.TempDir()) {
		t.Error("IsWorkTree said yes for a directory that is not in a repository")
	}
	if !repo.HasCommit(dir, "HEAD") || !repo.HasCommit(dir, first) || repo.HasCommit(dir, "no-such-ref") {
		t.Error("HasCommit does not tell a commit from a name that is none")
	}
	if !repo.HasPath(dir, first, "agents") || repo.HasPath(dir, first, "pi") {
		t.Error("HasPath does not tell a path the commit holds from one it does not")
	}
	if !repo.IsAncestor(dir, first, second) || repo.IsAncestor(dir, second, first) || !repo.IsAncestor(dir, second, second) {
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
