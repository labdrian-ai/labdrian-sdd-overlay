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
	return r.runner.Output(ctx, r.options.Env, "git", append([]string{"-C", dir}, args...)...)
}

// succeeds is whether git exits 0 for the arguments.
func (r Repo) succeeds(dir string, args ...string) bool {
	_, _, err := r.run(dir, args...)
	return err == nil
}

// IsWorkTree reports whether dir is inside the working tree of a repository.
func (r Repo) IsWorkTree(dir string) bool {
	return r.succeeds(dir, "rev-parse", "--is-inside-work-tree")
}

// HasCommit reports whether ref names a commit.
func (r Repo) HasCommit(dir, ref string) bool {
	return r.succeeds(dir, "cat-file", "-e", ref+"^{commit}")
}

// HasPath reports whether the tree of rev holds path.
func (r Repo) HasPath(dir, rev, path string) bool {
	return r.succeeds(dir, "cat-file", "-e", rev+":"+path)
}

// IsAncestor reports whether ancestor is reachable from descendant.
func (r Repo) IsAncestor(dir, ancestor, descendant string) bool {
	return r.succeeds(dir, "merge-base", "--is-ancestor", ancestor, descendant)
}

// Resolve is the id of the commit ref names.
func (r Repo) Resolve(dir, ref string) (string, error) {
	out, _, err := r.run(dir, "rev-parse", ref)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// HasChanges reports whether any of paths has an uncommitted change, tracked or untracked.
func (r Repo) HasChanges(dir string, paths ...string) (bool, error) {
	args := append([]string{"status", "--porcelain", "--untracked-files=all", "--"}, paths...)
	out, _, err := r.run(dir, args...)
	if err != nil {
		return false, err
	}
	return len(strings.TrimSpace(string(out))) > 0, nil
}

// LatestTag is the newest tag matching pattern that is reachable from rev (the checked-out
// commit when rev is empty).
func (r Repo) LatestTag(dir, rev, pattern string) (string, error) {
	args := []string{"describe", "--tags", "--abbrev=0", "--match", pattern}
	if rev != "" {
		args = append(args, rev)
	}
	out, _, err := r.run(dir, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// Export is a tar archive of paths as they are at rev. A failure of git is reported with what it
// said on standard error; a failure to start it, or to finish before the deadline, is told apart.
func (r Repo) Export(dir, rev string, paths []string) ([]byte, error) {
	args := append([]string{"archive", "--format=tar", rev, "--"}, paths...)
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
