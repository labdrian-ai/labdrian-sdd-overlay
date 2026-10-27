package review

import (
	"errors"
	"fmt"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewverdict"
)

// This file is what a head must satisfy to be exempted from review instead of reviewed (own-rdd T4, S5). The owner's
// decision (L15-L16) is that work that takes no review is delivered on the evidence of the verifier alone, as long as
// its risk is validated too. So an exemption is held to two things, which the command checks before it stores
// anything and the delivery check checks again from the stored file:
//
//   - the risk the agent declares names no item of the high-risk list of the orchestrator (CheckRisk). The engine
//     never computes a risk from a diff: it holds the declaration to a small grammar and refuses a head whose
//     declaration names any item, because a change that names one is exactly what a review is for;
//   - the report of the verifier is the object reviewverdict.ParseReport reads and it is clear (CheckReport).
//
// An exemption is evidence that a head was checked and a risk was declared. It grants nothing by itself: the check of
// the delivery is what lets a head through, and a stored result that blocks always wins over an exemption.

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
	if declared == NoRisk {
		return nil
	}
	seen := make([]bool, len(highRiskItems)+1)
	var named []string
	for _, field := range strings.Split(declared, ",") {
		item, ok := riskItem(field)
		if !ok || seen[item] {
			return fmt.Errorf("%w: %q is not %q or a list of the numbers 1 to %d of the high-risk items, separated by commas with no space and none written twice (for example %q)",
				ErrInvalidRisk, declared, NoRisk, len(highRiskItems), "2,4")
		}
		seen[item] = true
		named = append(named, fmt.Sprintf("%d (%s)", item, highRiskItems[item-1]))
	}
	return fmt.Errorf("%w: the declared risk names %s", ErrHighRisk, strings.Join(named, ", "))
}

// riskItem reads one entry of a declaration: the number of an item of the list, written as one digit.
func riskItem(field string) (int, bool) {
	if len(field) != 1 || field[0] < '1' || field[0] > byte('0'+len(highRiskItems)) {
		return 0, false
	}
	return int(field[0] - '0'), true
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
