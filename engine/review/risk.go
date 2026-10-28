package review

import (
	"errors"
	"fmt"
	"strings"
)

// This file is how a commit declares its risk (own-rdd T6b1, S6 a, L34 a): the git trailer "Risk: none" or
// "Risk: <items>" at the end of its message. The grammar of the declaration is the one of "review exempt --risk"
// and lives in one place, ParseRisk, which CheckRisk calls too, so the two never drift apart. The engine reads the
// declaration and never computes it (S5); a commit whose trailer fails is unknown risk and never a pass.

// ErrRiskTrailer is a commit whose message does not carry its risk as the single, well-formed "Risk:" trailer: it has
// none, has more than one, wraps it onto more than one line, or gives a value outside the grammar of ParseRisk. The
// commit is unknown risk.
var ErrRiskTrailer = errors.New("review: the commit does not declare its risk in a Risk trailer")

// riskKey is the key of the trailer, compared without regard to case as git does.
const riskKey = "Risk"

// Trailer is one trailer git parsed from the end of a message: its key as written and its value, unfolded.
type Trailer struct {
	Key   string
	Value string
}

// ParseRisk reads a declared risk: NoRisk, which gives the empty list of items, or the numbers 1 to 6 of the
// high-risk items separated by commas with no space and none written twice ("2,4"), which gives them in the order
// they were written. Anything else, including an empty declaration, "None" or a number outside the list, is
// refused with an error that wraps ErrInvalidRisk and says what is accepted.
func ParseRisk(declared string) ([]int, error) {
	if declared == NoRisk {
		return []int{}, nil
	}
	seen := make([]bool, len(highRiskItems)+1)
	var items []int
	for _, field := range strings.Split(declared, ",") {
		item, ok := riskItem(field)
		if !ok || seen[item] {
			return nil, fmt.Errorf("%w: %q is not %q or a list of the numbers 1 to %d of the high-risk items, separated by commas with no space and none written twice (for example %q)",
				ErrInvalidRisk, declared, NoRisk, len(highRiskItems), "2,4")
		}
		seen[item] = true
		items = append(items, item)
	}
	return items, nil
}

// riskItem reads one entry of a declaration: the number of an item of the list, written as one digit.
func riskItem(field string) (int, bool) {
	if len(field) != 1 || field[0] < '1' || field[0] > byte('0'+len(highRiskItems)) {
		return 0, false
	}
	return int(field[0] - '0'), true
}

// DeclaredRisk is the risk the commit declares: its raw message and the trailers git parsed from that message, as
// the Git port gives them. The declaration is the one trailer whose key is "Risk" in any case, and its value is held
// to ParseRisk. It returns the items it names, empty for NoRisk. Each failure is an error that wraps ErrRiskTrailer
// (a value outside the grammar wraps ErrInvalidRisk as well) and names the commit: no trailer, more than one, a
// trailer that git unfolded because it wraps onto more than one line of the message, and a value outside the grammar.
func DeclaredRisk(commit, message string, trailers []Trailer) ([]int, error) {
	var found []Trailer
	for _, t := range trailers {
		if strings.EqualFold(t.Key, riskKey) {
			found = append(found, t)
		}
	}
	switch {
	case len(found) == 0:
		return nil, fmt.Errorf("%w: commit %s: it has no %q trailer in the final trailer block of its message", ErrRiskTrailer, commit, riskKey+":")
	case len(found) > 1:
		return nil, fmt.Errorf("%w: commit %s: it has more than one %q trailer", ErrRiskTrailer, commit, riskKey+":")
	case !oneLine(message, found[0]):
		return nil, fmt.Errorf("%w: commit %s: its %q trailer wraps onto more than one line", ErrRiskTrailer, commit, riskKey+":")
	}
	items, err := ParseRisk(found[0].Value)
	if err != nil {
		return nil, fmt.Errorf("%w: commit %s: its %q trailer %q is not %q or a list of the numbers 1 to %d separated by commas with no space (for example %q): %w",
			ErrRiskTrailer, commit, riskKey+":", found[0].Value, NoRisk, len(highRiskItems), "2,4", err)
	}
	return items, nil
}

// oneLine says whether the trailer is one line of the raw message, which it is not when git unfolded it from several:
// some line begins with its key and a colon and holds its whole value after that.
func oneLine(message string, t Trailer) bool {
	for _, line := range strings.Split(message, "\n") {
		if len(line) <= len(t.Key) || !strings.EqualFold(line[:len(t.Key)+1], t.Key+":") {
			continue
		}
		if strings.TrimSpace(line[len(t.Key)+1:]) == t.Value {
			return true
		}
	}
	return false
}
