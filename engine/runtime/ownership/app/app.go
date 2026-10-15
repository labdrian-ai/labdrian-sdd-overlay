// Package app is the use case behind `engine installed-targets`: it records the runtimes the overlay
// has installed longterm-mem for, takes one out, and decides, when the overlay uninstalls, whether
// the shared binary and the registration record go with the last of them (Phase 9, B3). The rules
// are engine/runtime/ownership'; where the record, the lock around it and the binary are is
// reached through the two ports this package owns, which engine/runtime/ownership/fsstore
// answers and the composition root wires.
//
// Every read-modify-write over the record is one critical section: the use case takes the lock of the
// store, reads the record again inside it (it never trusts a read made before), decides, writes, and
// lets the lock go on every way out. Two overlay commands running at once take turns, and a target
// recorded by one is never lost to a rewrite the other made from a record that did not have it yet. A
// report (Tracked, Assess) takes no lock: it is a look, and the removal that follows it looks again.
//
// It prints nothing and exits nowhere: the words a person reads and the exit code are the command's.
package app

import (
	"errors"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/ownership"
)

// ErrBusy is what a lock that stayed taken for the whole time the store waits for it is, as
// errors.Is sees it: nothing was changed, and the call can be tried again.
var ErrBusy = errors.New("the lock of the record of the installed targets is held by another process")

// Store is how the use case reaches the record of the installed targets and the lock around it.
type Store interface {
	// Exists says whether there is a record: a regular file, a link to one followed.
	Exists() bool
	// MakeRecordDir makes the directory the record lives in, and its parents (the state directory
	// among them), when there is none. Append and Replace write into that directory and need it
	// made; Exists, Read and Delete do not (a record in a directory that is not there is absent),
	// and neither does Lock.
	MakeRecordDir() error
	// Lock takes the lock that makes a read-modify-write over the record one critical section, and
	// returns the function that lets it go (which may be called twice). The lock lives in the state
	// directory, which Lock makes when there is none: a caller that locks needs no other directory
	// made first. A lock that stays taken is an error for which errors.Is(err, ErrBusy) holds; a
	// state directory that cannot be made is a *StateDirError.
	Lock() (release func(), err error)
	// Read returns how the record is found and, when it is Readable, its content. A record that is
	// there and cannot be read is Unreadable, never Absent and never empty.
	Read() (ownership.State, []byte)
	// Append adds a line holding name to the end of the record, making it when there is none. It
	// writes nothing else: whatever the record held stays as it was.
	Append(name string) error
	// Replace puts content in the record whole: a reader running beside the write sees the record as
	// it was or as it is, never half of either. A failure is a *ReplaceError.
	Replace(content []byte) error
	// Delete removes the record. A record that is not there is not an error.
	Delete() error
}

// Binaries is how the use case reaches the shared binary.
type Binaries interface {
	// Exists says whether there is a file at path, a link followed.
	Exists(path string) bool
	// Remove removes the file at path.
	Remove(path string) error
}

// StateDirError is the state directory, where the lock lives, that could not be made.
type StateDirError struct{ Err error }

func (e *StateDirError) Error() string { return "cannot make the state directory: " + e.Err.Error() }
func (e *StateDirError) Unwrap() error { return e.Err }

// RecordDirError is the directory of the record that could not be made.
type RecordDirError struct{ Err error }

func (e *RecordDirError) Error() string {
	return "cannot make the directory of the record: " + e.Err.Error()
}
func (e *RecordDirError) Unwrap() error { return e.Err }

// LockError is the lock of the record that could not be taken. When it stayed taken for the whole
// wait, errors.Is(err, ErrBusy) holds.
type LockError struct{ Err error }

func (e *LockError) Error() string { return "cannot take the lock of the record: " + e.Err.Error() }
func (e *LockError) Unwrap() error { return e.Err }

// UnreadableError is a record that is there and cannot be read. Nothing is written over a record
// that could not be inspected.
type UnreadableError struct{}

func (e *UnreadableError) Error() string { return "the record exists but cannot be read" }

// AppendError is a name that could not be added to the record.
type AppendError struct{ Err error }

func (e *AppendError) Error() string { return "cannot add to the record: " + e.Err.Error() }
func (e *AppendError) Unwrap() error { return e.Err }

// Stage is the step of a rewrite of the record that failed.
type Stage int

// The stages of a rewrite.
const (
	// StageCreate: the temporary file the rewrite goes through could not be made.
	StageCreate Stage = iota + 1
	// StageWrite: the new content could not be written to it.
	StageWrite
	// StagePublish: it could not be put in the place of the record.
	StagePublish
)

// ReplaceError is a rewrite of the record that failed at a stage. The record is as it was. The zero
// Stage is a failure the store did not place (see replaceFailure), which is said as a failed update.
type ReplaceError struct {
	Stage Stage
	Err   error
}

func (e *ReplaceError) Error() string {
	switch e.Stage {
	case StageCreate:
		return "cannot make a temporary file for the record: " + e.Err.Error()
	case StageWrite:
		return "cannot write the new record: " + e.Err.Error()
	default:
		return "cannot replace the record: " + e.Err.Error()
	}
}
func (e *ReplaceError) Unwrap() error { return e.Err }

// BinaryRemoveError is the shared binary that could not be removed.
type BinaryRemoveError struct {
	Path string
	Err  error
}

func (e *BinaryRemoveError) Error() string { return "cannot remove " + e.Path + ": " + e.Err.Error() }
func (e *BinaryRemoveError) Unwrap() error { return e.Err }

// RecordRemoveError is the record that could not be removed.
type RecordRemoveError struct{ Err error }

func (e *RecordRemoveError) Error() string { return "cannot remove the record: " + e.Err.Error() }
func (e *RecordRemoveError) Unwrap() error { return e.Err }

// Service runs the use case over the ports it is given.
type Service struct {
	store    Store
	binaries Binaries
	knows    ownership.Vocabulary
}

// New returns the use case over the store of the record and the shared binary. knows says which
// names this build can install longterm-mem for; only what is recorded is held to it.
func New(store Store, binaries Binaries, knows ownership.Vocabulary) *Service {
	return &Service{store: store, binaries: binaries, knows: knows}
}

// lock takes the lock of the store. A failure that is not the state directory is a *LockError.
func (s *Service) lock() (func(), error) {
	release, err := s.store.Lock()
	if err == nil {
		return release, nil
	}
	var state *StateDirError
	if errors.As(err, &state) {
		return nil, err
	}
	return nil, &LockError{Err: err}
}

// observe reads the record as it is.
func (s *Service) observe() ownership.Observation {
	state, content := s.store.Read()
	return ownership.Observe(state, content)
}

// Record records that the overlay installed longterm-mem for name, once. A name this build cannot
// install for is refused before anything is touched (an *ownership.UnknownTargetError). A record
// that cannot be read is an *UnreadableError and is left alone: appending to a record that was not
// inspected could record a name twice or hide what the record holds. What the record held stays as it
// was, whether or not it is a name: the name is added after it.
func (s *Service) Record(name string) error {
	if err := ownership.Admit(name, s.knows); err != nil {
		return err
	}
	if err := s.store.MakeRecordDir(); err != nil {
		return &RecordDirError{Err: err}
	}
	release, err := s.lock()
	if err != nil {
		return err
	}
	defer release()

	obs := s.observe()
	if obs.State == ownership.Unreadable {
		return &UnreadableError{}
	}
	if obs.Targets.Contains(name) {
		return nil
	}
	if err := s.store.Append(name); err != nil {
		return &AppendError{Err: err}
	}
	return nil
}

// Release records that the overlay no longer has longterm-mem installed for name. It rewrites the
// record from what the record vouches for, so that garbage in it is compacted away, a name only a newer
// build knows stays, and the last name leaves an empty record, which is not an absent one. With no
// record it does nothing. A record that cannot be read is an *UnreadableError and is not rewritten:
// a rewrite from a read that found nothing would truncate the very record that keeps the binary.
func (s *Service) Release(name string) error {
	if !s.store.Exists() {
		return nil
	}
	if err := s.store.MakeRecordDir(); err != nil {
		return &RecordDirError{Err: err}
	}
	release, err := s.lock()
	if err != nil {
		return err
	}
	defer release()

	obs := s.observe()
	if obs.State == ownership.Unreadable {
		return &UnreadableError{}
	}
	if err := s.store.Replace(obs.Targets.Without(name).Content()); err != nil {
		return replaceFailure(err)
	}
	return nil
}

// replaceFailure normalizes what a store's Replace fails with, as lock() does for the lock: a
// *ReplaceError is the store saying at which stage, and is returned as it is; any other error is a
// rewrite that failed in a way the store did not place, and is wrapped in a *ReplaceError with no
// stage, so the caller says every failed rewrite the same way.
func replaceFailure(err error) error {
	var replace *ReplaceError
	if errors.As(err, &replace) {
		return err
	}
	return &ReplaceError{Err: err}
}

// Tracked returns the record as it is now, read without the lock.
func (s *Service) Tracked() ownership.Observation { return s.observe() }

// Assessment is what Assess found: the verdict, and the record it was reached from.
type Assessment struct {
	Verdict     ownership.Verdict
	Observation ownership.Observation
}

// Assess decides, from the record as it is now, what an uninstall does with the registration record and
// the shared binary. It reads without the lock and removes nothing: Retire reads again under the lock
// before it removes anything.
func (s *Service) Assess(in ownership.Intent) Assessment {
	obs := s.observe()
	return Assessment{Verdict: ownership.Settle(in, obs), Observation: obs}
}

// RetireRequest asks for the shared binary and the record to go, when the rule allows.
type RetireRequest struct {
	// Binary is the path of the shared binary, as the caller deployed it. The use case does not
	// derive it: the path written into every runtime's entry and the path removed here must be the
	// same string.
	Binary string
	// Purge removes them whatever the record says.
	Purge bool
}

// BinaryFate is what became of the shared binary in a Retire.
type BinaryFate int

// The fates of the binary.
const (
	// BinaryUntouched: the rule kept it, and it was not looked at.
	BinaryUntouched BinaryFate = iota
	// BinaryRemoved: it was there and was removed.
	BinaryRemoved
	// BinaryAbsent: it was not there.
	BinaryAbsent
)

// Retirement is what a Retire decided and did.
type Retirement struct {
	Disposal    ownership.Disposal
	Observation ownership.Observation
	Binary      BinaryFate
}

// Retire removes the shared binary and the record when the record, read again under the lock, says that
// nothing depends on the binary or when Purge forces it. A target recorded since the verdict that led
// here keeps the binary. The binary goes first and the record after it: a binary that cannot be removed
// leaves the record that vouches for it.
//
// Unlike Record and Release it does not make the directory of the record, on purpose, and needs none:
// the lock lives in the state directory, which Lock makes (so a first retire on a machine that has
// never recorded anything can take it), the record is read as absent when its directory is not there,
// and removing a record that is not there is already done. Only a write into the record's directory
// (Append, Replace) needs MakeRecordDir first.
func (s *Service) Retire(req RetireRequest) (Retirement, error) {
	release, err := s.lock()
	if err != nil {
		return Retirement{}, err
	}
	defer release()

	obs := s.observe()
	out := Retirement{Disposal: ownership.Dispose(req.Purge, obs), Observation: obs}
	if out.Disposal != ownership.DisposeOfBinary {
		return out, nil
	}
	if s.binaries.Exists(req.Binary) {
		if err := s.binaries.Remove(req.Binary); err != nil {
			return out, &BinaryRemoveError{Path: req.Binary, Err: err}
		}
		out.Binary = BinaryRemoved
	} else {
		out.Binary = BinaryAbsent
	}
	if err := s.store.Delete(); err != nil {
		return out, &RecordRemoveError{Err: err}
	}
	return out, nil
}
