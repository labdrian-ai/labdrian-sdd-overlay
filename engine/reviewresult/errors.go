package reviewresult

import "errors"

// The reasons Parse refuses a reply. An error it returns wraps exactly one of them, so a caller tells them
// apart with errors.Is and reads the detail (the member, the id, the paths) in the message. The message is
// what the parent records as the reason the review did not run.
var (
	// ErrInvalidBinding is the caller's fault and not the reviewer's: the Binding does not name a
	// candidate (a base or head that is not a full git object name, a diff_sha256 that is not a digest, or
	// a path listed twice), so no reply can be held to it.
	ErrInvalidBinding = errors.New("reviewresult: invalid binding")

	// ErrUnparseable is a reply that is not the one JSON object of the contract: it is not valid UTF-8,
	// not one JSON value, it writes a key twice, a member has the wrong type, a member is missing or null
	// (the id of a finding aside: see ErrMissingID), or an array holds a null.
	ErrUnparseable = errors.New("reviewresult: unparseable reply")

	// ErrUnknownMember is a reply with a member the contract does not name, in any object of it.
	ErrUnknownMember = errors.New("reviewresult: member outside the contract")

	// ErrCandidateMismatch is a reply whose echoed base, head or diff_sha256 differs from the binding.
	ErrCandidateMismatch = errors.New("reviewresult: candidate echo differs from the binding")

	// ErrInspectionNotCompleted is a reply whose inspection.status is not "completed".
	ErrInspectionNotCompleted = errors.New("reviewresult: inspection not completed")

	// ErrPathsMismatch is a reply whose inspected paths are not the paths of the diff: a path of the diff
	// it does not list, a path the diff does not change, or a path it lists more than once.
	ErrPathsMismatch = errors.New("reviewresult: inspected paths differ from the paths of the diff")

	// ErrMissingID is a finding with no id: the member is missing or null, or the id is blank.
	ErrMissingID = errors.New("reviewresult: finding without an id")

	// ErrDuplicateID is a finding whose id an earlier finding of the reply already has.
	ErrDuplicateID = errors.New("reviewresult: finding id repeated")

	// ErrOutsideEnum is a finding whose severity, class of evidence or causal disposition is not a value
	// the contract lists.
	ErrOutsideEnum = errors.New("reviewresult: value outside its enumeration")

	// ErrMissingProof is a BLOCKER or CRITICAL finding that lists no proof reference.
	ErrMissingProof = errors.New("reviewresult: severe finding without a proof reference")
)
