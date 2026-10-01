package projection

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"
)

// This file is the domain's half of keeping a binding: the port that says what a
// place for bindings must do (BindingStore), what the bytes of a binding file mean
// (ClassifyBinding) and when a binding may be created, replaced or removed
// (AdmitBind, AdmitReplace, AdmitUnbind, AdmitRemoval). The rules are pure: they take
// values and return values, and know nothing of the file, the lock, or the
// directory the bytes came from. A BindingStore adapter owns those (see
// engine/projection/fsstore) and applies the rules in the same order every time: take
// the repository's lock, load, classify, admit, write or remove.
//
// The messages keep the wording the store has always printed ("projection store:
// ..."); callers show them to a person, and they do not change with where the code
// lives.

// Classification is the closed vocabulary Load reports for the on-disk state
// of one repository's binding. Only ClassificationAbsent and
// ClassificationOwned accept a write; every other value is preserved exactly as
// found and reported through the named refusal errors of Bind and Unbind. The
// vocabulary is the one engine/workflow uses for a workflow log, without the
// drifted value: a binding has no hash chain to drift.
type Classification string

const (
	// ClassificationAbsent means no binding file exists for the repository.
	ClassificationAbsent Classification = "absent"
	// ClassificationOwned means the file parses strictly as a Binding v1 and
	// its repo_key equals the key in its file name. Loaded.Binding is set.
	ClassificationOwned Classification = "owned"
	// ClassificationForeign means the file is valid JSON that is not ours: it
	// does not parse strictly as a Binding v1 (another schema or version, an
	// unknown or duplicated field, a field of the wrong shape, a document that
	// is not an object), or it is a valid Binding that names another
	// repository than its file name does. A foreign file is never overwritten
	// or removed.
	ClassificationForeign Classification = "foreign"
	// ClassificationMalformed means the file cannot be read as one JSON
	// document at all: it is empty, larger than MaxBindingBytes, not valid
	// UTF-8, not valid JSON, or valid JSON followed by more data. A malformed
	// file is never overwritten or removed.
	ClassificationMalformed Classification = "malformed"
	// ClassificationUnavailable means the state could not be read: a
	// permission error, a state home or store directory that is not a plain
	// directory (a symlink, or a file), or a binding path that is not a
	// regular file (a symlink, a directory, a FIFO). Nothing about the content
	// is known, so it is never overwritten or removed. It is the one
	// classification ClassifyBinding never returns: it describes a failure to
	// obtain bytes, which is the adapter's.
	ClassificationUnavailable Classification = "unavailable"
)

// Loaded is the result of Load: the classification, the parsed Binding only
// when the classification is ClassificationOwned, and, for every
// classification other than absent and owned, a human-readable Detail saying
// why.
type Loaded struct {
	Classification Classification
	Binding        Binding
	Detail         string
}

// Sentinel errors returned by a BindingStore. Wrap with %w so callers can tell the
// failures apart with errors.Is.
var (
	// ErrRefuseForeignBinding is returned by Bind and Unbind when the on-disk
	// binding is foreign; the file is left byte-for-byte unchanged.
	ErrRefuseForeignBinding = errors.New("projection store: refusing to change the binding: the on-disk file is foreign")
	// ErrRefuseMalformedBinding is returned by Bind and Unbind when the
	// on-disk binding is malformed; the file is left byte-for-byte unchanged.
	ErrRefuseMalformedBinding = errors.New("projection store: refusing to change the binding: the on-disk file is malformed")
	// ErrBindingUnavailable is returned by Bind and Unbind when the on-disk
	// state could not be read; nothing is written or removed.
	ErrBindingUnavailable = errors.New("projection store: refusing to change the binding: the on-disk state is unavailable")
	// ErrAlreadyBound is returned by Bind when the repository is owned by a
	// binding to a different workflow and the caller did not ask to replace it.
	// The message names the workflow it is bound to.
	ErrAlreadyBound = errors.New("projection store: the repository is already bound to a different workflow")
	// ErrBindingBusy is returned by Bind, Unbind, BindIfUnchanged, and
	// UnbindIfUnchanged when the repository's lock stays taken past the store's
	// lock wait. Nothing was changed; the call can be retried.
	ErrBindingBusy = errors.New("projection store: another bind or unbind is in progress for this repository; retry")
	// ErrBindingChanged is returned by BindIfUnchanged and UnbindIfUnchanged
	// when the stored binding is no longer the one the caller read: it was
	// replaced, or became a file that is not ours (BindIfUnchanged also reports
	// a binding that was removed). The message says what is there now. Nothing
	// was written or removed.
	ErrBindingChanged = errors.New("projection store: the binding changed since it was read")
)

// BindingStore is where bindings are kept, one per repository. The domain owns this
// port and adapters implement it. Every method takes the repository's key (64
// lowercase hex digits) and reports a problem with the state itself through
// Classification, not through an error; an error is for input that is refused before
// anything is read, a lock that stays taken, and a failure to write. The decisions
// are the functions of this file; an adapter holds the lock that makes them still
// true when it writes.
type BindingStore interface {
	// Load classifies the on-disk state of one repository's binding and, for
	// ClassificationOwned, returns the parsed Binding. Load never writes.
	Load(repoKey string) (Loaded, error)
	// Bind binds the repository to workflow projectID/workflowID, recording now
	// as when (see AdmitBind for what is created, kept, replaced or refused).
	Bind(repoKey, projectID, workflowID string, now time.Time, replace bool) error
	// BindIfUnchanged replaces the binding only if it is still exactly expected,
	// the binding the caller read and judged (see AdmitReplace).
	BindIfUnchanged(repoKey, projectID, workflowID string, now time.Time, expected Binding) error
	// Unbind removes the binding and reports whether it removed one (see
	// AdmitUnbind).
	Unbind(repoKey string) (removed bool, err error)
	// UnbindIfUnchanged removes the binding only if it is still exactly expected
	// (see AdmitRemoval).
	UnbindIfUnchanged(repoKey string, expected Binding) (removed bool, err error)
}

// NewBinding is the binding of repoKey to workflow projectID/workflowID made at now,
// recorded in UTC without fractions of a second.
func NewBinding(repoKey, projectID, workflowID string, now time.Time) Binding {
	return Binding{
		Version:    BindingVersion,
		RepoKey:    repoKey,
		ProjectID:  projectID,
		WorkflowID: workflowID,
		BoundAt:    now.UTC().Format(time.RFC3339),
	}
}

// ClassifyBinding classifies the bytes of the binding file of repoKey. It is pure:
// see Classification for the rules. The order matters. What cannot be read as one
// JSON document is malformed. What can, but is not a Binding v1 for this key, is
// foreign. It never returns ClassificationUnavailable or ClassificationAbsent, which
// describe a file that was not read.
func ClassifyBinding(repoKey string, data []byte) Loaded {
	switch {
	case len(data) == 0:
		return Loaded{Classification: ClassificationMalformed, Detail: "binding file is empty"}
	case len(data) > MaxBindingBytes:
		return Loaded{Classification: ClassificationMalformed, Detail: fmt.Sprintf("binding file exceeds the maximum of %d bytes", MaxBindingBytes)}
	case !utf8.Valid(data):
		return Loaded{Classification: ClassificationMalformed, Detail: "binding file is not valid UTF-8"}
	case !json.Valid(data):
		// json.Valid is false for invalid syntax and for a document followed
		// by more data alike.
		return Loaded{Classification: ClassificationMalformed, Detail: "binding file is not one valid JSON document (invalid syntax, or data after the document)"}
	}
	b, err := ParseBinding(data)
	if err != nil {
		return Loaded{Classification: ClassificationForeign, Detail: fmt.Sprintf("binding file is not a binding we recognize: %v", err)}
	}
	if b.RepoKey != repoKey {
		return Loaded{Classification: ClassificationForeign, Detail: fmt.Sprintf("binding file names repo_key %q, but its file name says %q", b.RepoKey, repoKey)}
	}
	return Loaded{Classification: ClassificationOwned, Binding: b}
}

// AdmitBind decides what Bind does with the current state loaded when the caller
// asks for record (the binding it would write):
//
//   - An absent binding is created.
//   - An owned binding to the same workflow is an idempotent no-op: nothing is
//     rewritten and the original bound_at is kept, whether or not replace is set.
//   - An owned binding to a different workflow is refused with ErrAlreadyBound,
//     whose message names the bound workflow, unless replace is true, in which case
//     it is replaced.
//   - A foreign, malformed, or unavailable file is refused with
//     ErrRefuseForeignBinding, ErrRefuseMalformedBinding, or ErrBindingUnavailable,
//     replace or not.
//
// It returns whether to write record, or the refusal. It performs no I/O.
func AdmitBind(loaded Loaded, record Binding, replace bool) (write bool, err error) {
	switch loaded.Classification {
	case ClassificationAbsent:
		return true, nil
	case ClassificationOwned:
		bound := loaded.Binding
		if bound.ProjectID == record.ProjectID && bound.WorkflowID == record.WorkflowID {
			return false, nil
		}
		if !replace {
			return false, fmt.Errorf("%w: it is bound to workflow %q of project %q", ErrAlreadyBound, bound.WorkflowID, bound.ProjectID)
		}
		return true, nil
	default:
		return false, refusal(loaded)
	}
}

// CheckExpected refuses, before anything is read or created, an expected binding
// that is invalid or names another repository than repoKey. op names the operation
// in the message ("bind if unchanged", "unbind if unchanged"). Such a refusal is
// input that is wrong, not a binding that changed, so it is not ErrBindingChanged.
func CheckExpected(op, repoKey string, expected Binding) error {
	if err := expected.Validate(); err != nil {
		return fmt.Errorf("projection store: %s: expected binding: %w", op, err)
	}
	if expected.RepoKey != repoKey {
		return fmt.Errorf("projection store: %s: expected binding names repo_key %q, not %q", op, expected.RepoKey, repoKey)
	}
	return nil
}

// UnchangedSince returns nil only when loaded is an owned binding equal to expected,
// bound_at included, so a workflow that was unbound and bound again counts as a
// different binding. Every other state is ErrBindingChanged, saying what is there
// now: another binding, nothing, or a file that is foreign, malformed, or
// unavailable. It performs no I/O.
func UnchangedSince(loaded Loaded, expected Binding) error {
	switch loaded.Classification {
	case ClassificationOwned:
		if loaded.Binding == expected {
			return nil
		}
		now := loaded.Binding
		return fmt.Errorf("%w: it now names workflow %q of project %q, bound at %s", ErrBindingChanged, now.WorkflowID, now.ProjectID, now.BoundAt)
	case ClassificationAbsent:
		return fmt.Errorf("%w: it was removed", ErrBindingChanged)
	default:
		return fmt.Errorf("%w: it is now %s: %s", ErrBindingChanged, loaded.Classification, loaded.Detail)
	}
}

// AdmitReplace decides what BindIfUnchanged does: the stored binding must still be
// exactly expected (UnchangedSince), or nothing is written and the answer is
// ErrBindingChanged. When it is, and it already names the workflow record binds,
// the call is an idempotent no-op like AdmitBind's: nothing is rewritten and the
// original bound_at is kept. Otherwise record is written. It performs no I/O.
func AdmitReplace(loaded Loaded, expected, record Binding) (write bool, err error) {
	if err := UnchangedSince(loaded, expected); err != nil {
		return false, err
	}
	if expected.ProjectID == record.ProjectID && expected.WorkflowID == record.WorkflowID {
		return false, nil
	}
	return true, nil
}

// AdmitUnbind decides what Unbind does with the current state loaded: an absent
// binding is nothing to remove and not an error (unbinding twice is fine), an owned
// binding is removed, and a foreign, malformed, or unavailable file is refused with
// the same named errors as AdmitBind. It performs no I/O.
func AdmitUnbind(loaded Loaded) (remove bool, err error) {
	switch loaded.Classification {
	case ClassificationAbsent:
		return false, nil
	case ClassificationOwned:
		return true, nil
	default:
		return false, refusal(loaded)
	}
}

// AdmitRemoval decides what UnbindIfUnchanged does: a binding that is already gone is
// nothing to remove and not an error (the state the caller wanted holds), the
// binding the caller read is removed, and any other state is ErrBindingChanged,
// saying what is there now. It performs no I/O.
func AdmitRemoval(loaded Loaded, expected Binding) (remove bool, err error) {
	if loaded.Classification == ClassificationAbsent {
		return false, nil
	}
	if err := UnchangedSince(loaded, expected); err != nil {
		return false, err
	}
	return true, nil
}

// refusal maps a classification that must not be written over to its named
// error, keeping the detail that explains it.
func refusal(loaded Loaded) error {
	switch loaded.Classification {
	case ClassificationForeign:
		return fmt.Errorf("%w: %s", ErrRefuseForeignBinding, loaded.Detail)
	case ClassificationMalformed:
		return fmt.Errorf("%w: %s", ErrRefuseMalformedBinding, loaded.Detail)
	case ClassificationUnavailable:
		return fmt.Errorf("%w: %s", ErrBindingUnavailable, loaded.Detail)
	default:
		return fmt.Errorf("projection store: unknown classification %q", loaded.Classification)
	}
}
