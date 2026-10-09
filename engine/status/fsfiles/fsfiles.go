// Package fsfiles is the adapter of the status package's Files port: the operating system's file
// system, asked for the permission bits of a path and the bytes of a file. It follows symbolic
// links, as stat and open do, and reports every failure as the system did, so an absent path
// satisfies fs.ErrNotExist and nothing else does.
package fsfiles

import (
	"io/fs"
	"os"
)

// Files is the file system of the machine. It holds nothing.
type Files struct{}

// Stat answers the mode of the file at path, which carries the permission bits and says whether it
// is a directory.
func (Files) Stat(path string) (fs.FileMode, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Mode(), nil
}

// ReadFile answers the content of the file at path.
func (Files) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }
