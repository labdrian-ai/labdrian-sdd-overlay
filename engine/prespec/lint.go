package prespec

import (
	"regexp"
	"strings"
)

// LintResult is the outcome of a lint check on a proposed interview question.
type LintResult struct {
	// Accepted is true when the question passes all rules.
	Accepted bool
	// Rule is the name of the first failing rule, empty when Accepted is true.
	Rule string
	// Reason is a human-readable explanation, empty when Accepted is true.
	Reason string
}

// lintRule pairs a compiled regexp with the rule name and rejection reason.
type lintRule struct {
	name   string
	re     *regexp.Regexp
	reason string
}

// lintRules are evaluated in order; the first match wins (R-007).
// Rule 1: smuggles-answer — question presupposes the user wants a specific thing.
// Rule 2: presupposes-solution — question names implementation artifacts.
// Rule 3: bundles-concerns — question joins two independent clauses with " and ".
var lintRules = []lintRule{
	{
		name:   "smuggles-answer",
		re:     regexp.MustCompile(`(?i)\bwould(n't| not)?\b|\bdo you want\b`),
		reason: "question steers toward a solution ('would you', 'do you want', 'wouldn't it be nice'); ask what the user needs, not whether they want a specific thing",
	},
	{
		name: "presupposes-solution",
		// minimal: rung 6 — the vocabulary list is the specification; no lower rung can enumerate it.
		re: regexp.MustCompile(
			`(?i)\b(feature|dashboard|api|module|integration|plugin|service|microservice|algorithm|solution)\b`,
		),
		reason: "question names an implementation artifact; ask about the problem or outcome instead",
	},
	{
		name:   "bundles-concerns",
		re:     regexp.MustCompile(`(?i)\band\b`),
		reason: "question bundles multiple concerns with 'and'; ask one thing at a time",
	},
}

// Lint checks whether question is a valid Socratic probe (R-007).
// It applies three deterministic regex rules in order and returns the first failure.
// The check is case-insensitive and never makes an LLM call.
func Lint(question string) LintResult {
	q := strings.TrimSpace(question)
	for _, rule := range lintRules {
		if rule.re.MatchString(q) {
			return LintResult{Accepted: false, Rule: rule.name, Reason: rule.reason}
		}
	}
	return LintResult{Accepted: true}
}
