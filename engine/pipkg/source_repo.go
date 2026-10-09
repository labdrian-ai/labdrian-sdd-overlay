package pipkg

import "errors"

// SourceRepo is what the package builder asks of the git repository an overlay is checked out
// from: which commit a ref names, what the tree of a commit holds, whether the sources have
// uncommitted changes, and the newest version tag. It is the port the builder owns; the process
// that runs git is an adapter (pipkg/gitsource) the composition root hands in, so the builder
// starts no process and reads no environment.
//
// Every method takes the directory of the overlay and speaks of refs and paths as git does.
//
// The four questions that have a yes or a no (IsWorkTree, HasCommit, HasPath, IsAncestor) return
// it as a value: git answering no, or not being installed (a machine without git has a tree that
// is not under version control), is a false and no error. They return an error only when git
// could not be asked (stopped at a deadline, killed by a signal, not startable) or the adapter
// refused a ref, and the builder surfaces any such error rather than build as if the overlay had
// no history. The methods that return a value (Resolve, HasChanges, LatestTag, Export) report
// every failure as an error, as git does for an unknown ref or a tag that is not there; the
// failures that are not an answer carry an `Unavailable() bool` method in their chain that
// returns true, and the builder surfaces those and reads the others as "there is none".
type SourceRepo interface {
	// IsWorkTree reports whether dir is inside the working tree of a repository.
	IsWorkTree(dir string) (bool, error)
	// HasCommit reports whether ref names a commit.
	HasCommit(dir, ref string) (bool, error)
	// HasPath reports whether the tree of rev holds path.
	HasPath(dir, rev, path string) (bool, error)
	// IsAncestor reports whether ancestor is reachable from descendant, a commit being its own
	// ancestor.
	IsAncestor(dir, ancestor, descendant string) (bool, error)
	// Resolve is the id of the commit ref names, without surrounding space.
	Resolve(dir, ref string) (string, error)
	// HasChanges reports whether any of paths has an uncommitted change, tracked or untracked.
	HasChanges(dir string, paths ...string) (bool, error)
	// LatestTag is the newest tag matching pattern that is reachable from rev, or from the
	// checked-out commit when rev is empty.
	LatestTag(dir, rev, pattern string) (string, error)
	// Export is a tar archive of paths as they are at rev. A path the commit does not hold
	// makes it fail, so the caller asks HasPath for the ones that may be missing.
	Export(dir, rev string, paths []string) ([]byte, error)
}

// couldNotAnswer is whether err says git was not asked successfully, as against git having
// answered no (see SourceRepo).
func couldNotAnswer(err error) bool {
	var marked interface{ Unavailable() bool }
	return errors.As(err, &marked) && marked.Unavailable()
}

// ErrNoRepository is what NoRepository answers to every question that has a value.
var ErrNoRepository = errors.New("not a git repository")

// NoRepository is the SourceRepo of a source tree that is not under version control: it answers
// that nothing is a repository. A caller that has no git to offer (a test of another package, a
// tree known to be a plain directory) hands it in; the package builder then builds as it does
// for any overlay that is not a repository: a package without a recorded commit, compared with
// the worktree.
type NoRepository struct{}

func (NoRepository) IsWorkTree(string) (bool, error)                 { return false, nil }
func (NoRepository) HasCommit(string, string) (bool, error)          { return false, nil }
func (NoRepository) HasPath(string, string, string) (bool, error)    { return false, nil }
func (NoRepository) IsAncestor(string, string, string) (bool, error) { return false, nil }
func (NoRepository) Resolve(string, string) (string, error)          { return "", ErrNoRepository }
func (NoRepository) HasChanges(string, ...string) (bool, error) {
	return false, ErrNoRepository
}
func (NoRepository) LatestTag(string, string, string) (string, error) {
	return "", ErrNoRepository
}
func (NoRepository) Export(string, string, []string) ([]byte, error) {
	return nil, ErrNoRepository
}
