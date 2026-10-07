package skillsfs

import (
	"os"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// ProjectLocks is the skills.ProjectLockStore of the real file system: the lock of a project is
// read from the path the domain names (skills.ProjectLockPath). It holds nothing: the zero value
// is ready, and it returns the failure of the system as it is, so a project with no lock is an
// error that is fs.ErrNotExist and the words of every other refusal are the system's.
type ProjectLocks struct{}

var _ skills.ProjectLockStore = ProjectLocks{}

// ReadLock reads the lock of the project whose root directory is root.
func (ProjectLocks) ReadLock(root string) ([]byte, error) {
	return os.ReadFile(skills.ProjectLockPath(root))
}
