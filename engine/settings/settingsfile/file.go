// Package settingsfile is the adapter that reads and writes a Claude Code settings.json on the
// machine. It is the file half of the settings package: settings.Document is the content and the
// rules for the hook entries, this package is where the bytes come from and go to.
//
// A write is what it has always been, on atomicfile: the new content is staged in the same
// directory, the file it replaces is copied to settings.json.bak, and the staged file is renamed
// into place, so a reader sees all of the old file or all of the new and a failure leaves the
// original untouched. The file written is 0600 whatever the one it replaces was, and the backup is
// 0644 (less the umask), written in place. A settings.json that is a symbolic link is not replaced
// (see File.Write).
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

// backupMode is the permission bits asked for the backup, before the umask.
const backupMode = 0o644

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

// Read parses the file. A file that does not exist is the empty document, found false, ready to be
// written as a new file. A file that cannot be read, or whose JSON is not an object, is an error
// that names the path and says the file was not modified.
func (f File) Read() (doc settings.Document, found bool, err error) {
	data, found, err := f.Bytes()
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

// Write puts doc at the path. The directory must exist. If a file is there, its content is first
// copied to <path>.bak, and a backup that cannot be written stops the write with the original
// untouched. A symbolic link at the path is refused (atomicfile.ErrSymlink), checked before
// anything is staged or copied: replacing it would silently break the link of a dotfile manager,
// and writing through it would put the new content where the caller did not name.
func (f File) Write(doc settings.Document) error {
	data, err := doc.Bytes()
	if err != nil {
		return err
	}
	if info, err := os.Lstat(f.Path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("settings: %s is a symbolic link and is not replaced (nothing was changed): %w", f.Path, atomicfile.ErrSymlink)
	}

	staged, err := atomicfile.Stage(filepath.Dir(f.Path), data, atomicfile.Options{Perm: writeMode, TempPattern: tempPattern})
	if err != nil {
		return fmt.Errorf("settings: create temp: %w", err)
	}
	defer staged.Discard()

	if _, err := os.Stat(f.Path); err == nil {
		backup := f.Path + ".bak"
		if err := copyFile(f.Path, backup); err != nil {
			return fmt.Errorf("settings: backup to %s: %w", backup, err)
		}
	}
	if err := staged.Replace(f.Path); err != nil {
		return fmt.Errorf("settings: rename temp to %s: %w", f.Path, err)
	}
	return nil
}

// copyFile copies src to dst, creating dst if it does not exist.
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, backupMode)
}
