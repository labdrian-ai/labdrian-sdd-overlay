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

// The store that works by path: it looks at each directory of the path, uses the path, and looks at the
// directories again. It is what every platform can do with the standard library, and what the store does where
// it cannot hold a directory open (see held_linux.go for the one that can).

// namesByPath is Names for a platform that cannot hold a directory open.
func (s Store) namesByPath(root, feature string) ([]string, error) {
	loc, found, err := s.walk(root, feature, false)
	if err != nil || !found {
		return nil, err
	}
	s.at(pointWalked, loc.dir)
	entries, err := os.ReadDir(loc.dir)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", loc.dir, err)
	}
	if err = unchanged(loc.seen); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if isTemporary(e.Name()) {
			continue
		}
		names = append(names, e.Name())
	}
	return names, nil
}

// createByPath is Create for a platform that cannot hold a directory open. The directories are looked at again
// right before the file is linked, and nothing is published into one that changed (ErrChanged); one that
// changes after the link is reported, with the file already linked.
func (s Store) createByPath(root, feature, name string, data []byte) error {
	loc, _, err := s.walk(root, feature, true)
	if err != nil {
		return err
	}
	s.at(pointWalked, loc.dir)
	staged, err := atomicfile.Stage(loc.dir, data, atomicfile.Options{Perm: filePerm, Sync: true, TempPattern: tempPattern})
	if err != nil {
		return fmt.Errorf("stage %s: %w", name, err)
	}
	defer staged.Discard()
	if err = unchanged(loc.seen); err != nil {
		return err
	}
	path := filepath.Join(loc.dir, name)
	if err = staged.Create(path); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("%w: %s", app.ErrExists, path)
		}
		return fmt.Errorf("store %s: %w", path, err)
	}
	s.at(pointLinked, loc.dir)
	if err = unchanged(loc.seen); err != nil {
		return fmt.Errorf("%s was linked, but the result may not be where it was meant to be: %w", path, err)
	}
	return nil
}

// seenDir is a directory of the path to the results of a feature as the store looked at it: where it is, and
// what os.Lstat said, which is what it is compared with when the store looks again.
type seenDir struct {
	path string
	info fs.FileInfo
}

// location is the directory of a feature and the directories on the way to it, outermost first.
type location struct {
	dir  string
	seen []seenDir
}

// walk goes from root to the directory of the feature, one directory at a time, looking at each without
// following it. A directory that is not there ends the walk with found false when create is false, and is
// made when create is true. A link, or anything that is not a directory, is ErrNotADirectory.
func (s Store) walk(root, feature string, create bool) (location, bool, error) {
	if err := review.CheckFeature(feature); err != nil {
		return location{}, false, err
	}
	loc := location{dir: root}
	for _, part := range append(strings.Split(review.ResultsDir, "/"), feature) {
		loc.dir = filepath.Join(loc.dir, part)
		info, err := os.Lstat(loc.dir)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			if !create {
				return location{}, false, nil
			}
			s.at(pointMissing, loc.dir)
			if info, err = makeDir(loc.dir); err != nil {
				return location{}, false, err
			}
		case err != nil:
			return location{}, false, fmt.Errorf("inspect %s: %w", loc.dir, err)
		default:
			if err = checkDir(loc.dir, info); err != nil {
				return location{}, false, err
			}
		}
		loc.seen = append(loc.seen, seenDir{path: loc.dir, info: info})
	}
	return loc, true, nil
}

// unchanged looks again at the directories the store saw and requires each to be the very one: a real
// directory that os.SameFile says is the same object. A swap undone before the look cannot be seen.
func unchanged(seen []seenDir) error {
	for _, d := range seen {
		info, err := os.Lstat(d.path)
		if err != nil {
			return fmt.Errorf("%w: %s: %v", ErrChanged, d.path, err)
		}
		if err = checkDir(d.path, info); err != nil {
			return fmt.Errorf("%w: %w", ErrChanged, err)
		}
		if !os.SameFile(d.info, info) {
			return fmt.Errorf("%w: %s is not the directory that was looked at", ErrChanged, d.path)
		}
	}
	return nil
}

// makeDir makes the directory dir with mode 0755, whatever the umask says, and returns it as os.Lstat sees
// it. A directory that another writer made meanwhile is looked at like any other.
func makeDir(dir string) (fs.FileInfo, error) {
	if err := os.Mkdir(dir, dirPerm); err != nil {
		if !errors.Is(err, fs.ErrExist) {
			return nil, fmt.Errorf("make %s: %w", dir, err)
		}
		return lookAt(dir)
	}
	if err := os.Chmod(dir, dirPerm); err != nil {
		return nil, fmt.Errorf("set the mode of %s: %w", dir, err)
	}
	return lookAt(dir)
}

// lookAt is os.Lstat of dir, which must be a directory that is not a link.
func lookAt(dir string) (fs.FileInfo, error) {
	info, err := os.Lstat(dir)
	if err != nil {
		return nil, fmt.Errorf("inspect %s: %w", dir, err)
	}
	if err = checkDir(dir, info); err != nil {
		return nil, err
	}
	return info, nil
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
