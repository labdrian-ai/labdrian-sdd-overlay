package promote

import "time"

// IndexReader is the port through which the page check reads the vault's master catalog, wiki/index.md: the
// one question its inbound-link rule puts to it. The package owns it and never touches the catalog itself.
type IndexReader interface {
	// ReadIndex returns the whole content of the catalog, following a link to it. A catalog that is not
	// there is an error for which errors.Is(err, fs.ErrNotExist) holds; anything else that keeps it from
	// being read (it is a directory, the wiki directory is a file) is the file system's own error. Both
	// name the file, and neither carries any prefix: the callers word the failure around the catalog they
	// were reading.
	ReadIndex() (string, error)
}

// IndexRepository is the port through which promotion registers a page in the vault's master catalog: the
// read half, and the replacement of the catalog. The composition root hands it an adapter (internal/vaultfs,
// which keeps the catalog where vaultlayout says), and a test hands it a fake.
type IndexRepository interface {
	IndexReader
	// IndexLocation names the catalog, as an operator would find it, for the messages that tell which file
	// failed. It touches no file.
	IndexLocation() string
	// WriteIndex makes the catalog hold content, whole. It is durable and atomic: a reader sees the old
	// catalog or the new one and never half of either, and a failure leaves the old one as it was. The wiki
	// directory is created when it is not there; a catalog that exists keeps its mode and a link to it is
	// written through, because the vault is the user's own and shared with other tools; one it creates is
	// readable by its owner alone.
	WriteIndex(content string) error
}

// LogRepository is the port through which promotion records a promotion in the vault's append-only log,
// wiki/log.md: the read of the log and its replacement, on the terms of IndexRepository.
type LogRepository interface {
	// ReadLog returns the whole content of the log, following a link to it, and fails as ReadIndex does.
	ReadLog() (string, error)
	// LogLocation names the log, for the messages that tell which file failed. It touches no file.
	LogLocation() string
	// WriteLog makes the log hold content, whole, as WriteIndex does the catalog.
	WriteLog(content string) error
}

// SyncStateRecorder is the port through which a sync records when it completed, in the vault's sync-state file.
// The record is the vault's own: what it holds and how it is written are the adapter's to say.
type SyncStateRecorder interface {
	// RecordSyncCompleted makes the vault's sync state say that a sync completed at the instant given. It is
	// durable and atomic, as WriteIndex is: after it returns nil the record survives the process, and a
	// failure leaves the record as it was.
	RecordSyncCompleted(at time.Time) error
}
