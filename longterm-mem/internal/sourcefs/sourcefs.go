// Package sourcefs is the adapter that answers what ingestion asks of the machine that holds the local sources it
// reads (ingest.SourceReader), on the file system of the machine it runs on: the directory a relative path begins
// at, which is the directory the process works in; what an entry is, without following a link; a file's bytes;
// the names in a directory. It only reads. Only the composition root names it.
package sourcefs

import (
	"fmt"
	"io/fs"
	"os"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/ingest"
)

// Reader is the local machine as a SourceReader. The zero Reader is ready to use.
type Reader struct{}

var _ ingest.SourceReader = Reader{}

// WorkingDirectory is the absolute directory the process works in, asked at every call. A process that has lost
// its directory cannot tell where it is, and the failure is the operating system's own.
func (Reader) WorkingDirectory() (string, error) {
	return os.Getwd()
}

// Lstat reports the kind of the entry at path without following a link, and the size of a regular file. The
// failure is the operating system's own, so a missing entry satisfies errors.Is(err, fs.ErrNotExist).
func (Reader) Lstat(path string) (ingest.Entry, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return ingest.Entry{}, err
	}
	switch mode := info.Mode(); {
	case mode&fs.ModeSymlink != 0:
		return ingest.Entry{Kind: ingest.EntrySymlink}, nil
	case mode.IsDir():
		return ingest.Entry{Kind: ingest.EntryDirectory}, nil
	case mode.IsRegular():
		return ingest.Entry{Kind: ingest.EntryFile, Size: info.Size()}, nil
	default:
		return ingest.Entry{Kind: ingest.EntryOther}, nil
	}
}

// ReadFile returns the bytes of the regular file at path. It looks at the entry first, as Lstat does, and refuses
// a link (never following it), a directory and any other kind, which it never opens (opening a named pipe would
// wait for a writer). It reads the whole file: bounding the size is the caller's, from the Size Lstat reported.
// An entry replaced between that look and the read is not defended against. Every other failure is the operating
// system's own, so a missing entry satisfies errors.Is(err, fs.ErrNotExist).
func (r Reader) ReadFile(path string) ([]byte, error) {
	entry, err := r.Lstat(path)
	if err != nil {
		return nil, err
	}
	if entry.Kind != ingest.EntryFile {
		return nil, fmt.Errorf("sourcefs: %s is a %s, not a file: not read", path, entry.Kind)
	}
	return os.ReadFile(path)
}

// ReadDir returns the names of every entry in the directory at path, links and other kinds included, without .
// and ..; the failure is the operating system's own. The order is os.ReadDir's, which the port does not promise.
func (Reader) ReadDir(path string) ([]string, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	return names, nil
}
