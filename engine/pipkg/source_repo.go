package pipkg

import "errors"

// SourceRepo is what the package builder asks of the git repository an overlay is checked out
// from: which commit a ref names, what the tree of a commit holds, whether the sources have
// uncommitted changes, and the newest version tag. It is the port the builder owns; the process
// that runs git is an adapter (pipkg/gitsource) the composition root hands in, so the builder
// starts no process and reads no environment.
//
// Every method takes the directory of the overlay and speaks of refs and paths as git does. A
// question that can only be answered yes or no answers no when git cannot be asked at all (not
// installed, not a repository, a deadline): the builder treats an overlay it cannot ask the same
// way whatever the reason, as a tree that is not under version control.
type SourceRepo interface {
	// IsWorkTree reports whether dir is inside the working tree of a repository.
	IsWorkTree(dir string) bool
	// HasCommit reports whether ref names a commit.
	HasCommit(dir, ref string) bool
	// HasPath reports whether the tree of rev holds path.
	HasPath(dir, rev, path string) bool
	// IsAncestor reports whether ancestor is reachable from descendant, a commit being its own
	// ancestor.
	IsAncestor(dir, ancestor, descendant string) bool
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

// ErrNoRepository is what NoRepository answers to every question that has a value.
var ErrNoRepository = errors.New("not a git repository")

// NoRepository is the SourceRepo of a source tree that is not under version control: it answers
// that nothing is a repository. A caller that has no git to offer (a test of another package, a
// tree known to be a plain directory) hands it in; the package builder then builds as it does
// for any overlay that is not a repository: a package without a recorded commit, compared with
// the worktree.
type NoRepository struct{}

func (NoRepository) IsWorkTree(string) bool                 { return false }
func (NoRepository) HasCommit(string, string) bool          { return false }
func (NoRepository) HasPath(string, string, string) bool    { return false }
func (NoRepository) IsAncestor(string, string, string) bool { return false }
func (NoRepository) Resolve(string, string) (string, error) { return "", ErrNoRepository }
func (NoRepository) HasChanges(string, ...string) (bool, error) {
	return false, ErrNoRepository
}
func (NoRepository) LatestTag(string, string, string) (string, error) {
	return "", ErrNoRepository
}
func (NoRepository) Export(string, string, []string) ([]byte, error) {
	return nil, ErrNoRepository
}
