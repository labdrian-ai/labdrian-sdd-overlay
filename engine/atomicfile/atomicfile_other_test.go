//go:build !linux && !darwin

package atomicfile

import (
	"errors"
	"path/filepath"
	"testing"
)

// A platform without a no-follow open has no opener, and so refuses a backup. The
// refusal itself is tested on every platform, with an ops whose opener is nil
// (TestWithoutANoFollowOpenABackupFailsClosedBeforeAnythingChanges); this pins that
// the real ops of these platforms are such an ops.
func TestThisPlatformHasNoNoFollowOpenAndRefusesABackup(t *testing.T) {
	if realOps().openNoFollow != nil {
		t.Fatal("realOps has a no-follow opener on a platform that does not have the open")
	}
	path := filepath.Join(t.TempDir(), "f")
	if err := WriteFile(path, []byte("new"), Options{Backup: true}); !errors.Is(err, ErrBackupUnsupported) {
		t.Errorf("WriteFile with a backup = %v, want ErrBackupUnsupported", err)
	}
}
