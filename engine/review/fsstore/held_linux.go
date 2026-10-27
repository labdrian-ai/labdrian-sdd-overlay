package fsstore

import (
	"errors"
	"fmt"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
)

// The store that holds the directories: from the root it opens the directories of the path one below the other,
// each relative to the one above and without following a link, and it keeps the directory of the feature open
// for the whole of a call. What it does then (list, stage a file, link it into place) is relative to that
// directory, not to its path, so another process that moves a directory of the path, or puts a link in its
// place, after the store has opened it changes nothing: the store works in the directories it opened. The
// standard library offers these calls only in the syscall package and only on linux, and has no linkat, which
// is made here with the raw system call. Where they are not offered the store works by path (bypath.go).

func (s Store) names(root, feature string) ([]string, error) {
	return s.namesHeld(root, feature)
}

func (s Store) create(root, feature, name string, data []byte) error {
	return s.createHeld(root, feature, name, data)
}

// openDir is how every directory of the path is opened: for reading, only if it is a directory, and never by
// following a link in its name (the kernel answers ENOTDIR to a link, to a file and to what is neither).
const openDir = syscall.O_RDONLY | syscall.O_DIRECTORY | syscall.O_NOFOLLOW | syscall.O_CLOEXEC

// maxTempTries is how many names the store tries for a temporary file before it gives up; one that is taken
// is rare, since the name is random.
const maxTempTries = 100

// namesHeld is Names where a directory can be held open.
func (s Store) namesHeld(root, feature string) ([]string, error) {
	dir, path, err := s.openHeld(root, feature, false)
	if err != nil || dir == nil {
		return nil, err
	}
	defer dir.Close()
	s.at(pointWalked, path)
	entries, err := dir.ReadDir(-1)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if isTemporary(e.Name()) {
			continue
		}
		names = append(names, e.Name())
	}
	slices.Sort(names)
	return names, nil
}

// createHeld is Create where a directory can be held open: the content is staged in a temporary file of the
// directory that is held, and linked, in that directory, to its name.
func (s Store) createHeld(root, feature, name string, data []byte) error {
	dir, path, err := s.openHeld(root, feature, true)
	if err != nil {
		return err
	}
	defer dir.Close()
	s.at(pointWalked, path)
	fd := int(dir.Fd())
	temp, err := stageAt(fd, path, data)
	if err != nil {
		return fmt.Errorf("stage %s: %w", name, err)
	}
	defer unlinkat(fd, temp)
	target := filepath.Join(path, name)
	if err = linkat(fd, temp, fd, name); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("%w: %s", app.ErrExists, target)
		}
		return fmt.Errorf("store %s: %w", target, err)
	}
	s.at(pointLinked, path)
	// The new name survives a crash only once the directory is flushed, as atomicfile does with Options.Sync.
	if err = s.flush(dir); err != nil {
		// The name is published: say so, so the result is not stored a second time.
		return fmt.Errorf("%w: %s: %w", app.ErrNotFlushed, target, err)
	}
	return nil
}

// openHeld goes from root to the directory of the feature and returns that directory open, with the path it was
// reached by. A directory that is not there ends the walk with no directory when create is false, and is made
// when create is true. A link, or anything that is not a directory, is ErrNotADirectory. The root is opened as
// it was given, links in it followed: it is the working tree the caller named.
func (s Store) openHeld(root, feature string, create bool) (*os.File, string, error) {
	if err := review.CheckFeature(feature); err != nil {
		return nil, "", err
	}
	fd, err := open(root, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) && !create {
			return nil, "", nil
		}
		return nil, "", fmt.Errorf("open %s: %w", root, err)
	}
	dir, path := os.NewFile(uintptr(fd), root), root
	for _, part := range append(strings.Split(review.ResultsDir, "/"), feature) {
		path = filepath.Join(path, part)
		below, err := s.openBelow(dir, path, part, create)
		dir.Close()
		if err != nil || below == nil {
			return nil, "", err
		}
		dir = below
	}
	return dir, path, nil
}

// openBelow opens the directory name of parent, which is at path. It returns no directory and no error when
// there is none and create is false; when create is true it makes the directory, with mode 0755 whatever the
// umask says, and a directory that another writer made meanwhile is opened like any other.
func (s Store) openBelow(parent *os.File, path, name string, create bool) (*os.File, error) {
	fd, err := openat(int(parent.Fd()), name, openDir, 0)
	made := false
	if errors.Is(err, fs.ErrNotExist) {
		if !create {
			return nil, nil
		}
		s.at(pointMissing, path)
		err = mkdirat(int(parent.Fd()), name, uint32(dirPerm))
		switch {
		case err == nil:
			made = true
		case !errors.Is(err, fs.ErrExist):
			return nil, fmt.Errorf("make %s: %w", path, &fs.PathError{Op: "mkdir", Path: path, Err: err})
		}
		fd, err = openat(int(parent.Fd()), name, openDir, 0)
	}
	if err != nil {
		if !create && !errors.Is(err, syscall.ENOTDIR) && !errors.Is(err, syscall.ELOOP) {
			// Names: the raw syscall carries no operation or path, so the error is given the words os.ReadDir
			// gave before the store held its directories open ("read <path>: open <path>: ..."), which a test
			// pins. The "make <path>: mkdir <path>: ..." of Create is built above, where mkdirat fails. A link
			// or a file on the way is worded by notADirectory instead.
			return nil, fmt.Errorf("read %s: %w", path, &fs.PathError{Op: "open", Path: path, Err: err})
		}
		return nil, notADirectory(path, err)
	}
	dir := os.NewFile(uintptr(fd), path)
	if made {
		if err = dir.Chmod(dirPerm); err != nil {
			dir.Close()
			return nil, fmt.Errorf("set the mode of %s: %w", path, err)
		}
	}
	return dir, nil
}

// notADirectory is the error of a directory of the path that could not be opened as one. A link and a file are
// both ENOTDIR to the kernel, so the refusal looks at the path, for the words only, to say which it is.
func notADirectory(path string, err error) error {
	if !errors.Is(err, syscall.ENOTDIR) && !errors.Is(err, syscall.ELOOP) {
		return fmt.Errorf("inspect %s: %w", path, err)
	}
	info, statErr := os.Lstat(path)
	switch {
	case statErr != nil:
		return fmt.Errorf("%w: %s is a symbolic link or not a directory", ErrNotADirectory, path)
	case info.Mode()&fs.ModeSymlink != 0:
		return fmt.Errorf("%w: %s is a symbolic link", ErrNotADirectory, path)
	default:
		return fmt.Errorf("%w: %s is not a directory", ErrNotADirectory, path)
	}
}

// stageAt writes data to a new temporary file of the directory dirfd, which is dir for the messages, and
// returns its name, as atomicfile.Stage does for a path: the file is made private, given its mode before there
// is anything in it, written whole and flushed. A name that is taken is not used again.
func stageAt(dirfd int, dir string, data []byte) (string, error) {
	for try := 0; try < maxTempTries; try++ {
		name := temporaryName(strconv.FormatUint(rand.Uint64(), 10))
		fd, err := openat(dirfd, name, syscall.O_WRONLY|syscall.O_CREAT|syscall.O_EXCL|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0o600)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("create a temporary file in %s: %w", dir, err)
		}
		if err = fill(os.NewFile(uintptr(fd), name), data); err != nil {
			unlinkat(dirfd, name)
			return "", err
		}
		return name, nil
	}
	return "", fmt.Errorf("create a temporary file in %s: %d names are taken", dir, maxTempTries)
}

// fill sets the mode, writes the data, flushes it and closes the file.
func fill(f *os.File, data []byte) error {
	if err := f.Chmod(filePerm); err != nil {
		f.Close()
		return fmt.Errorf("set the mode of the temporary file: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return fmt.Errorf("write the temporary file: %w", err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return fmt.Errorf("sync the temporary file: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close the temporary file: %w", err)
	}
	return nil
}

// The system calls, repeated when a signal interrupts them as the os package does.

func open(name string, flags int) (fd int, err error) {
	for {
		if fd, err = syscall.Open(name, flags, 0); err != syscall.EINTR {
			return fd, err
		}
	}
}

func openat(dirfd int, name string, flags int, mode uint32) (fd int, err error) {
	for {
		if fd, err = syscall.Openat(dirfd, name, flags, mode); err != syscall.EINTR {
			return fd, err
		}
	}
}

func mkdirat(dirfd int, name string, mode uint32) error {
	for {
		if err := syscall.Mkdirat(dirfd, name, mode); err != syscall.EINTR {
			return err
		}
	}
}

// unlinkat removes name from the directory dirfd, and says nothing when it cannot: it is the cleanup of a
// temporary file, after the work that matters.
func unlinkat(dirfd int, name string) {
	for {
		if err := syscall.Unlinkat(dirfd, name); err != syscall.EINTR {
			return
		}
	}
}

// linkat makes newname of the directory newdirfd another name for oldname of the directory olddirfd, and fails
// with EEXIST, never replacing, when newname is taken. The syscall package has no linkat, so this is the raw
// call.
func linkat(olddirfd int, oldname string, newdirfd int, newname string) error {
	oldp, err := syscall.BytePtrFromString(oldname)
	if err != nil {
		return err
	}
	newp, err := syscall.BytePtrFromString(newname)
	if err != nil {
		return err
	}
	for {
		_, _, errno := syscall.Syscall6(syscall.SYS_LINKAT, uintptr(olddirfd), uintptr(unsafe.Pointer(oldp)), uintptr(newdirfd), uintptr(unsafe.Pointer(newp)), 0, 0)
		switch errno {
		case 0:
			return nil
		case syscall.EINTR:
		default:
			return errno
		}
	}
}
