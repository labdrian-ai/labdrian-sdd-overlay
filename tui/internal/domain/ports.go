package domain

// The ports of the TUI, owned by the domain: the questions it asks of the
// world around it. The use cases (package app) and the screens (package ui)
// talk to these interfaces and never run a process or read a file to answer
// them; the adapter that does lives in internal/adapters/overlaycli, and the
// composition root (main) is the only place that picks it.

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

// Execution is what one run of the backend left behind.
type Execution struct {
	// Output is what the backend wrote, standard output and standard error
	// together, in the order it wrote them.
	Output string
	// ExitCode is the exit status of the process; NoExitStatus when it did not
	// end by exiting (it could not be started, or it was killed).
	ExitCode int
	// Err is nil when the backend exited with status 0, and says why not
	// otherwise.
	Err error
	// Verdicts are the per-target verdicts of the sync-check report the output
	// holds, read by the adapter, which is the one that knows how the backend
	// spells them; none when the output holds no report. Whether the screen
	// shows them is up to the action (Action.ResultKind), not to what the
	// output happens to look like.
	Verdicts []TargetVerdict
}

// Backend is where the TUI has an action run. It takes the arguments of one
// invocation of the backend (the subcommand first) and returns what the run
// left behind. It is the only way the TUI changes anything: which invocations
// to make, and in which order, is decided by the domain and the use cases.
type Backend interface {
	Run(args []string) Execution
}

// RepoProbe is where the TUI learns how far the repository is behind its
// origin and its latest release, without the network: the report of a
// cached-only sync-check, whatever the backend's exit status, as the verdicts
// it holds. A probe that cannot be run, or whose report holds no verdict,
// answers with none, and BehindOf degrades that to "unknown" rather than to
// "up to date".
type RepoProbe interface {
	Report() []TargetVerdict
}
