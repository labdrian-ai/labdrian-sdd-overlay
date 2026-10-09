// Package fsstore is the adapter of the propagate use case's RegistryStore port (Phase 9 unit
// H28): it reads the registry of a project from the file system and writes it back whole.
package fsstore

import (
	"os"
	"path/filepath"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/propagator/app"
)

// registryMode is the mode the registry is left with: it is a file of the repository that other
// people read, and the temporary file it is written through starts at 0600.
const registryMode os.FileMode = 0o644

// Registry is the registry of a project as a file. It holds no state.
type Registry struct{}

// Read returns the file at path. A file that does not exist is an *app.NotFoundError that carries
// the error of the system, so the words a person reads are the system's; every other failure is
// returned as it came.
func (Registry) Read(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, &app.NotFoundError{Err: err}
		}
		return nil, err
	}
	return data, nil
}

// Write puts content at path through a temporary file in the same directory and a rename. Unlike
// os.WriteFile, which truncates first, a reader running beside the write sees the old registry or
// the new one, never a partial or empty file: rename(2) is atomic on POSIX file systems. A failure
// is returned as the system reported it, and the temporary file is removed.
func (Registry) Write(path string, content []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	// Cleanup on any failure; a no-op after the rename has moved the file.
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, registryMode); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
