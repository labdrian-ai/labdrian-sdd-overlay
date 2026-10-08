package skillsfs

import (
	"fmt"
	"io/fs"
	"os"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/pathguard/fsresolve"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// Project is the skills.ProjectFS of the real file system: the files of a project that `skills
// install`, `adopt` and the project verbs read and write, and the files of an overlay that its
// verbs replace. It holds nothing: the zero value is ready.
type Project struct{}

var _ skills.ProjectFS = Project{}

// Stat describes name, following links.
func (Project) Stat(name string) (fs.FileInfo, error) { return os.Stat(name) }

// ReadDir lists the directory name, sorted by file name.
func (Project) ReadDir(name string) ([]fs.DirEntry, error) { return os.ReadDir(name) }

// MkdirAll makes dir and every directory above it that is missing, at the mode perm (less the
// mask of the process).
func (Project) MkdirAll(dir string, perm fs.FileMode) error { return os.MkdirAll(dir, perm) }

// WriteTemp writes data to a fresh temporary file in dir, at the mode perm whatever the mask of
// the process, and returns its path; the caller owns the rename. The name of the file begins
// ".tmp-skills-", which is how a copier of a tree knows it for half of a write
// (skills.SkipWhenCopying). The data is on disk when it returns. A failure removes the file and
// says the step it failed at: create temp, write, sync, close or chmod.
func (Project) WriteTemp(dir string, data []byte, perm fs.FileMode) (string, error) {
	tmp, err := os.CreateTemp(dir, ".tmp-skills-*")
	if err != nil {
		return "", fmt.Errorf("create temp: %w", err)
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return "", fmt.Errorf("write: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(name)
		return "", fmt.Errorf("sync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return "", fmt.Errorf("close: %w", err)
	}
	// os.CreateTemp creates at 0600; without this the mode asked for would never reach the
	// file that is renamed into place.
	if err := os.Chmod(name, perm); err != nil {
		os.Remove(name)
		return "", fmt.Errorf("chmod: %w", err)
	}
	return name, nil
}

// Rename moves oldPath to newPath, replacing a file that is there.
func (Project) Rename(oldPath, newPath string) error { return os.Rename(oldPath, newPath) }

// Remove deletes the file or the empty directory name.
func (Project) Remove(name string) error { return os.Remove(name) }

// ResolvePath returns name with every link in its existing ancestry resolved, keeping the
// components that do not exist as they are written. It resolves a path exactly as the planners
// of the domain did (fsresolve.KeepingMissing), so that the proof made at the moment of
// writing is the proof made when the plan was built.
func (Project) ResolvePath(name string) (string, error) {
	return fsresolve.KeepingMissing(name)
}
