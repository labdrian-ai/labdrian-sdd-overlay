package promote

// Failure says why a run of SyncCardsProject failed, in the terms of what the operator can do about it, so that the
// command that ran it picks its exit code from the answer and never from the words of an error.
type Failure int

const (
	// NotFailed is the zero value: nothing failed.
	NotFailed Failure = iota
	// IndexRebuildFailed: the vault's index could not be rebuilt, the one step of a sync that runs the vault's own
	// scripts. An operator who is told so goes and looks at the vault's tooling.
	IndexRebuildFailed
	// MemoryUnavailable: the memory the run reads can no longer answer the questions a sync asks of it, and
	// no vault script had to fail for that to happen.
	MemoryUnavailable
	// OtherFailure: anything else a sync can fail at (a page that could not be written, the record of the sync's
	// completion, the precedence store, an observation it could not process).
	OtherFailure
)
