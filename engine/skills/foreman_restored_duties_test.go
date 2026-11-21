package skills

// T18 fixed the lint budgets of odd-feature-doc, role-chain and inception-pipeline by deleting rules; the
// review of b2575e8d required them back, with only examples and explanations moved to the references of
// each skill. One assertion per restored rule, and one per moved passage in its new home.

import (
	"strings"
	"testing"
)

// The format of a task line is a rule, not an example: the body states its parts in one compact line,
// and only the full example lines stay in the reference (review of 368afea2, F1).
func TestOddFeatureDocBodyStatesTheTaskLineFormat(t *testing.T) {
	_, _, body := oddRoleSkillFile(t, "odd-feature-doc")
	_, under := roleChainSections(body)
	for _, want := range []string{
		"`- [ ] T<n> — S# — inline|delegated — title — [shaped: L<n> — ]commit: -`",
		"box = status",
	} {
		if !strings.Contains(under["Hard Rules"], want) {
			t.Errorf("the Hard Rules must carry %q", want)
		}
	}
}

// The criteria that admit a question to the prototyper are a rule; only the example questions live in
// the reference (review of 368afea2, F3). The explanation of exit 2 lives in a reference of its own.
func TestRoleChainBodyStatesThePrototyperCriteria(t *testing.T) {
	_, _, body := roleChainSkill(t)
	_, under := roleChainSections(body)
	const want = "a premise about an installed tool, a number, or fit within the appetite"
	if !strings.Contains(under["Triggers"], want) {
		t.Errorf("the Triggers section must state the criteria of a prototyper question: %q", want)
	}
	ref := readRepoFile(t, skillsRepoRoot(t), "skills/role-chain/references/review-record.md")
	for _, frag := range []string{"`candidate`", "`paths`", "`inspection.status`", "`completed`", "any other rule of the reviewer's contract"} {
		if !strings.Contains(ref, frag) {
			t.Errorf("references/review-record.md must carry %q", frag)
		}
	}
}

func TestOddFeatureDocRestoredDuties(t *testing.T) {
	whole, _, body := oddRoleSkillFile(t, "odd-feature-doc")
	_, under := roleChainSections(body)
	_, rows := oddSkillTable(under["Decision Gates"])
	byRole := map[string][]string{}
	for _, r := range rows {
		byRole[r[0]] = r
	}
	for name, check := range map[string]func() (bool, string){
		"the shaped bullet says what L<n> is, the antecedent of 'such entry'": func() (bool, string) {
			const want = "`L<n>` is its shaper Log entry"
			return strings.Contains(under["Hard Rules"], want), want
		},
		"the task line rule names the commit field": func() (bool, string) {
			const want = "commit `-` until the task is committed"
			return strings.Contains(under["Hard Rules"], want), want
		},
		"the foreman row names the memorycard mirror": func() (bool, string) {
			const want = "memorycard mirror `odd/<feature>/tasks`"
			return strings.Contains(byRole["foreman"][1], want), want
		},
		"a blank line separates the Activation Contract from Hard Rules": func() (bool, string) {
			return strings.Contains(whole, ".\n\n## Hard Rules"), "a blank line before ## Hard Rules"
		},
		"the builder logs anything found that the task did not ask for": func() (bool, string) {
			const want = "anything found that the task did not ask for"
			return strings.Contains(byRole["builder"][1], want), want
		},
		"delivery lists the tasks [x] and not, each with its commit": func() (bool, string) {
			const want = "the tasks `[x]` and not, each with its commit"
			return strings.Contains(byRole["delivery"][1], want), want
		},
		"the foreman numbers its entries after the last entry and marks them with its role": func() (bool, string) {
			const want = "`(<date>, foreman)`, `L<n>` next after the last entry"
			return strings.Contains(byRole["foreman"][1], want), want
		},
		"the mark of a shaped task holds only while the Log has exactly one such entry": func() (bool, string) {
			const want = "holds only while the Log has exactly one such entry"
			return strings.Contains(under["Hard Rules"], want), want
		},
		"a spec holds its scope and its acceptance checks": func() (bool, string) {
			const want = "its scope, its acceptance checks"
			return strings.Contains(under["Hard Rules"], want), want
		},
		"the body links the examples of the reference": func() (bool, string) {
			const want = "`references/format.md`"
			return strings.Contains(under["Hard Rules"], want), want
		},
	} {
		t.Run(name, func(t *testing.T) {
			if ok, want := check(); !ok {
				t.Errorf("odd-feature-doc must carry %q", want)
			}
		})
	}
}

// Guards the lint budget of odd-feature-doc: the linter warns above 700 estimated tokens
// (BodyRecommendedTokens) and fails above 1000 (BodyHardTokens), engine/skills/lint.go:42-44, and this
// skill is bound to oddFeatureDocBodyBound (750). The body is all duties and formats, so the next
// addition that would pass that bound must move text to references/format.md, never reword a rule.
func TestOddFeatureDocBodyStaysWithinItsLintBound(t *testing.T) {
	_, _, body := oddRoleSkillFile(t, "odd-feature-doc")
	if n := EstimateBodyTokens(body); n > oddFeatureDocBodyBound {
		t.Errorf("the body is an estimated %d tokens, want at most %d", n, oddFeatureDocBodyBound)
	}
}

// Only the token-count finding of `body-recommended` is skipped for odd-feature-doc: another finding of
// that rule, or the token finding of another rule, is not.
func TestOnlyTheTokenCountRecommendedWarningIsSkipped(t *testing.T) {
	for _, tc := range []struct {
		w    Warning
		want bool
	}{
		{Warning{"body-recommended", "body is an estimated 715 tokens, exceeds the recommended budget of 700"}, true},
		{Warning{"body-recommended", "body has no examples"}, false},
		{Warning{"other-rule", "body is an estimated 715 tokens, exceeds the recommended budget of 700"}, false},
	} {
		if got := isRecommendedTokenWarning(tc.w); got != tc.want {
			t.Errorf("isRecommendedTokenWarning(%+v) = %v, want %v", tc.w, got, tc.want)
		}
	}
}

// The raised bound of odd-feature-doc stays above the linter's recommended warning and under its hard
// error, so it neither falls back to the default nor hides a hard failure.
func TestOddFeatureDocBodyBoundLiesBetweenTheLinterBounds(t *testing.T) {
	if oddFeatureDocBodyBound <= BodyRecommendedTokens {
		t.Errorf("the bound %d is not above the recommended %d: the raise is moot", oddFeatureDocBodyBound, BodyRecommendedTokens)
	}
	if oddFeatureDocBodyBound >= BodyHardTokens {
		t.Errorf("the bound %d reaches the hard error at %d", oddFeatureDocBodyBound, BodyHardTokens)
	}
}

// The overview that moved out of the odd-feature-doc Activation Contract (who writes, works from and
// closes the document) is in its reference.
func TestOddFeatureDocReferenceHoldsTheOverview(t *testing.T) {
	ref := readRepoFile(t, skillsRepoRoot(t), "skills/odd-feature-doc/references/format.md")
	const want = "The shaper writes it, the builder works from it, delivery closes it, the foreman tracks it."
	if !strings.Contains(ref, want) {
		t.Errorf("references/format.md must carry the overview %q", want)
	}
}

// The examples and explanations moved out of the odd-feature-doc body are in its reference.
func TestOddFeatureDocReferenceHoldsTheMovedExamples(t *testing.T) {
	ref := readRepoFile(t, skillsRepoRoot(t), "skills/odd-feature-doc/references/format.md")
	for _, want := range []string{
		"`- [ ] T1 — S1-S2 — inline — what it delivers — commit: -`",
		"`- [ ] T1 — S1 — delegated — what it delivers — shaped: L2 — commit: -`",
		"The box is the status, the route is `inline` or `delegated`, and the commit is `-` until the task is committed.",
		"holds a user correction verbatim, a decision or evidence",
		"Two or three lines: the objective, the branch, the delivery strategy, the test runner.",
		"`- **L7** (2026-11-16, foreman):",
	} {
		if !strings.Contains(ref, want) {
			t.Errorf("references/format.md must carry %q", want)
		}
	}
}

func TestRoleChainRestoredDuties(t *testing.T) {
	_, _, body := roleChainSkill(t)
	_, under := roleChainSections(body)
	for _, want := range []string{
		"`reviewer → builder` (rework), the file as ledger.",
		"Delivery recomputes `diff_sha256` and trusts the record only on equality.",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(under["Binding"], want) {
				t.Errorf("the Binding section must carry %q", want)
			}
		})
	}
	if !strings.Contains(under["Triggers"], "`references/prototyper.md`") {
		t.Error("the Triggers section must link references/prototyper.md")
	}
	ref := readRepoFile(t, skillsRepoRoot(t), "skills/role-chain/references/prototyper.md")
	for _, want := range []string{"does installed tool X behave as premise P says", "what is number N", "can X be done within the appetite"} {
		if !strings.Contains(ref, want) {
			t.Errorf("references/prototyper.md must carry %q", want)
		}
	}
}

func TestInceptionPipelineRestoredDuties(t *testing.T) {
	skill := readRepoFile(t, actualsInstrumentationRepoRoot(t), inceptionPipelineSkillRelPath)
	section := markdownSectionBody(t, skill, "## Chain entry")
	for _, want := range []string{
		"`prev_sha256` has 64 zeros",
		"Builder records progress in its Log and delivery adds its final Log entry in place",
		"`progress` and `close-report` are handoff kinds, not files",
		"`references/chain-entry.md`",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(section, want) {
				t.Errorf("the Chain entry section must carry %q", want)
			}
		})
	}
	ref := readRepoFile(t, actualsInstrumentationRepoRoot(t), "skills/inception-pipeline/references/chain-entry.md")
	for _, want := range []string{`{"version":1,"project_id":"acme-shop"`, `"payload_kind":"feature-doc"`, `"status":"completed"}`} {
		if !strings.Contains(ref, want) {
			t.Errorf("references/chain-entry.md must carry %q", want)
		}
	}
}
