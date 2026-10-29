package review

import (
	"errors"
	"fmt"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewverdict"
)

// This file is what a head must satisfy to be exempted from review instead of reviewed (own-rdd T4, S5). The owner's
// decision (L15-L16) is that work that takes no review is delivered on the evidence of the verifier alone, as long as
// its risk is validated too. So an exemption is held to three things, which the command checks before it stores
// anything and the delivery check checks again from the stored file:
//
//   - the risk the agent declares names no item of the high-risk list of the orchestrator (CheckRisk). The engine
//     never computes a risk from a diff: it holds the declaration to a small grammar and refuses a head whose
//     declaration names any item, because a change that names one is exactly what a review is for;
//   - the report of the verifier is the object reviewverdict.ParseReport reads and it is clear (CheckReport);
//   - the exemption is for one commit (CheckOneCommit): reviews are per commit (L18), and an exemption that spans
//     several would step over the reviews of the commits in between, which a walk of the chain would never meet.
//
// An exemption is evidence that a head was checked and a risk was declared. It grants nothing by itself: Cleared is
// what lets a head through, and a stored result that blocks always wins over an exemption.

// NoRisk is the only declaration an exemption is made on: the Risk line of a change that names no item of the
// high-risk list.
const NoRisk = "none"

var (
	// ErrInvalidRisk is a declared risk that is neither NoRisk nor a list of the numbers of the high-risk items: it
	// is refused as a declaration the engine cannot read, never taken for the absence of risk.
	ErrInvalidRisk = errors.New("review: invalid risk declaration")
	// ErrHighRisk is a declared risk that names an item of the high-risk list. A change that names one is never
	// exempt from review, whatever else was checked.
	ErrHighRisk = errors.New("review: a change that names a high-risk item is never exempt from review")
	// ErrNotOneCommit is the range of an exemption that is not one commit: its base is not the first parent of its
	// head, or its head has no parent. An exemption of several commits is refused, whatever else was checked.
	ErrNotOneCommit = errors.New("review: an exemption is for one commit, so its base must be the first parent of its head")
)

// highRiskItems are the six items of the high-risk list of the orchestrator (Task Size), in the order of their
// numbers: the item numbered n is highRiskItems[n-1].
var highRiskItems = [...]string{
	"data or irreversible effects",
	"security",
	"contracts others consume",
	"concurrency",
	"delivery or environment",
	"no test would catch a regression",
}

// CheckRisk holds the risk an agent declares for an exemption to the grammar, which is NoRisk or a list of the
// numbers 1 to 6 of the high-risk items, separated by commas with no space and none written twice ("2,4"), and
// returns nil only for NoRisk. A declaration that names an item is refused with an error that wraps ErrHighRisk and
// names every item in the order it was written; a declaration that is anything else, including an empty one, "None"
// or a number outside the list, is refused with an error that wraps ErrInvalidRisk and says what is accepted.
func CheckRisk(declared string) error {
	items, err := ParseRisk(declared)
	if err != nil || len(items) == 0 {
		return err
	}
	named := make([]string, 0, len(items))
	for _, item := range items {
		named = append(named, fmt.Sprintf("%d (%s)", item, highRiskItems[item-1]))
	}
	return fmt.Errorf("%w: the declared risk names %s", ErrHighRisk, strings.Join(named, ", "))
}

// CheckReport holds the report an exemption is made on to its contract: the object reviewverdict.ParseReport reads,
// and clear (reviewverdict.Report.Clear), which a report that names no check, or a check that failed or did not run,
// is not. The error wraps the reason reviewverdict gives, so a caller tells a report that is missing or malformed
// from one that is not clear with errors.Is.
func CheckReport(report []byte) error {
	parsed, err := reviewverdict.ParseReport(report)
	if err != nil {
		return err
	}
	return parsed.Clear()
}

// CheckOneCommit holds the range of an exemption to the one commit it is for: base must be parent, the first parent of
// head as git gives it, or "" when head has none, all full object names. For a merge the first parent is the one the
// merge was made on, so the range is the diff the merge brought in. Anything else is refused with an error that wraps
// ErrNotOneCommit and says both commits. The parent is git's and is asked by the caller; this holds the comparison.
func CheckOneCommit(base, head, parent string) error {
	switch {
	case parent == "":
		return fmt.Errorf("%w: %s has no parent", ErrNotOneCommit, head)
	case base != parent:
		return fmt.Errorf("%w: the base is %s and the first parent of %s is %s", ErrNotOneCommit, base, head, parent)
	}
	return nil
}

// CheckExemption holds a stored exemption to the rules it was made under, again, in this order: its binding names a
// candidate (reviewresult.Binding.Check), as the binding of a stored result must before anything is done with it, so
// that a base that is not a full object name is refused before git is ever given it; its declared risk names no
// high-risk item (CheckRisk); and its report is clear (CheckReport). The engine decides from what the file stores and
// never from the file existing, so an exemption that was edited to say something else is refused. The binding is not
// compared with git here: CheckBinding holds it to the candidate git gives, and CheckOneCommit to the parent git gives.
func CheckExemption(e StoredExemption) error {
	if err := e.Binding.Check(); err != nil {
		return err
	}
	if err := CheckRisk(e.Risk); err != nil {
		return err
	}
	return CheckReport(e.Report)
}
