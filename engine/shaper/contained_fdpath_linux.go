//go:build linux

package shaper

import (
	"fmt"
	"os"
	"strings"
)

// fdPath returns the path the kernel records for the open descriptor, read
// from /proc/self/fd. It fails closed when /proc is unavailable and when the
// file was unlinked after the open (the kernel appends " (deleted)").
func fdPath(f *os.File) (string, error) {
	var (
		path    string
		readErr error
	)
	conn, err := f.SyscallConn()
	if err != nil {
		return "", fmt.Errorf("descriptor path: %w", err)
	}
	if err := conn.Control(func(fd uintptr) {
		path, readErr = os.Readlink(fmt.Sprintf("/proc/self/fd/%d", fd))
	}); err != nil {
		return "", fmt.Errorf("descriptor path: %w", err)
	}
	if readErr != nil {
		return "", fmt.Errorf("descriptor path: %w", readErr)
	}
	if strings.HasSuffix(path, " (deleted)") {
		return "", fmt.Errorf("descriptor path: opened file was unlinked")
	}
	return path, nil
}
