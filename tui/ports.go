package main

// The ports the TUI's application logic depends on. The logic (model.go,
// run.go) talks to these interfaces and never runs a process or reads a file
// to answer them; the adapter that does lives in overlaycli.go, and main.go
// is the only place that picks it.

// TargetCatalog is where the TUI learns which targets exist. It is the single
// source: the TUI keeps no list of its own, so what it shows is what the
// backend's `--target all` acts on.
type TargetCatalog interface {
	// Targets returns the backend's targets in the order it lists them. An
	// error means the catalog is unknown, and the caller must then offer no
	// target action rather than fall back to a guess.
	Targets() ([]Target, error)
}

// Backup is one retained backup of a target, as the backend reports it.
type Backup struct {
	// Timestamp names the backup (the backend's UTC timestamp, with a "-N"
	// suffix when two were taken in the same second). It is the only handle
	// the backend gives, and what the confirm screen shows.
	Timestamp string
	// Version is the release the target was at when the backup was taken;
	// empty when the backend reports it as unknown.
	Version string
}

// BackupQuery is where the TUI learns what restore could roll back to. The
// backend owns where backups live and how they are ordered, so the TUI asks
// instead of reading the state directory itself.
type BackupQuery interface {
	// LatestBackup returns target's most recent retained backup. ok is false,
	// with a nil error, when the target has none. A non-nil error means the
	// backend could not say; callers must then not offer a restore for it.
	LatestBackup(target string) (backup Backup, ok bool, err error)
}
