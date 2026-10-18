package projectid

import "github.com/labdrian-ai/labdrian-sdd-overlay/identity/resolve"

// Repository is where the repository a directory belongs to is, in the words of the shared identity rules
// (identity/resolve). The adapter that answers for it has made every path absolute, cleaned and, where it can,
// symlink-free, which is what keeps a main checkout and each of its linked worktrees from resolving to different
// identities.
type Repository = resolve.Repository

// RepositoryInspector is what identity resolution asks of the repository a directory belongs to: where it is,
// the declaration its main checkout carries, and its git configuration. It is the port of the shared rules
// (resolve.Repositories, C23 M2), so the engine and this module read a repository through the same questions;
// where the answers come from (the metadata git keeps on a disk, a table in a test) is the adapter's, which the
// composition root chooses. The rules themselves touch no file and run no process.
type RepositoryInspector = resolve.Repositories

// Resolver applies the shared identity rules to the repositories its inspector answers for, and adds what this
// module needs on top of them: the correspondence of a given name, and adoption.
type Resolver struct {
	repositories RepositoryInspector
	rules        resolve.Resolver
}

// NewResolver returns the rules reading repositories through inspector.
func NewResolver(inspector RepositoryInspector) Resolver {
	return Resolver{repositories: inspector, rules: resolve.New(inspector)}
}
