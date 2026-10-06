//go:build !unix

package main

import "os/exec"

// ownProcessGroup does nothing where process groups are not: the kill at the deadline reaches the
// program only, and cmd.WaitDelay (see runWithin) is what keeps a descendant that holds the
// output pipes from holding the run.
func ownProcessGroup(*exec.Cmd) {}
