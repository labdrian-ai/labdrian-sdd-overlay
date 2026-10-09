// Package gitsource is the adapter of the package builder's SourceRepo port (pipkg): it asks the
// git of the machine, through a Runner, and nothing else. It starts no process itself and reads
// no environment: the Runner starts git, and the environment git sees and the deadline of each
// call are the Options it was made with, chosen by the composition root. It does not import
// pipkg, which only asks for the methods; a test checks that it satisfies the port.
package gitsource

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Runner starts a program and returns its standard output and standard error apart.
// execrunner.Runner is the process adapter; a test hands a fake.
type Runner interface {
	Output(ctx context.Context, env []string, bin string, args ...string) (stdout, stderr []byte, err error)
}

// Options are the choices of the composition root.
type Options struct {
	// Env is the whole environment git runs under. Nil is the environment of the process,
	// which is how git ran before the adapter existed; the caller who wants git to see less
	// (no GIT_DIR from a hook, say) says so here.
	Env []string
	// Timeout stops a git call that runs longer. Zero means no limit, which is how git ran
	// before the adapter existed.
	Timeout time.Duration
}

// Repo is the git of the machine, asked for the repository of a directory.
type Repo struct {
	runner  Runner
	options Options
}

// New returns the adapter over runner.
func New(runner Runner, options Options) Repo { return Repo{runner: runner, options: options} }

// run asks git, in dir, for the subcommand and its arguments.
func (r Repo) run(dir string, args ...string) (stdout, stderr []byte, err error) {
	ctx := context.Background()
	if r.options.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.options.Timeout)
		defer cancel()
	}
	stdout, stderr, err = r.runner.Output(ctx, r.options.Env, "git", append([]string{"-C", dir}, args...)...)
	return stdout, stderr, classify(err)
}

// InvalidRefError is a ref that starts with a dash. git would read it as an option (an
// `--output=<file>` given to `git archive` writes that file), so the adapter refuses it before it
// runs git. The ref is named so the person who typed it can find it.
type InvalidRefError struct {
	Ref string
}

func (e *InvalidRefError) Error() string {
	return fmt.Sprintf("git ref %q starts with %q and would be read as an option", e.Ref, "-")
}

// refs refuses the first of the refs that starts with a dash. An empty ref is not one.
func refs(candidates ...string) error {
	for _, ref := range candidates {
		if strings.HasPrefix(ref, "-") {
			return &InvalidRefError{Ref: ref}
		}
	}
	return nil
}

// unavailableError marks a failure after which git has not answered: it was stopped at the
// deadline, killed by a signal, or could not be started for a reason other than not being
// installed. The package builder surfaces it instead of building as for an unversioned tree.
type unavailableError struct{ err error }

func (e *unavailableError) Error() string     { return e.err.Error() }
func (e *unavailableError) Unwrap() error     { return e.err }
func (e *unavailableError) Unavailable() bool { return true }

// classify marks the failures that are not an answer. An exit status of git is one (the
// predicates read it as "no"), and so is git not being installed, which has always meant the
// tree is not under version control. Anything else is "could not answer".
func classify(err error) error {
	if err == nil || errors.Is(err, exec.ErrNotFound) {
		return err
	}
	var exit interface{ ExitCode() int }
	if errors.As(err, &exit) && exit.ExitCode() >= 0 {
		return err
	}
	return &unavailableError{err: err}
}

// answers is whether git exits 0 for the arguments: yes on 0, no on an exit status or when git
// is not installed, and an error, marked unavailable, when git could not answer.
func (r Repo) answers(dir string, args ...string) (bool, error) {
	_, _, err := r.run(dir, args...)
	if err == nil {
		return true, nil
	}
	var unavailable *unavailableError
	if errors.As(err, &unavailable) {
		return false, err
	}
	return false, nil
}

// IsWorkTree reports whether dir is inside the working tree of a repository.
func (r Repo) IsWorkTree(dir string) (bool, error) {
	return r.answers(dir, "rev-parse", "--is-inside-work-tree")
}

// HasCommit reports whether ref names a commit.
func (r Repo) HasCommit(dir, ref string) (bool, error) {
	if err := refs(ref); err != nil {
		return false, err
	}
	return r.answers(dir, "cat-file", "-e", "--end-of-options", ref+"^{commit}")
}

// HasPath reports whether the tree of rev holds path.
func (r Repo) HasPath(dir, rev, path string) (bool, error) {
	if err := refs(rev); err != nil {
		return false, err
	}
	return r.answers(dir, "cat-file", "-e", "--end-of-options", rev+":"+path)
}

// IsAncestor reports whether ancestor is reachable from descendant.
func (r Repo) IsAncestor(dir, ancestor, descendant string) (bool, error) {
	if err := refs(ancestor, descendant); err != nil {
		return false, err
	}
	return r.answers(dir, "merge-base", "--is-ancestor", "--end-of-options", ancestor, descendant)
}

// Resolve is the id of the commit ref names.
func (r Repo) Resolve(dir, ref string) (string, error) {
	if err := refs(ref); err != nil {
		return "", err
	}
	out, _, err := r.run(dir, "rev-parse", "--verify", "--end-of-options", ref)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// HasChanges reports whether any of paths has an uncommitted change, tracked or untracked. The
// paths follow `--`, which is their terminator.
func (r Repo) HasChanges(dir string, paths ...string) (bool, error) {
	args := append([]string{"status", "--porcelain", "--untracked-files=all", "--"}, paths...)
	out, _, err := r.run(dir, args...)
	if err != nil {
		return false, err
	}
	return len(strings.TrimSpace(string(out))) > 0, nil
}

// LatestTag is the newest tag matching pattern that is reachable from rev (the checked-out
// commit when rev is empty). The pattern is the value of --match.
func (r Repo) LatestTag(dir, rev, pattern string) (string, error) {
	if err := refs(rev); err != nil {
		return "", err
	}
	args := []string{"describe", "--tags", "--abbrev=0", "--match", pattern}
	if rev != "" {
		args = append(args, "--end-of-options", rev)
	}
	out, _, err := r.run(dir, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// Export is a tar archive of paths as they are at rev. A failure of git is reported with what it
// said on standard error; a failure to start it, or to finish before the deadline, is told apart.
// The paths follow the rev, so they are pathspecs and cannot be read as options.
func (r Repo) Export(dir, rev string, paths []string) ([]byte, error) {
	if err := refs(rev); err != nil {
		return nil, err
	}
	args := append([]string{"archive", "--format=tar", "--end-of-options", rev}, paths...)
	out, stderr, err := r.run(dir, args...)
	if err == nil {
		return out, nil
	}
	if !ranAndFailed(err) {
		return nil, fmt.Errorf("starting git archive: %w", err)
	}
	return nil, fmt.Errorf("git archive %s: %w (%s)", rev, err, strings.TrimSpace(string(stderr)))
}

// ranAndFailed is whether err is git having run and ended badly (an exit status, or a deadline
// that stopped it), as against git never having started.
func ranAndFailed(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var exit interface{ ExitCode() int }
	return errors.As(err, &exit)
}
