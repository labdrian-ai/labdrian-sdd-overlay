package projection

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
)

// RepoLocator is how the use cases find the repository a directory belongs to. The domain owns
// the port; an adapter at the edge (engine/gitfs) answers it by reading the files of the
// repository, and the composition root wires the two. A test of a use case hands it a fake.
type RepoLocator interface {
	// RepoKey returns the key of the repository that holds dir, and whether there is one. The key
	// is RepoKeyOf the git common directory of the repository, so every worktree of one repository
	// has the same key. dir must be absolute; a relative or empty dir, a dir with no repository
	// above it, and a repository entry the adapter cannot use are all ("", false).
	RepoKey(dir string) (key string, ok bool)
}

// RepoKeyOf is the key that names the binding of the repository whose git common directory is
// commonDir: the lowercase hex SHA-256 of the cleaned path, 64 digits, which is what ValidateRepoKey
// accepts. It is the rule and only the rule. Finding the common directory and resolving its
// symbolic links, so that every spelling of one path gives one key, needs the file system and is
// the adapter's; a path that cannot be resolved is passed as it is, and its key then depends on
// that spelling.
func RepoKeyOf(commonDir string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(commonDir)))
	return hex.EncodeToString(sum[:])
}
