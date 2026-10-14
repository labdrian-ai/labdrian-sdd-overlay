package promote

import (
	"context"
	"errors"
)

// Failure says why a run of SyncProject failed, in the terms of what the operator can do about it, so that the
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

// ProjectSyncReport is what SyncProject did for one project: the report of each pass, and, when the run failed,
// why.
type ProjectSyncReport struct {
	// Sync is the promotion pass's report, and Propagate the propagation pass's. Both are filled in whatever the
	// other did.
	Sync      SyncReport
	Propagate PropagateReport
	// Failure is why the run failed, or NotFailed.
	Failure Failure
}

// SyncProject syncs a project into its vault: it promotes every eligible, unpromoted-or-revised observation (Sync),
// rebuilds the vault's index and records the completion of the sync, and propagates the soft-delete and supersession
// status of Engram to the pages already promoted (Propagate). It is the whole of what `longterm-mem sync` does to
// a project.
//
// Both passes always run. Sync's failures are per observation by design -- it steps over a broken observation
// rather than letting it wedge the run -- so returning at its error would rebuild that wedge one level up: a single
// unpromotable observation would stop Propagate from ever running, and the statuses of the memory would never land
// again for the project, identically on every retry. The run reports what did land and an error that is the errors
// of the two passes together, so a partial run is visible without being fatal to the work that can still be done.
//
// When the run failed, Failure says why. The vault's index comes first, being the one step that runs the vault's
// own scripts; then a memory that can no longer answer, which is told by asking the memory itself and not by
// matching the text of an error, so that the answer stays bound to the condition it names and not to a message;
// then anything else. The memory is asked only after a failure, so a run that went well pays nothing for it.
func SyncProject(ctx context.Context, deps Deps, project string) (ProjectSyncReport, error) {
	syncReport, syncErr := Sync(ctx, deps, project)
	propagateReport, propagateErr := Propagate(ctx, deps, project)
	report := ProjectSyncReport{Sync: syncReport, Propagate: propagateReport}

	err := errors.Join(syncErr, propagateErr)
	if err == nil {
		return report, nil
	}
	var rebuild *IndexRebuildError
	switch {
	case errors.As(syncErr, &rebuild):
		report.Failure = IndexRebuildFailed
	case !memoryAnswers(deps.Memory, project):
		report.Failure = MemoryUnavailable
	default:
		report.Failure = OtherFailure
	}
	return report, err
}

// memoryAnswers reports whether the memory can still answer the two questions a sync asks of it: the listing of a
// project's live observations, which the promotion pass reads, and the relations of an observation, which the
// propagation pass reads. They are two reads of two tables, reported into one joined error, so one probe cannot
// stand for both: probing the observations alone answered "readable" for a database whose relations table had
// been dropped or renamed out from under it, which is exactly as unavailable.
//
// The relations question is put for an observation id no row can carry, so that the query matches nothing and
// costs nothing; what is under test is whether the statement can be prepared and run at all.
func memoryAnswers(m Memory, project string) bool {
	if _, err := m.ListObservations(project); err != nil {
		return false
	}
	if _, err := m.RelatedEdges(0); err != nil {
		return false
	}
	return true
}
