// Package projectidentity is the adapter of the skills domain to the sources that know which
// project a directory is. It implements the port engine/skills owns, skills.ProjectIdentity, as
// four small values the program chains in engine/cmd, in the order the owner chose (Phase 9,
// decision Q8): Explicit, what the person said with --project-id; GitOrigin, what the repository
// says of itself; DirectoryName, what the directory is called; and Chain, which asks them in
// turn.
//
// The rule that turns a remote url into a name is not here: it is the identity module's, shared
// with longterm-mem (decision D2), and GitOrigin only finds and reads the file it applies to. The
// repository is read as files and no process is started, so the answer does not depend on a git
// on the machine, on its configuration, or on its refusal to read a repository another user owns.
//
// It imports the domain and the identity module and nothing else of the program; the dependency
// points from the adapter to the domain, never back.
package projectidentity

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
	"github.com/labdrian-ai/labdrian-sdd-overlay/identity"
)

// Explicit answers with the id the person gave, and has no answer when they gave none.
type Explicit struct{}

// Identify is skills.ProjectIdentity.
func (Explicit) Identify(q skills.ProjectQuery) (skills.ProjectID, bool, error) {
	return q.Explicit, q.Explicit != "", nil
}

// DirectoryName answers with the name of the directory, whatever else was given. It is the last
// resort of a chain: it always has an answer.
type DirectoryName struct{}

// Identify is skills.ProjectIdentity.
func (DirectoryName) Identify(q skills.ProjectQuery) (skills.ProjectID, bool, error) {
	return skills.ProjectID(filepath.Base(q.Dir)), true, nil
}

// GitOrigin answers with the origin remote of the repository the directory is in, reduced to
// "host/path" by the identity module: git@github.com:acme/demo.git and https://github.com/acme/demo
// are both github.com/acme/demo. It finds the repository by walking up to the nearest .git, which
// is a directory in a main checkout and a file naming the worktree's own git directory in a linked
// worktree (or a submodule); the worktree's commondir file points back at the directory that holds
// the config, so a main checkout and every worktree of it answer alike.
//
// It has no answer where there is no repository, no config, no origin, or an origin with no host
// to key on (a path on the machine). A file it cannot read, a .git file that holds no gitdir line,
// a pointer that leads to no git directory (a worktree or a submodule that moved or was removed)
// and a directory it cannot look into are errors: the source could not tell, and a chain must not
// go on to name the project by something less than what the repository says.
//
// A pointer is trusted only as far as the directory it names is a git directory, which has a HEAD:
// a .git file in a repository one has only checked out cannot make the program read the config of
// a directory that is no repository's. Git itself follows the same pointers and checks the same
// thing.
type GitOrigin struct{}

// Identify is skills.ProjectIdentity.
func (GitOrigin) Identify(q skills.ProjectQuery) (skills.ProjectID, bool, error) {
	commonDir, found, err := commonGitDir(q.Dir)
	if err != nil || !found {
		return "", false, err
	}
	path := filepath.Join(commonDir, "config")
	raw, err := os.ReadFile(path)
	switch {
	case os.IsNotExist(err):
		return "", false, nil
	case err != nil:
		return "", false, fmt.Errorf("reading %s: %w", path, unwrapPathError(err))
	}
	remote, ok := identity.OriginRemote(string(raw))
	if !ok {
		return "", false, nil
	}
	return skills.ProjectID(remote), true, nil
}

// commonGitDir finds the git directory that holds the config of the repository dir is in, and
// reports false when dir is in none.
func commonGitDir(dir string) (string, bool, error) {
	for cur := dir; ; {
		dotGit := filepath.Join(cur, ".git")
		info, err := os.Stat(dotGit)
		switch {
		case err == nil && info.IsDir():
			return dotGit, true, nil
		case err == nil:
			return commonDirOfPointer(dotGit, cur)
		case !os.IsNotExist(err):
			// The search goes through every directory above dir, so the one it names is the one
			// that stopped it. Naming the project is the way past it: the chain asks the person's
			// id first and then never looks.
			return "", false, fmt.Errorf("cannot look for a git repository in %s (%w); give --project-id to name the project without looking", cur, unwrapPathError(err))
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", false, nil
		}
		cur = parent
	}
}

// commonDirOfPointer follows the .git file of a linked worktree or a submodule: its gitdir line
// names the git directory of the worktree, and the commondir file in that directory, relative to
// it, names the shared one. Without a commondir file the git directory is the shared one.
func commonDirOfPointer(gitFile, root string) (string, bool, error) {
	raw, err := os.ReadFile(gitFile)
	if err != nil {
		return "", false, fmt.Errorf("reading %s: %w", gitFile, unwrapPathError(err))
	}
	rest, ok := strings.CutPrefix(strings.TrimSpace(string(raw)), "gitdir:")
	gitDir := strings.TrimSpace(rest)
	if !ok || gitDir == "" {
		return "", false, fmt.Errorf("%s is a file and holds no \"gitdir:\" line, so it names no git directory", gitFile)
	}
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(root, gitDir)
	}
	if err := requireGitDir(gitFile, gitDir); err != nil {
		return "", false, err
	}
	pointer := filepath.Join(gitDir, "commondir")
	raw, err = os.ReadFile(pointer)
	switch {
	case os.IsNotExist(err):
		return gitDir, true, nil
	case err != nil:
		return "", false, fmt.Errorf("reading %s: %w", pointer, unwrapPathError(err))
	}
	target := strings.TrimSpace(string(raw))
	if target == "" {
		return gitDir, true, nil
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(gitDir, target)
	}
	target = filepath.Clean(target)
	if err := requireGitDir(gitFile, target); err != nil {
		return "", false, err
	}
	return target, true, nil
}

// requireGitDir says why dir, which the pointer file names, cannot be the git directory it is
// followed to: it is not one (a git directory has a HEAD, and one that is gone has none), or it
// cannot be looked at. The way on is the same as for a directory that cannot be searched: name the
// project.
func requireGitDir(pointerFile, dir string) error {
	info, err := os.Stat(filepath.Join(dir, "HEAD"))
	switch {
	case err == nil && !info.IsDir():
		return nil
	case err == nil || os.IsNotExist(err):
		return fmt.Errorf("%s points to %s, which is not a git directory (it has no HEAD; was the worktree or the repository moved or removed?); give --project-id to name the project", pointerFile, dir)
	}
	return fmt.Errorf("%s points to %s, which cannot be looked at (%w); give --project-id to name the project", pointerFile, dir, unwrapPathError(err))
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

// Chain asks its sources in order and answers with the first answer there is. A source that
// cannot tell stops the chain with its error; none having an answer is no answer, and the verb
// that asked says it could not name the project.
func Chain(sources ...skills.ProjectIdentity) skills.ProjectIdentity {
	return chain(append([]skills.ProjectIdentity(nil), sources...))
}

type chain []skills.ProjectIdentity

// Identify is skills.ProjectIdentity.
func (c chain) Identify(q skills.ProjectQuery) (skills.ProjectID, bool, error) {
	for _, source := range c {
		id, ok, err := source.Identify(q)
		if err != nil {
			return "", false, err
		}
		if ok {
			return id, true, nil
		}
	}
	return "", false, nil
}
