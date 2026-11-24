//go:build unix

package execrunner

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// ownGroup starts cmd in a process group of its own and makes the kill at the deadline reach the whole group, so what the
// program forked does not outlive the call. A group that has no process left (ESRCH) is the program done, as exec.Cmd
// reads os.ErrProcessDone.
func ownGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
}
