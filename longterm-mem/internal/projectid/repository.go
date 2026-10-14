package projectid

// Repository is where the repository a directory belongs to is, in the words the identity rules need. The
// inspector that answers for it has made every path absolute, cleaned and symlink-free, which is what keeps a
// main checkout and each of its linked worktrees from resolving to different identities.
type Repository struct {
	// WorktreeRoot is the root of the working tree containing the queried directory: the main checkout, or one
	// linked worktree.
	WorktreeRoot string
	// MainWorktreeRoot is the main checkout's root. It is empty when the repository has none the inspector can
	// name (a bare repository).
	MainWorktreeRoot string
	// CommonDir is the git common directory: the one directory a main checkout and every linked worktree share.
	CommonDir string
}

// RepositoryInspector is what identity resolution asks of the repository a directory belongs to: where it is,
// the declaration its main checkout carries, and its git configuration. The rules own this port and the words
// of it; where the answers come from (the metadata git keeps on a disk, a table in a test) is the adapter's,
// which the composition root chooses. The rules themselves touch no file and run no process.
type RepositoryInspector interface {
	// Locate finds the repository dir belongs to. An error that satisfies errors.Is(err, ErrNotARepository)
	// says that dir is in none; any other error says that the repository could not be read.
	Locate(dir string) (Repository, error)
	// DeclaredProject returns the text of the file named DeclaredFileName in root, and whether it is there.
	// The error says why it could not be read when it is there but unreadable.
	DeclaredProject(root string) (text string, found bool, err error)
	// GitConfig returns the text of the git configuration file of the repository whose common directory is
	// commonDir. It is empty when the file cannot be read: a repository whose configuration cannot be read
	// simply has no remote to derive a name from.
	GitConfig(commonDir string) string
}

// Resolver applies the identity rules to the repositories its inspector answers for.
type Resolver struct {
	repositories RepositoryInspector
}

// NewResolver returns the rules reading repositories through inspector.
func NewResolver(inspector RepositoryInspector) Resolver {
	return Resolver{repositories: inspector}
}
