package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// pathLookup finds an executable regular file called name in the absolute directories of path, the
// way a shell does, without starting anything (the runtime reaches no process except through a
// port). An empty path gives no lookup at all: the adapter then writes bare names and says nothing.
func pathLookup(path string) func(string) (string, error) {
	if path == "" {
		return nil
	}
	return func(name string) (string, error) {
		if name == "" || strings.ContainsRune(name, '/') {
			return "", fmt.Errorf("%q is not a program name", name)
		}
		for _, dir := range filepath.SplitList(path) {
			if !filepath.IsAbs(dir) {
				continue
			}
			candidate := filepath.Join(dir, name)
			if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0 {
				return candidate, nil
			}
		}
		return "", fmt.Errorf("%s not found on PATH", name)
	}
}
