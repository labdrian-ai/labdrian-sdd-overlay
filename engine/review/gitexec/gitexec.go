// Package gitexec is the adapter of the Git port of review/app: it asks the git program of the machine, through
// a Runner, the questions `labdrian review bind` and `labdrian review record` put about a repository, and the
// one the stop hook puts about a branch (Standing). It
// starts git only with an argument vector it composes (a revision is a value after "--end-of-options", never
// an option) and only under an environment it scrubs, and it reads and writes no file of the repository but
// the worktree of a review (Worktree, AddWorktree).
//
// # The canonical diff
//
// The diff_sha256 of a binding is the SHA-256 of the bytes this command prints, run in the root of a working
// tree of the repository, with <base> and <head> full object names (DiffArgs is the argument vector):
//
//	git -c core.quotePath=true -c diff.suppressBlankEmpty=false -c core.attributesFile=/dev/null \
//	    -c core.compression=0 -c core.looseCompression=0 --attr-source=<empty tree> \
//	    diff-tree -p -r --full-index --binary --no-renames --no-color --no-ext-diff --no-textconv \
//	    --ignore-submodules=none --src-prefix=a/ --dst-prefix=b/ -U3 <base> <head>
//
// The environment of that git is the environment of the process without any variable whose name begins with
// GIT_, with GIT_ATTR_NOSYSTEM=1 and LC_ALL=C added. The empty tree is EmptyTreeSHA1 in a repository that
// names its objects with 40 characters and EmptyTreeSHA256 in one that names them with 64.
//
// The definition has one purpose: the same two commits give the same bytes on every run, so that `record` and
// a later check recompute the digest the reply was bound to. Everything in it was measured with git 2.43; the
// tests of this package run the real git against each part.
//
//   - It is the plumbing diff-tree, which compares two trees and reads neither the index nor the working tree for
//     their content, and ignores the settings a person changes for the porcelain (diff.algorithm, diff.context,
//     diff.noprefix, diff.renames, diff.external, diff.ignoreSubmodules, color.*, core.abbrev and the like).
//   - Five parts are there because the plumbing itself reads what they pin, and a diff printed without them
//     changes with the configuration or the checkout: core.quotePath (the quoting of a name that is not ASCII),
//     diff.suppressBlankEmpty (whether a blank context line keeps its space), core.attributesFile, which with
//     the attribute source below keeps every file of attributes out (the global one, the one of XDG, the
//     one of the working tree and of the index), the attribute source, which is the empty tree so that no
//     .gitattributes of the candidate, of the commit checked out or of the working tree decides what is binary
//     and no diff driver of the user is named, and core.compression and core.looseCompression, which choose the
//     level of the zlib stream of a binary patch (0 stores it).
//   - The rest is the same definition written out so that a later git cannot change it: no rename or copy
//     detection, no color, no external diff and no textconv, the full name of every object in the index lines,
//     binary files as patches, a submodule shown as a change of commit whatever its settings, the prefixes a/
//     and b/, and three lines of context.
//   - The variables of git are removed because they win over the command line: GIT_DIFF_OPTS changes the
//     context, GIT_CONFIG_* the configuration, GIT_DIR and GIT_INDEX_FILE the repository; the system file of
//     attributes is not read; the messages are not translated.
//
// What is not pinned, and so what can make two machines disagree: $GIT_DIR/info/attributes of the repository
// (a file the user keeps by hand, outside any commit), and the zlib of the machine, whose output for a binary
// patch is the same on the zlibs we know and is not promised to be on all. A digest that does not agree is
// refused, never accepted: a reply bound to a digest the check does not recompute is not found. It needs a git
// that has --attr-source (2.40 or later).
//
// The list of the paths of the diff is the NUL-terminated output of
//
//	git diff-tree -r --name-only -z --no-renames --ignore-submodules=none <base> <head>
//
// (PathsArgs): the names as git holds them, byte for byte, and a rename is the removal of its old path and the
// creation of the new one, which is also how the diff above shows it.
package gitexec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/gitobject"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/gitprov"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
)

// The empty tree of each hash function git names objects with: the source of the attributes of the canonical
// diff. Git knows both without having them in the repository.
const (
	EmptyTreeSHA1   = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"
	EmptyTreeSHA256 = "6ef19b41225c5369f1c104d45d8d85efa9b057b53b14b4b9b939dd74decc5321"
)

// gitProgram is the program started; the runner looks it up on the PATH of the process.
const gitProgram = "git"

// maxSaid bounds the part of what git printed on its standard error that an error message repeats.
const maxSaid = 2000

// DefaultTimeout is how long one start of git may run when the options give no limit. Every question this
// adapter puts is local and answers in milliseconds, and the checkout of a worktree of a large repository in
// seconds: a git past five minutes is stuck. There is no way to run git without a limit.
const DefaultTimeout = 5 * time.Minute

// Runner starts a program with exactly the environment it is given and returns what it printed on its standard
// output and on its standard error, apart. The process adapter is execrunner.Runner.
type Runner interface {
	Output(ctx context.Context, env []string, bin string, args ...string) (stdout, stderr []byte, err error)
	// OutputStdin is Output for a program that reads its standard input: it is handed exactly stdin.
	OutputStdin(ctx context.Context, env []string, stdin []byte, bin string, args ...string) (stdout, stderr []byte, err error)
}

// Options are what the adapter needs of the process it runs in.
type Options struct {
	// Environ is the environment of the process, os.Environ() in production. Git is started under it with
	// every variable of git removed; Locate refuses the ones that redirect git to another repository.
	Environ []string
	// Timeout is how long one start of git may run. Zero, or any duration that is not positive, is
	// DefaultTimeout: there is no way to start git without a limit.
	Timeout time.Duration
}

// Git implements app.Git over a Runner. The zero value has no Runner and cannot be used; New makes one.
type Git struct {
	runner  Runner
	ambient []string // the environment as it was given: Locate looks for what redirects git
	env     []string // the environment git is started under
	timeout time.Duration
}

// New is the adapter over runner under the environment and the deadline of the options (DefaultTimeout when
// they give none).
func New(runner Runner, o Options) Git {
	timeout := o.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return Git{runner: runner, ambient: append([]string(nil), o.Environ...), env: gitEnvironment(o.Environ), timeout: timeout}
}

// gitEnvironment is environ without the variables of git and of the locale, with the two this adapter sets. It
// is never nil: a nil environment would give git the one of the process.
func gitEnvironment(environ []string) []string {
	env := make([]string, 0, len(environ)+2)
	for _, kv := range environ {
		name, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(name, "GIT_") || name == "LC_ALL" {
			continue
		}
		env = append(env, kv)
	}
	return append(env, "GIT_ATTR_NOSYSTEM=1", "LC_ALL=C")
}

// DiffArgs is the argument vector of the canonical diff from base to head (full object names), without the
// program and without the directory.
func DiffArgs(base, head string) []string {
	return []string{
		"-c", "core.quotePath=true",
		"-c", "diff.suppressBlankEmpty=false",
		"-c", "core.attributesFile=" + os.DevNull,
		"-c", "core.compression=0",
		"-c", "core.looseCompression=0",
		"--attr-source=" + emptyTree(head),
		"diff-tree", "-p", "-r", "--full-index", "--binary", "--no-renames", "--no-color", "--no-ext-diff", "--no-textconv",
		"--ignore-submodules=none", "--src-prefix=a/", "--dst-prefix=b/", "-U3",
		base, head,
	}
}

// PathsArgs is the argument vector of the list of the paths the diff from base to head changes.
func PathsArgs(base, head string) []string {
	return []string{"diff-tree", "-r", "--name-only", "-z", "--no-renames", "--ignore-submodules=none", base, head}
}

// emptyTree is the empty tree of the hash function the name head is written with. head must be a full object
// name (gitobject.IsName): 40 characters in a repository that names its objects with SHA-1 and 64 in one that
// uses SHA-256, which is what Commit returns. The length alone tells the two apart, so a head of any other
// length is not refused here and is given the SHA-1 empty tree; keeping such a name out is up to the caller.
func emptyTree(head string) string {
	if len(head) == len(EmptyTreeSHA256) {
		return EmptyTreeSHA256
	}
	return EmptyTreeSHA1
}

// Locate finds the working tree that holds dir (absolute) with gitprov, which refuses a bare repository, a
// gitfile that does not lead back, and an environment that redirects git. The main working tree is the one
// the common git directory sits in (<main>/.git); a repository whose git directory is kept elsewhere has no
// main working tree to find, and the working tree the command ran in stands for it.
func (g Git) Locate(dir string) (app.Repository, error) {
	obs, err := g.observer().Locate(dir)
	if err != nil {
		return app.Repository{}, err
	}
	main := obs.Toplevel
	if filepath.Base(obs.CommonDir) == ".git" {
		main = filepath.Dir(obs.CommonDir)
	}
	return app.Repository{Root: obs.Toplevel, Main: main, CommonDir: obs.CommonDir}, nil
}

// Commit resolves rev to the commit it names, a tag peeled, and returns its full object name.
func (g Git) Commit(repo app.Repository, rev string) (string, error) {
	return g.commit(repo.Root, rev)
}

func (g Git) commit(dir, rev string) (string, error) {
	out, err := g.output(dir, "rev-parse", []string{"rev-parse", "--verify", "--end-of-options", rev + "^{commit}"})
	if err != nil {
		return "", err
	}
	name := strings.TrimSuffix(string(out), "\n")
	if !gitobject.IsName(name) {
		return "", fmt.Errorf("git rev-parse printed %q for %q, want one full object name", clip(string(out)), rev)
	}
	return name, nil
}

// Parent returns the first parent of commit, a full object name, and "" when the commit has none. git prints the
// commit and then its parents on one line, and the first of them is the one a merge was made on; anything else is
// not an answer to the question and is an error.
func (g Git) Parent(repo app.Repository, commit string) (string, error) {
	out, err := g.output(repo.Root, "rev-list", []string{"rev-list", "--parents", "-n", "1", "--end-of-options", commit})
	if err != nil {
		return "", err
	}
	line, whole := strings.CutSuffix(string(out), "\n")
	names := strings.Split(line, " ")
	if !whole || strings.Contains(line, "\n") || names[0] != commit {
		return "", fmt.Errorf("git rev-list printed %q for %q, want the commit and then its parents on one line", clip(string(out)), commit)
	}
	for _, parent := range names[1:] {
		if !gitobject.IsName(parent) {
			return "", fmt.Errorf("git rev-list printed the parent %q of %q, want a full object name", clip(parent), commit)
		}
	}
	if len(names) == 1 {
		return "", nil
	}
	return names[1], nil
}

// Parents returns every parent of commit, a full object name, first parent first, and none for a commit that has none.
// git prints the commit and then its parents on one line; anything else is not an answer to the question and is an error.
func (g Git) Parents(repo app.Repository, commit string) ([]string, error) {
	out, err := g.output(repo.Root, "rev-list", []string{"rev-list", "--parents", "-n", "1", "--end-of-options", commit})
	if err != nil {
		return nil, err
	}
	line, whole := strings.CutSuffix(string(out), "\n")
	names := strings.Split(line, " ")
	if !whole || strings.Contains(line, "\n") || names[0] != commit {
		return nil, fmt.Errorf("git rev-list printed %q for %q, want the commit and then its parents on one line", clip(string(out)), commit)
	}
	for _, parent := range names[1:] {
		if !gitobject.IsName(parent) {
			return nil, fmt.Errorf("git rev-list printed the parent %q of %q, want a full object name", clip(parent), commit)
		}
	}
	return names[1:], nil
}

// MergeBases returns the best common ancestors of a and b. `git merge-base --all` exits 1 with nothing printed for
// histories that share no commit, which is none and no error; any other failure, and any line that is not a full object
// name, is an error.
func (g Git) MergeBases(repo app.Repository, a, b string) ([]string, error) {
	ctx, cancel := g.context()
	defer cancel()
	stdout, stderr, err := g.runner.Output(ctx, g.env, gitProgram, "-C", repo.Root, "merge-base", "--all", "--end-of-options", a, b)
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 && len(stdout) == 0 && strings.TrimSpace(string(stderr)) == "" {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("git merge-base: %w%s", err, said(stderr))
	}
	text, whole := strings.CutSuffix(string(stdout), "\n")
	if !whole {
		return nil, fmt.Errorf("git merge-base printed %q for %q and %q, want full object names, one to a line", clip(string(stdout)), a, b)
	}
	bases := strings.Split(text, "\n")
	for _, name := range bases {
		if !gitobject.IsName(name) {
			return nil, fmt.Errorf("git merge-base printed %q for %q and %q, want full object names, one to a line", clip(string(stdout)), a, b)
		}
	}
	return bases, nil
}

// Tree returns the full object name of the tree of commit, a full object name.
func (g Git) Tree(repo app.Repository, commit string) (string, error) {
	out, err := g.output(repo.Root, "rev-parse", []string{"rev-parse", "--verify", "--end-of-options", commit + "^{tree}"})
	if err != nil {
		return "", err
	}
	name := strings.TrimSuffix(string(out), "\n")
	if !gitobject.IsName(name) {
		return "", fmt.Errorf("git rev-parse printed %q for the tree of %q, want one full object name", clip(string(out)), commit)
	}
	return name, nil
}

// Diff returns the bytes of the canonical diff, untouched.
func (g Git) Diff(repo app.Repository, base, head string) ([]byte, error) {
	return g.output(repo.Root, "diff-tree", DiffArgs(base, head))
}

// Paths returns the paths of the diff, in the order git prints them.
func (g Git) Paths(repo app.Repository, base, head string) ([]string, error) {
	out, err := g.output(repo.Root, "diff-tree", PathsArgs(base, head))
	if err != nil {
		return nil, err
	}
	return splitPaths(out)
}

// splitPaths reads the NUL-terminated names of out: a name without its terminator was cut, and an empty name
// is no name.
func splitPaths(out []byte) ([]string, error) {
	paths := []string{}
	if len(out) == 0 {
		return paths, nil
	}
	if out[len(out)-1] != 0 {
		return nil, fmt.Errorf("git diff-tree printed a list of paths that does not end with a NUL byte (it ends with %q)", clip(string(out[max(0, len(out)-20):])))
	}
	for _, name := range bytes.Split(out[:len(out)-1], []byte{0}) {
		if len(name) == 0 {
			return nil, errors.New("git diff-tree printed an empty path")
		}
		paths = append(paths, string(name))
	}
	return paths, nil
}

// Worktree says what is at path. Something that is not a directory (a file, a link: links are not followed),
// a directory that is not the root of a working tree of this repository, and a working tree git cannot vouch
// for are all taken. A working tree of this repository says which commit it has checked out.
//
// Taken is what git answered, and nothing else is taken. A git that gave no answer (it could not be started,
// it was killed, or its time ran out) and a question that could not be put to it (a path that is not
// absolute, an environment that redirects git) say nothing about the path, and they are an error: calling the
// path taken then would send a person to move away a worktree that may be the one they need.
func (g Git) Worktree(repo app.Repository, path string) (app.WorktreeState, error) {
	info, err := os.Lstat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return app.WorktreeState{}, nil
	case err != nil:
		return app.WorktreeState{}, fmt.Errorf("inspect %s: %w", path, err)
	case !info.IsDir():
		return app.WorktreeState{Present: true}, nil
	}
	taken := app.WorktreeState{Present: true}
	asked := false
	observer := g.observer()
	run := observer.Run
	observer.Run = func(dir string, env []string, args []string) ([]byte, error) {
		asked = true
		return run(dir, env, args)
	}
	obs, err := observer.Locate(path)
	if err != nil {
		var silent *failure
		if !asked || errors.As(err, &silent) {
			return app.WorktreeState{}, err
		}
		return taken, nil
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return app.WorktreeState{}, fmt.Errorf("resolve %s: %w", path, err)
	}
	if obs.Toplevel != real || obs.CommonDir != repo.CommonDir {
		return taken, nil
	}
	head, err := g.commit(path, "HEAD")
	if err != nil {
		return app.WorktreeState{}, fmt.Errorf("read the HEAD of the worktree %s: %w", path, err)
	}
	return app.WorktreeState{Present: true, OfRepository: true, Head: head}, nil
}

// AddWorktree makes path a detached worktree of the repository at head. No hook runs: the hooks of the
// repository are the user's, and a checkout of a branch of someone else must not run anything.
func (g Git) AddWorktree(repo app.Repository, path, head string) error {
	_, err := g.output(repo.Root, "worktree add", []string{"-c", "core.hooksPath=" + os.DevNull, "worktree", "add", "--detach", path, head})
	return err
}

// originRefs is where the branches of the remote called origin are kept.
const originRefs = "refs/remotes/origin/"

// Standing says which branch HEAD is on, which branch is the default one and the commits of HEAD that the default
// branch does not have (own-rdd T6a, S6). A detached HEAD ("HEAD" is what git prints for it) and a repository with
// no default branch are told without the commits.
//
// The default branch is the one origin/HEAD names when the repository has it, else main, else master: the first of
// them that exists as a local branch or as the branch origin keeps. The commits are what
//
//	git rev-list --first-parent --reverse --end-of-options <HEAD> ^<tip> ...
//
// lists, with a tip for each of the two places the default branch is, so that a local branch behind its remote, and
// one ahead of it, both leave their commits out; a branch that merged the default branch lists its own line of
// commits and not the ones merged in.
func (g Git) Standing(repo app.Repository) (app.Standing, error) {
	out, err := g.output(repo.Root, "rev-parse", []string{"rev-parse", "--abbrev-ref", "HEAD"})
	if err != nil {
		return app.Standing{}, err
	}
	branch := strings.TrimSuffix(string(out), "\n")
	switch {
	case branch == "HEAD":
		return app.Standing{}, nil
	case branch == "" || strings.Contains(branch, "\n"):
		return app.Standing{}, fmt.Errorf("git rev-parse printed %q for the branch of HEAD, want one branch name", clip(string(out)))
	}
	name, tips, err := g.defaultBranch(repo.Root)
	if err != nil || name == "" {
		return app.Standing{Branch: branch}, err
	}
	head, err := g.commit(repo.Root, "HEAD")
	if err != nil {
		return app.Standing{}, err
	}
	args := []string{"rev-list", "--first-parent", "--reverse", "--end-of-options", head}
	for _, tip := range tips {
		args = append(args, "^"+tip)
	}
	out, err = g.output(repo.Root, "rev-list", args)
	if err != nil {
		return app.Standing{}, err
	}
	commits, err := commitNames("rev-list", out)
	if err != nil {
		return app.Standing{}, err
	}
	return app.Standing{Branch: branch, Default: name, Commits: commits}, nil
}

// Message returns the raw message of commit. `git log -1 --format=%B` ends the message with a newline of its own
// after the one the message ends with; that one is removed, so what comes back is the message as the commit stores it.
func (g Git) Message(repo app.Repository, commit string) (string, error) {
	out, err := g.output(repo.Root, "log", []string{"log", "-1", "--no-show-signature", "--format=%B", "--end-of-options", commit})
	if err != nil {
		return "", err
	}
	message, whole := strings.CutSuffix(string(out), "\n")
	if !whole {
		return "", fmt.Errorf("git log printed a message for %q that does not end with a newline (it ends with %q)", commit, clip(string(out[max(0, len(out)-20):])))
	}
	return message, nil
}

// Trailers returns the trailers `git interpret-trailers --parse` finds in message, one per line of its output as
// "<key>: <value>", split at the first colon. git reads the message from a file the adapter makes in the temporary
// directory of the process, never in the repository, and removes it. A line without a colon is no trailer and is an
// error: --parse prints trailers only.
func (g Git) Trailers(repo app.Repository, message string) ([]review.Trailer, error) {
	file, err := os.CreateTemp("", "labdrian-message-*")
	if err != nil {
		return nil, fmt.Errorf("make the file git reads the message from: %w", err)
	}
	defer os.Remove(file.Name())
	if _, err := file.WriteString(message); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("write the message for git: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("write the message for git: %w", err)
	}
	out, err := g.output(repo.Root, "interpret-trailers", []string{"interpret-trailers", "--parse", file.Name()})
	if err != nil {
		return nil, err
	}
	var trailers []review.Trailer
	for _, line := range strings.Split(strings.TrimSuffix(string(out), "\n"), "\n") {
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return nil, fmt.Errorf("git interpret-trailers printed %q, want <key>: <value>", clip(line))
		}
		trailers = append(trailers, review.Trailer{Key: key, Value: strings.TrimSpace(value)})
	}
	return trailers, nil
}

// DeletedOrRenamedArgs is the argument vector of the list of the paths the diff from base to head removes: the paths
// of PathsArgs filtered to the removed ones. Renames are not detected, so the old path of a rename is a removal.
func DeletedOrRenamedArgs(base, head string) []string {
	return []string{"diff-tree", "-r", "--name-only", "-z", "--no-renames", "--diff-filter=DR", "--ignore-submodules=none", base, head}
}

// DeletedOrRenamed returns the paths the diff from base to head removes, in the order git prints them (own-rdd T6b3, S6 b).
func (g Git) DeletedOrRenamed(repo app.Repository, base, head string) ([]string, error) {
	out, err := g.output(repo.Root, "diff-tree", DeletedOrRenamedArgs(base, head))
	if err != nil {
		return nil, err
	}
	return splitPaths(out)
}

// ChangedLinesArgs is the argument vector of the diff of one path with no context, from base to head. The path is
// literal: it is never a pattern.
func ChangedLinesArgs(base, head, file string) []string {
	return []string{
		"--literal-pathspecs", "-c", "core.quotePath=false",
		"diff-tree", "-p", "-r", "-U0", "--no-renames", "--no-color", "--no-ext-diff", "--no-textconv", "--ignore-submodules=none",
		base, head, "--", file,
	}
}

// ChangedLines returns the removed and the added lines of the .go and .sh files the diff from base to head changes,
// in the order of their paths. It asks for the diff of one path at a time, so the name of a file is the one git listed
// (Paths) and never read from the header of a diff, where a name with a space or a quote would be ambiguous. Binary
// files have no lines; a path with no removed line is not in the first list, and one with no added line is not in the
// second.
func (g Git) ChangedLines(repo app.Repository, base, head string) (removed, added []review.ChangedLines, err error) {
	paths, err := g.Paths(repo, base, head)
	if err != nil {
		return nil, nil, err
	}
	for _, p := range paths {
		if !strings.HasSuffix(p, ".go") && !strings.HasSuffix(p, ".sh") {
			continue
		}
		out, err := g.output(repo.Root, "diff-tree", ChangedLinesArgs(base, head, p))
		if err != nil {
			return nil, nil, err
		}
		gone, came := hunkLines(string(out))
		if len(gone) > 0 {
			removed = append(removed, review.ChangedLines{Path: p, Lines: gone})
		}
		if len(came) > 0 {
			added = append(added, review.ChangedLines{Path: p, Lines: came})
		}
	}
	return removed, added, nil
}

// hunkLines is the removed and the added lines of the diff of one file, without the leading '-' and '+'. Everything
// before the first hunk is the header of the diff and holds none.
func hunkLines(diff string) (removed, added []string) {
	inHunk := false
	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "@@"):
			inHunk = true
		case !inHunk:
		case strings.HasPrefix(line, "-"):
			removed = append(removed, line[1:])
		case strings.HasPrefix(line, "+"):
			added = append(added, line[1:])
		}
	}
	return removed, added
}

// Modules returns the directories that hold a go.mod at commit: the files named go.mod of its tree, by directory, "." for
// the root, sorted.
func (g Git) Modules(repo app.Repository, commit string) ([]string, error) {
	out, err := g.output(repo.Root, "ls-tree", []string{"ls-tree", "-r", "--name-only", "-z", "--end-of-options", commit})
	if err != nil {
		return nil, err
	}
	files, err := splitPaths(out)
	if err != nil {
		return nil, err
	}
	var modules []string
	for _, f := range files {
		if path.Base(f) == "go.mod" {
			modules = append(modules, path.Dir(f))
		}
	}
	slices.Sort(modules)
	return modules, nil
}

// mentionsBatch is how many names one start of `git grep` is asked for: the cost of a sweep grows with the names in
// steps of this size, and 256 names of the length of an identifier stay far under the limit of the arguments.
const mentionsBatch = 256

// Mentions returns, for each of names, the paths of the tree of commit that hold it as a whole word, matched as a fixed
// string, once each, in the order git prints them (own-rdd T6b3 and T6b4a, S6 b). The names go to `git grep -I -z -o -w
// -F` in batches of mentionsBatch; -o prints the text each match found after the path, and that text is the name. A
// name nothing mentions is not a key. A name that is inside the match of a longer name at the same place is not
// reported there, which cannot happen to the identifiers a sweep asks about. git exits 1 when nothing matches, which is
// an answer; any other failure is not.
func (g Git) Mentions(repo app.Repository, commit string, names []string) (map[string][]string, error) {
	for _, name := range names {
		if name == "" || strings.Contains(name, "\n") {
			return nil, fmt.Errorf("the name %q to search for is empty or has a line break", name)
		}
	}
	found := map[string][]string{}
	for start := 0; start < len(names); start += mentionsBatch {
		batch := names[start:min(start+mentionsBatch, len(names))]
		if err := g.mentionsOf(repo, commit, batch, found); err != nil {
			return nil, err
		}
	}
	return found, nil
}

// mentionsOf puts one batch of names to git and adds what it found to found.
func (g Git) mentionsOf(repo app.Repository, commit string, batch []string, found map[string][]string) error {
	ctx, cancel := g.context()
	defer cancel()
	args := []string{"-C", repo.Root, "grep", "-I", "-z", "-o", "-w", "-F", "--no-color"}
	for _, name := range batch {
		args = append(args, "-e", name)
	}
	args = append(args, commit)
	stdout, stderr, err := g.runner.Output(ctx, g.env, gitProgram, args...)
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 && len(stdout) == 0 && strings.TrimSpace(string(stderr)) == "" {
		return nil
	}
	if err != nil {
		return fmt.Errorf("git grep: %w%s", err, said(stderr))
	}
	asked := map[string]bool{}
	for _, name := range batch {
		asked[name] = true
	}
	rest := stdout
	for len(rest) > 0 {
		entry, after, ok := bytes.Cut(rest, []byte{0})
		match, after, okMatch := bytes.Cut(after, []byte{'\n'})
		p, hasCommit := strings.CutPrefix(string(entry), commit+":")
		if !ok || !okMatch || !hasCommit || p == "" || !asked[string(match)] {
			return fmt.Errorf("git grep printed %q, want <commit>:<path>, a NUL byte, a name asked for and a line feed", clip(string(rest)))
		}
		if !slices.Contains(found[string(match)], p) {
			found[string(match)] = append(found[string(match)], p)
		}
		rest = after
	}
	return nil
}

// IsCommit says whether name is a commit git has. `git cat-file -e` exits 1 with nothing printed for an object git does
// not have, which is false and not an error; `git cat-file -t` then names the kind of the object, and only "commit"
// is true. Anything else git does, a failure to start, a message or another status, is an error.
func (g Git) IsCommit(repo app.Repository, name string) (bool, error) {
	if !gitobject.IsName(name) {
		return false, fmt.Errorf("%q is not a full object name", name)
	}
	ctx, cancel := g.context()
	defer cancel()
	stdout, stderr, err := g.runner.Output(ctx, g.env, gitProgram, "-C", repo.Root, "cat-file", "-e", name)
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 && len(stdout) == 0 && strings.TrimSpace(string(stderr)) == "" {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("git cat-file -e: %w%s", err, said(stderr))
	}
	out, err := g.output(repo.Root, "cat-file -t", []string{"cat-file", "-t", name})
	if err != nil {
		return false, err
	}
	switch kind := strings.TrimSuffix(string(out), "\n"); kind {
	case "commit":
		return true, nil
	case "tree", "blob", "tag":
		return false, nil
	default:
		return false, fmt.Errorf("git cat-file -t printed %q for %q, want the kind of the object", clip(string(out)), name)
	}
}

// Status returns the lines of `git status --porcelain` with every untracked file on its own line, as the working tree is.
// It takes no optional lock, so asking changes nothing in the repository.
func (g Git) Status(repo app.Repository) ([]string, error) {
	out, err := g.output(repo.Root, "status", []string{"--no-optional-locks", "-c", "core.quotePath=false", "status", "--porcelain=v1", "--untracked-files=all"})
	if err != nil {
		return nil, err
	}
	text := strings.TrimSuffix(string(out), "\n")
	if text == "" {
		return nil, nil
	}
	return strings.Split(text, "\n"), nil
}

// Archive fills dir with the exact bytes of the tree of commit, built from the objects and not from an export: no
// attribute, filter or end-of-line setting, in the tree, the repository or the configuration, changes a byte of it. The
// listing comes from `git ls-tree -r -z -l --full-tree` and the bytes of every blob from one `git cat-file --batch`,
// asked for the names of the blobs in the order of the listing. A file of mode 100644 or 100755 is written with that
// mode, a 120000 entry as a link whose target is the blob, and a 160000 entry (a submodule) is skipped; any other mode,
// a path that is not local to dir, a file whose directory is a link, a listing or an answer of git that is not as
// described, and a blob whose size is not the one the listing gives are refused.
func (g Git) Archive(repo app.Repository, commit, dir string) error {
	if !filepath.IsAbs(dir) {
		return fmt.Errorf("the directory %q to unpack into is not an absolute path", dir)
	}
	if !gitobject.IsName(commit) {
		return fmt.Errorf("%q is not a full object name", commit)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read %s: %w", dir, err)
	}
	if len(entries) != 0 {
		return fmt.Errorf("the directory %s to unpack into is not empty", dir)
	}
	listing, err := g.output(repo.Root, "ls-tree", []string{"ls-tree", "-r", "-z", "-l", "--full-tree", commit})
	if err != nil {
		return err
	}
	files, err := parseListing(listing)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return nil
	}
	var asked strings.Builder
	for _, file := range files {
		asked.WriteString(file.object + "\n")
	}
	answers, err := g.outputStdin(repo.Root, "cat-file --batch", []byte(asked.String()), []string{"cat-file", "--batch"})
	if err != nil {
		return err
	}
	for _, file := range files {
		var content []byte
		if content, answers, err = nextBlob(answers, file); err != nil {
			return err
		}
		if err := writeFile(dir, file, content); err != nil {
			return err
		}
	}
	if len(answers) != 0 {
		return fmt.Errorf("git cat-file --batch printed %q after the last blob asked for", clip(string(answers)))
	}
	return nil
}

// treeFile is an entry of the listing that is written to the copy: a file or a link.
type treeFile struct {
	mode, object, path string
	size               int64
}

// parseListing reads the NUL-terminated entries `<mode> SP <type> SP <object> SP <size> TAB <path>` of
// `git ls-tree -r -z -l`, drops the submodules and refuses an entry of a mode other than 100644, 100755 and 120000, one
// that is not a blob, a name that is not a full object name, a size that is not a number, and a path that is not local.
func parseListing(listing []byte) ([]treeFile, error) {
	var files []treeFile
	for rest := listing; len(rest) > 0; {
		record, after, ok := bytes.Cut(rest, []byte{0})
		if !ok {
			return nil, fmt.Errorf("git ls-tree printed %q, want entries each ended by a NUL byte", clip(string(rest)))
		}
		rest = after
		meta, p, ok := strings.Cut(string(record), "\t")
		fields := strings.Fields(meta)
		if !ok || len(fields) != 4 || p == "" {
			return nil, fmt.Errorf("git ls-tree printed %q, want <mode> <type> <object> <size> TAB <path>", clip(string(record)))
		}
		mode, kind, object, shown := fields[0], fields[1], fields[2], fields[3]
		switch mode {
		case "160000":
			continue
		case "100644", "100755", "120000":
		default:
			return nil, fmt.Errorf("the tree holds %q with the mode %s, which is neither a file, a link nor a submodule", p, mode)
		}
		size, err := strconv.ParseInt(shown, 10, 64)
		if kind != "blob" || !gitobject.IsName(object) || err != nil || size < 0 {
			return nil, fmt.Errorf("git ls-tree printed %q, want a blob with a full object name and a size", clip(string(record)))
		}
		if !filepath.IsLocal(filepath.FromSlash(path.Clean(p))) {
			return nil, fmt.Errorf("the tree holds %q, a name that leads out of the directory", p)
		}
		files = append(files, treeFile{mode: mode, object: object, path: p, size: size})
	}
	return files, nil
}

// nextBlob takes the answer for file, `<object> blob <size>` LF, the bytes and LF, off the front of answers and returns
// the bytes and what is left. The answer must name the object asked, be a blob, and have the size the listing gives.
func nextBlob(answers []byte, file treeFile) (content, rest []byte, err error) {
	header, rest, ok := bytes.Cut(answers, []byte{'\n'})
	if !ok {
		return nil, nil, fmt.Errorf("git cat-file --batch printed %q for %s, want a header line", clip(string(answers)), file.object)
	}
	want := fmt.Sprintf("%s blob %d", file.object, file.size)
	if string(header) != want {
		return nil, nil, fmt.Errorf("git cat-file --batch printed %q for %q, want %q: the object, the kind or the size is not the one the listing gives", clip(string(header)), file.path, want)
	}
	if int64(len(rest)) < file.size+1 || rest[file.size] != '\n' {
		return nil, nil, fmt.Errorf("git cat-file --batch printed less than the %d bytes and the line feed of %q", file.size, file.path)
	}
	return rest[:file.size], rest[file.size+1:], nil
}

// writeFile writes file, with the bytes content, under dir.
func writeFile(dir string, file treeFile, content []byte) error {
	target := filepath.Join(dir, filepath.FromSlash(path.Clean(file.path)))
	if err := noLinkBelow(dir, filepath.Dir(target)); err != nil {
		return fmt.Errorf("%s: %w", file.path, err)
	}
	var err error
	switch file.mode {
	case "100644":
		err = writeEntry(target, 0o644, bytes.NewReader(content))
	case "100755":
		err = writeEntry(target, 0o755, bytes.NewReader(content))
	default: // 120000: parseListing let nothing else through
		if err = os.MkdirAll(filepath.Dir(target), 0o755); err == nil {
			err = os.Symlink(string(content), target)
		}
	}
	if err != nil {
		return fmt.Errorf("write %s: %w", file.path, err)
	}
	return nil
}

// writeEntry makes the file at target, which must not exist, with the bytes of r.
func writeEntry(target string, perm fs.FileMode, r io.Reader) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm|0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// noLinkBelow refuses a directory below root, up to and including dir, that is a link: a file written through it would
// land outside root.
func noLinkBelow(root, dir string) error {
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return err
	}
	current := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return nil
		case err != nil:
			return err
		case info.Mode()&fs.ModeSymlink != 0:
			return fmt.Errorf("%s is a link, and an entry of the tree would be written through it", current)
		}
	}
	return nil
}

// defaultBranch finds the default branch of the repository in dir and the commits it is at, none when it has none.
// A branch name that origin/HEAD gives is used only when it holds nothing that git would read as a pattern.
func (g Git) defaultBranch(dir string) (string, []string, error) {
	out, err := g.output(dir, "for-each-ref", []string{"for-each-ref", "--format=%(symref)", originRefs + "HEAD"})
	if err != nil {
		return "", nil, err
	}
	candidates := []string{"main", "master"}
	if named, ok := strings.CutPrefix(strings.TrimSuffix(string(out), "\n"), originRefs); ok && named != "" && !strings.ContainsAny(named, "*?[\\\n") {
		candidates = append([]string{named}, candidates...)
	}
	for _, name := range candidates {
		out, err := g.output(dir, "for-each-ref", []string{"for-each-ref", "--format=%(objectname)", "refs/heads/" + name, originRefs + name})
		if err != nil {
			return "", nil, err
		}
		tips, err := commitNames("for-each-ref", out)
		if err != nil {
			return "", nil, err
		}
		if len(tips) > 0 {
			slices.Sort(tips)
			return name, slices.Compact(tips), nil
		}
	}
	return "", nil, nil
}

// commitNames reads out, what git printed for command, as a list of full object names, one on each line, and none
// for no output. A line that is not a full object name is not an answer.
func commitNames(command string, out []byte) ([]string, error) {
	if len(out) == 0 {
		return nil, nil
	}
	var names []string
	for _, line := range strings.Split(strings.TrimSuffix(string(out), "\n"), "\n") {
		if !gitobject.IsName(line) {
			return nil, fmt.Errorf("git %s printed %q, want full object names one on each line", command, clip(line))
		}
		names = append(names, line)
	}
	return names, nil
}

// observer is gitprov over this adapter's runner and the environment as it was given.
func (g Git) observer() gitprov.Observer {
	return gitprov.Observer{Run: g.observe, Environ: g.ambient}
}

// observe is the gitprov.Runner: git in dir under the environment gitprov scrubbed. An error that is not git's
// own answer is returned as a *failure, with the text it always had.
func (g Git) observe(dir string, env []string, args []string) ([]byte, error) {
	ctx, cancel := g.context()
	defer cancel()
	stdout, stderr, err := g.runner.Output(ctx, env, gitProgram, append([]string{"-C", dir}, args...)...)
	if err != nil {
		told := fmt.Errorf("%w%s", err, said(stderr))
		if !answered(ctx, err) {
			return nil, &failure{told}
		}
		return nil, told
	}
	return stdout, nil
}

// failure is the error of a start of git that gave no answer. gitprov words every refusal it makes the same
// way, whether git said the directory is no repository or git never ran, and passes the error of the runner on
// with %w; this type is what lets Worktree tell the two apart afterwards.
type failure struct{ err error }

func (f *failure) Error() string { return f.err.Error() }
func (f *failure) Unwrap() error { return f.err }

// answered says whether err, the error of a start of git that ran under ctx, is an answer of git: it ran to an
// exit status, and its time was not up. A git that could not be started, that a signal ended, or that the
// deadline stopped answered nothing. It must be called before ctx is cancelled. ExitCode is -1 both for a
// process a signal ended and for an ExitError with no process state, so either reads as no answer.
func answered(ctx context.Context, err error) bool {
	var exit *exec.ExitError
	return ctx.Err() == nil && errors.As(err, &exit) && exit.ExitCode() >= 0
}

// output starts git in dir with args under the scrubbed environment and returns what it printed on its
// standard output. label names the command in the error, which wraps the error of the runner and repeats what
// git said.
func (g Git) output(dir, label string, args []string) ([]byte, error) {
	ctx, cancel := g.context()
	defer cancel()
	stdout, stderr, err := g.runner.Output(ctx, g.env, gitProgram, append([]string{"-C", dir}, args...)...)
	if err != nil {
		return nil, fmt.Errorf("git %s: %w%s", label, err, said(stderr))
	}
	return stdout, nil
}

// outputStdin is output for a git that reads its standard input: it is handed exactly stdin.
func (g Git) outputStdin(dir, label string, stdin []byte, args []string) ([]byte, error) {
	ctx, cancel := g.context()
	defer cancel()
	stdout, stderr, err := g.runner.OutputStdin(ctx, g.env, stdin, gitProgram, append([]string{"-C", dir}, args...)...)
	if err != nil {
		return nil, fmt.Errorf("git %s: %w%s", label, err, said(stderr))
	}
	return stdout, nil
}

// context is the context of one start of git, with its deadline.
func (g Git) context() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), g.timeout)
}

// said is what git printed on its standard error as the end of an error message, or nothing.
func said(stderr []byte) string {
	text := strings.TrimSpace(string(stderr))
	if text == "" {
		return ""
	}
	return ": " + clip(text)
}

// clip bounds s to maxSaid bytes of valid text.
func clip(s string) string {
	if len(s) <= maxSaid {
		return s
	}
	return strings.ToValidUTF8(s[:maxSaid], "") + "..."
}
