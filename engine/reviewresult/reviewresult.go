// Package reviewresult is the contract of what the reviewer role returns, and the rules that refuse a
// reply that does not keep it. The reviewer (roles/reviewer/body.md, agents/reviewer.md) replies with one
// JSON object and no prose. That reply is evidence about a candidate and never a claim to trust, so Parse
// reads it against the candidate the parent launched the reviewer on and refuses it, saying why, unless
// every rule below holds. A refused reply is a review that did not run, never a review that approved.
//
// The rules, in the order Parse applies them. It returns the first one broken, so the same reply is always
// refused for the same reason, and the error wraps exactly one of the Err values:
//
//   - the Binding names a candidate: base and head are full git object names and diff_sha256 is a SHA-256
//     digest (ErrInvalidBinding). A binding that names nothing holds a reply to nothing: with an empty one,
//     a reply that echoes empty values would be "the same candidate";
//   - the reply is the object of the contract (ErrUnparseable): valid UTF-8, one JSON value, no key written
//     twice, and in every object (the reply, its candidate, its inspection and each finding) every member
//     the contract names, none of them null and each of its type. An empty array says "nothing"; a missing
//     or null member says nothing at all and is never read as "nothing", so a reply with no findings member
//     is not a clean review. The one exception in name is a finding whose id member is missing or null: it
//     is found with the shape and is ErrMissingID, not ErrUnparseable;
//   - no object has a member the contract does not name (ErrUnknownMember), matched by exact name, since
//     the JSON decoder would read "Severity" after "severity" as the same member and keep the last. A
//     result of ours is check evidence in the role chain and never review authority: it carries no schema,
//     lineage, receipt or GENTLE_AI_* member;
//   - the candidate it echoes is the binding, member by member (ErrCandidateMismatch): a result applies to
//     the bytes it was launched on and to no other;
//   - inspection.status is "completed" (ErrInspectionNotCompleted); an incomplete inspection carries the
//     reason the reviewer gave;
//   - the paths it says it inspected are the paths of the diff, compared as sets (ErrPathsMismatch): a path
//     of the diff it did not list is a file nobody read;
//   - each finding, in order, has an id that is not blank (ErrMissingID) and that no earlier finding has
//     (ErrDuplicateID); a severity, class of evidence and causal disposition that are values the contract
//     lists (ErrOutsideEnum); and, when it is a BLOCKER or a CRITICAL, at least one proof reference that is
//     not blank (ErrMissingProof).
//
// The contract itself is the object that the third line of the delta of roles/reviewer/body.md spells
// out; tests in engine/skills pin that text and require Parse to accept the object it spells out, so the
// agent's instructions and these rules cannot drift apart without a test failing.
//
// It is a domain package: pure standard library and the domain packages that say what a git object name
// and a SHA-256 digest are. It reads no file and runs no git: the caller computes the Binding, the paths of
// the diff included, and hands it in as a value. The exported types carry no JSON tag; the wire form of a
// reply stays inside the package.
package reviewresult

// Binding is the candidate the parent launched the reviewer on, as the parent computed it. Parse computes
// none of it.
type Binding struct {
	// Base and Head are the full object names of the two commits of the diff: 40 lowercase hexadecimal
	// characters (64 in a SHA-256 repository).
	Base string
	Head string
	// DiffSHA256 is the SHA-256 of the exact bytes of `git diff base head`, as 64 lowercase hexadecimal
	// characters.
	DiffSHA256 string
	// Paths are the paths the diff changes, as `git diff --name-only base head` gives them: byte for byte,
	// one entry per path. The reviewer's list of inspected paths is held to this one as a set.
	Paths []string
}

// Result is a reply of the reviewer that Parse accepted.
type Result struct {
	// Candidate is what the reviewer echoed; Parse accepts it only when it equals the Binding.
	Candidate  Candidate
	Inspection Inspection
	// Findings are the defects reported, in the order the reviewer wrote them. A clean review has none.
	Findings []Finding
	// Evidence says what the reviewer inspected, in its own words.
	Evidence []string
}

// Candidate is the candidate a reply echoes: the three values the reviewer copies from its prompt.
type Candidate struct {
	Base       string
	Head       string
	DiffSHA256 string
}

// Inspection is the reviewer's account of what it read.
type Inspection struct {
	Status InspectionStatus
	// Paths are the changed paths the reviewer says it inspected.
	Paths []string
	// Reason says why an inspection is incomplete, and is empty otherwise.
	Reason string
}

// InspectionStatus is how far the reviewer got through the candidate.
type InspectionStatus string

// InspectionCompleted is the only status that makes a reply a review: every changed path was inspected.
// The other status the contract has, "incomplete", is the reviewer saying a path could not be inspected.
const InspectionCompleted InspectionStatus = "completed"

// Finding is one defect the reviewer reports.
type Finding struct {
	// ID names the finding within its reply; the verifier answers per ID.
	ID string
	// Location is path:line or path:start-end.
	Location string
	Severity Severity
	Claim    string
	// EvidenceClass says whether the reviewer knows the finding by a check or by reasoning.
	EvidenceClass EvidenceClass
	// Disposition says how the candidate relates to the defect: the "causal_disposition" of the reply.
	Disposition Disposition
	// ProofRefs are the changed hunks, created paths, differentials or before and after that prove the
	// claim. A BLOCKER or a CRITICAL has at least one.
	ProofRefs []string
}

// Severity is how serious a finding is.
type Severity string

// The severities the contract lists.
const (
	SeverityBlocker    Severity = "BLOCKER"
	SeverityCritical   Severity = "CRITICAL"
	SeverityWarning    Severity = "WARNING"
	SeveritySuggestion Severity = "SUGGESTION"
)

// Severe reports whether s is one of the two severities that can block a review, BLOCKER and CRITICAL.
func (s Severity) Severe() bool { return s == SeverityBlocker || s == SeverityCritical }

// EvidenceClass is how the reviewer came to know a finding.
type EvidenceClass string

// The classes of evidence the contract lists.
const (
	ClassDeterministic EvidenceClass = "deterministic"
	ClassInferential   EvidenceClass = "inferential"
)

// Disposition is the causal disposition of a finding: how the candidate relates to the defect.
type Disposition string

// The causal dispositions the contract lists.
const (
	DispositionIntroduced        Disposition = "introduced"
	DispositionBehaviorActivated Disposition = "behavior-activated"
	DispositionWorsened          Disposition = "worsened"
	DispositionPreExisting       Disposition = "pre-existing"
	DispositionBaseOnly          Disposition = "base-only"
	DispositionUnknown           Disposition = "unknown"
)

// Parse reads the reply of a reviewer and holds it to the binding and to the rules in the package comment.
// It returns the Result, or the zero Result and the first rule the reply breaks, as an error that wraps one
// of the Err values.
func Parse(data []byte, binding Binding) (Result, error) {
	if err := binding.check(); err != nil {
		return Result{}, err
	}
	result, err := decode(data)
	if err != nil {
		return Result{}, err
	}
	if err := result.check(binding); err != nil {
		return Result{}, err
	}
	return result, nil
}
