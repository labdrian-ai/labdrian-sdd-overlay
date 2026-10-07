package skillsfs_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills/skillsfs"
)

func TestProjectLocksReadTheFileTheDomainNames(t *testing.T) {
	root := t.TempDir()
	put(t, skills.ProjectLockPath(root), `{"version": 1}`)

	got, err := skillsfs.ProjectLocks{}.ReadLock(root)
	if err != nil || string(got) != `{"version": 1}` {
		t.Errorf("ReadLock = %q, %v, want the bytes of the project lock", got, err)
	}
	if want := filepath.Join(root, ".labdrian", "procedural-skills.lock.json"); skills.ProjectLockPath(root) != want {
		t.Errorf("ProjectLockPath = %q, want %q", skills.ProjectLockPath(root), want)
	}
}

func TestProjectLocksSayAProjectWithNoLockWithTheErrorThatMeansAbsent(t *testing.T) {
	if _, err := (skillsfs.ProjectLocks{}).ReadLock(t.TempDir()); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ReadLock of a project with no lock: %v, want fs.ErrNotExist", err)
	}
}

// A lock that is a directory is not "no lock": the failure is the system's and is not
// fs.ErrNotExist, so a verb refuses instead of replacing it with an empty one.
func TestProjectLocksDoNotTakeALockThatCannotBeReadForAbsent(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(skills.ProjectLockPath(root), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := (skillsfs.ProjectLocks{}).ReadLock(root); err == nil || errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ReadLock of a lock that is a directory: %v, want a failure that is not fs.ErrNotExist", err)
	}
}
