//go:build !unix

package fsstore

import "os"

// openRead is how Read opens a file. This platform has no named pipe that a path can lead to.
const openRead = os.O_RDONLY
