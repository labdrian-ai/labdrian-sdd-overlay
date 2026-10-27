//go:build unix

package fsstore

import (
	"os"
	"syscall"
)

// openRead is how Read opens a file: without waiting, so that a named pipe put in its place after the look
// is opened at once, and refused as not the file that was looked at, instead of waiting for a writer.
const openRead = os.O_RDONLY | syscall.O_NONBLOCK
