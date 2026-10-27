// Package fsstore is the adapter of the Results port of review/app: the directory
// odd/review-results/<feature>/ under the root of a working tree, as files.
//
// What it promises, because the results it keeps are evidence a later command trusts:
//
//   - A file is published whole or not at all, and never over a file that is there. Create stages the
//     content in a temporary file of the same directory, flushes it, and links it to its name, so a
//     reader sees the whole file or no file, a crash leaves no half-written result, and the name that is
//     taken by another writer, whether before the call or while it ran, is reported (app.ErrExists) and
//     left as it is. The temporary files of a write in flight (.review-*.tmp) are hidden from Names.
//   - Nothing a command line gave leads out of odd/review-results/. The feature is a single safe path
//     segment (review.CheckFeature), the name is one plain file name (ErrInvalidName), and every directory
//     on the way from the root (odd, odd/review-results, the directory of the feature) is looked at
//     without being followed: a symbolic link or a file where a directory should be is refused
//     (ErrNotADirectory) and nothing is written through it. A directory that is not there is made, with
//     mode 0755, and the file is made with mode 0644, whatever the umask says. A directory that another
//     writer made meanwhile is used as it is, its mode included.
//
// Two ways to do it. On linux the store holds the directories open (held_linux.go): it opens each directory
// of the path relative to the one above, and lists, stages and links relative to the directory of the feature
// that it holds, so what another process does to the paths after the store has opened them, a directory moved
// or a link put in its place, changes nothing: the store works in the directories it opened. The standard
// library has no linkat, so the link is the raw system call. Anywhere else it works by path (bypath.go),
// because the standard library does not offer to create or link a file relative to a directory that is held
// open there (syscall has no openat or linkat on those platforms, os.Root needs a newer Go than the module's
// go.mod names, and atomicfile, which the engine shares, works on paths). There the check of a directory and
// the use of it are two calls, and a process that can write to the working tree can change a directory between
// them. The store looks at the directories again, after a listing has been read, before the link that
// publishes a file, and after it; a directory that is no longer the one it first saw is refused (ErrChanged)
// and nothing that was done there is trusted. That makes the change loud and shortens the window; it does not
// close it.
//
// What neither does. A process that can write to the working tree can put a real directory at
// odd/review-results/<feature> before the store walks to it, and could forge a result without the store at
// all. The store keeps a result out of a place the repository's own files point to, which is what a checkout
// of someone else's branch can bring.
//
// Read, which the verification of a stored result needs (own-rdd T3), works by path on every platform: the
// directories are looked at as for any other call, the file is looked at without being followed (a link, a
// directory or anything else that is not a regular file is refused, ErrNotAFile), opened, compared with what
// was looked at, read up to review.MaxStoredBytes (ErrTooLarge) and the directories are looked at again
// (ErrChanged). A read is not a write, so there is nothing to publish into a directory that changed; the
// bytes are simply not returned.
package fsstore

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
)

// ErrInvalidName is a file name that is not one plain name: empty, a dot name, hidden (the temporary files of
// a write are), holding a separator or a NUL byte, or longer than a file name can be.
var ErrInvalidName = errors.New("review: invalid result file name")

// ErrNotADirectory is a directory of the path to the results of a feature that is a symbolic link or not a
// directory at all.
var ErrNotADirectory = errors.New("review: a directory of the results path is a link or not a directory")

// ErrChanged is a directory of the path to the results of a feature that is not the one the store looked at:
// it was replaced, by a link or by another directory, while the store was using it. Only the store that works
// by path says it; the store that holds the directories (linux) works in the ones it opened and has nothing to
// say.
var ErrChanged = errors.New("review: a directory of the results path changed while the store was using it")

// ErrNotAFile is a file of the results of a feature that Read was asked for and that is a symbolic link, a
// directory or anything else that is not a regular file.
var ErrNotAFile = errors.New("review: a result is a link or not a regular file")

// ErrTooLarge is a file of the results of a feature that is above review.MaxStoredBytes.
var ErrTooLarge = errors.New("review: a stored file is too large")

const (
	dirPerm  fs.FileMode = 0o755
	filePerm fs.FileMode = 0o644
	// tempPattern names the temporary files of a write; a name that begins with '.' is never a result.
	tempPattern = ".review-*.tmp"
	maxNameLen  = 255
)

// The points of the work at which a Store calls its race hook, with the directory each is about.
const (
	// pointMissing: a directory was found missing and is about to be made.
	pointMissing = "missing"
	// pointWalked: every directory of the path was looked at and the store is about to use them.
	pointWalked = "walked"
	// pointLinked: the file was linked into place and the store is about to look at the directories again.
	pointLinked = "linked"
	// pointLooked: Read looked at the file and is about to open it.
	pointLooked = "looked"
)

// Store is the results directories of the repositories under their roots. It holds no state, and the zero
// value is ready to use.
type Store struct {
	// race, when it is set, is called with the name of a point of the work and the directory it is about, at
	// the moments another process could change the directories under the store. A test makes that change
	// there, at the exact moment, which no test can do to a real process on purpose. New leaves it nil.
	race func(point, dir string)
	// syncDir, when it is set, flushes a held directory instead of (*os.File).Sync, so a test can make the
	// flush fail. New leaves it nil.
	syncDir func(*os.File) error
}

// flush flushes the held directory dir with the store's flush.
func (s Store) flush(dir *os.File) error {
	if s.syncDir != nil {
		return s.syncDir(dir)
	}
	return dir.Sync()
}

// temporaryName is the name of a temporary file of a write, built from tempPattern so that isTemporary always
// recognises it.
func temporaryName(token string) string { return strings.Replace(tempPattern, "*", token, 1) }

// New is the Store.
func New() Store { return Store{} }

// at calls the race hook, when there is one.
func (s Store) at(point, dir string) {
	if s.race != nil {
		s.race(point, dir)
	}
}

// Names lists the names in the directory of the feature under root, sorted, and none when the directory is
// not there. It makes nothing. The temporary files of a write in flight (tempPattern) are not results and
// are left out. Where the store works by path, a directory that is not the one the listing was read from by
// the time it was read is ErrChanged, and no name is returned.
func (s Store) Names(root, feature string) ([]string, error) {
	return s.names(root, feature)
}

// isTemporary says whether name is one a write in flight stages its content under (tempPattern).
func isTemporary(name string) bool {
	matched, err := filepath.Match(tempPattern, name)
	return err == nil && matched
}

// Create stores data under name in the directory of the feature under root, making the directories that are
// not there. app.ErrExists says the name is taken; the file that is there is not touched. Where the store
// works by path, the directories are looked at again right before the file is linked, and nothing is
// published into one that changed (ErrChanged); one that changes after the link is reported, with the file
// already linked.
func (s Store) Create(root, feature, name string, data []byte) error {
	if err := checkName(name); err != nil {
		return err
	}
	return s.create(root, feature, name, data)
}

// Read returns the bytes of the file called name in the directory of the feature under root, and makes
// nothing. app.ErrNotFound says the file, its directory or the root is not there. The file must be a regular
// file that is not a link (ErrNotAFile) of at most review.MaxStoredBytes (ErrTooLarge), and the directories
// must be the ones the store looked at when the read ends (ErrChanged); with any of those the bytes are not
// returned.
func (s Store) Read(root, feature, name string) ([]byte, error) {
	if err := checkName(name); err != nil {
		return nil, err
	}
	loc, found, err := s.walk(root, feature, false)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(root, review.ResultsDir, feature, name)
	if !found {
		return nil, fmt.Errorf("%w: %s", app.ErrNotFound, path)
	}
	s.at(pointWalked, loc.dir)
	data, err := s.readFile(filepath.Join(loc.dir, name))
	if err != nil {
		return nil, err
	}
	if err = unchanged(loc.seen); err != nil {
		return nil, err
	}
	return data, nil
}

// readFile reads the regular file at path: it looks at it without following it, opens it, requires what it
// opened to be what it looked at, and reads one byte past the limit to know a file is above it.
func (s Store) readFile(path string) ([]byte, error) {
	looked, err := os.Lstat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, fmt.Errorf("%w: %s", app.ErrNotFound, path)
	case err != nil:
		return nil, fmt.Errorf("inspect %s: %w", path, err)
	case !looked.Mode().IsRegular():
		return nil, fmt.Errorf("%w: %s", ErrNotAFile, path)
	}
	s.at(pointLooked, filepath.Dir(path))
	f, err := os.OpenFile(path, openRead, 0)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	defer f.Close()
	// A file removed and another made at its name can be given the same inode, so what was opened must be a
	// regular file as well as the one that was looked at.
	if opened, err := f.Stat(); err != nil || !opened.Mode().IsRegular() || !os.SameFile(looked, opened) {
		return nil, fmt.Errorf("%w: %s is not the file that was looked at", ErrChanged, path)
	}
	data, err := io.ReadAll(io.LimitReader(f, review.MaxStoredBytes+1))
	switch {
	case err != nil:
		return nil, fmt.Errorf("read %s: %w", path, err)
	case len(data) > review.MaxStoredBytes:
		return nil, fmt.Errorf("%w: %s is above %d bytes", ErrTooLarge, path, review.MaxStoredBytes)
	}
	return data, nil
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
