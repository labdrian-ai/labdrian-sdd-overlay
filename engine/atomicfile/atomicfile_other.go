//go:build !linux && !darwin

package atomicfile

import (
	"fmt"
	"io/fs"
	"os"
)

// openNoFollow opens path read-only and refuses a symlink at it with ErrSymlink.
// This platform has no no-follow open, so the check and the open are two steps
// here and a link swapped in between is not caught; the engine's stores refuse to
// run on such a platform (see engine/statestore), and this is the best a plain
// build can do.
func openNoFollow(path string) (*os.File, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return nil, fmt.Errorf("%w: %s", ErrSymlink, path)
	}
	return os.Open(path)
}
