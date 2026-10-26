package reviewresult

import (
	"fmt"
	"sort"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/gitobject"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/sha256hex"
)

// check requires the binding to name a candidate: two full git object names and a SHA-256 digest. A
// reply is held to the binding, so a binding that is not one holds it to nothing.
func (b Binding) check() error {
	if !gitobject.IsName(b.Base) {
		return fmt.Errorf("%w: base %q is not a full git object name", ErrInvalidBinding, b.Base)
	}
	if !gitobject.IsName(b.Head) {
		return fmt.Errorf("%w: head %q is not a full git object name", ErrInvalidBinding, b.Head)
	}
	if !sha256hex.IsValid(b.DiffSHA256) {
		return fmt.Errorf("%w: diff_sha256 %q is not a SHA-256 digest", ErrInvalidBinding, b.DiffSHA256)
	}
	return nil
}

// check holds the result to the binding, rule by rule, and returns the first rule it breaks. The rules
// run in the order the package comment lists them, so the same result is always refused for the same
// reason.
func (r Result) check(b Binding) error {
	for _, rule := range []func(Result, Binding) error{
		checkCandidate,
		checkInspectionCompleted,
		checkPaths,
		checkFindings,
	} {
		if err := rule(r, b); err != nil {
			return err
		}
	}
	return nil
}

// checkCandidate requires the candidate the result echoes to be the binding, member by member. The
// reviewer copies the three values from its prompt and never computes them, and a result applies to the
// bytes it was launched on and to no other.
func checkCandidate(r Result, b Binding) error {
	for _, member := range []struct{ name, echoed, bound string }{
		{"base", r.Candidate.Base, b.Base},
		{"head", r.Candidate.Head, b.Head},
		{"diff_sha256", r.Candidate.DiffSHA256, b.DiffSHA256},
	} {
		if member.echoed != member.bound {
			return fmt.Errorf("%w: %s echoed as %q, the binding has %q", ErrCandidateMismatch, member.name, member.echoed, member.bound)
		}
	}
	return nil
}

// checkInspectionCompleted requires the inspection to be completed. The reviewer says "incomplete" when
// any changed path could not be inspected, and only a completed inspection is a review. The error carries
// the reason the reviewer gave, which is the reason the review did not run.
func checkInspectionCompleted(r Result, _ Binding) error {
	if r.Inspection.Status == InspectionCompleted {
		return nil
	}
	reason := ""
	if r.Inspection.Reason != "" {
		reason = "; reason: " + r.Inspection.Reason
	}
	return fmt.Errorf("%w: inspection.status is %q, want %q%s", ErrInspectionNotCompleted, r.Inspection.Status, InspectionCompleted, reason)
}

// checkPaths requires the paths the reviewer says it inspected to be the paths of the diff, as sets: a
// path of the diff that is not listed is a file nobody read, and a path listed that the diff does not
// change belongs to another candidate. The error names every path on either side.
func checkPaths(r Result, b Binding) error {
	notInspected := without(b.Paths, r.Inspection.Paths)
	notInDiff := without(r.Inspection.Paths, b.Paths)
	if len(notInspected) == 0 && len(notInDiff) == 0 {
		return nil
	}
	var parts []string
	if len(notInspected) > 0 {
		parts = append(parts, "not inspected: "+quoteAll(notInspected))
	}
	if len(notInDiff) > 0 {
		parts = append(parts, "not in the diff: "+quoteAll(notInDiff))
	}
	return fmt.Errorf("%w: %s", ErrPathsMismatch, strings.Join(parts, "; "))
}

// without returns the paths of from that are not in remove, each once, sorted.
func without(from, remove []string) []string {
	removed := make(map[string]bool, len(remove))
	for _, path := range remove {
		removed[path] = true
	}
	var left []string
	for _, path := range from {
		if !removed[path] {
			left = append(left, path)
			removed[path] = true
		}
	}
	sort.Strings(left)
	return left
}

// quoteAll writes each path in Go quotes, so a name with a space or a quote of its own reads as one.
func quoteAll(paths []string) string {
	quoted := make([]string, len(paths))
	for i, path := range paths {
		quoted[i] = fmt.Sprintf("%q", path)
	}
	return strings.Join(quoted, ", ")
}

// checkFindings holds each finding, in the order the reviewer wrote them, to the rules of a finding, and
// returns the first rule broken. A finding has an id that is not blank and that no earlier finding has: the
// verifier answers per id, so an id that names two findings answers for both.
func checkFindings(r Result, _ Binding) error {
	firstWith := make(map[string]int, len(r.Findings))
	for i, f := range r.Findings {
		if strings.TrimSpace(f.ID) == "" {
			return fmt.Errorf("%w: findings[%d] has a blank id", ErrMissingID, i)
		}
		if first, repeated := firstWith[f.ID]; repeated {
			return fmt.Errorf("%w: id %q is on findings[%d] and findings[%d]", ErrDuplicateID, f.ID, first, i)
		}
		firstWith[f.ID] = i
		if err := checkEnumerations(f); err != nil {
			return err
		}
		if err := checkProof(f); err != nil {
			return err
		}
	}
	return nil
}

// The values of the three enumerations of a finding, as agents/reviewer.md lists them.
var (
	severities      = []Severity{SeverityBlocker, SeverityCritical, SeverityWarning, SeveritySuggestion}
	evidenceClasses = []EvidenceClass{ClassDeterministic, ClassInferential}
	dispositions    = []Disposition{
		DispositionIntroduced, DispositionBehaviorActivated, DispositionWorsened,
		DispositionPreExisting, DispositionBaseOnly, DispositionUnknown,
	}
)

// checkEnumerations requires the severity, the class of evidence and the causal disposition of a finding
// to be values the contract lists. Nothing is read as the nearest value.
func checkEnumerations(f Finding) error {
	if err := oneOf(f, "severity", f.Severity, severities); err != nil {
		return err
	}
	if err := oneOf(f, "evidence_class", f.EvidenceClass, evidenceClasses); err != nil {
		return err
	}
	return oneOf(f, "causal_disposition", f.Disposition, dispositions)
}

// oneOf requires value, the named member of f, to be one of allowed.
func oneOf[T ~string](f Finding, member string, value T, allowed []T) error {
	names := make([]string, len(allowed))
	for i, candidate := range allowed {
		if candidate == value {
			return nil
		}
		names[i] = string(candidate)
	}
	return fmt.Errorf("%w: finding %q has %s %q, want one of %s", ErrOutsideEnum, f.ID, member, string(value), strings.Join(names, ", "))
}

// checkProof requires a severe finding, which can block the review, to list at least one proof reference
// that is not blank: the changed hunk, the created path, the differential or the before and after that
// shows the candidate introduced, activated or worsened what it claims.
func checkProof(f Finding) error {
	if !f.Severity.Severe() {
		return nil
	}
	for _, ref := range f.ProofRefs {
		if strings.TrimSpace(ref) != "" {
			return nil
		}
	}
	return fmt.Errorf("%w: finding %q is a %s and lists no proof reference", ErrMissingProof, f.ID, f.Severity)
}
