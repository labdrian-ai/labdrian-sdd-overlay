package archguard

import (
	"fmt"
	"regexp"
	"sort"
)

// debtLine is one known violation, owed to the work unit that removes it.
type debtLine struct {
	from   string // the package directory
	target string // as violation.target
	unit   string // the work unit id
}

func (d debtLine) edge() string { return d.from + " -> " + d.target }

// lines flattens the debt into lines sorted by package, then target.
func (d Debt) lines() []debtLine {
	var out []debtLine
	for from, targets := range d {
		for target, unit := range targets {
			out = append(out, debtLine{from: from, target: target, unit: unit})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].edge() < out[j].edge() })
	return out
}

// reconcile splits the violations found into those with no debt line (new, and a
// failure) and returns the debt lines whose violation no longer exists (stale,
// also a failure).
func reconcile(found []violation, lines []debtLine) (unlisted []violation, stale []debtLine) {
	listed := map[string]bool{}
	for _, d := range lines {
		listed[d.edge()] = true
	}
	live := map[string]bool{}
	for _, v := range found {
		live[v.edge()] = true
		if !listed[v.edge()] {
			unlisted = append(unlisted, v)
		}
	}
	for _, d := range lines {
		if !live[d.edge()] {
			stale = append(stale, d)
		}
	}
	return unlisted, stale
}

var workUnitID = regexp.MustCompile(`^[HLTB][0-9]+$`)

// validateDebts returns a message for every debt line that cannot be trusted: no
// target, no work unit, a package without a ring, or a package whose ring has no
// rule to owe against.
func validateDebts(lines []debtLine, rings map[string]Ring) []string {
	var problems []string
	for _, d := range lines {
		r, declared := rings[d.from]
		switch {
		case d.target == "":
			problems = append(problems, fmt.Sprintf("known debt of %s has an empty target", d.from))
		case !workUnitID.MatchString(d.unit):
			problems = append(problems, fmt.Sprintf("known debt %s: unit %q is not a work unit id such as H4 or L3", d.edge(), d.unit))
		case !declared:
			problems = append(problems, fmt.Sprintf("known debt %s: %s is not declared in rings", d.edge(), d.from))
		case !r.pure():
			problems = append(problems, fmt.Sprintf("known debt %s: only a domain or application package can owe debt, and %s is %s", d.edge(), d.from, r))
		}
	}
	return problems
}
