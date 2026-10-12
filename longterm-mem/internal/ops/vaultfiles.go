package ops

import "errors"

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

// errNoIndexReader and errNoLogReader are what the registration check reports when Doctor was built without the
// reader of the file, and there is a promoted page to look for in it.
//
// internal/promote has an errNoIndexReader of its own, which the inbound-link rule of the address-map-integrity
// check reports (so one missing Index makes two checks say so, each in its own words). They are two errors on
// purpose: promote.IndexReader and this package's IndexReader are two ports, each package words the failure of
// the one it owns, and promote's is not exported. Their texts are
// the commands' output, pinned here (doctor_vaultfiles_port_test.go) and there (lint_index_port_test.go), and
// the prefix both carry is the one the owner has yet to decide on.
var (
	errNoIndexReader = errors.New("ops: no index reader was wired")
	errNoLogReader   = errors.New("ops: no log reader was wired")
)

// errNoRootInspector is what the vault-config check reports when Doctor was built without a RootInspector.
var errNoRootInspector = errors.New("ops: no root inspector was wired")

// errNoSyncStateReader is what Status answers when it was built without a SyncStateReader.
var errNoSyncStateReader = errors.New("ops: no sync state reader was wired")
