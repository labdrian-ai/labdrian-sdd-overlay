package ingest

import (
	"fmt"
	"path/filepath"
)

// SourceReader is the one door ingestion has to the machine that holds the local sources it reads: a port this
// package owns, so that nothing in it reaches a file, a directory or the process. internal/sourcefs answers it on
// a file system, and the composition root hands the answer to the ingestion use case.
//
// Today it answers one question, where a relative path begins. The questions that reading a source asks (what an
// entry is, without following a link, and how big; its bytes; the names in a directory) join it with the use
// case that asks them, in these terms: openspec/changes/longterm-mem-knowledge-ingestion, tasks 2b.2 and 2b.6.
type SourceReader interface {
	// WorkingDirectory is the absolute directory a relative path is taken to be relative to.
	WorkingDirectory() (string, error)
}

// ResolveOrigin makes the file or directory an origin names absolute: a relative path begins at the reader's
// working directory, as it did for the operator who typed it. Any other origin, and a path that is already
// absolute, is returned as it is and the reader is not asked. The reader's failure is returned as it came.
//
// SourceID derives an id from an absolute path and refuses any other, so an id is a function of the origin and
// never of the directory a process happens to be in.
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
