package promote

import (
	"fmt"
)

// SyncReport summarizes one SyncCards run: Promoted lists every memory
// Writer.Promote actually wrote (create or update); Skipped counts
// ineligible and already-current memories, neither of which costs a
// write; Bridged counts the memories left alone because they are the same
// piece of memory as one of another store that has a page; Failed names each
// memory the run could not process, so a caller can tell "nothing left to
// promote" from "memory N is broken" instead of inferring it from a single
// aborted error.
type SyncReport struct {
	Promoted []Result
	Skipped  int
	// Bridged counts the memories the run left alone because a memory of another store that is the same piece of
	// memory already has a page (the lineage of the memory names it): they are skipped, never a failure, and nothing
	// is written for them.
	Bridged int
	Failed  []SyncFailure
	// Withheld lists the memories the run left alone because it could not tell whether they already have a
	// page: no readable page carries them and a page could not be read. Nothing was written for them, no
	// address was taken, and a later run decides them once the unreadable pages are repaired or removed. It is
	// not a failure and costs the run nothing else.
	Withheld []int64
	// Unreadable names every page of the vault the run could not read and skipped.
	Unreadable []UnreadablePage
}

// SyncFailure is one memory SyncCards could not process, kept per
// memory rather than aborting the run (see SyncCards). ObservationID is the memory's id in its store; it is 0 when the
// id is not a number, and then Err names the real id.
type SyncFailure struct {
	ObservationID int64
	Err           error
}

// withIDError is err, and, when the memory's id is not a number (idErr, from numericID), what says so: a failure
// recorded under ObservationID 0 must still name the memory.
func withIDError(err, idErr error) error {
	if idErr == nil {
		return err
	}
	return fmt.Errorf("%w (%w)", err, idErr)
}

// IndexRebuildError is how SyncCards reports that the vault's index could not be rebuilt after the pages were written:
// a type of its own, so that a caller who needs to tell that failure from the others finds it with errors.As and
// does not read words. Its words are the ones the sync has always said.
type IndexRebuildError struct {
	// Err is the rebuild's own failure.
	Err error
}

func (e *IndexRebuildError) Error() string { return "promote: sync: rebuild index: " + e.Err.Error() }

func (e *IndexRebuildError) Unwrap() error { return e.Err }

// mergeWithheld joins the two passes' lists of observations left alone, each observation once, in the order
// first met: both passes ask the same page lookup of the same observation, so one unreadable page withholds an
// observation from both, and it is one observation.
func mergeWithheld(first, second []int64) []int64 {
	seen := make(map[int64]bool, len(first))
	for _, id := range first {
		seen[id] = true
	}
	// A copy: appending to first itself could write into its backing array, which is the caller's.
	merged := append([]int64(nil), first...)
	for _, id := range second {
		if seen[id] {
			continue
		}
		seen[id] = true
		merged = append(merged, id)
	}
	return merged
}

// mergeFailures joins the two passes' failure lists, dropping an entry the
// other pass already reported IDENTICALLY.
//
// Both passes call findPromotedPage on the same observation, so a page
// whose frontmatter cannot be parsed fails both with the same message, and
// one broken page arrived in the list twice: an operator reading "2
// failures" goes looking for a second broken page that does not exist.
//
// Only an exact repeat is dropped. Two DIFFERENT failures for the same
// observation are two real problems -- the promotion pass and the patch
// pass ask different questions of it -- and collapsing those would hide
// one, which is the same lie in the other direction.
func mergeFailures(first, second []SyncFailure) []SyncFailure {
	seen := make(map[string]bool, len(first))
	key := func(f SyncFailure) string {
		return fmt.Sprintf("%d\x00%v", f.ObservationID, f.Err)
	}
	for _, f := range first {
		seen[key(f)] = true
	}
	// A copy, as in mergeWithheld: the caller's backing array is not this function's to write into.
	merged := append([]SyncFailure(nil), first...)
	for _, f := range second {
		if seen[key(f)] {
			continue
		}
		seen[key(f)] = true
		merged = append(merged, f)
	}
	return merged
}
