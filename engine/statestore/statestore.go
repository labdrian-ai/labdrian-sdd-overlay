// Package statestore is the one copy of what the file-backed stores of the engine
// (the workflow event log, the session binding store, the role handoff chain and
// the shaper clearance store) each wrote by hand to keep their records under the
// user's state home:
//
//   - Home resolves the state home from the environment: $XDG_STATE_HOME, or
//     $HOME/.local/state, either of them absolute.
//   - EnsureDirs creates the chain of store directories below it, private (0700),
//     and CheckDirs walks the same chain without creating anything. Both refuse a
//     symlink or a non-directory at any component below the state home; the state
//     home itself may be a symlink, because it is the user's own choice.
//   - OpenNoFollow and ReadFile read a record without following a final symlink and
//     without blocking on a FIFO, and require a regular file.
//   - Publish stores an immutable record: idempotent for identical bytes, a refusal
//     for different bytes, never a replacement, safe against racing writers.
//
// Its errors carry no store name, so a caller adds its own ("workflow store: ...")
// and keeps the message it always had. The sentinel errors tell a caller what
// happened without parsing the text.
//
// What it does not do: it takes no lock (see engine/filelock), it does not decide
// where a store lives below the state home, and it does not validate the names a
// store builds its paths from. Write-and-replace without the immutability rule is
// engine/atomicfile.
package statestore

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/atomicfile"
)

var (
	// ErrUnsupported is returned where the platform has no no-follow open: only
	// linux and darwin have one (see Supported).
	ErrUnsupported = errors.New("statestore: unsupported platform (linux or darwin required)")
	// ErrSymlink: a symlink was found where a plain directory or file is required.
	ErrSymlink = errors.New("statestore: symlink where a plain file or directory is required")
	// ErrNotDir: a store component exists and is not a directory.
	ErrNotDir = errors.New("statestore: store component is not a directory")
	// ErrNotRegular: a record path exists and is not a regular file.
	ErrNotRegular = errors.New("statestore: not a regular file")
	// ErrImmutable: an existing record holds different bytes than the ones offered.
	ErrImmutable = errors.New("statestore: immutable record holds different bytes")
)

// refusal is an error whose text is exactly the message a store always printed and
// which still answers errors.Is for its kind.
type refusal struct {
	kind error
	text string
}

func refuse(kind error, format string, args ...any) error {
	return &refusal{kind: kind, text: fmt.Sprintf(format, args...)}
}

func (r *refusal) Error() string { return r.text }
func (r *refusal) Unwrap() error { return r.kind }

// Home resolves the state home from the process environment (see HomeFrom).
func Home() (string, error) { return HomeFrom(os.Getenv) }

// HomeFrom resolves the directory under which labdrian keeps its local state,
// outside every worktree: $XDG_STATE_HOME when it is set, otherwise
// $HOME/.local/state. A set but relative XDG_STATE_HOME, and an unset, empty, or
// relative HOME fallback, are refused. It reads only the environment through
// getenv: it does not check that the directory exists or is usable.
func HomeFrom(getenv func(string) string) (string, error) {
	if xdg := getenv("XDG_STATE_HOME"); xdg != "" {
		if !filepath.IsAbs(xdg) {
			return "", fmt.Errorf("XDG_STATE_HOME %q is not absolute", xdg)
		}
		return filepath.Clean(xdg), nil
	}
	home := getenv("HOME")
	if home == "" || !filepath.IsAbs(home) {
		return "", fmt.Errorf("XDG_STATE_HOME is unset and HOME %q is not an absolute path", home)
	}
	return filepath.Join(home, ".local", "state"), nil
}

// EnsureDirs makes sure the state home (parts[0]) exists, then walks every
// component below it, creating the missing ones with mode 0700 and refusing any
// that is a symlink (ErrSymlink) or not a directory (ErrNotDir). A directory that
// already exists is accepted as it is, whatever its mode. The state home itself
// may be a symlink to a directory.
func EnsureDirs(parts []string) error {
	home := parts[0]
	if _, err := os.Stat(home); errors.Is(err, fs.ErrNotExist) {
		if err := os.MkdirAll(home, 0o700); err != nil {
			return fmt.Errorf("create state home: %w", err)
		}
	}
	if info, err := os.Stat(home); err != nil {
		return err
	} else if !info.IsDir() {
		return fmt.Errorf("state home %q is not a directory", home)
	}

	current := home
	for _, part := range parts[1:] {
		current = filepath.Join(current, part)
		if err := os.Mkdir(current, 0o700); err == nil {
			if err := os.Chmod(current, 0o700); err != nil {
				return err
			}
		} else if !errors.Is(err, fs.ErrExist) {
			return err
		}
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if err := requirePlainDir(current, info); err != nil {
			return err
		}
	}
	return nil
}

// CheckDirs walks the same chain as EnsureDirs and creates nothing. A component
// that does not exist is reported as the error of its inspection, which satisfies
// errors.Is(err, fs.ErrNotExist): the store is absent, not broken. A symlink is
// ErrSymlink and a non-directory ErrNotDir. The state home itself is not checked.
func CheckDirs(parts []string) error {
	current := parts[0]
	for _, part := range parts[1:] {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if err := requirePlainDir(current, info); err != nil {
			return err
		}
	}
	return nil
}

func requirePlainDir(path string, info fs.FileInfo) error {
	if info.Mode()&fs.ModeSymlink != 0 {
		return refuse(ErrSymlink, "refusing symlinked store component %q", path)
	}
	if !info.IsDir() {
		return refuse(ErrNotDir, "store component %q is not a directory", path)
	}
	return nil
}

// ReadFile reads path without following a final symlink (ErrSymlink) and without
// blocking on a FIFO, and requires a regular file (ErrNotRegular). A positive limit
// reads at most that many bytes, so a caller that reads one byte past its cap can
// tell the file is too large without reading the rest. A missing file satisfies
// errors.Is(err, fs.ErrNotExist).
func ReadFile(path string, limit int64) ([]byte, error) {
	f, err := OpenNoFollow(path)
	if err != nil {
		if IsSymlinkRefusal(err) {
			return nil, refuse(ErrSymlink, "refusing symlinked file %q", path)
		}
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, refuse(ErrNotRegular, "%q is not a regular file", path)
	}
	var r io.Reader = f
	if limit > 0 {
		r = io.LimitReader(f, limit)
	}
	return io.ReadAll(r)
}

// recordOptions is how a record is stored: private, flushed to disk, in a hidden
// temporary file of its own. A record is never replaced, so it is never backed up.
func recordOptions() atomicfile.Options {
	return atomicfile.Options{Perm: 0o600, Sync: true, TempPattern: ".record-*.tmp"}
}

// Publish stores data as the immutable record at path, in a directory that already
// exists (EnsureDirs makes it). The record is written and flushed to a temporary
// file and hard-linked into place, which, unlike a rename, can never replace an
// existing record, so two writers racing for a name cannot lose each other's data:
// exactly one wins.
//
// Storing identical bytes again is a no-op. Different bytes under an existing name
// are refused with ErrImmutable and the stored record is left as it was. A symlink
// at path is refused with ErrSymlink, even when the bytes would match.
func Publish(path string, data []byte) error {
	if _, err := os.Lstat(path); err == nil {
		return sameBytes(path, data)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	staged, err := atomicfile.Stage(filepath.Dir(path), data, recordOptions())
	if err != nil {
		return err
	}
	defer staged.Discard()
	if err := staged.Create(path); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return sameBytes(path, data)
		}
		return err
	}
	return nil
}

// sameBytes succeeds only when the record at path holds exactly data.
func sameBytes(path string, data []byte) error {
	existing, err := ReadFile(path, 0)
	if err != nil {
		return err
	}
	if !bytes.Equal(existing, data) {
		return refuse(ErrImmutable, "refusing to replace immutable record %q with different bytes", path)
	}
	return nil
}
