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
		errno := errno
		failing := locker{flock: func(fd, how int) error { return errno }}
		unlock, err := failing.acquire(dir, Options{Clock: newFakeClock().Clock()}, true)

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
	failing := locker{flock: func(fd, how int) error { return syscall.ENOLCK }}

	_, err := failing.acquire(path, Options{Clock: Clock{Now: time.Now}}, false)
	if err == nil || errors.Is(err, ErrBusy) || strings.Contains(err.Error(), "directory") {
		t.Errorf("Acquire = %v, want a plain error that does not mention directories", err)
	}
}

// The seam is a field of the locker, not a variable of the package: a locker
// with a failing flock does not change what the package-level Acquire does, in
// this test or in any other that runs beside it.
func TestAFailingLockerDoesNotLeakIntoThePackageLevelAcquire(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".fixture.lock")
	failing := locker{flock: func(fd, how int) error { return syscall.ENOLCK }}
	if _, err := failing.acquire(path, Options{}, false); err == nil {
		t.Fatal("the failing locker acquired a lock")
	}
	unlock, err := Acquire(path, Options{})
	if err != nil {
		t.Fatalf("Acquire after a failing locker was used = %v, want a held lock", err)
	}
	unlock()
}

// Perm is the mode of a lock file an Exclusive acquire creates, before the umask,
// so a caller whose lock lives in a private directory can keep it private. The
// assertion holds the umask fixed, as the 0644 test does.
func TestPermSetsTheModeOfACreatedLockFileBeforeTheUmask(t *testing.T) {
	for _, tc := range []struct {
		umask int
		perm  os.FileMode
		want  os.FileMode
	}{
		{0o022, 0o600, 0o600},
		{0o022, 0o640, 0o640},
		{0o077, 0o640, 0o600},
		{0o000, 0o600, 0o600},
	} {
		old := syscall.Umask(tc.umask)
		path := filepath.Join(t.TempDir(), ".mode.lock")
		unlock, err := Acquire(path, Options{Perm: tc.perm})
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
			t.Errorf("umask %04o, Perm %04o: lock file mode %04o, want %04o", tc.umask, tc.perm, got, tc.want)
		}
	}
}

// Perm is the mode of a lock file an Exclusive acquire creates, so only that
// acquire uses it and only that acquire checks it. A directory lock creates no
// file and a Shared lock never creates one, so the same Options that an Exclusive
// file lock refuses must not stop AcquireDir in either mode, nor a Shared file
// lock whether or not the file is there; the Exclusive file lock still refuses
// them, and creates nothing when it does.
func TestAnInvalidPermStopsAnExclusiveFileLockOnly(t *testing.T) {
	invalid := os.ModeSetuid | 0o600
	dir := dirLockTarget(t)
	for _, mode := range []Mode{Exclusive, Shared} {
		unlock, err := AcquireDir(dir, Options{Mode: mode, Perm: invalid})
		if err != nil {
			t.Errorf("AcquireDir(mode %v) with a Perm that only applies to a created file = %v, want a held lock", mode, err)
			continue
		}
		unlock()
	}

	existing := filepath.Join(t.TempDir(), ".existing.lock")
	if unlock, err := Acquire(existing, Options{}); err != nil {
		t.Fatal(err)
	} else {
		unlock()
	}
	unlock, err := Acquire(existing, Options{Mode: Shared, Perm: invalid})
	if err != nil {
		t.Errorf("Acquire(Shared) of a lock file that exists, with a Perm that only applies to a created file = %v, want a held lock", err)
	} else {
		unlock()
	}

	missing := filepath.Join(t.TempDir(), ".missing.lock")
	unlock, err = Acquire(missing, Options{Mode: Shared, Perm: invalid})
	if err != nil {
		t.Errorf("Acquire(Shared) of a lock file that is missing, with a Perm that only applies to a created file = %v, want the provisional lock that holds nothing", err)
	} else {
		unlock()
	}
	if _, statErr := os.Stat(missing); statErr == nil {
		t.Error("a Shared acquire created the lock file")
	}

	path := filepath.Join(t.TempDir(), ".fixture.lock")
	if _, err := Acquire(path, Options{Perm: invalid}); err == nil || !strings.Contains(err.Error(), "permission bits") {
		t.Errorf("Acquire(Exclusive) with the same Perm = %v, want a refusal naming the permission bits", err)
	}
	if _, statErr := os.Stat(path); statErr == nil {
		t.Error("a lock file was created although Perm was refused")
	}
}

// An Exclusive acquire checks Perm whether or not the lock file exists yet, so the
// same Options behave the same on the first call as on the hundredth.
func TestAnInvalidPermStopsAnExclusiveAcquireOfAnExistingLockFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".existing.lock")
	unlock, err := Acquire(path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	unlock()

	if _, err := Acquire(path, Options{Perm: os.ModeSticky | 0o600}); err == nil || !strings.Contains(err.Error(), "permission bits") {
		t.Errorf("Acquire(Exclusive) of an existing lock file with an invalid Perm = %v, want a refusal naming the permission bits", err)
	}
}

func TestPermMustBePermissionBitsOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".fixture.lock")
	_, err := Acquire(path, Options{Perm: os.ModeSetuid | 0o600})
	if err == nil || !strings.Contains(err.Error(), "permission bits") {
		t.Errorf("Acquire with a setuid Perm = %v, want a refusal naming the permission bits", err)
	}
	if _, statErr := os.Stat(path); statErr == nil {
		t.Error("a lock file was created although Perm was refused")
	}
}
