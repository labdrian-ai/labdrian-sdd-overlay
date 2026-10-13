// Package projectidfs answers the questions the identity rules (internal/projectid) ask of a repository, from
// the metadata git keeps on a disk: where the repository is, the declaration file of its main checkout, and its
// git configuration.
//
// It never shells out: production code in this module may not import os/exec (see exec_allowlist_test.go,
// R-021), so every fact is read from the repository's own files instead of from git. The rules own the port
// (projectid.RepositoryInspector) and this adapter satisfies it; the composition root wires the two.
//
// The errors it returns keep the "projectid:" prefix the rules have always printed: they are what an operator
// reads when the working directory is no repository, and they are part of what the commands say.
package projectidfs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/projectid"
)

// Inspector reads repositories from the file system. It holds nothing: every question is answered from the
// disk as it is when asked.
type Inspector struct{}

var _ projectid.RepositoryInspector = Inspector{}

// Locate walks up from dir to the nearest .git entry and resolves the working tree root and the git common
// directory. A directory with no .git above it is in no repository (projectid.ErrNotARepository).
func (Inspector) Locate(dir string) (projectid.Repository, error) {
	// The queried directory is canonicalized itself, so a repository reached through a symlinked parent resolves
	// like the real path. (An empty directory is the working directory, which is what making it absolute does.)
	start, err := canonicalDir(dir)
	if err != nil {
		return projectid.Repository{}, err
	}

	for cur := start; ; {
		dotGit := filepath.Join(cur, ".git")
		// os.Stat, not os.Lstat: a `.git` that is a SYMLINK to the real
		// git directory is a layout git supports, and lstat reports it as
		// neither a directory nor a regular file. Lstat sent it down the
		// linked-worktree branch, which parsed a directory for a
		// "gitdir:" line and failed the repository outright. Following
		// the link costs nothing here -- the common dir is realpathed
		// either way -- and a dangling link still falls through to the
		// IsNotExist walk, exactly as no .git at all does.
		info, err := os.Stat(dotGit)
		switch {
		case err == nil && info.IsDir():
			// Main checkout: .git is the common dir itself.
			common, err := canonicalDir(dotGit)
			if err != nil {
				return projectid.Repository{}, err
			}
			// The main checkout's root is cur, which is KNOWN here --
			// not filepath.Dir(common), which merely happens to equal it
			// while the git directory sits inside the working tree. With
			// a symlinked or relocated .git it does not, and deriving it
			// pointed the declared-file read at a directory that is not
			// the repository at all.
			return projectid.Repository{WorktreeRoot: cur, MainWorktreeRoot: cur, CommonDir: common}, nil
		case err == nil:
			// Linked worktree (or a submodule): .git is a file naming the
			// worktree's own git dir, whose commondir file points back at
			// the shared one -- relatively, which is the trap.
			//
			// Here the main checkout's root can only be DERIVED, as the
			// parent of the common dir, which is how git presents it too.
			// A repository whose git directory has been relocated out of
			// its working tree therefore has no main root a linked
			// worktree can find; the declared rule then finds no file and
			// the chain falls through to a derived rule, which still
			// answers identically from every worktree. Wrong-but-shared
			// beats right-but-fragmented, and only the declared rule is
			// lost.
			common, err := commonDirFromGitFile(dotGit, cur)
			if err != nil {
				return projectid.Repository{}, err
			}
			return projectid.Repository{WorktreeRoot: cur, MainWorktreeRoot: filepath.Dir(common), CommonDir: common}, nil
		case !os.IsNotExist(err):
			return projectid.Repository{}, fmt.Errorf("projectid: inspecting %s: %w", dotGit, err)
		}

		parent := filepath.Dir(cur)
		if parent == cur {
			return projectid.Repository{}, fmt.Errorf("%w: %s", projectid.ErrNotARepository, start)
		}
		cur = parent
	}
}

// DeclaredProject reads the file that names the project from root. A file that is not there is not an error.
func (Inspector) DeclaredProject(root string) (string, bool, error) {
	path := filepath.Join(root, projectid.DeclaredFileName)
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("projectid: reading %s: %w", path, err)
	}
	return string(raw), true, nil
}

// GitConfig reads the configuration file of the repository whose common directory is commonDir. A file that
// cannot be read reads as empty: such a repository has no remote to derive a name from.
func (Inspector) GitConfig(commonDir string) string {
	raw, err := os.ReadFile(filepath.Join(commonDir, "config"))
	if err != nil {
		return ""
	}
	return string(raw)
}

// commonDirFromGitFile reads a linked worktree's .git file and returns the
// canonical common dir it ultimately points at.
func commonDirFromGitFile(gitFile, worktreeRoot string) (string, error) {
	raw, err := os.ReadFile(gitFile)
	if err != nil {
		return "", fmt.Errorf("projectid: reading %s: %w", gitFile, err)
	}
	line := strings.TrimSpace(string(raw))
	rest, ok := strings.CutPrefix(line, "gitdir:")
	if !ok {
		return "", fmt.Errorf("projectid: %s does not name a git directory: %w", gitFile, projectid.ErrNotARepository)
	}
	gitDir := strings.TrimSpace(rest)
	if gitDir == "" {
		return "", fmt.Errorf("projectid: %s names an empty git directory: %w", gitFile, projectid.ErrNotARepository)
	}
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(worktreeRoot, gitDir)
	}

	// commondir is git's own pointer back to the shared directory, and it
	// is written relative to gitDir ("../.."). Joining it and leaving the
	// ".." segments in place is exactly the fragmenting mistake; canonical
	// resolution below cleans and realpaths it.
	commonPointer := filepath.Join(gitDir, "commondir")
	if raw, err := os.ReadFile(commonPointer); err == nil {
		target := strings.TrimSpace(string(raw))
		if target != "" {
			if !filepath.IsAbs(target) {
				target = filepath.Join(gitDir, target)
			}
			return canonicalDir(target)
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("projectid: reading %s: %w", commonPointer, err)
	}

	// No commondir file: this git dir IS the common dir (a submodule, or a
	// plain worktree-less .git file).
	return canonicalDir(gitDir)
}

// canonicalDir makes p absolute, cleaned and symlink-free. Skipping any one
// of the three reproduces the measured fragmentation.
func canonicalDir(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("projectid: resolving %s: %w", p, err)
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("projectid: resolving %s: %w", abs, err)
	}
	return filepath.Clean(real), nil
}
