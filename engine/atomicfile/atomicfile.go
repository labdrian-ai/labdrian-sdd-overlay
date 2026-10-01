// Package atomicfile writes a file so that a reader sees all of the old content or
// all of the new, never a mix, and so that a crash or a failure never leaves a
// half-written file at the name.
//
// It is the one copy of what nine places in the engine each wrote by hand: write
// a temporary file in the target's directory, flush it, set its mode, and rename
// or link it into place. The strictest behavior of those copies is the behavior
// here:
//
//   - The temporary file is created exclusively in the target's directory (so the
//     rename never crosses a file system) and its mode is set on the open
//     descriptor before any content is written.
//   - With Options.Sync the file is flushed before the rename and the directory
//     after it, so a crash cannot leave the old name pointing at the new content's
//     blocks nor lose the rename.
//   - The temporary file is removed on every failure path.
//   - A symlink at the target is refused, never replaced: a rename would replace
//     the link itself, which silently breaks a dotfile manager's link, and writing
//     through it would put the content somewhere the caller did not name.
//   - With Options.Backup the replaced content is kept beside the file, with its
//     own mode, before anything is replaced; a backup that cannot be made stops the
//     write. The file is read through one no-follow descriptor, so a symlink put
//     at the name after Replace looked at it is refused, not read.
//
// Two operations cover the callers. Replace puts the staged content at a name,
// replacing what is there (WriteFile is Stage and Replace in one call). Create puts
// it at a name that must be free and never replaces an existing file, by hard link,
// which is how an immutable record is published. Stage, then Replace or Create,
// then Discard is also the shape for a caller that must stage two files and commit
// both.
//
// The package takes no position on where files live, who may write them, or what
// they contain; that is the caller's. It takes no lock either: two writers racing
// for one name each produce a whole file and the later rename wins.
package atomicfile

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// DefaultTempPattern names the temporary files of a write that does not choose
// its own. The name is hidden, and the suffix tells a person what a leftover is.
const DefaultTempPattern = ".atomicfile-*.tmp"

// ErrSymlink is returned (wrapped) when the target of a Replace is a symlink.
var ErrSymlink = errors.New("atomicfile: refusing to replace a symlink")

// Options tunes one write. The zero value writes a private file (0600), flushes
// nothing, and keeps no backup.
type Options struct {
	// Perm is the permission bits of the finished file, exactly, whatever the
	// umask. Zero means 0600. Any bit outside the permission bits is an error.
	Perm fs.FileMode
	// Sync flushes the file before it is put in place and the directory after, so
	// the write survives a crash. Without it the data may still be in the cache
	// when the call returns.
	Sync bool
	// Backup keeps the content a Replace overwrites at <path>.bak, with the mode
	// it had. A file that did not exist has nothing to back up. Create ignores it.
	Backup bool
	// TempPattern is the name of the temporary file, with a "*" where the random
	// part goes (see os.CreateTemp). Empty means DefaultTempPattern. A caller that
	// walks the directory the write happens in uses it to recognize and skip a
	// half-written file.
	TempPattern string
}

func (o Options) normalized() (Options, error) {
	if o.Perm == 0 {
		o.Perm = 0o600
	}
	if o.Perm&^fs.ModePerm != 0 {
		return o, fmt.Errorf("atomicfile: Perm %v has bits beyond the permission bits", o.Perm)
	}
	if o.TempPattern == "" {
		o.TempPattern = DefaultTempPattern
	}
	return o, nil
}

// ops groups the system calls whose order and failure the tests observe. They are
// fields, not package variables, so a test passes its own to the unexported
// functions and no test can leak a fake into another.
type ops struct {
	syncFile func(*os.File) error
	syncDir  func(dir string) error
}

func realOps() ops {
	return ops{syncFile: (*os.File).Sync, syncDir: SyncDir}
}

// SyncDir flushes a directory, so that a rename or a new name in it survives a
// crash.
func SyncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

// Staged is content written to a temporary file in its destination's directory,
// not yet at any name. Put it at a name with Replace or Create, and call Discard
// when done, whatever happened: it removes what is left.
type Staged struct {
	ops     ops
	opts    Options
	dir     string
	tmp     string
	renamed bool
	used    bool
}

// Stage writes data to a new temporary file in dir and returns it. Nothing at any
// final name changes.
func Stage(dir string, data []byte, opts Options) (*Staged, error) {
	return stage(realOps(), dir, data, opts)
}

func stage(o ops, dir string, data []byte, opts Options) (*Staged, error) {
	opts, err := opts.normalized()
	if err != nil {
		return nil, err
	}
	f, err := os.CreateTemp(dir, opts.TempPattern)
	if err != nil {
		return nil, fmt.Errorf("atomicfile: create temporary file: %w", err)
	}
	s := &Staged{ops: o, opts: opts, dir: dir, tmp: f.Name()}
	if err := fill(o, f, data, opts); err != nil {
		s.Discard()
		return nil, err
	}
	return s, nil
}

// fill sets the mode, writes the data, flushes if asked, and closes. The mode
// comes first, on the descriptor, so that the content is never in a file whose
// mode was not chosen.
func fill(o ops, f *os.File, data []byte, opts Options) error {
	if err := f.Chmod(opts.Perm); err != nil {
		f.Close()
		return fmt.Errorf("atomicfile: set mode of temporary file: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return fmt.Errorf("atomicfile: write temporary file: %w", err)
	}
	if opts.Sync {
		if err := o.syncFile(f); err != nil {
			f.Close()
			return fmt.Errorf("atomicfile: sync temporary file: %w", err)
		}
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("atomicfile: close temporary file: %w", err)
	}
	return nil
}

// Replace puts the staged content at path, replacing whatever file is there, in
// one rename. A symlink at path is refused (ErrSymlink). With Options.Backup the
// file it replaces is first copied to path+".bak". After a successful rename the
// directory is flushed if Options.Sync is set; a failure of that flush is
// returned although the content is already in place.
//
// A staged file is committed once. Replace on one that was already placed is an
// error.
func (s *Staged) Replace(path string) error {
	if err := s.claim(); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return fmt.Errorf("atomicfile: inspect %s: %w", path, err)
	case info.Mode()&fs.ModeSymlink != 0:
		return fmt.Errorf("%w: %s", ErrSymlink, path)
	case s.opts.Backup && info.Mode().IsRegular():
		if err := s.backUp(path); err != nil {
			return err
		}
	}
	if err := os.Rename(s.tmp, path); err != nil {
		return fmt.Errorf("atomicfile: rename into %s: %w", path, err)
	}
	s.renamed = true
	return s.flushDir(path)
}

// Create puts the staged content at path only if no file is there, by hard link,
// and never replaces one. It returns an error satisfying errors.Is(err,
// fs.ErrExist) when the name is taken, whether it was taken before the call or by
// a writer that raced it; exactly one racer wins. After a successful link the
// directory is flushed if Options.Sync is set.
//
// A staged file is committed once. Create on one that was already placed is an
// error.
func (s *Staged) Create(path string) error {
	if err := s.claim(); err != nil {
		return err
	}
	if err := os.Link(s.tmp, path); err != nil {
		return fmt.Errorf("atomicfile: publish %s: %w", path, err)
	}
	return s.flushDir(path)
}

// Discard removes the temporary file if it is still there. It is safe to call
// after Replace or Create, and more than once, and it is what keeps a failed
// write from leaving a file behind.
func (s *Staged) Discard() {
	if s.renamed {
		return
	}
	_ = os.Remove(s.tmp)
}

func (s *Staged) claim() error {
	if s.used {
		return errors.New("atomicfile: the staged file was already placed")
	}
	s.used = true
	return nil
}

func (s *Staged) flushDir(path string) error {
	if !s.opts.Sync {
		return nil
	}
	if err := s.ops.syncDir(filepath.Dir(path)); err != nil {
		return fmt.Errorf("atomicfile: sync directory of %s: %w", path, err)
	}
	return nil
}

// backUp copies the file at path to path+".bak", in the same atomic way and with
// the same mode as the original. It does not follow a backup name that is a
// symlink.
//
// The file is opened once, without following a symlink (openNoFollow), and its
// mode and content both come from that descriptor. Replace has looked at the name
// by then, but the name can change between that look and this open; reading
// through the descriptor means a link swapped in meanwhile is refused (ErrSymlink)
// instead of read, and the mode kept is that of the content kept. Anything that is
// not a regular file at that point is refused too.
func (s *Staged) backUp(path string) error {
	f, err := openNoFollow(path)
	if err != nil {
		if errors.Is(err, ErrSymlink) {
			return err
		}
		return fmt.Errorf("atomicfile: read %s for its backup: %w", path, err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("atomicfile: read %s for its backup: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("atomicfile: read %s for its backup: not a regular file", path)
	}
	old, err := io.ReadAll(f)
	if err != nil {
		return fmt.Errorf("atomicfile: read %s for its backup: %w", path, err)
	}
	opts := s.opts
	opts.Perm, opts.Backup = info.Mode().Perm(), false
	backup, err := stage(s.ops, s.dir, old, opts)
	if err != nil {
		return fmt.Errorf("atomicfile: stage the backup of %s: %w", path, err)
	}
	defer backup.Discard()
	if err := backup.Replace(path + ".bak"); err != nil {
		return fmt.Errorf("atomicfile: back up %s: %w", path, err)
	}
	return nil
}

// WriteFile writes data to path atomically: Stage in path's directory, Replace,
// and Discard, so a failure leaves neither a half-written file nor a temporary
// one.
func WriteFile(path string, data []byte, opts Options) error {
	return writeFile(realOps(), path, data, opts)
}

func writeFile(o ops, path string, data []byte, opts Options) error {
	staged, err := stage(o, filepath.Dir(path), data, opts)
	if err != nil {
		return err
	}
	defer staged.Discard()
	return staged.Replace(path)
}
