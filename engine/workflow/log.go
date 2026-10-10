package workflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// This file is the domain's half of a workflow's event log: what the bytes of a
// log mean (ClassifyLog) and when an event may be added to one (AdmitAppend). Both
// are pure: they take bytes and values and return values, and know nothing of the
// file, the lock, or the directory the bytes came from. An EventLog adapter owns
// those (see engine/workflow/filelog) and calls these two in the same order every
// time: take the lock, read the bytes, ClassifyLog them, AdmitAppend the event,
// publish the line it returns.
//
// The messages keep the wording the store has always printed ("workflow store:
// append: ..."); callers show them to a person, and they do not change with where
// the code lives.

// Classification is the closed vocabulary Load reports for the on-disk state
// of one workflow. Only ClassificationAbsent and ClassificationOwned accept
// further writes through Append; every other value is preserved exactly as
// found and reported through Append's named refusal errors.
type Classification string

const (
	// ClassificationAbsent means no file exists yet at the workflow's path.
	// The next Append must be the workflow's created event at seq 0.
	ClassificationAbsent Classification = "absent"
	// ClassificationOwned means every line parsed as a WorkflowEvent (version 1 or 2) for
	// this exact project_id/workflow_id, the hash chain and seq sequence
	// verify (VerifyEvents), and the lifecycle transitions replay cleanly
	// (Replay). Loaded.Events and Loaded.State are populated.
	ClassificationOwned Classification = "owned"
	// ClassificationForeign means the file exists and is not a workflow log
	// of ours: it may be valid JSON lacking our version/shape, or a
	// correctly shaped WorkflowEvent log for a different workflow_id or
	// project_id than requested. A foreign file is never overwritten.
	ClassificationForeign Classification = "foreign"
	// ClassificationMalformed means the file cannot be parsed as a sequence
	// of our JSONL records at all: not valid UTF-8, oversized beyond
	// MaxLogBytes, missing its final trailing newline, containing a
	// blank line, or containing a line that is not even syntactically valid
	// JSON. A malformed file is never overwritten.
	ClassificationMalformed Classification = "malformed"
	// ClassificationDrifted means every line parses as one of our events for
	// the right ids, but the hash chain, seq sequence, or lifecycle
	// transitions do not verify. A drifted file is never overwritten.
	ClassificationDrifted Classification = "drifted"
	// ClassificationUnavailable means the state root or the file itself
	// could not be read (for example, a permission error, or a symlink at a
	// store path component). Nothing about the file's content is known, so
	// it is never overwritten. It is the one classification ClassifyLog never
	// returns: it describes a failure to obtain bytes, which is the adapter's.
	ClassificationUnavailable Classification = "unavailable"
)

// Loaded is the result of Load: the classification, and, only when
// Classification is ClassificationOwned, the parsed event log and its
// replayed State. Detail carries a human-readable explanation for every
// classification other than ClassificationAbsent and ClassificationOwned.
type Loaded struct {
	Classification Classification
	Events         []WorkflowEvent
	State          State
	Detail         string
}

// Sentinel errors an EventLog's Append returns. Wrap with %w so callers can
// distinguish the failure with errors.Is.
var (
	// ErrRefuseForeignState is returned by Append when the current on-disk
	// state classifies as foreign; the file is left byte-for-byte unchanged.
	ErrRefuseForeignState = errors.New("workflow store: refusing to write: on-disk state is foreign")
	// ErrRefuseMalformedState is returned by Append when the current
	// on-disk state classifies as malformed; the file is left
	// byte-for-byte unchanged.
	ErrRefuseMalformedState = errors.New("workflow store: refusing to write: on-disk state is malformed")
	// ErrRefuseDriftedState is returned by Append when the current on-disk
	// state classifies as drifted; the file is left byte-for-byte
	// unchanged.
	ErrRefuseDriftedState = errors.New("workflow store: refusing to write: on-disk state is drifted")
	// ErrStateUnavailable is returned by Append when the current on-disk
	// state could not be read (classification unavailable); nothing is
	// written.
	ErrStateUnavailable = errors.New("workflow store: refusing to write: on-disk state is unavailable")
	// ErrAppendConflict is returned by Append when another Append for the
	// same project_id/workflow_id holds the append lock. Append does not
	// retry or wait: the caller decides whether to retry.
	ErrAppendConflict = errors.New("workflow store: append: a concurrent append is in progress for this workflow")
	// ErrStaleSeq is returned by Append when the event's seq is not the
	// next one: the log already advanced (for example, another appender
	// finished first) or the caller skipped ahead. Nothing is written. A
	// caller that raced another appender sees this instead of
	// ErrAppendConflict when the other append had already released the lock.
	ErrStaleSeq = errors.New("workflow store: append: seq is not the next seq")
)

// MaxLogBytes bounds the total size of one workflow's on-disk JSONL log that
// ClassifyLog will classify. It exists to give a log a documented, finite worst
// case; a log beyond this size is classified malformed. It is far larger than any
// workflow this phase's profiles produce (each event is bounded well under 64 KiB
// by MaxEventBytes), so it is not expected to be reached by normal use: even a
// workflow that recorded MaxStages (256) stages plus every other event kind would
// use a small fraction of this ceiling. 16 MiB also keeps an adapter's O(n)
// full-log rewrite on every append a fast, bounded, in-memory operation on every
// supported platform.
const MaxLogBytes = 16 * 1024 * 1024

// OversizedLog is how a log of size bytes, more than MaxLogBytes, classifies: it is
// malformed and says by how much. ClassifyLog gives this answer for bytes it was
// handed; an adapter calls it directly when it knows the log is too large without
// having read it all, which is the point of the bound: the log is never read past
// MaxLogBytes+1 bytes, whatever its size on disk.
func OversizedLog(size int64) Loaded {
	return Loaded{Classification: ClassificationMalformed, Detail: fmt.Sprintf("workflow log is %d bytes, exceeding the maximum of %d", size, MaxLogBytes)}
}

// ClassifyLog classifies the raw JSONL bytes of one workflow's log against the
// requested project_id/workflow_id. See Classification for the exact rules. It
// performs no I/O and never returns ClassificationUnavailable.
func ClassifyLog(projectID, workflowID string, data []byte) Loaded {
	if len(data) == 0 {
		return Loaded{Classification: ClassificationMalformed, Detail: "workflow log is empty"}
	}
	if len(data) > MaxLogBytes {
		return OversizedLog(int64(len(data)))
	}
	if !utf8.Valid(data) {
		return Loaded{Classification: ClassificationMalformed, Detail: "workflow log is not valid UTF-8"}
	}
	if data[len(data)-1] != '\n' {
		return Loaded{Classification: ClassificationMalformed, Detail: "workflow log does not end with a trailing newline"}
	}
	lines := strings.Split(string(data[:len(data)-1]), "\n")

	events := make([]WorkflowEvent, 0, len(lines))
	for i, line := range lines {
		// Detail messages report 1-based line numbers: a person reading the
		// raw file (or an editor's line gutter) counts lines from 1, not 0.
		lineNumber := i + 1
		if line == "" {
			return Loaded{Classification: ClassificationMalformed, Detail: fmt.Sprintf("line %d is blank", lineNumber)}
		}
		if !json.Valid([]byte(line)) {
			return Loaded{Classification: ClassificationMalformed, Detail: fmt.Sprintf("line %d is not valid JSON", lineNumber)}
		}
		e, err := ParseWorkflowEvent([]byte(line))
		if err != nil {
			return Loaded{Classification: ClassificationForeign, Detail: fmt.Sprintf("line %d is not a workflow event we recognize: %v", lineNumber, err)}
		}
		if e.ProjectID != projectID || e.WorkflowID != workflowID {
			return Loaded{Classification: ClassificationForeign, Detail: fmt.Sprintf("line %d declares project_id=%q workflow_id=%q, want %q/%q", lineNumber, e.ProjectID, e.WorkflowID, projectID, workflowID)}
		}
		events = append(events, e)
	}

	if err := VerifyEvents(events); err != nil {
		return Loaded{Classification: ClassificationDrifted, Detail: err.Error()}
	}
	state, err := Replay(events)
	if err != nil {
		return Loaded{Classification: ClassificationDrifted, Detail: err.Error()}
	}
	return Loaded{Classification: ClassificationOwned, Events: events, State: state}
}

// AdmitAppend decides whether next may be appended to the log of the workflow
// projectID/workflowID, whose current state is loaded (what ClassifyLog, or an
// adapter that could not read the log, reported). When it may, it returns the
// canonical JSONL line to add to the end of the log: next.MarshalLine(), the
// bytes EventDigest hashes plus the newline. When it may not, it returns the named
// refusal and no line, and the caller changes nothing.
//
// next is judged on its own first: it must name this workflow and be a valid
// event. The state of the log is judged second. Only an absent log (next must be
// the created event at seq 0) and an owned one (next must be the next seq, chain
// to the last stored event's digest, and pass CheckTransition against the replayed
// State) accept an event; every other classification is refused with its named
// error (ErrRefuseForeignState, ErrRefuseMalformedState, ErrRefuseDriftedState,
// ErrStateUnavailable), and a stale seq is ErrStaleSeq. It performs no I/O, so the
// caller must hold whatever lock makes loaded still true when the line is
// published.
func AdmitAppend(projectID, workflowID string, loaded Loaded, next WorkflowEvent) ([]byte, error) {
	if next.ProjectID != projectID || next.WorkflowID != workflowID {
		return nil, fmt.Errorf("workflow store: append: event project_id/workflow_id (%q/%q) does not match the requested workflow (%q/%q)", next.ProjectID, next.WorkflowID, projectID, workflowID)
	}
	if err := next.Validate(); err != nil {
		return nil, fmt.Errorf("workflow store: append: %w", err)
	}

	switch loaded.Classification {
	case ClassificationAbsent:
		// An absent workflow has no prior event, so its first append must
		// be the created event at seq 0: checked explicitly and locally
		// here (not only through CheckTransition(State{}, next) below,
		// which enforces the same rule against the zero State as a second,
		// independent line of defense; a future change to CheckTransition
		// cannot silently drop this invariant without also failing here).
		// The zero State also has no last stored digest, so next.PrevDigest
		// must be empty, mirroring the explicit prev_digest check the
		// ClassificationOwned branch below performs against its own last
		// stored event's digest.
		if next.Seq != 0 || next.Kind != KindCreated {
			return nil, fmt.Errorf("workflow store: append: the first event must be a created event at seq 0, got kind %q at seq %d", next.Kind, next.Seq)
		}
		if next.PrevDigest != "" {
			return nil, fmt.Errorf("workflow store: append: prev_digest must be empty for the first event, got %q", next.PrevDigest)
		}
		if err := CheckTransition(State{}, next); err != nil {
			return nil, fmt.Errorf("workflow store: append: %w", err)
		}
	case ClassificationOwned:
		last := loaded.Events[len(loaded.Events)-1]
		lastDigest, err := EventDigest(last)
		if err != nil {
			return nil, fmt.Errorf("workflow store: append: %w", err)
		}
		if next.Seq != len(loaded.Events) {
			return nil, fmt.Errorf("%w: got %d, want %d", ErrStaleSeq, next.Seq, len(loaded.Events))
		}
		if next.PrevDigest != lastDigest {
			return nil, fmt.Errorf("workflow store: append: prev_digest %q does not match the last stored event's digest %q", next.PrevDigest, lastDigest)
		}
		if err := CheckTransition(loaded.State, next); err != nil {
			return nil, fmt.Errorf("workflow store: append: %w", err)
		}
	case ClassificationForeign:
		return nil, fmt.Errorf("%w: %s", ErrRefuseForeignState, loaded.Detail)
	case ClassificationMalformed:
		return nil, fmt.Errorf("%w: %s", ErrRefuseMalformedState, loaded.Detail)
	case ClassificationDrifted:
		return nil, fmt.Errorf("%w: %s", ErrRefuseDriftedState, loaded.Detail)
	case ClassificationUnavailable:
		return nil, fmt.Errorf("%w: %s", ErrStateUnavailable, loaded.Detail)
	default:
		return nil, fmt.Errorf("workflow store: append: unknown classification %q", loaded.Classification)
	}

	line, err := next.MarshalLine()
	if err != nil {
		return nil, fmt.Errorf("workflow store: append: %w", err)
	}
	return line, nil
}
