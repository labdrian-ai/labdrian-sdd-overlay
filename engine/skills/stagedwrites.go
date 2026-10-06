package skills

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

// StagedWrites is the port through which a verb writes a file so that a reader sees all of the
// old content or all of the new, never a mix: it stages the new content in a temporary file
// next to the destination (WriteTemp), then moves it into place (Rename), or throws it away
// (Remove). The two files of `add` and `remove` are staged first and renamed after, which is
// the order that keeps the registry and the manifest from being seen between their two writes
// (ADR-9). The port is owned here and implemented by engine/skills/skillsfs; ProjectFS embeds it,
// because the files of a project are written the same way.
type StagedWrites interface {
	// WriteTemp writes data to a fresh temporary file in dir at mode perm, whatever the mask of
	// the process, and returns its path; the caller owns the rename. The file is created in the
	// destination's own directory, written, synced and closed before it returns, and its name
	// begins ".tmp-skills-" followed by a unique suffix, which is how a copier of a tree knows
	// it for half of a write (SkipWhenCopying). A failure leaves no file and says the step it
	// failed at in its first words (create temp, write, sync, close, chmod); the caller words
	// the rest for its verb.
	WriteTemp(dir string, data []byte, perm fs.FileMode) (string, error)
	// Rename moves oldPath to newPath, replacing a file that is there.
	Rename(oldPath, newPath string) error
	// Remove deletes the file or the empty directory name.
	Remove(name string) error
}

// atomicTempPrefix begins the name of every temporary file a writer of an overlay or a project
// makes, in the directory of the file it is about to replace. A copier that walks a directory
// another verb may be writing in skips names that carry it and a unique suffix (see
// SkipWhenCopying): such a file is half a write, never skill content. WriteTemp makes the names;
// this is the rule a reader applies to them.
const atomicTempPrefix = ".tmp-skills-"

// overlayFileMode is the mode of the registry and the manifest a verb writes: readable and
// writable by their owner only, as a temporary file is made.
const overlayFileMode fs.FileMode = 0o600

// writeFileAtomic stages data in a temporary file in the directory of path, synced, and returns
// the temporary file's path. The caller finishes with Rename onto path, or Remove. A failure is
// worded 'writeFileAtomic: <the step it failed at>: <the cause>', as it always has been.
func writeFileAtomic(files StagedWrites, path string, data []byte, perm fs.FileMode) (string, error) {
	dir := path[:strings.LastIndex(path, string(filepath.Separator))+1]
	if dir == "" {
		dir = "."
	}
	tmp, err := files.WriteTemp(dir, data, perm)
	if err != nil {
		return "", fmt.Errorf("writeFileAtomic: %w", err)
	}
	return tmp, nil
}
