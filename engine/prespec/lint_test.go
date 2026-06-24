package prespec

import "testing"

// TestLintAccepted verifies clean questions pass all three rules.
func TestLintAccepted(t *testing.T) {
	cases := []struct {
		name     string
		question string
	}{
		{"plain open question", "What is the main job users need to get done?"},
		{"why question", "Why is the current tool failing them?"},
		{"who question", "Who are the primary stakeholders affected?"},
		{"how-often question", "How often does this situation occur?"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := Lint(tc.question)
			if !r.Accepted {
				t.Errorf("expected accepted; got rule=%q reason=%q", r.Rule, r.Reason)
			}
		})
	}
}

// TestLintSmuggleAnswer verifies the smuggles-answer rule fires on
// leading/embedded solution language.
func TestLintSmuggleAnswer(t *testing.T) {
	cases := []struct {
		name     string
		question string
	}{
		{"would you", "Would you like to use a dashboard?"},
		{"do you want", "Do you want a notification system?"},
		{"wouldn't it be nice", "Wouldn't it be nice to have auto-sync?"},
		{"wouldn't", "Wouldn't a microservice solve this?"},
		{"would this help", "Would this feature help your team?"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := Lint(tc.question)
			if r.Accepted {
				t.Errorf("expected rejected by smuggles-answer; question=%q", tc.question)
			}
			if r.Rule != "smuggles-answer" {
				t.Errorf("expected rule=smuggles-answer; got %q", r.Rule)
			}
		})
	}
}

// TestLintPresupposesSolution verifies the presupposes-solution rule fires on
// feature/solution nouns.
func TestLintPresupposesSolution(t *testing.T) {
	cases := []struct {
		name     string
		question string
	}{
		{"feature noun", "What feature is most important?"},
		{"dashboard noun", "Should the dashboard show live metrics?"},
		{"api noun", "Does the API need versioning?"},
		{"module noun", "Which module is most critical?"},
		{"integration noun", "Is integration with Slack required?"},
		{"plugin noun", "Is a plugin the right approach?"},
		{"service noun", "Is a service the right abstraction?"},
		{"microservice noun", "Should we use a microservice?"},
		{"algorithm noun", "Which algorithm fits best?"},
		{"solution noun", "Is this solution viable?"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := Lint(tc.question)
			if r.Accepted {
				t.Errorf("expected rejected by presupposes-solution; question=%q", tc.question)
			}
			if r.Rule != "presupposes-solution" {
				t.Errorf("expected rule=presupposes-solution; got %q (question=%q)", r.Rule, tc.question)
			}
		})
	}
}

// TestLintBundlesConcerns verifies the bundles-concerns rule fires when a question
// contains " and " joining two distinct clauses.
func TestLintBundlesConcerns(t *testing.T) {
	cases := []struct {
		name     string
		question string
	}{
		{"two clauses with and", "What is the job to be done and who are the users?"},
		{"three clauses", "What is the goal and what is the constraint and who benefits?"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := Lint(tc.question)
			if r.Accepted {
				t.Errorf("expected rejected by bundles-concerns; question=%q", tc.question)
			}
			if r.Rule != "bundles-concerns" {
				t.Errorf("expected rule=bundles-concerns; got %q (question=%q)", r.Rule, tc.question)
			}
		})
	}
}

// TestLintFirstFailingRuleWins verifies smuggles-answer takes priority when
// multiple rules would match.
func TestLintFirstFailingRuleWins(t *testing.T) {
	// This matches smuggles-answer AND bundles-concerns.
	q := "Would you want a dashboard and an API?"
	r := Lint(q)
	if r.Accepted {
		t.Fatalf("expected rejected")
	}
	if r.Rule != "smuggles-answer" {
		t.Errorf("expected first rule smuggles-answer; got %q", r.Rule)
	}
}

// TestLintReasonNonEmpty verifies that a rejected result always has a non-empty reason.
func TestLintReasonNonEmpty(t *testing.T) {
	r := Lint("Would you like a feature?")
	if r.Accepted {
		t.Fatal("expected rejected")
	}
	if r.Reason == "" {
		t.Error("rejected result must have non-empty Reason")
	}
}
