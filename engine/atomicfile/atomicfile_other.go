//go:build !linux && !darwin

package atomicfile

import "os"

// noFollowOpener is nil here: this platform has no no-follow open, so the check of
// a name and the open of it would be two steps with a window between them in which
// a symlink can be swapped in. A Replace that asks for a backup is refused with
// ErrBackupUnsupported instead (see Staged.Replace); everything else is
// unaffected, because only the backup reads the file it replaces.
func noFollowOpener() func(path string) (*os.File, error) { return nil }
