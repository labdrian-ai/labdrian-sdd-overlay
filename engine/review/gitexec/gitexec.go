// Package gitexec is the adapter of the Git port of review/app: it asks the git program of the machine, through
// a Runner, the questions `labdrian review bind` and `labdrian review record` put about a repository. It
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
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/gitobject"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/gitprov"
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

// Runner starts a program with exactly the environment it is given and returns what it printed on its standard
// output and on its standard error, apart. The process adapter is execrunner.Runner.
type Runner interface {
	Output(ctx context.Context, env []string, bin string, args ...string) (stdout, stderr []byte, err error)
}

// Options are what the adapter needs of the process it runs in.
type Options struct {
	// Environ is the environment of the process, os.Environ() in production. Git is started under it with
	// every variable of git removed; Locate refuses the ones that redirect git to another repository.
	Environ []string
	// Timeout is how long one start of git may run; zero is no limit.
	Timeout time.Duration
}

// Git implements app.Git over a Runner. The zero value has no Runner and cannot be used; New makes one.
type Git struct {
	runner  Runner
	ambient []string // the environment as it was given: Locate looks for what redirects git
	env     []string // the environment git is started under
	timeout time.Duration
}

// New is the adapter over runner under the environment and the deadline of the options.
func New(runner Runner, o Options) Git {
	return Git{runner: runner, ambient: append([]string(nil), o.Environ...), env: gitEnvironment(o.Environ), timeout: o.Timeout}
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

// emptyTree is the empty tree of the hash function the name head is written with.
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
	obs, err := g.observer().Locate(path)
	if err != nil {
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

// observer is gitprov over this adapter's runner and the environment as it was given.
func (g Git) observer() gitprov.Observer {
	return gitprov.Observer{Run: g.observe, Environ: g.ambient}
}

// observe is the gitprov.Runner: git in dir under the environment gitprov scrubbed.
func (g Git) observe(dir string, env []string, args []string) ([]byte, error) {
	ctx, cancel := g.context()
	defer cancel()
	stdout, stderr, err := g.runner.Output(ctx, env, gitProgram, append([]string{"-C", dir}, args...)...)
	if err != nil {
		return nil, fmt.Errorf("%w%s", err, said(stderr))
	}
	return stdout, nil
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

// context is the context of one start of git: with the deadline of the options when there is one.
func (g Git) context() (context.Context, context.CancelFunc) {
	if g.timeout > 0 {
		return context.WithTimeout(context.Background(), g.timeout)
	}
	return context.WithCancel(context.Background())
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
