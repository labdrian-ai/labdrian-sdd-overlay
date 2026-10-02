//go:build !linux && !darwin

package fsadapter

import (
	"fmt"
	"os"
	"runtime"
)

// There is no no-follow open here (statestore.OpenNoFollow refuses with ErrUnsupported, so
// the read stops before this), and this platform cannot ask the kernel which path a
// descriptor names either.

// fdPath fails closed on unsupported platforms.
func fdPath(f *os.File) (string, error) {
	return "", fmt.Errorf("descriptor path is unsupported on %s", runtime.GOOS)
}
