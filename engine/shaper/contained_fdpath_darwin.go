//go:build darwin

package shaper

import (
	"bytes"
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

// fGetPath is fcntl's F_GETPATH command on darwin; the syscall package does
// not export it.
const fGetPath = 50

// fdPath returns the path the kernel records for the open descriptor, via
// fcntl(F_GETPATH). It fails closed on any error.
func fdPath(f *os.File) (string, error) {
	buf := make([]byte, 1024) // MAXPATHLEN on darwin
	var errno syscall.Errno
	conn, err := f.SyscallConn()
	if err != nil {
		return "", fmt.Errorf("descriptor path: %w", err)
	}
	if err := conn.Control(func(fd uintptr) {
		_, _, errno = syscall.Syscall(syscall.SYS_FCNTL, fd, fGetPath, uintptr(unsafe.Pointer(&buf[0])))
	}); err != nil {
		return "", fmt.Errorf("descriptor path: %w", err)
	}
	if errno != 0 {
		return "", fmt.Errorf("descriptor path: %w", errno)
	}
	n := bytes.IndexByte(buf, 0)
	if n < 0 {
		return "", fmt.Errorf("descriptor path: unterminated result")
	}
	return string(buf[:n]), nil
}
