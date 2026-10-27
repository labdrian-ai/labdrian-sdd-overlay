package reviewverdict

import (
	"errors"
	"fmt"
	"strings"
)

// The report of the exemption of a head (own-rdd T4, S5). A head whose work takes no review is exempted on the
// evidence of the verifier: the verifier is launched with no finding to answer, the prompt names the checks, and
// the verifier replies with one JSON object and no prose, the checks it ran in the order it ran them:
//
//	{"checks": [{"command": "go test ./review/...", "result": "pass", "output": "ok  review 0.009s"}]}
//
// ParseReport applies its rules in this order and returns the first one broken, so the same reply is always
// refused for the same reason; the error wraps exactly one of the Err values the verdicts use:
//
//   - the reply is one JSON value in valid UTF-8 with no key written twice (ErrUnparseable), so a reply that is
//     empty is a report that is missing;
//   - its objects, looked at one at a time in the order the reply holds them (the reply, then each check), have
//     no member the contract does not name, matched by exact name (ErrUnknownMember), every member it names and
//     none of them null, and each value of its type (ErrUnparseable);
//   - each check, in order, has a result the contract lists (ErrOutsideEnum) and carries its evidence: the
//     command that was run and an output, or the reason it did not run (ErrMissingEvidence).
//
// A report that parses is not yet clear. Clear says whether it is, and a report that is not is ErrNotClear.
// Like the verdicts, the report is evidence and never a claim to trust: the engine decides from the checks, and
// a check that did not run never passes. It is evidence that a head was checked, and it grants nothing.

// ErrNotClear is a report that parses and is not clear: it names no check, or a check failed or did not run.
var ErrNotClear = errors.New("reviewverdict: the report is not clear")

// CheckResult is what a check found when the verifier ran it.
type CheckResult string

// The values the contract lists. CheckNotRun says the check could not run (or was not tried), and is never read as
// CheckPassed: a check that cannot run proves nothing. The result is the verifier's, because whether a command
// passed is not always its exit code (a search that finds nothing exits 1 and is the check passing).
const (
	CheckPassed CheckResult = "pass"
	CheckFailed CheckResult = "fail"
	CheckNotRun CheckResult = "not-run"
)

// Check is one check the verifier ran, or tried to run.
type Check struct {
	// Command is the command the verifier ran, or would have run when the check did not run.
	Command string
	// Result is what the check found.
	Result CheckResult
	// Output is what the command printed, or why it did not run.
	Output string
}

// Report is the checks a verifier reported, in the order it reported them.
type Report struct{ Checks []Check }

// ParseReport reads the reply of a verifier that was asked for the report of an exemption and holds it to the
// rules in the comment above. It returns the report with the checks in the order the verifier wrote them, or the
// empty Report and the first rule the reply breaks, as an error that wraps one of the Err values.
func ParseReport(data []byte) (Report, error) {
	checks, err := decodeReport(data)
	if err != nil {
		return Report{}, err
	}
	for i, c := range checks {
		if err := c.check(i); err != nil {
			return Report{}, err
		}
	}
	return Report{Checks: checks}, nil
}

// check holds one check to the enumeration and the evidence rule. i is its place in the reply.
func (c Check) check(i int) error {
	if c.Result != CheckPassed && c.Result != CheckFailed && c.Result != CheckNotRun {
		return fmt.Errorf("%w: checks[%d] has result %q, want %q, %q or %q",
			ErrOutsideEnum, i, c.Result, CheckPassed, CheckFailed, CheckNotRun)
	}
	if strings.TrimSpace(c.Command) == "" {
		return fmt.Errorf("%w: checks[%d] names no command", ErrMissingEvidence, i)
	}
	if strings.TrimSpace(c.Output) == "" {
		return fmt.Errorf("%w: checks[%d] (%q) has no output", ErrMissingEvidence, i, c.Command)
	}
	return nil
}

// Clear says whether the report is clear: it names at least one check and every check passed. It returns nil for
// one that is, and for one that is not an error that wraps ErrNotClear and names the first check that keeps it
// from being clear, or says that the report names none.
func (r Report) Clear() error {
	if len(r.Checks) == 0 {
		return fmt.Errorf("%w: it names no check, so nothing was checked", ErrNotClear)
	}
	for i, c := range r.Checks {
		if c.Result != CheckPassed {
			return fmt.Errorf("%w: checks[%d] (%q) is %s", ErrNotClear, i, c.Command, c.Result)
		}
	}
	return nil
}
