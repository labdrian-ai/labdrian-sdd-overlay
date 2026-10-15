// Package fsstore is the adapter of the ports of the installed-targets use case
// (engine/runtime/ownership/app): the record of the runtimes longterm-mem is installed for as
// a file under the state directory, the lock around it as an advisory file lock, and the shared
// binary as a file.
//
// The record is the file the overlay script always kept, <state dir>/longterm-mem/installed-targets,
// one name per line. It is appended to (a name is one write, which O_APPEND makes atomic) and
// rewritten whole through a temporary file in its own directory and a rename, so a reader never sees
// half of it. The lock is a file of its own in the state directory, outside the directory of the
// record on purpose: a writer that must say that it cannot write in that directory still has to take,
// and let go, a lock to say so. It is taken through engine/filelock, so the kernel lets it go when
// the process ends however it ends: there is no abandoned lock to recognise and break.
package fsstore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/filelock"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/ownership"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/ownership/app"
)

const (
	// recordName and recordDirName are where the record has always been: the file the overlay script
	// kept under a directory of longterm-mem's own in the state directory.
	recordName    = "installed-targets"
	recordDirName = "longterm-mem"
	// lockName is the lock. The script kept its lock as a directory called .installed-targets.lock;
	// this is another name, so that a directory a killed run of the script left behind is only
	// garbage and not something a lock file cannot be made over.
	lockName = ".installed-targets.flock"
	// tempPattern is the pattern of the temporary file a rewrite goes through.
	tempPattern = ".installed-targets.*"
)

// Layout is where the files of the record are under a state directory.
type Layout struct{ StateDir string }

// RecordDir is the directory the record lives in.
func (l Layout) RecordDir() string { return filepath.Join(l.StateDir, recordDirName) }

// Record is the record of the installed targets.
func (l Layout) Record() string { return filepath.Join(l.StateDir, recordDirName, recordName) }

// Lock is the lock around the record: a file in the state directory, outside the directory of the
// record.
func (l Layout) Lock() string { return filepath.Join(l.StateDir, lockName) }

// Store is the record as files. The zero LockOptions wait filelock.DefaultWait for a lock that is
// taken.
type Store struct {
	Layout      Layout
	LockOptions filelock.Options
	// syncFile flushes the temporary file of a rewrite to the disk, and syncDir the directory of the
	// record after the rename; nil means (*os.File).Sync and syncDirectory. readFile reads the record;
	// nil means os.ReadFile. (The unexported fields are how this package's own tests see the moments of
	// the syncs, and make a read fail for a user who can read anything.)
	syncFile func(*os.File) error
	syncDir  func(dir string) error
	readFile func(path string) ([]byte, error)
}

// New returns the store for the state directory.
func New(stateDir string) *Store { return &Store{Layout: Layout{StateDir: stateDir}} }

// Exists says whether the record is there: a regular file, a link to one followed. A directory, or
// anything the system cannot say about, is not a record.
func (s *Store) Exists() bool {
	info, err := os.Stat(s.Layout.Record())
	return err == nil && info.Mode().IsRegular()
}

// MakeRecordDir makes the directory of the record, and its parents, as mkdir -p does.
func (s *Store) MakeRecordDir() error { return os.MkdirAll(s.Layout.RecordDir(), 0o777) }

// Lock takes the lock of the record. The state directory is made when there is none (a failure is a
// *app.StateDirError), and a lock that stays taken for the whole wait is an error for which
// errors.Is(err, app.ErrBusy) holds and that names the lock file.
func (s *Store) Lock() (func(), error) {
	if err := os.MkdirAll(s.Layout.StateDir, 0o777); err != nil {
		return nil, &app.StateDirError{Err: err}
	}
	release, err := filelock.Acquire(s.Layout.Lock(), s.LockOptions)
	if err != nil {
		if errors.Is(err, filelock.ErrBusy) {
			return nil, fmt.Errorf("%w: %v", app.ErrBusy, err)
		}
		return nil, err
	}
	return release, nil
}

// Read returns how the record is found and, when it is readable, its bytes as they are. A record that
// is there and cannot be opened or read is Unreadable; what is not a regular file is Absent.
func (s *Store) Read() (ownership.State, []byte) {
	if !s.Exists() {
		return ownership.Absent, nil
	}
	read := s.readFile
	if read == nil {
		read = os.ReadFile
	}
	content, err := read(s.Layout.Record())
	if err != nil {
		return ownership.Unreadable, nil
	}
	return ownership.Readable, content
}

// Append writes name and a line end to the end of the record, in one write, making the record when
// there is none. It leaves the record at the mode a new file has under the umask of the process.
//
// It is not atomic against a crash or a full disk: a write that stops short leaves a partial last
// line in the record, with no line end. That is safe to leave, and is left (the script left it the
// same way, and a pin holds what the next append does after one): the reader drops whatever follows
// the last line end (ownership.Parse), so a partial name is never install state and holds nothing; the
// price is that the next name appended runs into it, and the two together are one well-formed name
// nobody knows, which holds the shared binary as an unknown name does, until --purge. A rewrite
// (Release) compacts a partial last line away only while nothing has been appended after it.
func (s *Store) Append(name string) error {
	f, err := os.OpenFile(s.Layout.Record(), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o666)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(name + "\n"); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// Replace puts content in the record through a temporary file in the directory of the record and a
// rename, so that a reader sees the record as it was or as it is. The temporary file is made at 0600
// and the rename keeps that mode; it is synced before the rename, so a crash cannot leave the rename
// standing over bytes that never reached the disk, and the directory of the record is synced after
// it, so that a crash cannot lose the rename itself and bring the old record back; the temporary file
// is removed on every failure. A failure is a *app.ReplaceError that says at which stage. At every
// stage but app.StageSync the record is as it was; at StageSync the rename was made, the record is the
// new one and only its durability is in doubt (a filesystem that cannot sync a directory is not a
// failure: there is nothing more to do for the rename there).
func (s *Store) Replace(content []byte) error {
	tmp, err := os.CreateTemp(s.Layout.RecordDir(), tempPattern)
	if err != nil {
		return &app.ReplaceError{Stage: app.StageCreate, Err: err}
	}
	tmpPath := tmp.Name()
	// Cleanup on any failure; a no-op once the rename has moved the file.
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return &app.ReplaceError{Stage: app.StageWrite, Err: err}
	}
	sync := s.syncFile
	if sync == nil {
		sync = (*os.File).Sync
	}
	if err := sync(tmp); err != nil {
		tmp.Close()
		return &app.ReplaceError{Stage: app.StageWrite, Err: err}
	}
	if err := tmp.Close(); err != nil {
		return &app.ReplaceError{Stage: app.StageWrite, Err: err}
	}
	if err := os.Rename(tmpPath, s.Layout.Record()); err != nil {
		return &app.ReplaceError{Stage: app.StagePublish, Err: err}
	}
	syncDir := s.syncDir
	if syncDir == nil {
		syncDir = syncDirectory
	}
	if err := syncDir(s.Layout.RecordDir()); err != nil {
		return &app.ReplaceError{Stage: app.StageSync, Err: err}
	}
	return nil
}

// syncDirectory flushes a directory to the disk, which is what makes a rename inside it durable. A
// filesystem that cannot sync a directory (the call answers that it is invalid or not supported for it)
// has nothing more to give, and is not an error; any other failure is.
func syncDirectory(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	err = d.Sync()
	if closeErr := d.Close(); err == nil {
		err = closeErr
	}
	return ignoreUnsupported(err)
}

// ignoreUnsupported is nil for the errors a filesystem answers when it cannot sync a directory.
func ignoreUnsupported(err error) error {
	if errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.ENOTSUP) {
		return nil
	}
	return err
}

// Delete removes the record, as rm -f does: one that is not there is not an error, a link is removed
// and not what it points to, and a directory is refused and left where it is.
func (s *Store) Delete() error { return removeFile(s.Layout.Record()) }

// Binaries is the shared binary as a file. It holds no state; the zero value is the adapter.
type Binaries struct{}

// Exists says whether there is a file at path, a link followed.
func (Binaries) Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Remove removes the file at path as rm -f does.
func (Binaries) Remove(path string) error { return removeFile(path) }

// removeFile removes the file at path. A path with nothing at it is already what was asked for. A
// directory is refused: os.Remove would take an empty one, and the program that is removed is a file.
func removeFile(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.IsDir() {
		return &os.PathError{Op: "remove", Path: path, Err: errors.New("is a directory")}
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
