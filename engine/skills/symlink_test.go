package skills

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// symlinkUnavailable reports whether err, returned by os.Symlink on goos, means
// that this platform or this account cannot make a symbolic link at all, which is
// the one reason a test that needs a link may skip. Windows without the privilege
// is the known case (goos is windows and the error is not a plain permission
// error, so the platform alone decides), and a permission or unsupported-operation
// error covers a restricted account or a file system with no links. Every other
// error is a real failure: CI runs on Linux, where a link can always be made, and
// a skip there would hide the regression the test exists to catch.
func symlinkUnavailable(goos string, err error) bool {
	return goos == "windows" || errors.Is(err, fs.ErrPermission) || errors.Is(err, errors.ErrUnsupported)
}

// makeSymlink creates the link newname -> oldname for a test whose premise is the
// link. It skips the test only when symlinkUnavailable says links cannot be made
// here, and fails it on any other error.
func makeSymlink(t *testing.T, oldname, newname string) {
	t.Helper()
	err := os.Symlink(oldname, newname)
	if err == nil {
		return
	}
	if symlinkUnavailable(runtime.GOOS, err) {
		t.Skipf("cannot make a symbolic link here: %v", err)
	}
	t.Fatalf("symlink %s -> %s: %v", newname, oldname, err)
}

func TestSymlinkUnavailable_OnlyThePlatformOrTheAccountMaySkip(t *testing.T) {
	linkErr := func(err error) error {
		return &os.LinkError{Op: "symlink", Old: "old", New: "new", Err: err}
	}
	for _, tc := range []struct {
		name string
		goos string
		err  error
		want bool
	}{
		{"windows, whatever the error says", "windows", errors.New("A required privilege is not held by the client."), true},
		{"a permission error on linux", "linux", linkErr(fs.ErrPermission), true},
		{"an unsupported operation on linux", "linux", linkErr(errors.ErrUnsupported), true},
		{"the name already exists on linux", "linux", linkErr(fs.ErrExist), false},
		{"the parent is missing on linux", "linux", linkErr(fs.ErrNotExist), false},
		{"an unknown error on linux", "linux", errors.New("boom"), false},
		{"an unknown error on darwin", "darwin", linkErr(errors.New("boom")), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := symlinkUnavailable(tc.goos, tc.err); got != tc.want {
				t.Errorf("symlinkUnavailable(%q, %v) = %v, want %v", tc.goos, tc.err, got, tc.want)
			}
		})
	}
}

func TestMakeSymlink_MakesTheLinkWhereLinksCanBeMade(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	makeSymlink(t, target, link)
	if got, err := os.Readlink(link); err != nil || got != target {
		t.Errorf("Readlink = %q, %v, want %q", got, err, target)
	}
}
