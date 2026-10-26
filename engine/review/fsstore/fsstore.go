// Package fsstore is the adapter of the Results port of review/app: the directory
// odd/review-results/<feature>/ under the root of a working tree, as files.
//
// What it promises, because the results it keeps are evidence a later command trusts:
//
//   - A file is published whole or not at all, and never over a file that is there. Create stages the
//     content in a temporary file of the same directory, flushes it, and links it to its name, so a
//     reader sees the whole file or no file, a crash leaves no half-written result, and the name that is
//     taken by another writer, whether before the call or while it ran, is reported (app.ErrExists) and
//     left as it is. This is atomicfile's Create.
//   - Nothing a command line gave leads out of odd/review-results/. The feature is a single safe path
//     segment (review.CheckFeature), the name is one plain file name (ErrInvalidName), and every directory
//     on the way from the root (odd, odd/review-results, the directory of the feature) is looked at
//     without being followed: a symbolic link or a file where a directory should be is refused
//     (ErrNotADirectory) and nothing is written through it. A directory that is not there is made, with
//     mode 0755, and the file is made with mode 0644, whatever the umask says.
//
// The check of a directory and the use of it are two calls, so a link put in place by another process
// between them is not seen: the store keeps a result out of a place the repository's own files point to,
// which is what a checkout of someone else's branch can bring, and does not defend against a process that
// can already write to the working tree.
package fsstore

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/atomicfile"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
)

// ErrInvalidName is a file name that is not one plain name: empty, a dot name, hidden (the temporary files of
// a write are), holding a separator or a NUL byte, or longer than a file name can be.
var ErrInvalidName = errors.New("review: invalid result file name")

// ErrNotADirectory is a directory of the path to the results of a feature that is a symbolic link or not a
// directory at all.
var ErrNotADirectory = errors.New("review: a directory of the results path is a link or not a directory")

const (
	dirPerm  fs.FileMode = 0o755
	filePerm fs.FileMode = 0o644
	// tempPattern names the temporary files of a write; a name that begins with '.' is never a result.
	tempPattern = ".review-*.tmp"
	maxNameLen  = 255
)

// Store is the results directories of the repositories under their roots. It holds no state.
type Store struct{}

// New is the Store.
func New() Store { return Store{} }

// Names lists the names in the directory of the feature under root, sorted, and none when the directory is
// not there. It makes nothing.
func (Store) Names(root, feature string) ([]string, error) {
	dir, found, err := walk(root, feature, false)
	if err != nil || !found {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names, nil
}

// Create stores data under name in the directory of the feature under root, making the directories that are
// not there. app.ErrExists says the name is taken; the file that is there is not touched.
func (Store) Create(root, feature, name string, data []byte) error {
	if err := checkName(name); err != nil {
		return err
	}
	dir, _, err := walk(root, feature, true)
	if err != nil {
		return err
	}
	staged, err := atomicfile.Stage(dir, data, atomicfile.Options{Perm: filePerm, Sync: true, TempPattern: tempPattern})
	if err != nil {
		return fmt.Errorf("stage %s: %w", name, err)
	}
	defer staged.Discard()
	path := filepath.Join(dir, name)
	if err := staged.Create(path); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("%w: %s", app.ErrExists, path)
		}
		return fmt.Errorf("store %s: %w", path, err)
	}
	return nil
}

// checkName requires one plain file name.
func checkName(name string) error {
	switch {
	case name == "" || name == "." || name == "..":
		return fmt.Errorf("%w: %q", ErrInvalidName, name)
	case name[0] == '.':
		return fmt.Errorf("%w: %q starts with '.'", ErrInvalidName, name)
	case len(name) > maxNameLen:
		return fmt.Errorf("%w: %q is longer than %d bytes", ErrInvalidName, name, maxNameLen)
	case strings.ContainsAny(name, "/\\\x00"):
		return fmt.Errorf("%w: %q holds a separator or a NUL byte", ErrInvalidName, name)
	}
	return nil
}

// walk goes from root to the directory of the feature, one directory at a time, looking at each without
// following it. A directory that is not there ends the walk with found false when create is false, and is
// made when create is true. A link, or anything that is not a directory, is ErrNotADirectory.
func walk(root, feature string, create bool) (dir string, found bool, err error) {
	if err := review.CheckFeature(feature); err != nil {
		return "", false, err
	}
	dir = root
	for _, part := range append(strings.Split(review.ResultsDir, "/"), feature) {
		dir = filepath.Join(dir, part)
		info, err := os.Lstat(dir)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			if !create {
				return "", false, nil
			}
			if err := makeDir(dir); err != nil {
				return "", false, err
			}
			continue
		case err != nil:
			return "", false, fmt.Errorf("inspect %s: %w", dir, err)
		}
		if err := checkDir(dir, info); err != nil {
			return "", false, err
		}
	}
	return dir, true, nil
}

// makeDir makes the directory dir with mode 0755, whatever the umask says. A directory that another writer
// made meanwhile is looked at like any other.
func makeDir(dir string) error {
	if err := os.Mkdir(dir, dirPerm); err != nil {
		if !errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("make %s: %w", dir, err)
		}
		info, statErr := os.Lstat(dir)
		if statErr != nil {
			return fmt.Errorf("inspect %s: %w", dir, statErr)
		}
		return checkDir(dir, info)
	}
	if err := os.Chmod(dir, dirPerm); err != nil {
		return fmt.Errorf("set the mode of %s: %w", dir, err)
	}
	return nil
}

// checkDir requires info, as os.Lstat gave it for dir, to be a directory that is not a link.
func checkDir(dir string, info fs.FileInfo) error {
	if info.Mode()&fs.ModeSymlink != 0 {
		return fmt.Errorf("%w: %s is a symbolic link", ErrNotADirectory, dir)
	}
	if !info.IsDir() {
		return fmt.Errorf("%w: %s is not a directory", ErrNotADirectory, dir)
	}
	return nil
}
