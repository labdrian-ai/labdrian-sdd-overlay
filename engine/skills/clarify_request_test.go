package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// The skill clarify-request replaces prespec-malandra (odd/tasks/clarify-request.md, S1-S7). A skill
// is a runtime instruction contract for a model, so what a test can hold of it is the text of its
// rules: these are prose pins, in the manner of inception_chain_entry_test.go. Each pin judges the
// text it is given and returns the ways that text breaks its rule, so the same pin runs twice: over
// the real skill, where it must hold, and over a copy with that rule removed, where it must fail
// (TestClarifyRequestPinsAreLoadBearing), or the pin does not hold the rule it names.

const (
	clarifyRequestID           = "clarify-request"
	clarifyRequestSkillRelPath = "skills/clarify-request/SKILL.md"
	prespecMalandraID          = "prespec-malandra"
	prespecMalandraDirRelPath  = "skills/prespec-malandra"
)

// TestClarifyRequestReplacesPrespecMalandra pins the rename (S1) and the deletion of the grid's
// reference (S4): the skill, the registry, the manifest, the approval baseline and the help text
// all stop naming prespec-malandra and name clarify-request.
func TestClarifyRequestReplacesPrespecMalandra(t *testing.T) {
	root := actualsInstrumentationRepoRoot(t)

	t.Run("S1_the_skill_file_is_named_clarify_request_with_a_valid_frontmatter", func(t *testing.T) {
		data := readRepoFile(t, root, clarifyRequestSkillRelPath)
		frontmatter, _, err := SplitSkillFile([]byte(data))
		if err != nil {
			t.Fatalf("splitting %s: %v", clarifyRequestSkillRelPath, err)
		}
		fm := parseFrontmatterFields(frontmatter)
		if fm.Name != clarifyRequestID {
			t.Errorf("name = %q, want %q", fm.Name, clarifyRequestID)
		}
		if fm.License != "Apache-2.0" {
			t.Errorf("license = %q, want Apache-2.0", fm.License)
		}
		if fm.MetaAuthor == "" {
			t.Error("metadata.author is empty")
		}
		if fm.MetaVersion != "2.0" {
			t.Errorf("metadata.version = %q, want %q: the rewrite is a new major version of the skill", fm.MetaVersion, "2.0")
		}
		for _, want := range []string{"Trigger:", "explore an idea", "clarify a request", "prepare requirements", "understand a problem"} {
			if !strings.Contains(fm.Description, want) {
				t.Errorf("the description must name the trigger %q, got %q", want, fm.Description)
			}
		}
		for _, stale := range []string{"prespec", "Socratic", "grid"} {
			if strings.Contains(fm.Description, stale) {
				t.Errorf("the description still carries %q of the replaced skill: %q", stale, fm.Description)
			}
		}
		hard, warnings := LintSkillFile([]byte(data))
		for _, e := range hard {
			t.Errorf("hard lint finding: %v", e)
		}
		for _, w := range warnings {
			// Advisory: the skill may grow to the hard budget, which is the finding above.
			t.Logf("lint warning [%s]: %s", w.Rule, w.Msg)
		}
	})

	t.Run("S1_the_prespec_malandra_skill_directory_is_gone", func(t *testing.T) {
		if _, err := os.Stat(filepath.Join(root, prespecMalandraDirRelPath)); !os.IsNotExist(err) {
			t.Errorf("%s is still there (Stat err = %v)", prespecMalandraDirRelPath, err)
		}
	})

	t.Run("S1_the_registry_names_the_skill_by_the_new_id_and_path", func(t *testing.T) {
		registry := readRepoFile(t, root, "skills.registry.yaml")
		entry := regexp.MustCompile(`(?m)^\s*- id:\s*clarify-request\s*\n\s*path:\s*clarify-request\s*$`)
		if n := len(entry.FindAllString(registry, -1)); n != 1 {
			t.Errorf("skills.registry.yaml has %d entries with the id and the path %s, want 1", n, clarifyRequestID)
		}
		if strings.Contains(registry, prespecMalandraID) {
			t.Errorf("skills.registry.yaml still names %s", prespecMalandraID)
		}
	})

	t.Run("S1_the_manifest_has_the_new_skill_row_and_no_row_of_the_old_skill", func(t *testing.T) {
		want := ManifestRow{Path: clarifyRequestID + "/SKILL.md", Tag: "custom"}
		newRows := 0
		for _, row := range ReadManifestRows([]byte(readRepoFile(t, root, "overlay.manifest"))) {
			switch {
			case row == want:
				newRows++
			case strings.HasPrefix(row.Path, prespecMalandraID+"/"):
				t.Errorf("the manifest still has the row %+v of the replaced skill %s", row, prespecMalandraID)
			case strings.Contains(row.Path, "coverage-taxonomy"):
				t.Errorf("the manifest still has the row %+v of the deleted coverage taxonomy", row)
			}
		}
		if newRows != 1 {
			t.Errorf("the manifest has %d rows %+v, want 1", newRows, want)
		}
	})

	t.Run("S1_the_approval_baseline_drops_the_old_entry_and_adds_none", func(t *testing.T) {
		for _, entry := range ApprovalBaseline() {
			switch entry.ID {
			case prespecMalandraID:
				t.Errorf("the approval baseline still grandfathers the replaced skill %s", prespecMalandraID)
			case clarifyRequestID:
				t.Errorf("the approval baseline grandfathers %s: it needs an owner approval record (S8), and a baseline entry would exempt it", clarifyRequestID)
			}
		}
	})

	t.Run("S1_the_help_and_comment_text_say_no_skill_calls_the_prespec_verbs", func(t *testing.T) {
		texts := map[string]string{
			"bin/lib/usage.sh":        clarifyRequestUsageEntry(readRepoFile(t, root, "bin/lib/usage.sh")),
			"bin/lib/engine_verbs.sh": clarifyRequestCommentAbove(readRepoFile(t, root, "bin/lib/engine_verbs.sh"), "cmd_prespec()"),
		}
		for path, text := range texts {
			if text == "" {
				t.Errorf("%s: the text about the prespec verbs was not found", path)
				continue
			}
			for _, want := range []string{"no skill calls", clarifyRequestID} {
				if !strings.Contains(strings.ToLower(text), strings.ToLower(want)) {
					t.Errorf("%s: the text about the prespec verbs must say %q, got %q", path, want, text)
				}
			}
			if strings.Contains(text, prespecMalandraID+" skill") {
				t.Errorf("%s still says the %s skill uses the prespec verbs: %q", path, prespecMalandraID, text)
			}
		}
	})

	t.Run("S4_the_coverage_taxonomy_reference_is_deleted", func(t *testing.T) {
		for _, rel := range []string{
			prespecMalandraDirRelPath + "/references/coverage-taxonomy.md",
			"skills/clarify-request/references/coverage-taxonomy.md",
		} {
			if _, err := os.Stat(filepath.Join(root, rel)); !os.IsNotExist(err) {
				t.Errorf("%s is still there (Stat err = %v): the 10-cell grid it describes is replaced by the hidden checklist", rel, err)
			}
		}
	})
}

// TestClarifyRequestSkillKeepsTheSpecifiedRules runs every pin over the real skill (S2-S7).
func TestClarifyRequestSkillKeepsTheSpecifiedRules(t *testing.T) {
	root := actualsInstrumentationRepoRoot(t)
	for _, rule := range clarifyRequestRules {
		t.Run(rule.name, func(t *testing.T) {
			skill := readRepoFile(t, root, clarifyRequestSkillRelPath)
			for _, problem := range rule.pin(skill) {
				t.Errorf("%s: %s", clarifyRequestSkillRelPath, problem)
			}
		})
	}
}

// TestClarifyRequestPinsAreLoadBearing removes or inverts one rule at a time in a copy of the
// skill and requires the pin of that rule to fail on the copy. A pin that still passes does not
// hold its rule: the rule could be deleted from the skill and the suite would stay green. Every
// rule has at least one mutant, and a mutant whose target text is gone from the skill proves
// nothing, so it fails instead of passing.
func TestClarifyRequestPinsAreLoadBearing(t *testing.T) {
	root := actualsInstrumentationRepoRoot(t)

	t.Run("every_rule_has_a_mutant", func(t *testing.T) {
		covered := map[string]bool{}
		for _, m := range clarifyRequestMutants {
			covered[m.rule] = true
		}
		known := map[string]bool{}
		for _, rule := range clarifyRequestRules {
			known[rule.name] = true
			if !covered[rule.name] {
				t.Errorf("the rule %s has no mutant, so nothing shows its pin is load-bearing", rule.name)
			}
		}
		for _, m := range clarifyRequestMutants {
			if !known[m.rule] {
				t.Errorf("the mutant %q targets %q, which is no rule", m.what, m.rule)
			}
		}
	})

	for _, m := range clarifyRequestMutants {
		what := strings.ReplaceAll(m.what, "_", " ")
		t.Run(m.rule+"_fails_when_"+m.what, func(t *testing.T) {
			skill := readRepoFile(t, root, clarifyRequestSkillRelPath)
			var pin clarifyRequestPin
			for _, rule := range clarifyRequestRules {
				if rule.name == m.rule {
					pin = rule.pin
				}
			}
			if pin == nil {
				t.Fatalf("no rule is named %q", m.rule)
			}
			if problems := pin(skill); len(problems) > 0 {
				t.Fatalf("the skill itself already breaks %s: %v", m.rule, problems)
			}
			mutated := m.mutate(skill)
			if mutated == skill {
				t.Fatalf("the mutation (%s) left the skill unchanged: its target text is gone, so it proves nothing", what)
			}
			problems := pin(mutated)
			if len(problems) == 0 {
				t.Fatalf("the pin of %s still passes after %s: it does not hold the rule", m.rule, what)
			}
			if all := strings.Join(problems, "\n"); !strings.Contains(all, m.expect) {
				t.Errorf("the pin of %s fails after %s, but not for the rule that was removed: want a message with %q, got:\n%s", m.rule, what, m.expect, all)
			}
		})
	}
}

// clarifyRequestPin judges the text of the skill against one rule and returns a message for each
// way the text breaks it; no message means the text keeps the rule.
type clarifyRequestPin func(skill string) []string

type clarifyRequestRule struct {
	name string
	pin  clarifyRequestPin
}

// clarifyRequestRules are the rules S2-S7 as pins, in the order of the spec.
var clarifyRequestRules = []clarifyRequestRule{
	{"S2_the_skill_runs_inline_in_the_parent_conversation", clarifyRequestInlinePin},
	{"S3_the_loop_is_five_numbered_steps", clarifyRequestLoopShapePin},
	{"S3_step_1_grounds_before_asking_anything", clarifyRequestStepPin(1,
		"Ground before asking anything",
		"read the request",
		"`mem_search` for the project and topic",
		"the project manifest when one exists",
		"the files the user named")},
	{"S3_step_2_restates_in_3_to_6_lines_tagged_said_or_inferred", clarifyRequestStepPin(2,
		"Restate in 3-6 lines",
		"`said` (the user's words)",
		"`inferred` (from the repository or memory, naming the source)")},
	{"S3_step_3_asks_only_blocking_gaps_one_quoted_question_at_a_time", clarifyRequestStepPin(3,
		"Ask only the gaps that block a decision",
		"one question per message",
		"quoting the user's words or a repository artifact",
		"ask nothing")},
	{"S3_step_4_updates_the_restatement_and_never_asks_again", clarifyRequestStepPin(4,
		"Update the restatement with each answer",
		"Never ask again about something already answered or already found in the grounding")},
	{"S3_step_5_stops_hard_at_3_questions", clarifyRequestStepPin(5,
		"Hard stop at 3 questions in total",
		"record any open gap as an assumption",
		"stop asking")},
	{"S3_the_worked_example_follows_the_rules_of_the_loop", clarifyRequestExamplePin},
	{"S3_the_wording_is_plain_and_in_the_users_language", clarifyRequestTonePin},
	{"S4_a_hidden_checklist_of_six_items_replaces_the_grid", clarifyRequestChecklistPin},
	{"S5_mom_test_is_for_a_third_partys_problem_only", clarifyRequestMomTestPin},
	{"S6_none_of_the_removed_mechanisms_is_left", clarifyRequestRemovedPin},
	{"S7_the_brief_is_saved_on_confirmation_under_the_clarify_topic", clarifyRequestBriefPin},
	{"S7_the_skill_derives_no_change_name_and_hands_off", clarifyRequestHandoffPin},
}

// clarifyRequestMutant is one rule removed or inverted in a copy of the skill. The pin of the rule
// must fail on the copy, and one of its messages must contain expect, so that the pin fails for
// the rule the mutation removed and not by accident.
type clarifyRequestMutant struct {
	rule   string
	what   string
	mutate func(skill string) string
	expect string
}

// clarifyRequestReplace is the mutation that replaces every occurrence of old.
func clarifyRequestReplace(old, replacement string) func(string) string {
	return func(skill string) string { return strings.ReplaceAll(skill, old, replacement) }
}

var clarifyRequestMutants = []clarifyRequestMutant{
	{"S2_the_skill_runs_inline_in_the_parent_conversation", "framing_the_skill_as_a_sub_agent",
		clarifyRequestReplace("## Activation Contract\n", "You are a sub-agent. Do not delegate further.\n\n## Activation Contract\n"),
		`must not say "sub-agent"`},
	{"S3_the_loop_is_five_numbered_steps", "adding_a_sixth_step",
		clarifyRequestReplace("## Output Contract", "6. Ask one more question.\n\n## Output Contract"),
		"exactly 5 numbered steps"},
	{"S3_step_1_grounds_before_asking_anything", "dropping_the_memory_search",
		clarifyRequestReplace("`mem_search` for the project and topic, ", ""),
		"`mem_search` for the project and topic"},
	{"S3_step_2_restates_in_3_to_6_lines_tagged_said_or_inferred", "lifting_the_line_bound",
		clarifyRequestReplace("Restate in 3-6 lines", "Restate"),
		"Restate in 3-6 lines"},
	{"S3_step_3_asks_only_blocking_gaps_one_quoted_question_at_a_time", "dropping_the_one_question_rule",
		clarifyRequestReplace(", one question per message", ""),
		"one question per message"},
	{"S3_step_4_updates_the_restatement_and_never_asks_again", "dropping_the_never_ask_again_rule",
		clarifyRequestReplace("Never ask again about something already answered or already found in the grounding.", ""),
		"Never ask again about something already answered"},
	{"S3_step_5_stops_hard_at_3_questions", "raising_the_cap_to_5",
		clarifyRequestReplace("Hard stop at 3 questions in total", "Hard stop at 5 questions in total"),
		"Hard stop at 3 questions in total"},
	{"S3_the_worked_example_follows_the_rules_of_the_loop", "untagging_an_example_line",
		clarifyRequestReplace("- inferred: no time target is recorded (mem_search found none)", "- no time target is recorded"),
		"must be tagged"},
	{"S3_the_worked_example_follows_the_rules_of_the_loop", "asking_about_words_nobody_said",
		clarifyRequestReplace("Question: you said \"the export is too slow\":", "Question: you said \"it is slow\":"),
		"which no `said` line has"},
	{"S3_the_wording_is_plain_and_in_the_users_language", "dropping_the_therapist_rule",
		clarifyRequestReplace("no therapist phrasing, ", ""),
		"no therapist phrasing"},
	{"S3_the_wording_is_plain_and_in_the_users_language", "dropping_the_feelings_question_ban",
		clarifyRequestReplace("Never ask how the user feels about it or what deeper need lies behind it. ", ""),
		"Never ask how the user feels"},
	{"S4_a_hidden_checklist_of_six_items_replaces_the_grid", "treating_every_empty_item_as_a_gap",
		clarifyRequestReplace("An item is a gap only when its answer would change what gets built; otherwise mark it `n/a` or record it as an assumption.", ""),
		"An item is a gap only when its answer would change what gets built"},
	{"S4_a_hidden_checklist_of_six_items_replaces_the_grid", "showing_the_checklist_as_a_score",
		clarifyRequestReplace("never shown to the user as a grid or a score", "shown to the user as a score"),
		"never shown to the user as a grid or a score"},
	{"S5_mom_test_is_for_a_third_partys_problem_only", "using_mom_test_on_the_users_own_request",
		clarifyRequestReplace("never use Mom-Test framing", "use Mom-Test framing"),
		`must say "never"`},
	{"S6_none_of_the_removed_mechanisms_is_left", "calling_a_prespec_verb",
		clarifyRequestReplace("## Decision Gates", "Run `labdrian prespec rank` to pick the next question.\n\n## Decision Gates"),
		`must not say "prespec"`},
	{"S7_the_brief_is_saved_on_confirmation_under_the_clarify_topic", "saving_under_sdd",
		clarifyRequestReplace(`topic_key: "project/{project}/clarify/`, `topic_key: "sdd/{change}/clarify/`),
		"must be under project/{project}/clarify/"},
	{"S7_the_brief_is_saved_on_confirmation_under_the_clarify_topic", "capturing_the_prompt",
		clarifyRequestReplace("capture_prompt: false", "capture_prompt: true"),
		"capture_prompt: false"},
	{"S7_the_skill_derives_no_change_name_and_hands_off", "deriving_a_change_name",
		clarifyRequestReplace("Never derive a change-name", "Derive a change-name"),
		"Never derive a change-name"},
}

// clarifyRequestSay returns a message for each phrase the text lacks, ignoring case.
func clarifyRequestSay(text string, phrases ...string) []string {
	lower := strings.ToLower(text)
	var problems []string
	for _, phrase := range phrases {
		if !strings.Contains(lower, strings.ToLower(phrase)) {
			problems = append(problems, fmt.Sprintf("must say %q", phrase))
		}
	}
	return problems
}

// clarifyRequestNeverSay returns a message for each phrase the text has, ignoring case.
func clarifyRequestNeverSay(text string, phrases ...string) []string {
	lower := strings.ToLower(text)
	var problems []string
	for _, phrase := range phrases {
		if strings.Contains(lower, strings.ToLower(phrase)) {
			problems = append(problems, fmt.Sprintf("must not say %q", phrase))
		}
	}
	return problems
}

// clarifyRequestSection is the body of the section with the exact heading, or a message when the
// skill has no such section.
func clarifyRequestSection(skill, heading string) (string, []string) {
	body, err := sliceMarkdownSection(skill, heading)
	if err != nil {
		return "", []string{fmt.Sprintf("must have the section %q (%v)", heading, err)}
	}
	return body, nil
}

// clarifyRequestLineWith is the first line of the text that has the marker, ignoring case.
func clarifyRequestLineWith(text, marker string) string {
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(strings.ToLower(line), strings.ToLower(marker)) {
			return line
		}
	}
	return ""
}

var clarifyRequestStepLine = regexp.MustCompile(`^(\d+)\. (.*)$`)

// clarifyRequestSteps are the numbered steps of the Execution Steps section, by number, each with
// the lines that continue it up to the next blank line.
func clarifyRequestSteps(skill string) (map[int]string, []string) {
	section, problems := clarifyRequestSection(skill, "## Execution Steps")
	steps := map[int]string{}
	open := 0
	for _, line := range strings.Split(section, "\n") {
		if m := clarifyRequestStepLine.FindStringSubmatch(line); m != nil {
			n, _ := strconv.Atoi(m[1])
			steps[n], open = m[2], n
			continue
		}
		if text := strings.TrimSpace(line); text != "" && open != 0 {
			steps[open] += " " + text
		} else {
			open = 0
		}
	}
	return steps, problems
}

// clarifyRequestStepPin is the pin of one step of the loop: the step must exist and say the phrases.
func clarifyRequestStepPin(n int, phrases ...string) clarifyRequestPin {
	return func(skill string) []string {
		steps, problems := clarifyRequestSteps(skill)
		step, ok := steps[n]
		if !ok {
			problems = append(problems, fmt.Sprintf("must have step %d in the Execution Steps section", n))
		}
		return append(problems, clarifyRequestSay(step, phrases...)...)
	}
}

// S2: the skill runs inline, in the conversation with the user, and carries no sub-agent framing.
func clarifyRequestInlinePin(skill string) []string {
	activation, problems := clarifyRequestSection(skill, "## Activation Contract")
	problems = append(problems, clarifyRequestSay(activation, "Run inline", "parent conversation")...)
	return append(problems, clarifyRequestNeverSay(skill, "sub-agent", "subagent", "do not delegate")...)
}

// S3: the loop is the five numbered steps, and no sixth step comes after the hard stop.
func clarifyRequestLoopShapePin(skill string) []string {
	steps, problems := clarifyRequestSteps(skill)
	for n := 1; n <= 5; n++ {
		if _, ok := steps[n]; !ok {
			problems = append(problems, fmt.Sprintf("must have step %d", n))
		}
	}
	if len(steps) != 5 {
		problems = append(problems, fmt.Sprintf("the loop must be exactly 5 numbered steps, got %d", len(steps)))
	}
	return problems
}

var (
	clarifyRequestSaidLine     = regexp.MustCompile(`^- said: (.+)$`)
	clarifyRequestInferredLine = regexp.MustCompile(`^- inferred: .+ \([^()]+\)$`)
	clarifyRequestQuoted       = regexp.MustCompile(`"([^"]+)"`)
)

// S3: the worked example restates in 3-6 lines, each tagged `said`, or `inferred` with its source,
// and its question quotes words that a `said` line has.
func clarifyRequestExamplePin(skill string) []string {
	section, problems := clarifyRequestSection(skill, "## Execution Steps")
	var block, after []string
	inFence, closed := false, false
	for _, line := range strings.Split(section, "\n") {
		text := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(text, "```"):
			if inFence {
				inFence, closed = false, true
			} else if !closed {
				inFence = true
			}
		case inFence && text != "":
			block = append(block, text)
		case closed && text != "":
			after = append(after, text)
		}
	}
	if len(block) < 3 || len(block) > 6 {
		problems = append(problems, fmt.Sprintf("the worked example must restate in 3-6 lines, got %d", len(block)))
	}
	var said []string
	inferred := 0
	for _, line := range block {
		switch {
		case clarifyRequestSaidLine.MatchString(line):
			said = append(said, line)
		case clarifyRequestInferredLine.MatchString(line):
			inferred++
		default:
			problems = append(problems, fmt.Sprintf("the example line %q must be tagged `said`, or `inferred` with its source in parentheses", line))
		}
	}
	if len(said) == 0 || inferred == 0 {
		problems = append(problems, "the worked example must show at least one `said` line and one `inferred` line")
	}
	question := ""
	for _, line := range after {
		if strings.HasPrefix(line, "Question:") {
			question = line
			break
		}
	}
	quoted := clarifyRequestQuoted.FindStringSubmatch(question)
	switch {
	case quoted == nil:
		problems = append(problems, "the worked example must show a `Question:` line that quotes the user's words")
	case !strings.Contains(strings.Join(said, "\n"), quoted[1]):
		problems = append(problems, fmt.Sprintf("the example question quotes %q, which no `said` line has", quoted[1]))
	}
	return problems
}

// S3: the owner's complaint about the old skill is the tone: plain wording, about the user's own
// request, in the user's language.
func clarifyRequestTonePin(skill string) []string {
	activation, problems := clarifyRequestSection(skill, "## Activation Contract")
	rules, more := clarifyRequestSection(skill, "## Hard Rules")
	problems = append(problems, more...)
	problems = append(problems, clarifyRequestSay(activation, "the user's actual request", "in the user's language")...)
	return append(problems, clarifyRequestSay(rules, "no therapist phrasing", "Never ask how the user feels about it or what deeper need lies behind it")...)
}

// S4: six items, each may be n/a, never ranked, never shown as a grid or a score; the ten cells of
// the old grid are gone.
func clarifyRequestChecklistPin(skill string) []string {
	rules, problems := clarifyRequestSection(skill, "## Hard Rules")
	problems = append(problems, clarifyRequestSay(rules,
		"hidden checklist of 6 items",
		"goal, current gap, constraints, done-when, alternatives, risks",
		"`n/a`",
		"not ranked",
		"never shown to the user as a grid or a score",
		"An item is a gap only when its answer would change what gets built")...)
	return append(problems, clarifyRequestNeverSay(skill,
		"10-cell", "jtbd", "current-gap", "why-now", "user-segment", "success-metric", "risk-unknowns",
		"DefaultCells", "coverage-taxonomy")...)
}

// S5: Mom-Test framing is the row of a third party's problem, is refused for the user's own
// request, and is not part of the loop every request goes through.
func clarifyRequestMomTestPin(skill string) []string {
	gates, problems := clarifyRequestSection(skill, "## Decision Gates")
	steps, more := clarifyRequestSection(skill, "## Execution Steps")
	problems = append(problems, more...)
	own := clarifyRequestLineWith(gates, "own request")
	third := clarifyRequestLineWith(gates, "third party")
	if own == "" || third == "" {
		problems = append(problems, "Decision Gates must have a row for the user's own request and a row for a third party's problem")
	}
	problems = append(problems, clarifyRequestSay(own, "never", "Mom-Test")...)
	problems = append(problems, clarifyRequestSay(third, "Mom-Test", "past behavior", "not hypotheticals", "a client, a user, a stakeholder")...)
	return append(problems, clarifyRequestNeverSay(steps, "Mom-Test")...)
}

// S6: the skill says it has no menu of business archetypes and no ban on the repository's own
// words, and keeps nothing of the engine verbs, the bounce, the menu, the question lint or the
// readiness gate of the old skill.
func clarifyRequestRemovedPin(skill string) []string {
	rules, problems := clarifyRequestSection(skill, "## Hard Rules")
	problems = append(problems, clarifyRequestSay(rules,
		"no menu of business archetypes",
		"the repository's own terms (API, module, plugin)")...)
	return append(problems, clarifyRequestNeverSay(skill,
		"prespec", "readiness", "needs-more-input", "anti-solution", "That sounds like a solution",
		"MCQ", "smuggles-answer", "presupposes-solution", "bundles-concerns")...)
}

var (
	clarifyRequestSaveType = regexp.MustCompile(`type:\s*"?architecture"?`)
	clarifyRequestTopicKey = regexp.MustCompile(`topic_key:\s*"([^"]*)"`)
)

// S7: the brief has its four parts, is saved only once the user confirms it, with mem_save under
// the clarify topic, as an architecture record, without capturing the prompt; the one topic the
// skill names is a clarify topic.
func clarifyRequestBriefPin(skill string) []string {
	contract, problems := clarifyRequestSection(skill, "## Output Contract")
	problems = append(problems, clarifyRequestSay(contract,
		"the final restatement",
		"the checklist (with `n/a` items)",
		"the questions asked with their answers",
		"the assumptions",
		"When the user confirms it",
		"mem_save(",
		"project/{project}/clarify/{YYYYMMDD-HHMM}",
		"capture_prompt: false")...)
	if !clarifyRequestSaveType.MatchString(contract) {
		problems = append(problems, "the save must be `type: architecture`")
	}
	if confirm, save := strings.Index(contract, "confirms"), strings.Index(contract, "mem_save("); confirm < 0 || save < 0 || confirm > save {
		problems = append(problems, "the brief must be saved only after the user confirms it")
	}
	topics := clarifyRequestTopicKey.FindAllStringSubmatch(skill, -1)
	if len(topics) == 0 {
		problems = append(problems, "the save must name a topic_key")
	}
	for _, topic := range topics {
		if !strings.HasPrefix(topic[1], "project/{project}/clarify/") {
			problems = append(problems, fmt.Sprintf("the topic_key %q must be under project/{project}/clarify/", topic[1]))
		}
	}
	return problems
}

// S7: the skill never derives a change-name or saves under sdd/, and leaves the change-name and
// the EARS requirements to requirements-from-transcripts.
func clarifyRequestHandoffPin(skill string) []string {
	rules, problems := clarifyRequestSection(skill, "## Hard Rules")
	contract, more := clarifyRequestSection(skill, "## Output Contract")
	problems = append(problems, more...)
	problems = append(problems, clarifyRequestSay(rules, "Never derive a change-name", "`sdd/`")...)
	problems = append(problems, clarifyRequestSay(contract, "`requirements-from-transcripts`", "the change-name", "the EARS requirements")...)
	return append(problems, clarifyRequestNeverSay(skill, "kebab")...)
}

// clarifyRequestCollapse joins lines into one line with single spaces.
func clarifyRequestCollapse(lines []string) string {
	return strings.Join(strings.Fields(strings.Join(lines, " ")), " ")
}

// clarifyRequestUsageEntry is the `prespec <verb>` entry of the help in bin/lib/usage.sh: its
// first line and the lines indented under it, up to the next entry.
func clarifyRequestUsageEntry(usage string) string {
	var entry []string
	in := false
	for _, line := range strings.Split(usage, "\n") {
		switch {
		case !in && strings.HasPrefix(line, "  prespec <verb>"):
			in = true
		case in && !strings.HasPrefix(line, "   "):
			return clarifyRequestCollapse(entry)
		}
		if in {
			entry = append(entry, line)
		}
	}
	return clarifyRequestCollapse(entry)
}

// clarifyRequestCommentAbove is the comment block that sits directly above the first line that
// starts with marker, without its comment marks.
func clarifyRequestCommentAbove(source, marker string) string {
	lines := strings.Split(source, "\n")
	for i, line := range lines {
		if !strings.HasPrefix(line, marker) {
			continue
		}
		var comment []string
		for j := i - 1; j >= 0 && strings.HasPrefix(lines[j], "#"); j-- {
			comment = append([]string{strings.TrimPrefix(lines[j], "#")}, comment...)
		}
		return clarifyRequestCollapse(comment)
	}
	return ""
}
