// Package fsstore is the adapter of the ports of the SDD registry repair (engine/sddregistry/app):
// the registry of a project as a file, and the skills a target keeps as files.
package fsstore

import (
	"os"
	"path/filepath"

	propagatorfs "github.com/labdrian-ai/labdrian-sdd-overlay/engine/propagator/fsstore"
)

// directoryMode is the mode of a directory the registry is made in: the one `mkdir -p` makes under
// the default umask.
const directoryMode os.FileMode = 0o755

// Registry is the registry of a project as a file. It holds no state; the zero value is the
// adapter. Its write is the one `engine propagate` writes with, so the two write the same way: a
// temporary file in the directory of the registry and a rename, which leaves the registry at mode
// 0644.
type Registry struct {
	whole propagatorfs.Registry
}

// Read returns the file at path. A file that is not there is an error for which errors.Is(err,
// fs.ErrNotExist) holds; any other failure is returned as the system reported it.
func (Registry) Read(path string) ([]byte, error) { return os.ReadFile(path) }

// Write puts content at path whole, as `engine propagate` does, after making the directory the
// registry lives in when there is none (the first repair of a project makes its .atl directory).
func (r Registry) Write(path string, content []byte) error {
	if err := r.Home(path); err != nil {
		return err
	}
	return r.whole.Write(path, content)
}

// Home makes the directory the registry at path lives in, and its parents, when there is none.
// The lock that serializes the writers of a registry is a file beside it, so a caller that takes
// the lock before it writes asks for the directory first.
func (Registry) Home(path string) error { return os.MkdirAll(filepath.Dir(path), directoryMode) }

// Skills is the skills a target keeps, as files. It holds no state; the zero value is the adapter.
type Skills struct{}

// IsFile reports whether path is a regular file, following a link.
func (Skills) IsFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// ReadFile returns the file at path.
func (Skills) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }
