// Package gitfs answers the port of the shared identity rules (resolve.Repositories) from the
// metadata git keeps on a disk: where the repository a directory belongs to is, the declaration
// file of its main checkout, and its config. It is the one adapter of the identity module that
// touches the file system, and the engine's composition root hands it to the rules (C23, M2).
//
// The repository is read as files and no process is started, so the answer does not depend on a git
// on the machine, on its configuration, or on its refusal to read a repository another user owns.
// It finds the repository by walking up to the nearest .git, which is a directory in a main checkout
// and a file naming the worktree's own git directory in a linked worktree (or a submodule); that
// directory's commondir file points back at the shared one, so a main checkout and every worktree
// of it answer alike. Paths are made absolute and symlink-free where they can be, so the name of a
// main checkout does not depend on the link a working directory was reached through.
//
// A pointer is trusted only as far as the directory it names is a git directory, which has a HEAD:
// a .git file in a repository one has only checked out cannot make the program read the config of a
// directory that is no repository's. Git itself follows the same pointers and checks the same thing.
// A .git file that holds no gitdir line, a pointer that leads to no git directory and a directory
// that cannot be looked into are errors, not "no repository": the rules must not go on to name the
// project by something less than what the repository says.
package gitfs

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/identity/resolve"
)

// Repositories reads repositories from the file system. It holds no state: every question is
// answered from the disk as it is when asked.
type Repositories struct {
	// Stat looks at a file or directory of the repository, and is os.Stat when it is nil: the seam
	// through which a test makes a stat fail in the way a platform's would.
	Stat func(path string) (fs.FileInfo, error)
	// ExplicitFlag is the flag through which the caller's person names the project without the
	// repository being looked at (the engine's --project-id). A refusal ends by saying to give it;
	// with none, it says nothing of a flag.
	ExplicitFlag string
}

var _ resolve.Repositories = Repositories{}

func (g Repositories) stat(path string) (fs.FileInfo, error) {
	if g.Stat != nil {
		return g.Stat(path)
	}
	return os.Stat(path)
}

// Locate is resolve.Repositories.
func (g Repositories) Locate(dir string) (resolve.Repository, error) {
	start := canonical(dir)
	for cur := start; ; {
		dotGit := filepath.Join(cur, ".git")
		info, err := g.stat(dotGit)
		switch {
		case err == nil && info.IsDir():
			// A main checkout: its root is cur, which is known here, and .git is the common directory.
			return resolve.Repository{WorktreeRoot: cur, MainWorktreeRoot: cur, CommonDir: canonical(dotGit)}, nil
		case err == nil:
			// A linked worktree or a submodule. The main checkout's root can only be derived, as the
			// parent of the common directory, which is how git presents it too.
			common, err := g.commonDirOfPointer(dotGit, cur)
			if err != nil {
				return resolve.Repository{}, err
			}
			return resolve.Repository{WorktreeRoot: cur, MainWorktreeRoot: filepath.Dir(common), CommonDir: common}, nil
		case !os.IsNotExist(err):
			// The search goes through every directory above dir, so the one it names is the one that
			// stopped it. Naming the project is the way past it: the rules ask the explicit id first and
			// then never look.
			return resolve.Repository{}, fmt.Errorf("cannot look for a git repository in %s (%w)%s", cur, unwrapPathError(err), g.wayOn(" without looking"))
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return resolve.Repository{}, fmt.Errorf("%w: %s", resolve.ErrNotARepository, start)
		}
		cur = parent
	}
}

// DeclaredProject is resolve.Repositories.
func (g Repositories) DeclaredProject(root string) (string, bool, error) {
	path := filepath.Join(root, resolve.DeclaredFileName)
	raw, err := os.ReadFile(path)
	switch {
	case os.IsNotExist(err):
		return "", false, nil
	case err != nil:
		return "", false, fmt.Errorf("reading %s: %w", path, unwrapPathError(err))
	}
	return string(raw), true, nil
}

// GitConfig is resolve.Repositories.
func (g Repositories) GitConfig(commonDir string) (string, error) {
	path := filepath.Join(commonDir, "config")
	raw, err := os.ReadFile(path)
	switch {
	case os.IsNotExist(err):
		return "", nil
	case err != nil:
		return "", fmt.Errorf("reading %s: %w", path, unwrapPathError(err))
	}
	return string(raw), nil
}

// commonDirOfPointer follows the .git file of a linked worktree or a submodule: its gitdir line
// names the git directory of the worktree, and the commondir file in that directory, relative to
// it, names the shared one. Without a commondir file the git directory is the shared one.
func (g Repositories) commonDirOfPointer(gitFile, root string) (string, error) {
	raw, err := os.ReadFile(gitFile)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", gitFile, unwrapPathError(err))
	}
	rest, ok := strings.CutPrefix(strings.TrimSpace(string(raw)), "gitdir:")
	gitDir := strings.TrimSpace(rest)
	if !ok || gitDir == "" {
		return "", fmt.Errorf("%s is a file and holds no \"gitdir:\" line, so it names no git directory", gitFile)
	}
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(root, gitDir)
	}
	if err := g.requireGitDir(gitFile, gitDir); err != nil {
		return "", err
	}
	pointer := filepath.Join(gitDir, "commondir")
	raw, err = os.ReadFile(pointer)
	switch {
	case os.IsNotExist(err):
		return canonical(gitDir), nil
	case err != nil:
		return "", fmt.Errorf("reading %s: %w", pointer, unwrapPathError(err))
	}
	target := strings.TrimSpace(string(raw))
	if target == "" {
		return canonical(gitDir), nil
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(gitDir, target)
	}
	target = filepath.Clean(target)
	if err := g.requireGitDir(gitFile, target); err != nil {
		return "", err
	}
	return canonical(target), nil
}

// requireGitDir says why dir, which the pointer file names, cannot be the git directory it is
// followed to, by what its HEAD is: missing, a directory, or not to be looked at.
func (g Repositories) requireGitDir(pointerFile, dir string) error {
	info, err := g.stat(filepath.Join(dir, "HEAD"))
	switch {
	case err == nil && info.IsDir():
		return fmt.Errorf("%s points to %s, which is not a git directory (its HEAD is a directory)%s", pointerFile, dir, g.wayOn(""))
	case err == nil:
		return nil
	case os.IsNotExist(err):
		return fmt.Errorf("%s points to %s, which is not a git directory (it has no HEAD; was the worktree or the repository moved or removed?)%s", pointerFile, dir, g.wayOn(""))
	default:
		return fmt.Errorf("%s points to %s, whose HEAD cannot be looked at (%w)%s", pointerFile, dir, unwrapPathError(err), g.wayOn(""))
	}
}

// wayOn is how a refusal ends: the project can be named without looking, through the caller's
// flag. It is empty when the caller has none.
func (g Repositories) wayOn(suffix string) string {
	if g.ExplicitFlag == "" {
		return ""
	}
	return "; give " + g.ExplicitFlag + " to name the project" + suffix
}

// canonical makes p absolute, clean and symlink-free. Where the links cannot be resolved (a
// directory that cannot be looked into, or one that is not there), it is the absolute, clean path,
// and the walk that follows reports what stopped it.
func canonical(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return filepath.Clean(p)
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		return filepath.Clean(real)
	}
	return abs
}

// unwrapPathError is the error of the file system behind the path an *os.PathError names, for a
// message that names the path once.
func unwrapPathError(err error) error {
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return pathErr.Err
	}
	return err
}
