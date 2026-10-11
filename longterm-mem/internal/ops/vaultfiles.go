package ops

// IndexReader is the port through which Doctor reads the master catalog of the vault it inspects, wiki/index.md,
// for the check that every promoted page is registered in it. The package owns it, asks for the read half only (a
// diagnostic never writes), and is satisfied by the vault file system adapter and by promote.IndexReader
// alike. A catalog that cannot be read, for whatever reason, is a catalog the check cannot find the page in.
type IndexReader interface {
	// ReadIndex returns the whole content of the catalog, following a link to it.
	ReadIndex() (string, error)
}

// LogReader is the port through which Doctor reads the promotion log of the vault it inspects, wiki/log.md, for
// the same check: the package owns it, asks for the read half only, and the vault file system adapter satisfies
// it. A log that cannot be read is a log the check cannot find the page in.
type LogReader interface {
	// ReadLog returns the whole content of the log, following a link to it.
	ReadLog() (string, error)
}

// RootInspector is the port through which Doctor asks what is at the vault root it was told. The package owns it
// and the vault file system adapter satisfies it.
type RootInspector interface {
	// RootIsDirectory reports whether the vault root, following a link, is a directory. A root that cannot be
	// examined (it is not there, a link to nothing, a path through a file) is the file system's own error,
	// untouched, and no answer.
	RootIsDirectory() (bool, error)
}

// SyncStateReader is the port through which Status reads the record of the last sync that completed. The package
// owns it and the vault file system adapter satisfies it; what the record holds and how it is read are the
// adapter's to say, and the failures it returns say what they could not read or understand, behind whatever
// prefix the composition root asked of the adapter.
type SyncStateReader interface {
	// LastSyncCompletedAt returns the completion time the record holds, as it holds it. A vault with no record,
	// and a record that holds no completion time, answer the empty string and no error: nothing has completed.
	LastSyncCompletedAt() (string, error)
}
