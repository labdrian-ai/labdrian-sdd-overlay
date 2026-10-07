package skills

import "path/filepath"

// ProjectLockStore is where the project lock is read from: the file that records which skills a
// project has, the procedural ones that `project-register` wrote and the install records of
// `skills install` and `adopt`. The domain decides where it lives (ProjectLockPath) and what a
// missing or unreadable one means; the store only reads. Every verb that reads a project's lock
// reads it through here, so a test or another medium can stand where the files of a project stand.
//
// What a verb writes back goes through ProjectFS with the plan that carries it, because the lock
// is the commit marker of the plan and is staged and renamed with the files it covers.
type ProjectLockStore interface {
	// ReadLock reads the lock of the project whose root directory is root. An error that is
	// fs.ErrNotExist means the project has no lock yet, which is an answer for the verbs that
	// create it; any other means the lock could not be read, and a verb refuses rather than
	// replace what it cannot read with an empty lock, which would disown everything recorded in it.
	ReadLock(root string) ([]byte, error)
}

// ProjectLockPath returns where the lock of the project whose root directory is root lives.
func ProjectLockPath(root string) string {
	return filepath.Join(root, filepath.FromSlash(ProjectLockRelPath))
}
