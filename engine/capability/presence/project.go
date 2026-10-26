package presence

import (
	"strings"
	"unicode"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

// ProjectContext is what the prober asks of the project a workflow is created in. The prober owns the
// port, so it knows nothing of how a project is found or of how its files are read: the composition
// root answers it with an adapter (engine/cmd), and a test answers it with a fake. Each question is
// asked in the names the project's own records use, and only when the capability it belongs to is
// probed.
//
// This is the one signal the prober does not decide by stat, and it does not read the file itself
// either: the package has no way to open a file (the static test pins it), so the adapter reads it.
type ProjectContext interface {
	// ReadLock reads the procedural skills lock of the project (C10c). The adapter reads it with the
	// reader the SessionStart probe reads it with, so that the record made when a workflow is created
	// and a session cannot disagree on whether a lock can be used. It never fails: what went wrong is
	// the State of the reading, and the Reason when the lock is there and cannot be used.
	ReadLock() LockReading
}

// LockState is what a reading of the procedural skills lock found.
type LockState int

const (
	// LockProjectUnknown, the zero value, is that the adapter could not say which project this is (no
	// working directory, or one that is in no worktree), so no lock was looked for. It is what the
	// prober assumes of a reading it has no word for.
	LockProjectUnknown LockState = iota
	// LockAbsent is that the project has no lock.
	LockAbsent
	// LockValid is that the lock is there and can be used.
	LockValid
	// LockInvalid is that the lock is there and cannot be used: it cannot be opened, or it is not a
	// valid lock. Reason says why.
	LockInvalid
)

// LockReading is the answer to ReadLock.
type LockReading struct {
	State LockState
	// Reason says why a lock that is there cannot be used, in the words of the reader, which are the
	// words the session probe gives. Keeping the path of the project out of it is the adapter's: an
	// observation is kept in the log of the workflow and names no path. The prober shortens a long
	// Reason (see MaxReasonRunes). It is empty for any other State.
	Reason string
}

// MaxReasonRunes is how much of a Reason an observation keeps. The reason is what the lock file makes
// it (a skill id that is listed twice is quoted whole), so it can be as long as the file; a detail
// over workflow.MaxObservationDetailLength would make the event that records it invalid.
const MaxReasonRunes = 200

// noReasonGiven stands in the detail for a lock that cannot be used when the adapter gave no reason.
const noReasonGiven = "no reason was given"

// observeProcedural asks the project whether its procedural skills lock can be used. It does not look
// for the lock itself: with no project to ask, or one that cannot be named, the signal is unavailable.
// The words of an invalid lock are the session probe's ("the procedural skills lock cannot be read:
// <reason>"); the others are the words the stat-level check used.
func (p Prober) observeProcedural(name string) workflow.Observation {
	const subject = "the procedural skills lock"
	var reading LockReading
	if p.Project != nil {
		reading = p.Project.ReadLock()
	}
	switch reading.State {
	case LockValid:
		return workflow.Observation{Capability: name, Status: workflow.ObservationAvailable, Detail: subject + " is present and valid (the skills it lists were not checked)"}
	case LockAbsent:
		return unavailable(name, subject+" was not found: no such file")
	case LockInvalid:
		return unavailable(name, subject+" cannot be read: "+shortReason(reading.Reason))
	}
	return unavailable(name, "the project directory is unknown, so "+subject+" was not looked for")
}

// shortReason makes a reason fit an observation: one line of valid text of at most MaxReasonRunes
// runes, cut with "..." where it is cut. A reason with nothing in it is said not to have been given.
func shortReason(reason string) string {
	oneLine := strings.Fields(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, strings.ToValidUTF8(reason, "")))
	reason = strings.Join(oneLine, " ")
	if reason == "" {
		return noReasonGiven
	}
	if runes := []rune(reason); len(runes) > MaxReasonRunes {
		return string(runes[:MaxReasonRunes-3]) + "..."
	}
	return reason
}
