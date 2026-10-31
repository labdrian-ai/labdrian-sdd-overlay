// Package fsdoc is the adapter of the Documents port of engine/oddtask/app: it reads a feature document
// from the file system.
package fsdoc

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/oddtask"
)

// Reader reads feature documents from the file system.
type Reader struct {
	// Root is the root of the git worktree that holds dir, or "" when no repository holds it. The adapter
	// imports no git package: the caller hands the lookup in.
	Root func(dir string) string
}

// Read returns the document at path. A relative path is resolved against the root of the git worktree that
// holds cwd, not against cwd, and is refused when it has no cwd (oddtask.CauseNoCwd), when it leaves the
// repository (oddtask.CauseOutsideRepository) or when no repository holds cwd (oddtask.CauseNoRepository),
// each as an *oddtask.ReadError whose Path is the path as cited. An absolute path is read as it is. The last
// component of the path must not be a symbolic link and must be a regular file of at most
// oddtask.MaxDocumentBytes; a link in a directory of the path is followed. Every other refusal is an
// *oddtask.ReadError that names the resolved path.
func (r Reader) Read(path, cwd string) (oddtask.File, error) {
	resolved := filepath.Clean(path)
	if !filepath.IsAbs(resolved) {
		refuseCited := func(cause string) (oddtask.File, error) {
			return oddtask.File{}, &oddtask.ReadError{Path: path, Cause: cause}
		}
		if cwd == "" {
			return refuseCited(oddtask.CauseNoCwd)
		}
		if !filepath.IsLocal(resolved) {
			return refuseCited(oddtask.CauseOutsideRepository)
		}
		root := ""
		if r.Root != nil {
			root = r.Root(cwd)
		}
		if root == "" {
			return refuseCited(oddtask.CauseNoRepository)
		}
		resolved = filepath.Join(root, resolved)
	}
	refuse := func(cause string) (oddtask.File, error) {
		return oddtask.File{}, &oddtask.ReadError{Path: resolved, Cause: cause}
	}
	info, err := os.Lstat(resolved)
	if err != nil {
		return refuse(causeOf(err))
	}
	if cause := refusalOf(info); cause != "" {
		return refuse(cause)
	}
	f, err := os.Open(resolved)
	if err != nil {
		return refuse(causeOf(err))
	}
	defer f.Close()
	// The file opened is the one inspected: a path swapped for a link or another file in between is refused.
	opened, err := f.Stat()
	if err != nil {
		return refuse(causeOf(err))
	}
	if !os.SameFile(info, opened) {
		return refuse(oddtask.CauseNotRegular)
	}
	// One byte past the bound is enough to see the file grew over it since it was inspected.
	data, err := io.ReadAll(io.LimitReader(f, oddtask.MaxDocumentBytes+1))
	if err != nil {
		return refuse(causeOf(err))
	}
	if len(data) > oddtask.MaxDocumentBytes {
		return refuse(oddtask.CauseTooLarge)
	}
	return oddtask.File{Path: resolved, Text: string(data)}, nil
}

// refusalOf is why a file of this kind is not read, or "" when it is.
func refusalOf(info fs.FileInfo) string {
	switch {
	case info.Mode()&fs.ModeSymlink != 0:
		return oddtask.CauseLink
	case !info.Mode().IsRegular():
		return oddtask.CauseNotRegular
	case info.Size() > oddtask.MaxDocumentBytes:
		return oddtask.CauseTooLarge
	}
	return ""
}

// causeOf words a failure of the file system as the cause of a refusal.
func causeOf(err error) string {
	if errors.Is(err, fs.ErrNotExist) {
		return oddtask.CauseNotFound
	}
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Err.Error()
	}
	return err.Error()
}
