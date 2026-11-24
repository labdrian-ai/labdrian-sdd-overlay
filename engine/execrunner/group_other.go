//go:build !unix

package execrunner

import "os/exec"

// ownGroup does nothing here: a build that is not Unix has no process group in syscall.SysProcAttr (a Windows Job object is
// not used), so the deadline stops the program alone and a process it forked can outlive the call, up to killGrace.
func ownGroup(*exec.Cmd) {}
