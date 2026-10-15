// Package ownership is the rule for the record of the runtimes the overlay has installed
// longterm-mem for (Phase 9, B3).
//
// longterm-mem is one binary, deployed once and shared by every runtime it is registered with. The
// overlay keeps a record of which runtimes it registered it with, one name per line, and that record
// is the only thing that says whether a runtime still depends on the binary: the binary is removed
// when the record is empty, and not before. The bash entrypoint used to read and write that record
// itself; this package is what it decided, with no file in it. It holds the rules over bytes and
// names, and the application package next to it (ownership/app) reads and writes them
// through ports.
//
// The record is read as evidence, not as text. Its reader sorts what it finds into three piles,
// and the difference is the safety of the whole record:
//
//   - A well-formed name is install state, whether or not this build knows it. The likeliest way an
//     unknown name got there is that a newer build wrote it for a runtime this one has never heard
//     of, and that runtime is running the shared binary now. Reading it as "nothing installed" would
//     remove the binary from under it.
//   - Anything else (a NUL byte, an empty line, padding, a carriage return, a word too long to be a
//     name, a line the file ends in the middle of) is garbage. Nothing this program writes looks
//     like it, so none of it is install state, and dropping it keeps a hand edit or a torn write from
//     holding the binary forever.
//   - A record that is there and cannot be read says nothing either way, and is neither: it is
//     Unreadable, and a caller that cannot look does not act on the binary.
package ownership

import (
	"bytes"
	"fmt"
	"sort"
)

// maxNameLength is the longest a name can be. Without a bound a long run of letters inside a binary
// file would read as a name and hold the binary forever.
const maxNameLength = 64

// WellFormed says whether name is a plausible target name: a bare word of at most 64 bytes that
// begins with a letter or a digit and goes on with letters, digits, dots, underscores and dashes.
// It does not say that the name is one this build knows (Admit does).
func WellFormed(name string) bool {
	if name == "" || len(name) > maxNameLength {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case i > 0 && (c == '.' || c == '_' || c == '-'):
		default:
			return false
		}
	}
	return true
}

// Targets is the set of names a record vouches for, sorted by bytes and without repeats.
type Targets []string

// Parse reads the content of a record. Every line that ends in a line end and is a well-formed name
// is a name; the rest is garbage and is dropped, and so is whatever follows the last line end (a
// name that was still being written). A name that is there twice is one.
func Parse(content []byte) Targets {
	var names []string
	rest := content
	for {
		end := bytes.IndexByte(rest, '\n')
		if end < 0 {
			break
		}
		if line := string(rest[:end]); WellFormed(line) {
			names = append(names, line)
		}
		rest = rest[end+1:]
	}
	sort.Strings(names)
	kept := names[:0]
	for i, name := range names {
		if i == 0 || name != names[i-1] {
			kept = append(kept, name)
		}
	}
	if len(kept) == 0 {
		return nil
	}
	return Targets(kept)
}

// Contains says whether name is one of the targets.
func (t Targets) Contains(name string) bool {
	i := sort.SearchStrings(t, name)
	return i < len(t) && t[i] == name
}

// Without returns the targets but name. The receiver is not changed.
func (t Targets) Without(name string) Targets {
	var kept Targets
	for _, other := range t {
		if other != name {
			kept = append(kept, other)
		}
	}
	return kept
}

// Empty says whether there is no name.
func (t Targets) Empty() bool { return len(t) == 0 }

// Names returns the names, sorted. The slice is a copy.
func (t Targets) Names() []string {
	if len(t) == 0 {
		return nil
	}
	return append([]string(nil), t...)
}

// Content is the record that holds exactly these names: each on a line of its own, sorted. No name
// is an empty record, which is not an absent one.
func (t Targets) Content() []byte {
	record := make([]byte, 0, 16*len(t))
	for _, name := range t {
		record = append(record, name...)
		record = append(record, '\n')
	}
	return record
}

// State is how a record was found.
type State int

// The states of a record. The zero value is Absent.
const (
	// Absent: there is no record. It does not say that nothing is installed, only that this
	// program has nothing that says what is.
	Absent State = iota
	// Unreadable: there is a record and it could not be read.
	Unreadable
	// Readable: there is a record and it was read.
	Readable
)

// Observation is a record as found: how, and what it vouches for.
type Observation struct {
	State State
	// Targets holds the names of a Readable record, and none otherwise.
	Targets Targets
}

// Observe builds the observation of a record found in the given state with the given content. The
// content of a record that was not read says nothing, and is not looked at.
func Observe(state State, content []byte) Observation {
	if state != Readable {
		return Observation{State: state}
	}
	return Observation{State: Readable, Targets: Parse(content)}
}

// Known says whether there is a record at all, readable or not. An absent record is a program that
// does not know what is installed; a record it cannot read is one that does, and cannot look.
func (o Observation) Known() bool { return o.State != Absent }

// Vocabulary says which names this build can install longterm-mem for.
type Vocabulary func(name string) bool

// UnknownTargetError is a name this build cannot install longterm-mem for, offered to be recorded.
type UnknownTargetError struct{ Name string }

func (e *UnknownTargetError) Error() string {
	return fmt.Sprintf("refusing to record an unknown install target: %s", e.Name)
}

// Admit refuses to record a name this build cannot install for. It gates what is written and
// nothing that is read: a name already in the record is evidence whatever this build knows. A
// vocabulary that is not there knows no name.
func Admit(name string, knows Vocabulary) error {
	if knows == nil || !knows(name) {
		return &UnknownTargetError{Name: name}
	}
	return nil
}

// Intent is what an uninstall asks of the registration record and the shared binary.
type Intent struct {
	// Purge forces their removal whatever the record says.
	Purge bool
	// UnregisterFailed says that a target the uninstall was asked to unregister was not.
	UnregisterFailed bool
}

// Verdict is what an uninstall comes to once every target it was asked for has been dealt with.
type Verdict int

// The verdicts, in the order the rule asks its questions.
const (
	// FailedUnregister: a target was not unregistered, so its entry may still be in its runtime's
	// configuration, and neither the registration record nor the binary is safe to remove.
	FailedUnregister Verdict = iota + 1
	// UnreadableRecord: the record is there and cannot be read, so whether any runtime depends on the
	// binary is not known.
	UnreadableRecord
	// Retire: nothing depends on the binary (the record is there and empty), or the removal was
	// forced. The registration record and the binary go.
	Retire
	// NoRecord: there is no record, so what is installed is not known, and nothing goes.
	NoRecord
	// StillInstalled: the record holds a target, and nothing goes.
	StillInstalled
)

// Settle decides what an uninstall does with the registration record and the shared binary. A target
// that failed to unregister outranks every other answer but a forced removal; a record that cannot be
// read outranks the rest of what it would have said; and an absent record fails closed, because
// absent means unknown and never empty.
func Settle(in Intent, obs Observation) Verdict {
	switch {
	case in.UnregisterFailed && !in.Purge:
		return FailedUnregister
	case obs.State == Unreadable && !in.Purge:
		return UnreadableRecord
	case in.Purge || (obs.Known() && obs.Targets.Empty()):
		return Retire
	case !obs.Known():
		return NoRecord
	default:
		return StillInstalled
	}
}

// Disposal is what happens to the shared binary, once it is asked, under the lock, whether to go.
type Disposal int

// The disposals.
const (
	// DisposeOfBinary: the binary and the record go.
	DisposeOfBinary Disposal = iota + 1
	// HoldBinaryUnreadable: the record cannot be read, so the binary stays.
	HoldBinaryUnreadable
	// KeepBinary: a runtime may depend on the binary, or nothing says that none does.
	KeepBinary
)

// Dispose decides whether the shared binary and the record go, from the record as it is now: the
// answer given to Settle was given on a record read without the lock, and a target recorded since
// keeps the binary.
func Dispose(purge bool, obs Observation) Disposal {
	switch {
	case obs.State == Unreadable && !purge:
		return HoldBinaryUnreadable
	case purge || (obs.Known() && obs.Targets.Empty()):
		return DisposeOfBinary
	default:
		return KeepBinary
	}
}
