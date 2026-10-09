// Package settingsfile is the adapter that reads and writes a Claude Code settings.json on the
// machine. It is the file half of the settings package: settings.Document is the content and the
// rules for the hook entries, this package is where the bytes come from and go to.
//
// A write is on atomicfile: the new content is staged in the directory of the file it replaces, the
// file it replaces is kept as <file>.bak by atomicfile's own backup (whole or not at all, at the mode
// the file had, and refused when the backup name is a symbolic link), and the staged file is renamed
// into place, so a reader sees all of the old file or all of the new and a failure leaves the
// original untouched. The file written is 0600 whatever the one it replaces was. A settings.json
// that is a symbolic link is followed: the file it names is read and replaced, the link stays, and
// the backup sits next to that file (see File.resolve). The backup needs a no-follow open, which
// atomicfile has on linux and darwin only; elsewhere a write fails before it changes anything.
//
// SAFETY: nothing here chooses a path. The caller passes the settings file, and tests use
// t.TempDir(), so the live ~/.claude/settings.json is never named by this package.
package settingsfile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/atomicfile"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings"
)

// writeMode is the permission bits of the settings.json this package writes. The file it replaces
// may have had others; a rename never carries them over.
const writeMode = 0o600

// tempPattern names the temporary file a write stages in the directory of settings.json.
const tempPattern = ".settings-*.json.tmp"

// File is a settings.json at a path.
type File struct{ Path string }

// Bytes reads the file. found is false when there is none, which is not an error: a machine that
// has never been configured has no settings.json. Any other failure is returned as the system
// reported it.
func (f File) Bytes() (data []byte, found bool, err error) {
	data, err = os.ReadFile(f.Path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return data, true, nil
}

// resolve is the file the path stands for: the path itself, or, when it is a symbolic link, the
// file at the end of the chain of links. Reading and writing act on that file and the link stays a
// link, which is what a dotfile manager that keeps settings.json elsewhere expects. A link that
// leads nowhere or loops cannot be followed; it is refused, naming the path, before anything is
// read, staged or written.
func (f File) resolve() (string, error) {
	info, err := os.Lstat(f.Path)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		return f.Path, nil
	}
	target, err := filepath.EvalSymlinks(f.Path)
	if err != nil {
		return "", fmt.Errorf("settings: %s is a symbolic link that cannot be followed (nothing was changed): %w", f.Path, err)
	}
	return target, nil
}

// Read parses the file. A file that does not exist is the empty document, found false, ready to be
// written as a new file. A file that cannot be read, or whose JSON is not an object, is an error
// that names the path and says the file was not modified. A symbolic link is read through; one that
// cannot be followed is an error (see resolve).
func (f File) Read() (doc settings.Document, found bool, err error) {
	target, err := f.resolve()
	if err != nil {
		return settings.Document{}, false, err
	}
	data, found, err := File{Path: target}.Bytes()
	if err != nil {
		return settings.Document{}, false, fmt.Errorf("settings: read %s: %w", f.Path, err)
	}
	if !found {
		return settings.Empty(), false, nil
	}
	doc, err = settings.Parse(data)
	if err != nil {
		return settings.Document{}, true, fmt.Errorf("settings: %s contains invalid JSON (not modified): %w", f.Path, err)
	}
	return doc, true, nil
}

// Write puts doc at the path, or at the file the path is a link to (see resolve). The directory
// must exist. If a file is there, it is kept first as <file>.bak, next to it, at its own mode; a
// backup that cannot be made, or whose name is a symbolic link, stops the write with the original
// untouched. The new file is staged in the directory of the file it replaces, so the rename never
// crosses a file system and the link is never touched.
func (f File) Write(doc settings.Document) error {
	data, err := doc.Bytes()
	if err != nil {
		return err
	}
	target, err := f.resolve()
	if err != nil {
		return err
	}

	staged, err := atomicfile.Stage(filepath.Dir(target), data, atomicfile.Options{Perm: writeMode, Backup: true, TempPattern: tempPattern})
	if err != nil {
		return fmt.Errorf("settings: create temp: %w", err)
	}
	defer staged.Discard()

	if err := staged.Replace(target); err != nil {
		return fmt.Errorf("settings: replace %s: %w", target, err)
	}
	return nil
}
