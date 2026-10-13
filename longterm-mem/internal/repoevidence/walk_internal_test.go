package repoevidence

import (
	"errors"
	"io/fs"
	"testing"
)

// aFile is a directory entry that is a plain file.
type aFile struct{}

func (aFile) Name() string               { return "elsewhere.go" }
func (aFile) IsDir() bool                { return false }
func (aFile) Type() fs.FileMode          { return 0 }
func (aFile) Info() (fs.FileInfo, error) { return nil, errors.New("not needed") }

// An entry whose path cannot be made relative to the root is an error of the walk, not an entry that is left
// out: the listing is the whole tree or the reason it could not be listed. (A walk only meets paths below its
// root, so this cannot happen on a real tree; the callback is tried with one that is not.)
func TestCollect_FailsOnAPathThatIsNotBelowTheRootInsteadOfLeavingItOut(t *testing.T) {
	var files []string

	err := collect("relative/root", &files)("/absolute/elsewhere.go", aFile{}, nil)

	if err == nil || len(files) != 0 {
		t.Errorf("collect = %v with files %v; want the error of the path and no file listed", err, files)
	}
}

// The words for a walk that fails are the ones the report has always printed, whoever runs the tests: they are
// built from the error a refused directory gives, which a process with root's capability cannot be made to get
// from the file system.
func TestWalkError_SaysWhyInTheWordsTheReportHasAlwaysUsed(t *testing.T) {
	denied := &fs.PathError{Op: "open", Path: "/r/locked", Err: fs.ErrPermission}

	for name, tc := range map[string]struct {
		err  error
		want string
	}{
		"a directory that cannot be read": {denied, "staleness: walking /r: open /r/locked: permission denied"},
		"a root that is not there":        {&fs.PathError{Op: "lstat", Path: "/r", Err: fs.ErrNotExist}, "staleness: /r does not exist"},
		"any other failure":               {errors.New("boom"), "staleness: walking /r: boom"},
	} {
		got := walkError("/r", tc.err)
		if got.Error() != tc.want {
			t.Errorf("%s: walkError = %q, want %q", name, got, tc.want)
		}
	}
	if !errors.Is(walkError("/r", denied), fs.ErrPermission) {
		t.Error("walkError does not wrap the file system's own error")
	}
}
