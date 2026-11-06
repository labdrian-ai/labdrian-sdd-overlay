package ingest

import (
	"fmt"
	"path/filepath"
)

// SourceReader is the one door ingestion has to the machine that holds the local sources it reads: a port this
// package owns, so that nothing in it reaches a file, a directory or the process. internal/sourcefs answers it on
// a file system, and the composition root hands the answer to the ingestion use case.
//
// It answers where a relative path begins and what reading a source asks: what an entry is, without following a
// link, and how big; its bytes; the names in a directory. Every answer is in this package's own types, and an
// entry that is not there fails with an error that satisfies errors.Is(err, fs.ErrNotExist).
type SourceReader interface {
	// WorkingDirectory is the absolute directory a relative path is taken to be relative to.
	WorkingDirectory() (string, error)
	// Lstat reports the kind of the entry at path and, for a file, its size. A link is reported as a link and
	// never followed.
	Lstat(path string) (Entry, error)
	// ReadFile returns the bytes of the file at path.
	ReadFile(path string) ([]byte, error)
	// ReadDir returns the names (not paths) in the directory at path, without . and .., in no promised order.
	ReadDir(path string) ([]string, error)
}

// Entry is what Lstat reports of a path. Size is the size of a file, and zero for every other kind.
type Entry struct {
	Kind EntryKind
	Size int64
}

// EntryKind is the kind of an entry, as Lstat sees it without following a link.
type EntryKind string

const (
	EntryFile      EntryKind = "file"
	EntryDirectory EntryKind = "directory"
	EntrySymlink   EntryKind = "symlink"
	EntryOther     EntryKind = "other"
)

// ResolveOrigin makes the file or directory an origin names absolute: a relative path begins at the reader's
// working directory, as it did for the operator who typed it. Any other origin, and a path that is already
// absolute, is returned as it is and the reader is not asked. The reader's failure is returned as it came.
// Override is returned unchanged in every case: it is the caller's identity, not a path. A relative URI beside it
// is still resolved, and the reader's failure still returned, because URI is where the source is read from even
// when the id does not depend on it.
//
// SourceID derives an id from an Override as it is, or else from an absolute path and refuses any other, so an id
// is a function of the origin and never of the directory a process happens to be in.
func ResolveOrigin(reader SourceReader, o Origin) (Origin, error) {
	if (o.Kind != KindFile && o.Kind != KindDirectory) || o.URI == "" || filepath.IsAbs(o.URI) {
		return o, nil
	}
	dir, err := reader.WorkingDirectory()
	if err != nil {
		return Origin{}, err
	}
	if !filepath.IsAbs(dir) {
		return Origin{}, fmt.Errorf("ingest: the working directory %q is not absolute", dir)
	}
	o.URI = filepath.Join(dir, o.URI)
	return o, nil
}
