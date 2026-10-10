//go:build unix

package main

import (
	"syscall"
	"testing"
)

// fixedUmask sets the process umask to 022 for the test, so that the modes of the files and directories a
// golden file records do not depend on the umask of whoever runs the tests.
func fixedUmask(t *testing.T) {
	t.Helper()
	previous := syscall.Umask(0o022)
	t.Cleanup(func() { syscall.Umask(previous) })
}
