//go:build unix

package main

import (
	"os/exec"
	"syscall"
)

// ownProcessGroup puts the program in a process group of its own and makes the kill at the deadline
// reach the whole group: what the program started (the golden files show it forking binaries) does
// not outlive the run that reports it finished.
func ownProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
}
