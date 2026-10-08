package skills

import (
	"fmt"
	"io/fs"
)

// projectDirMode is the mode of every directory a project registration
// creates. design.md legislates the FILE mode (ProjectFileMode, 0644) but
// never a directory mode, and the only MkdirAll in this package (install.go)
// derives its mode from a source directory this capability does not have. 0755
// is chosen because a skill directory must be traversable by the agent
// runtimes that read it, and because it is what the umask-free default of a
// freshly cloned checkout looks like.
const projectDirMode fs.FileMode = 0o755

// ProjectFS is the port through which the skills domain reads and writes the files of a project
// and of an overlay, and the seam a test injects a failure at, at any single call. It is owned
// here and implemented by engine/skills/skillsfs. The method set is what the stage, commit and
// rollback of an execution need: MkdirAll for the target directories, a same-directory temp
// write for staging, Rename for the commit, Remove for leftover temps and created directories,
// Stat to learn which directories a run created, ReadDir to prove a created directory is empty
// before removing it, and ResolvePath for the check-then-act containment proof that a plan's
// point-in-time proof defers to the moment of writing.
type ProjectFS interface {
	Stat(name string) (fs.FileInfo, error)
	ReadDir(name string) ([]fs.DirEntry, error)
	MkdirAll(dir string, perm fs.FileMode) error
	// The staged writes of a file: WriteTemp, Rename and Remove.
	StagedWrites
	ResolvePath(name string) (string, error)
}

// writeProjectTemp stages data in a temporary file in dir through fsys, and words a failure as the
// executors of the project verbs always have: 'writeProjectTemp: <the step it failed at>: <the
// cause>'. The port says the step (see ProjectFS.WriteTemp); the verb's own words are the domain's.
func writeProjectTemp(fsys ProjectFS, dir string, data []byte, perm fs.FileMode) (string, error) {
	tmp, err := fsys.WriteTemp(dir, data, perm)
	if err != nil {
		return tmp, fmt.Errorf("writeProjectTemp: %w", err)
	}
	return tmp, nil
}
