// Package reviewverdict is the contract of what the verifier role returns about the findings of a stored
// review result (roles/verifier/body.md): one JSON object and no prose, one verdict per finding id of the
// result it was launched on. That reply is evidence and never a claim to trust, so Parse reads it against the
// result it answers and refuses it, saying why, unless every rule holds. A refused reply is a verification
// that did not run, never one that cleared.
//
// Parse applies the rules in this order and returns the first one broken, so the same reply is always
// refused for the same reason; the error wraps exactly one of the Err values:
//
//   - the reply is one JSON value in valid UTF-8 with no key written twice (ErrUnparseable);
//   - its objects, looked at one at a time in the order the reply holds them (the reply, then each verdict),
//     have no member the contract does not name, matched by exact name (ErrUnknownMember), every member it
//     names and none of them null, and each value of its type (ErrUnparseable);
//   - each verdict, in order, answers a finding of the result (ErrUnknownID) that no earlier verdict answered
//     (ErrDuplicateID), has a result and a base_result the contract lists (ErrOutsideEnum), and carries its
//     evidence: an output, and the command whenever a check ran, on head or on base (ErrMissingEvidence);
//   - every finding of the result is answered (ErrUnanswered): a finding nobody checked is never read as one
//     that did not reproduce.
//
// Decide turns a result and its verdicts into the decision of skills/role-chain/SKILL.md: whether the review
// blocks. The object is the one the delta of roles/verifier/body.md spells out, and tests in engine/skills
// require Parse to accept it, so the agent's instructions and these rules cannot drift apart.
//
// It is a domain package: pure standard library and reviewresult. It reads no file and runs no command, and
// the exported types carry no JSON tag; the wire form of a reply stays inside the package.
package reviewverdict

import (
	"errors"
	"fmt"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewresult"
)

// The reasons Parse refuses a reply. An error it returns wraps exactly one of them, so a caller tells them
// apart with errors.Is and reads the detail (the member, the id) in the message.
var (
	// ErrUnparseable is a reply that is not the one JSON object of the contract: it is not valid UTF-8, not
	// one JSON value, it writes a key twice, a member has the wrong type or is missing or null, or
	// "verdicts" is not an array.
	ErrUnparseable = errors.New("reviewverdict: unparseable reply")

	// ErrUnknownMember is a reply with a member the contract does not name, in any object of it.
	ErrUnknownMember = errors.New("reviewverdict: member outside the contract")

	// ErrUnknownID is a verdict whose id is the id of no finding of the result.
	ErrUnknownID = errors.New("reviewverdict: verdict for an id no finding has")

	// ErrDuplicateID is a verdict for a finding that an earlier verdict of the reply already answered.
	ErrDuplicateID = errors.New("reviewverdict: finding answered twice")

	// ErrUnanswered is a reply that leaves a finding of the result without a verdict.
	ErrUnanswered = errors.New("reviewverdict: finding without a verdict")

	// ErrOutsideEnum is a verdict whose result or base_result is not a value the contract lists.
	ErrOutsideEnum = errors.New("reviewverdict: value outside its enumeration")

	// ErrMissingEvidence is a verdict with no output, or with no command although a check ran.
	ErrMissingEvidence = errors.New("reviewverdict: verdict without evidence")
)

// Reproduction is what a check found when the verifier ran it.
type Reproduction string

// The values the contract lists. NotRun says the check could not run (or was not tried), and is never read
// as NotReproduced: a check that cannot run proves nothing.
const (
	Reproduced    Reproduction = "reproduced"
	NotReproduced Reproduction = "not-reproduced"
	NotRun        Reproduction = "not-run"
)

// Verdict is the verifier's answer for one finding of the result.
type Verdict struct {
	// ID is the id of the finding answered.
	ID string
	// Result is what the check found on head, the commit under review.
	Result Reproduction
	// Command is the command the verifier ran, on head and on base. It is empty only when no check ran.
	Command string
	// Output is what the command printed, or why no check ran.
	Output string
	// Base is what the same check found on base, the commit the candidate is compared to.
	Base Reproduction
}

// Parse reads the reply of a verifier and holds it to the result it answers and to the rules in the package
// comment. It returns the verdicts in the order the verifier wrote them, or no verdicts and the first rule
// the reply breaks, as an error that wraps one of the Err values.
func Parse(data []byte, answered reviewresult.Result) ([]Verdict, error) {
	verdicts, err := decode(data)
	if err != nil {
		return nil, err
	}
	if err := check(verdicts, answered); err != nil {
		return nil, err
	}
	return verdicts, nil
}

// check holds decoded verdicts to the result they answer: verdict by verdict, then the findings left out.
func check(verdicts []Verdict, answered reviewresult.Result) error {
	done := make(map[string]bool, len(answered.Findings))
	for _, f := range answered.Findings {
		done[f.ID] = false
	}
	for i, v := range verdicts {
		answeredBefore, known := done[v.ID]
		switch {
		case !known:
			return fmt.Errorf("%w: verdicts[%d] answers %q", ErrUnknownID, i, v.ID)
		case answeredBefore:
			return fmt.Errorf("%w: verdicts[%d] answers %q again", ErrDuplicateID, i, v.ID)
		}
		done[v.ID] = true
		if err := v.check(i); err != nil {
			return err
		}
	}
	var missing []string
	for _, f := range answered.Findings {
		if !done[f.ID] {
			missing = append(missing, fmt.Sprintf("%q", f.ID))
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%w: %s", ErrUnanswered, strings.Join(missing, ", "))
	}
	return nil
}

// check holds one verdict to the enumerations and the evidence rule. i is its place in the reply.
func (v Verdict) check(i int) error {
	for _, c := range []struct {
		member string
		value  Reproduction
	}{{"result", v.Result}, {"base_result", v.Base}} {
		if c.value != Reproduced && c.value != NotReproduced && c.value != NotRun {
			return fmt.Errorf("%w: verdicts[%d] has %s %q, want %q, %q or %q",
				ErrOutsideEnum, i, c.member, c.value, Reproduced, NotReproduced, NotRun)
		}
	}
	if strings.TrimSpace(v.Output) == "" {
		return fmt.Errorf("%w: verdicts[%d] (%q) has no output", ErrMissingEvidence, i, v.ID)
	}
	if v.ran() && strings.TrimSpace(v.Command) == "" {
		return fmt.Errorf("%w: verdicts[%d] (%q) ran a check and names no command", ErrMissingEvidence, i, v.ID)
	}
	return nil
}

// ran reports whether a check ran for the verdict, on head or on base.
func (v Verdict) ran() bool { return v.Result != NotRun || v.Base != NotRun }
