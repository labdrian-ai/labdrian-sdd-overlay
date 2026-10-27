package review

import (
	"errors"
	"fmt"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewresult"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/roles"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/sha256hex"
)

// This file is what a record of the role chain that closes a review is held to when it is appended (own-rdd T4,
// S5): that its evidence is the evidence of the files the engine stored, and not a claim about them. A record
// carries the range that was reviewed with the digest of its diff (EvidenceDiff), the stored result with its
// digest (EvidenceResult) and, when the verifier ran, the stored verdict with its digest (EvidenceVerifier). The
// record that closes the exemption of a head from review (PayloadExemption) is held in the same way to the range
// and to the stored exemption with its digest (EvidenceExemption, ExemptionRef). Everything here is pure: it works
// on the values of the record and on the bytes of a file somebody else read, and the use case that reads them is
// review/app's CheckEvidence.

// The reasons a piece of evidence is refused, as an EvidenceError wraps them. Each reads after
// "evidence <kind> <ref>: ", so that the message says which piece it is about and then what is wrong with it.
var (
	// ErrEvidenceMissing is evidence of a kind that a record needs and does not carry. RequiredEvidence adds which
	// record needs it.
	ErrEvidenceMissing = errors.New("missing")
	// ErrEvidenceRepeated is a kind of evidence that a record carries more than once, so that none of the
	// pieces is the evidence of its kind.
	ErrEvidenceRepeated = errors.New("given more than once, so none of the pieces is the evidence of its kind")
	// ErrEvidenceRef is a ref that is not a file the engine stores in the results directory, or not the one
	// that kind of evidence must name.
	ErrEvidenceRef = errors.New("not a file the engine stores")
	// ErrEvidenceDiffers is evidence whose digest or range is not that of the stored file it names.
	ErrEvidenceDiffers = errors.New("differs from the stored file")
)

// EvidenceError is a piece of evidence that does not hold to the stored files it names. Kind is its kind and
// Ref its ref, empty when the record has none of that kind to name. Err is the reason: one of the ErrEvidence
// values, wrapped or not, or what the store said when it could not read the file, which errors.Is reads
// through it.
type EvidenceError struct {
	Kind string
	Ref  string
	Err  error
}

func (e *EvidenceError) Error() string {
	if e.Ref == "" {
		return fmt.Sprintf("evidence %s: %v", e.Kind, e.Err)
	}
	return fmt.Sprintf("evidence %s %q: %v", e.Kind, e.Ref, e.Err)
}

func (e *EvidenceError) Unwrap() error { return e.Err }

// NeedsEvidence reports whether h is a record that must carry evidence of the stored files: a review-report or a
// review-exemption that is completed. An interrupted one says the review did not run, or the exemption was not
// made, and what did not run stores nothing, so its evidence would have no file to name; a record of any other kind
// is not about a review.
func NeedsEvidence(h roles.RoleHandoff) bool {
	return (h.PayloadKind == PayloadKind || h.PayloadKind == PayloadExemption) && h.Status == roles.StatusCompleted
}

// OptionalEvidence returns the one piece of evidence of the kind that h carries, and whether it carries one.
// A record with the kind more than once is refused with an *EvidenceError that wraps ErrEvidenceRepeated.
func OptionalEvidence(h roles.RoleHandoff, kind string) (roles.Evidence, bool, error) {
	var found []roles.Evidence
	for _, e := range h.Evidence {
		if e.Kind == kind {
			found = append(found, e)
		}
	}
	switch len(found) {
	case 0:
		return roles.Evidence{}, false, nil
	case 1:
		return found[0], true, nil
	}
	return roles.Evidence{}, false, &EvidenceError{Kind: kind, Err: fmt.Errorf("%w: it carries %d", ErrEvidenceRepeated, len(found))}
}

// RequiredEvidence is OptionalEvidence for a kind the record must carry: one that it does not is refused with
// an *EvidenceError that wraps ErrEvidenceMissing and says which kind of record needs it, the payload_kind of h.
func RequiredEvidence(h roles.RoleHandoff, kind string) (roles.Evidence, error) {
	e, found, err := OptionalEvidence(h, kind)
	if err == nil && !found {
		err = &EvidenceError{Kind: kind, Err: fmt.Errorf("%w: a completed %s record carries one", ErrEvidenceMissing, h.PayloadKind)}
	}
	return e, err
}

// The shapes of the one ref the engine reads for a stored result and for a stored exemption.
const (
	wantResultRef    = ResultsDir + "/<feature>/<head12>-<n>.json"
	wantExemptionRef = ResultsDir + "/<feature>/<head12>-<n>.exempt.json"
)

// ResultRef is the feature and the file name of the stored result that e names. The ref is a path relative to
// the repository, and it is exactly one file of the results directory: <ResultsDir>/<feature>/<head12>-<n>.json.
// Anything else is refused by its shape, before any file is looked at, with an *EvidenceError that wraps
// ErrEvidenceRef and the rule of the name that was broken (ErrInvalidFeature or ErrInvalidResultName): a path
// that leaves the directory, goes through a segment that is not a name, or names a verdict, an exemption, a
// temporary file or any other file. The feature and the name it returns are the ones the store takes, and the
// store walks to the file without following a link.
func ResultRef(e roles.Evidence) (feature, name string, err error) {
	return storedRef(e, wantResultRef, CheckResultName)
}

// ExemptionRef is the feature and the file name of the stored exemption that e names, as ResultRef is for a stored
// result: exactly one file of the results directory, <ResultsDir>/<feature>/<head12>-<n>.exempt.json, refused by
// its shape otherwise with an *EvidenceError that wraps ErrEvidenceRef and the rule of the name that was broken
// (ErrInvalidFeature or ErrInvalidExemptionName). A result and a verdict are not exemptions.
func ExemptionRef(e roles.Evidence) (feature, name string, err error) {
	return storedRef(e, wantExemptionRef, CheckExemptionName)
}

// storedRef is ResultRef and ExemptionRef: e must name one file of the results directory, <ResultsDir>/<feature>/
// <name>, the feature one safe path segment and the name one that checkName accepts. want is the shape the error
// says it wanted.
func storedRef(e roles.Evidence, want string, checkName func(string) error) (feature, name string, err error) {
	refuse := func(why error) (string, string, error) {
		return "", "", &EvidenceError{Kind: e.Kind, Ref: e.Ref, Err: why}
	}
	rest, underResults := strings.CutPrefix(e.Ref, ResultsDir+"/")
	feature, name, twoSegments := strings.Cut(rest, "/")
	if !underResults || !twoSegments || strings.Contains(name, "/") {
		return refuse(fmt.Errorf("%w: want %s", ErrEvidenceRef, want))
	}
	if err := CheckFeature(feature); err != nil {
		return refuse(fmt.Errorf("%w: %w", ErrEvidenceRef, err))
	}
	if err := checkName(name); err != nil {
		return refuse(fmt.Errorf("%w: %w", ErrEvidenceRef, err))
	}
	return feature, name, nil
}

// VerdictRef is the file name of the verdict that e names, when e names the verdict stored beside the result
// called result in the directory of feature, and only then: the ref is compared with the one path that result
// gives (ResultPath of VerdictName), so no other ref, the result itself included, is a verdict. A ref that is
// not that one is refused with an *EvidenceError that wraps ErrEvidenceRef; a result that is not named
// <head12>-<n>.json has no verdict to name, and the error wraps ErrInvalidResultName as well.
func VerdictRef(e roles.Evidence, feature, result string) (string, error) {
	name, err := VerdictName(result)
	if err != nil {
		return "", &EvidenceError{Kind: e.Kind, Ref: e.Ref, Err: fmt.Errorf("%w: the result it answers: %w", ErrEvidenceRef, err)}
	}
	if want := ResultPath(feature, name); e.Ref != want {
		return "", &EvidenceError{Kind: e.Kind, Ref: e.Ref, Err: fmt.Errorf("%w: want %s, the verdict stored beside the result", ErrEvidenceRef, want)}
	}
	return name, nil
}

// CheckFile holds e, evidence over a stored file, to the bytes of that file as they are now: its sha256 must be
// the digest of data. The error is an *EvidenceError that wraps ErrEvidenceDiffers and says both digests.
func CheckFile(e roles.Evidence, data []byte) error {
	if got := sha256hex.Sum(data); got != e.SHA256 {
		return &EvidenceError{Kind: e.Kind, Ref: e.Ref, Err: fmt.Errorf("%w: its sha256 is %s and the stored file's is %s", ErrEvidenceDiffers, e.SHA256, got)}
	}
	return nil
}

// CheckRange holds e, evidence over the range that was reviewed, to the binding stored with the result, or with
// the exemption: its ref must be <base>..<head> of the binding and its sha256 the diff_sha256 of it. The error is
// an *EvidenceError that wraps ErrEvidenceDiffers and says what the stored file holds instead. stored is what that
// file is called in the message, "result" or "exemption".
func CheckRange(e roles.Evidence, b reviewresult.Binding, stored string) error {
	if want := b.Base + ".." + b.Head; e.Ref != want {
		return &EvidenceError{Kind: e.Kind, Ref: e.Ref, Err: fmt.Errorf("%w: the stored %s is about the range %s", ErrEvidenceDiffers, stored, want)}
	}
	if e.SHA256 != b.DiffSHA256 {
		return &EvidenceError{Kind: e.Kind, Ref: e.Ref, Err: fmt.Errorf("%w: its sha256 is %s and the diff_sha256 stored with the %s is %s", ErrEvidenceDiffers, e.SHA256, stored, b.DiffSHA256)}
	}
	return nil
}
