//go:build unix

package fsstore

import (
	"os"
	"syscall"
)

// openRead is how Read opens a file: without waiting, so that a named pipe put in its place after the look
// is opened at once, and refused as not the file that was looked at, instead of waiting for a writer. The
// descriptor stays non-blocking for the read that follows, which changes nothing for a regular file, the only
// kind that is read.
const openRead = os.O_RDONLY | syscall.O_NONBLOCK
