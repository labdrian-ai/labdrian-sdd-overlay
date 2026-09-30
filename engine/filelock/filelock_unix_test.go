//go:build linux || darwin

package filelock

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Not every filesystem can flock a directory (some network and FUSE filesystems
// refuse it). When the kernel says so, AcquireDir must fail with an error that
// says what happened and that nothing was locked: never report a lock it does not
// hold, never pass for a busy lock, and never fall back to another place.
func TestAcquireDirOnAFilesystemThatCannotLockADirectoryFailsClosedWithAClearError(t *testing.T) {
	dir := dirLockTarget(t)
	before, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	for _, errno := range []syscall.Errno{syscall.ENOTSUP, syscall.ENOLCK, syscall.EBADF, syscall.EINVAL} {
		saved := flock
		flock = func(fd, how int) error { return errno }
		unlock, err := AcquireDir(dir, Options{Clock: newFakeClock().Clock()})
		flock = saved

		if unlock != nil || err == nil || errors.Is(err, ErrBusy) {
			t.Errorf("%v: AcquireDir = (unlock nil: %v, %v), want no lock and a plain error", errno, unlock == nil, err)
			continue
		}
		for _, want := range []string{dir, errno.Error(), "may not support locking a directory", "nothing was locked"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("%v: error %q does not contain %q", errno, err, want)
			}
		}
	}
	after, err := os.ReadDir(dir)
	if err != nil || len(after) != len(before) {
		t.Errorf("the directory changed: %v -> %v (%v)", before, after, err)
	}
}

// The lock file is created 0644 before the umask, not 0666: a checkout shared by
// several users can still lock it (a shared lock only needs to read it), but no
// other user can write into it. The assertion holds the umask fixed so that the
// mode the code asks for, not the machine's umask, is what is compared.
func TestTheLockFileIsCreatedWithMode0644BeforeTheUmask(t *testing.T) {
	for _, tc := range []struct {
		umask int
		want  os.FileMode
	}{
		{0o000, 0o644},
		{0o022, 0o644},
		{0o077, 0o600},
	} {
		old := syscall.Umask(tc.umask)
		path := filepath.Join(t.TempDir(), ".mode.lock")
		unlock, err := Acquire(path, Options{})
		syscall.Umask(old)
		if err != nil {
			t.Fatal(err)
		}
		unlock()

		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != tc.want {
			t.Errorf("umask %04o: lock file mode %04o, want %04o", tc.umask, got, tc.want)
		}
	}
}

// A file lock that fails for the same reason does not blame directories.
func TestAcquireOnAFailingFilesystemDoesNotMentionDirectories(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".fixture.lock")
	saved := flock
	flock = func(fd, how int) error { return syscall.ENOLCK }
	defer func() { flock = saved }()

	_, err := Acquire(path, Options{Clock: Clock{Now: time.Now}})
	if err == nil || errors.Is(err, ErrBusy) || strings.Contains(err.Error(), "directory") {
		t.Errorf("Acquire = %v, want a plain error that does not mention directories", err)
	}
}
