// Package durable holds the one file-replacement primitive every writer in
// longterm-mem uses to edit a file it does not own.
//
// The module previously carried two half-solutions side by side. Six
// writers did tmp+fsync+rename, which is atomic but destroys the file's
// identity: the replacement is a brand-new inode created by os.CreateTemp
// at 0o600, so a config the user had at 0o644 came back 0o600, a symlink
// into a dotfiles store was silently replaced by a regular file (leaving
// the user's real file unedited while register still reported success), and
// any hardlink was detached. Five more writers — including the rollback
// that exists specifically to undo a failed install, and all four .bak
// backups the whole recovery story rests on — used a plain truncating
// os.WriteFile, which preserves neither atomicity nor, in practice, the
// mode: os.WriteFile applies its perm argument only when it CREATES the
// file, so passing the captured mode of an existing file is dead code.
//
// WriteFile is the single replacement for both.
package durable

import (
	"fmt"
	"os"
	"path/filepath"
)

// maxSymlinkHops bounds the link-following loop in Resolve. Linux's own
// SYMLOOP_MAX is 40; a smaller budget is fine here because these are config
// paths, and the only thing a budget has to guarantee is that a cycle
// returns an error instead of spinning.
const maxSymlinkHops = 32

// WriteFile replaces path with data, preserving everything about the
// existing file that can be preserved alongside an atomic replacement.
//
// The sequence is:
//
//  1. resolve path through any symlinks, file and directory alike, to the
//     real file the user meant (Resolve). A dotfiles layout that points
//     ~/.claude.json at a tracked repository keeps working, and the temp
//     file lands in the RESOLVED file's directory — rename cannot cross
//     filesystems, and a dotfiles store is routinely a different mount;
//  2. create a temp file there, write data, fsync it, close it;
//  3. chmod the temp file to the existing file's permission bits — BEFORE
//     the rename, so the visible file never exists with the wrong mode for
//     even an instant. When the destination does not exist yet, createPerm
//     is used instead;
//  4. rename the temp file onto the destination.
//
// # What is deliberately NOT preserved
//
// Hardlinks. Replacing a file by rename always detaches it from any other
// name pointing at the old inode; the only way to keep a hardlink is to
// write through the existing inode, which means truncate-then-write — the
// exact non-atomic sequence that can leave a config half-written after a
// crash or a full disk. The trade-off is deliberate and it is not close:
// hardlinked config files are rare, and losing one is recoverable (the
// other name still holds the previous content, and a .bak sits next to the
// config), while a truncated ~/.claude.json is a broken editor with no copy
// of what used to be there. Atomicity wins.
//
// Ownership (uid/gid). A rename cannot carry them and chown needs
// privileges this process does not have, so a config owned by another user
// or a shared group becomes owned by the calling user. That only arises for
// files this process could not have safely edited in the first place.
//
// Setuid/setgid/sticky bits. Only the 0o777 permission bits are carried
// across. Re-applying a setuid bit to a file whose contents this process
// just rewrote is a privilege-escalation footgun, and no configuration file
// this module touches has any business carrying one.
//
// createPerm is applied exactly, without masking by the process umask,
// because it comes from a caller inside this module that has decided what
// the file should be — it is not a "default" the environment gets to
// soften. It only ever applies to a file that does not exist yet.
func WriteFile(path string, data []byte, createPerm os.FileMode) error {
	target, err := Resolve(path)
	if err != nil {
		return err
	}

	perm := createPerm
	if info, statErr := os.Stat(target); statErr == nil {
		perm = info.Mode().Perm()
	}

	dir := filepath.Dir(target)
	tmp, err := os.CreateTemp(dir, filepath.Base(target)+".tmp-*")
	if err != nil {
		return fmt.Errorf("durable: create temp file for %s: %w", target, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename below succeeds

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("durable: write temp file for %s: %w", target, err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("durable: fsync temp file for %s: %w", target, err)
	}
	// Chmod through the open handle rather than by name: the name is a
	// temp path in a directory other processes can write to, and going
	// through the descriptor removes the window between naming the file and
	// changing it.
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return fmt.Errorf("durable: set mode %o on temp file for %s: %w", perm, target, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("durable: close temp file for %s: %w", target, err)
	}

	if err := os.Rename(tmpName, target); err != nil {
		return fmt.Errorf("durable: rename temp file into %s: %w", target, err)
	}
	syncDir(dir)
	return nil
}

// Resolve follows path through symlinks — the file itself and every
// directory component — and returns the real path the write should land
// on.
//
// filepath.EvalSymlinks is not used directly because it fails outright when
// the final component does not exist, and both cases matter here: a config
// this module is about to create for the first time, and a DANGLING
// dotfiles symlink whose target has not been materialized yet. Both name
// exactly where the user wants the file, so both must resolve rather than
// error. The link chain is therefore walked by hand, and EvalSymlinks is
// used only on the parent directory, which does have to exist for a write
// to be possible at all.
func Resolve(path string) (string, error) {
	resolved := path
	for hop := 0; ; hop++ {
		if hop >= maxSymlinkHops {
			return "", fmt.Errorf("durable: %s: too many symlink levels (a symlink cycle, or a chain deeper than %d)", path, maxSymlinkHops)
		}
		info, err := os.Lstat(resolved)
		if err != nil {
			// Does not exist yet: this is where the file goes.
			break
		}
		if info.Mode()&os.ModeSymlink == 0 {
			break
		}
		dest, err := os.Readlink(resolved)
		if err != nil {
			return "", fmt.Errorf("durable: read symlink %s: %w", resolved, err)
		}
		if !filepath.IsAbs(dest) {
			dest = filepath.Join(filepath.Dir(resolved), dest)
		}
		resolved = dest
	}

	// The parent directory chain may itself contain symlinks (a whole
	// config directory linked into a dotfiles store). Resolving it keeps
	// the temp file on the same filesystem as the destination, which is
	// what makes the rename atomic rather than EXDEV.
	dir, base := filepath.Split(resolved)
	realDir, err := filepath.EvalSymlinks(filepath.Clean(dir))
	if err != nil {
		// The parent does not exist (or cannot be resolved). Leave the
		// path lexical and let the temp-file creation report the real
		// problem, which is a better error than anything invented here.
		return resolved, nil
	}
	return filepath.Join(realDir, base), nil
}

// syncDir fsyncs a directory so the rename that just committed survives a
// power loss, not merely a process crash.
//
// Its error is deliberately dropped. By the time it runs the rename has
// already succeeded and the new content IS the file every reader sees;
// reporting a failure here would tell the caller the write did not happen
// when it did. That lie is not academic — register's installWithRollback
// treats a config-write error as "nothing landed" and returns without
// rolling back, which is exactly the state (a config entry install-state
// has no record of) that makes every later run refuse. A directory whose
// fsync fails is a real problem, but it is not this function's to report.
func syncDir(dir string) {
	d, err := os.Open(dir)
	if err != nil {
		return
	}
	_ = d.Sync()
	_ = d.Close()
}
