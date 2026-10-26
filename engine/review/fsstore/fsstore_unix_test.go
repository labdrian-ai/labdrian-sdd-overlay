//go:build linux || darwin

package fsstore_test

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/fsstore"
)

// The modes of what the store makes are 0755 for a directory and 0644 for the file whatever the umask of the
// process says: a result is evidence that other users of the checkout read, and a umask of 077 must not turn
// the directories into ones they cannot enter. The umask is a setting of the whole process, so the test puts
// it back at once and no test of this package runs in parallel with it.
func TestTheModesOfWhatTheStoreMakesAreTheSameWhateverTheUmaskSays(t *testing.T) {
	for _, umask := range []int{0o000, 0o022, 0o077} {
		root := t.TempDir()
		old := syscall.Umask(umask)
		err := (fsstore.Store{}).Create(root, "own-rdd", resultName, []byte("x"))
		syscall.Umask(old)
		if err != nil {
			t.Fatalf("umask %04o: Create() error = %v, want nil", umask, err)
		}

		for _, tc := range []struct {
			path string
			want os.FileMode
		}{
			{"odd", 0o755},
			{"odd/review-results", 0o755},
			{"odd/review-results/own-rdd", 0o755},
			{"odd/review-results/own-rdd/" + resultName, 0o644},
		} {
			info, err := os.Stat(filepath.Join(root, filepath.FromSlash(tc.path)))
			if err != nil {
				t.Fatalf("umask %04o: %v", umask, err)
			}
			if got := info.Mode().Perm(); got != tc.want {
				t.Errorf("umask %04o: %s has mode %04o, want %04o", umask, tc.path, got, tc.want)
			}
		}
	}
}
