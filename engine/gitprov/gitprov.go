// Package gitprov observes which git worktree a directory is, as transient
// provenance evidence for a caller that has to say what repository state it
// looked at.
//
// An Observation is transient evidence, not a durable worktree identity: it
// is what git reported at one instant, from one process, and it may be stale
// the moment it is returned. It proves no currentness and grants no
// authority: it never authorizes, admits, or dispatches work, and a caller
// must never persist it as a worktree_id. HEAD is recorded as informational
// provenance only and is deliberately not bound by anything in this package.
//
// Observe fails closed on every ambiguity it can detect: a missing git
// binary, a bare repository, a root that is not the worktree toplevel, a
// repository-redirecting variable in the environment, and a linked worktree
// whose pointers do not agree, and a .git file that names a git dir which
// does not name this toplevel back (a forged or copied gitfile). Git is run with fixed argv and with every
// GIT_* variable removed from its environment, so ambient configuration such
// as GIT_CONFIG_* or GIT_CEILING_DIRECTORIES cannot steer discovery.
package gitprov

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// Observation is the worktree provenance git reported for one root. Every
// path is absolute with symlinks resolved.
type Observation struct {
	// Toplevel is the worktree toplevel; it always equals the resolved root.
	Toplevel string
	// GitDir is the per-worktree git directory.
	GitDir string
	// CommonDir is the git directory shared by all worktrees of the
	// repository; it equals GitDir for a main worktree.
	CommonDir string
	// Head is the full object name HEAD resolved to. Informational only: it
	// is not bound and proves nothing about what is checked out now.
	Head string
	// Linked reports whether the root is a linked worktree (GitDir differs
	// from CommonDir).
	Linked bool
}

// Runner runs git with args in dir under exactly env and returns its
// standard output. It is injected so tests never need a real git binary.
type Runner func(dir string, env []string, args []string) ([]byte, error)

// Observer observes worktree provenance through an injected Runner. Environ
// is the ambient environment Observe inspects and scrubs before handing it to
// Run; it is typically os.Environ().
type Observer struct {
	Run     Runner
	Environ []string
}

// redirectingVars name environment variables that point git at a repository
// other than the one discovered from the root. Their presence means the
// ambient process believes a different repository is in play, so Observe
// refuses instead of silently scrubbing them.
var redirectingVars = []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR"}

// The fixed argv Observe runs, in order. Nothing caller-supplied is ever
// appended to them.
var (
	argvIsBare     = []string{"rev-parse", "--is-bare-repository"}
	argvInWorkTree = []string{"rev-parse", "--is-inside-work-tree"}
	argvToplevel   = []string{"rev-parse", "--show-toplevel"}
	argvGitDir     = []string{"rev-parse", "--absolute-git-dir"}
	argvCommonDir  = []string{"rev-parse", "--git-common-dir"}
	argvHead       = []string{"rev-parse", "--verify", "HEAD^{commit}"}
	// argvCoreWorktree reads only the repository's own config file; --local
	// also leaves include.* directives unfollowed.
	argvCoreWorktree = []string{"config", "--local", "--get", "core.worktree"}
)

var objectName = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

// Observe observes root with the real git binary found on PATH and the
// process environment. See Observer.Observe.
func Observe(root string) (Observation, error) {
	return Observer{Run: ExecRunner, Environ: os.Environ()}.Observe(root)
}

// ExecRunner is the production Runner: it executes the git binary found on
// PATH. An empty env stays empty; it never falls back to the process
// environment.
func ExecRunner(dir string, env []string, args []string) ([]byte, error) {
	bin, err := exec.LookPath("git")
	if err != nil {
		return nil, fmt.Errorf("git binary not found: %w", err)
	}
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = append([]string{}, env...)
	return cmd.Output()
}

// Observe reports the worktree provenance of root, which must be absolute and
// must resolve to exactly the worktree toplevel. The first failed check wins
// and no partial Observation is returned alongside an error.
func (o Observer) Observe(root string) (Observation, error) {
	if o.Run == nil {
		return Observation{}, fmt.Errorf("no git runner configured")
	}
	if root == "" || !filepath.IsAbs(root) {
		return Observation{}, fmt.Errorf("root %q is not an absolute path", root)
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return Observation{}, fmt.Errorf("resolve root %q: %w", root, err)
	}
	env, err := scrubEnv(o.Environ)
	if err != nil {
		return Observation{}, err
	}
	run := func(argv []string) (string, error) {
		out, err := o.Run(resolvedRoot, env, argv)
		label := "git " + strings.Join(argv, " ")
		if err != nil {
			if errors.Is(err, exec.ErrNotFound) {
				return "", fmt.Errorf("%s: git binary not found: %w", label, err)
			}
			return "", fmt.Errorf("%s: %w", label, err)
		}
		return singleLine(label, out)
	}

	isBare, err := run(argvIsBare)
	if err != nil {
		return Observation{}, err
	}
	if isBare != "false" {
		return Observation{}, fmt.Errorf("%s is a bare repository or reported %q", resolvedRoot, isBare)
	}
	inWorkTree, err := run(argvInWorkTree)
	if err != nil {
		return Observation{}, err
	}
	if inWorkTree != "true" {
		return Observation{}, fmt.Errorf("%s is not inside a work tree (reported %q)", resolvedRoot, inWorkTree)
	}

	toplevel, err := runPath(run, argvToplevel, "")
	if err != nil {
		return Observation{}, err
	}
	if toplevel != resolvedRoot {
		return Observation{}, fmt.Errorf("root %s is not the worktree toplevel %s", resolvedRoot, toplevel)
	}
	gitDir, err := runPath(run, argvGitDir, "")
	if err != nil {
		return Observation{}, err
	}
	commonDir, err := runPath(run, argvCommonDir, resolvedRoot)
	if err != nil {
		return Observation{}, err
	}
	dotGitIsFile, err := checkDotGit(toplevel, gitDir)
	if err != nil {
		return Observation{}, err
	}
	linked := gitDir != commonDir
	switch {
	case linked:
		if err := checkLinked(toplevel, gitDir, commonDir); err != nil {
			return Observation{}, err
		}
	case dotGitIsFile:
		if err := checkCoreWorktree(run, toplevel, gitDir); err != nil {
			return Observation{}, err
		}
	}

	head, err := run(argvHead)
	if err != nil {
		return Observation{}, fmt.Errorf("HEAD does not name a commit: %w", err)
	}
	if !objectName.MatchString(head) {
		return Observation{}, fmt.Errorf("HEAD %q is not a full lowercase object name", head)
	}

	return Observation{Toplevel: toplevel, GitDir: gitDir, CommonDir: commonDir, Head: head, Linked: linked}, nil
}

// scrubEnv refuses a repository-redirecting variable and otherwise returns
// environ with every GIT_* entry removed.
func scrubEnv(environ []string) ([]string, error) {
	env := []string{}
	for _, kv := range environ {
		name, _, _ := strings.Cut(kv, "=")
		for _, v := range redirectingVars {
			if name == v {
				return nil, fmt.Errorf("refusing to observe with %s set in the environment", v)
			}
		}
		if strings.HasPrefix(name, "GIT_") {
			continue
		}
		env = append(env, kv)
	}
	return env, nil
}

// singleLine returns out without its one trailing newline, refusing empty or
// multi-line output.
func singleLine(label string, out []byte) (string, error) {
	s := strings.TrimSuffix(string(out), "\n")
	if s == "" {
		return "", fmt.Errorf("%s: empty output", label)
	}
	if strings.ContainsAny(s, "\r\n") {
		return "", fmt.Errorf("%s: output is not a single line", label)
	}
	return s, nil
}

// runPath runs argv and resolves its single-line output to an absolute,
// symlink-free path. When base is empty the output must already be absolute;
// otherwise a relative output is joined to base.
func runPath(run func([]string) (string, error), argv []string, base string) (string, error) {
	p, err := run(argv)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(p) {
		if base == "" {
			return "", fmt.Errorf("git %s: %q is not an absolute path", strings.Join(argv, " "), p)
		}
		p = filepath.Join(base, p)
	}
	resolved, err := filepath.EvalSymlinks(p)
	if err != nil {
		return "", fmt.Errorf("git %s: resolve %q: %w", strings.Join(argv, " "), p, err)
	}
	return resolved, nil
}

// checkDotGit requires toplevel/.git to lead to gitDir: either it is that
// directory, or it is a "gitdir: <path>" file naming it. It reports whether
// .git is a file, because a gitfile only proves where it points, not that
// the git dir it names belongs to this toplevel: the caller must still
// establish that from the git dir's side.
func checkDotGit(toplevel, gitDir string) (bool, error) {
	dotGit := filepath.Join(toplevel, ".git")
	info, err := os.Lstat(dotGit)
	if err != nil {
		return false, fmt.Errorf("inspect %s: %w", dotGit, err)
	}
	var target string
	isFile := false
	switch {
	case info.IsDir():
		target = dotGit
	case info.Mode().IsRegular():
		isFile = true
		target, err = readPointer(dotGit, "gitdir: ", toplevel)
		if err != nil {
			return false, err
		}
	default:
		return false, fmt.Errorf("%s is neither a directory nor a regular file", dotGit)
	}
	resolved, err := filepath.EvalSymlinks(target)
	if err != nil {
		return false, fmt.Errorf("resolve %s target %q: %w", dotGit, target, err)
	}
	if resolved != gitDir {
		return false, fmt.Errorf("%s leads to %s, not the reported git dir %s", dotGit, resolved, gitDir)
	}
	return isFile, nil
}

// checkCoreWorktree accepts a non-linked gitfile layout (a separate git dir
// or a submodule) only when the git dir's own core.worktree names toplevel.
// Without it, any directory holding a hand-written gitfile would pass as the
// worktree of the git dir it names, so an unset, unreadable, or mismatched
// core.worktree is refused. A relative value is resolved against gitDir, as
// git does.
func checkCoreWorktree(run func([]string) (string, error), toplevel, gitDir string) error {
	dotGit := filepath.Join(toplevel, ".git")
	value, err := run(argvCoreWorktree)
	if err != nil {
		return fmt.Errorf("%s is a gitfile naming non-linked git dir %s, whose core.worktree is not usable: %w", dotGit, gitDir, err)
	}
	if !filepath.IsAbs(value) {
		value = filepath.Join(gitDir, value)
	}
	resolved, err := filepath.EvalSymlinks(value)
	if err != nil {
		return fmt.Errorf("resolve core.worktree %q of %s: %w", value, gitDir, err)
	}
	if resolved != toplevel {
		return fmt.Errorf("gitfile mismatch: core.worktree of %s names %s, not %s", gitDir, resolved, toplevel)
	}
	return nil
}

// checkLinked requires a linked worktree's git dir to sit directly under
// commonDir/worktrees and its gitdir back-pointer to name toplevel/.git.
func checkLinked(toplevel, gitDir, commonDir string) error {
	if filepath.Dir(gitDir) != filepath.Join(commonDir, "worktrees") {
		return fmt.Errorf("linked git dir %s is not under %s", gitDir, filepath.Join(commonDir, "worktrees"))
	}
	backFile := filepath.Join(gitDir, "gitdir")
	back, err := readPointer(backFile, "", gitDir)
	if err != nil {
		return err
	}
	resolvedBack, err := filepath.EvalSymlinks(back)
	if err != nil {
		return fmt.Errorf("resolve %s target %q: %w", backFile, back, err)
	}
	if want := filepath.Join(toplevel, ".git"); resolvedBack != want {
		return fmt.Errorf("linked worktree mismatch: %s names %s, not %s", backFile, resolvedBack, want)
	}
	return nil
}

// readPointer reads a one-line pointer file, strips the required prefix, and
// resolves a relative target against base. It refuses anything that is not a
// regular file before opening it, so a symlink or a FIFO with no writer
// cannot make it block forever instead of failing closed.
func readPointer(path, prefix, base string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("inspect %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s is not a regular file", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	line, err := singleLine(path, data)
	if err != nil {
		return "", err
	}
	target, ok := strings.CutPrefix(line, prefix)
	if !ok || target == "" {
		return "", fmt.Errorf("%s: malformed pointer %q", path, line)
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(base, target)
	}
	return target, nil
}
