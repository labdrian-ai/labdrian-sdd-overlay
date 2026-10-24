package main

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
)

// inceptionSkillPath is skills/inception-pipeline/SKILL.md, read from the repository these tests
// run in (the directory of this package is engine/cmd).
const inceptionSkillPath = "../../skills/inception-pipeline/SKILL.md"

// TestInceptionChainEntryRecordRunsThroughTheRolesVerbs follows what the Chain entry of
// skills/inception-pipeline/SKILL.md documents for the first record of a chain through the real
// verbs (review finding R3-tests-are-phrase-pins of review-9bceb2ac6fb71431).
//
// engine/skills/inception_chain_entry_test.go pins the words of that skill. It cannot tell
// whether the record the skill shows is one the engine accepts, or whether the commands the
// skill writes run. This test takes the example record from the skill, the `labdrian roles
// append` command from the Chain entry and the `labdrian roles resume` command from
// Closure-Feedback, fills their {project} and {change} placeholders with the ids of the example,
// and runs them through roles append and roles resume in a state home of its own, never the real
// one. The record must be accepted, and resume must report the role the hand-off names as the
// next hop and a chain that is still running.
//
// The same run is then repeated over copies of the skill whose example is malformed (and whose
// command lost --stdin). Each must be noticed: a check that passed on those would pass on a
// skill whose example had rotted.
func TestInceptionChainEntryRecordRunsThroughTheRolesVerbs(t *testing.T) {
	data, err := os.ReadFile(inceptionSkillPath)
	if err != nil {
		t.Fatalf("reading the inception-pipeline skill: %v", err)
	}
	skill := string(data)

	t.Run("the documented first record is accepted and resume reports the estimator next", func(t *testing.T) {
		for _, problem := range inceptionFirstRecordProblems(t, skill, "estimator") {
			t.Error(problem)
		}
	})

	t.Run("the first record of a chain with an open premise is accepted and resume reports the shaper next", func(t *testing.T) {
		premiseOpen := inceptionWithExampleRecord(t, skill, func(record map[string]any) {
			record["from_role"], record["to_role"], record["payload_kind"] = "prototyper", "shaper", "exploration"
		})
		for _, problem := range inceptionFirstRecordProblems(t, premiseOpen, "shaper") {
			t.Error(problem)
		}
	})

	for _, m := range []struct {
		name string
		edit func(record map[string]any)
	}{
		{"numbers the first record 2", func(r map[string]any) { r["seq"] = 2 }},
		{"chains the first record to a digest", func(r map[string]any) { r["prev_sha256"] = strings.Repeat("b", 64) }},
		{"leaves out prev_sha256", func(r map[string]any) { delete(r, "prev_sha256") }},
		{"hands off from a role that cannot start a chain", func(r map[string]any) { r["from_role"] = "builder" }},
		{"carries a member a record does not have", func(r map[string]any) { r["payload"] = "tasks" }},
	} {
		t.Run("a skill whose example "+m.name+" is noticed", func(t *testing.T) {
			malformed := inceptionWithExampleRecord(t, skill, m.edit)
			if len(inceptionFirstRecordProblems(t, malformed, "estimator")) == 0 {
				t.Errorf("a skill whose example %s went through the roles verbs without a problem: this test would not notice the documented example rotting", m.name)
			}
		})
	}

	t.Run("a skill that documents the append command without --stdin is noticed", func(t *testing.T) {
		malformed := strings.Replace(skill, " --stdin`", "`", 1)
		if malformed == skill {
			t.Fatal("the skill has no code span ending in --stdin: the command this test malforms is gone")
		}
		if len(inceptionFirstRecordProblems(t, malformed, "estimator")) == 0 {
			t.Error("a skill that documents the append command without --stdin went through the roles verbs without a problem: this test would not notice the documented command rotting")
		}
	})

	t.Run("a skill whose Chain entry gains a second json block or append command is noticed", func(t *testing.T) {
		for name, extra := range map[string]string{
			"a second json block":     "```json\n{}\n```\n",
			"a second append command": "`labdrian roles append --project other --goal other --chain other --stdin`\n",
		} {
			malformed := strings.Replace(skill, "## Chain entry\n", "## Chain entry\n\n"+extra, 1)
			if malformed == skill {
				t.Fatal("the skill has no \"## Chain entry\" heading: the section this test malforms is gone")
			}
			problems := inceptionFirstRecordProblems(t, malformed, "estimator")
			if !strings.Contains(strings.Join(problems, "\n"), "want exactly one") {
				t.Errorf("a skill whose Chain entry gained %s was not reported as ambiguous (problems: %q): the test would pick one by position", name, problems)
			}
		}
	})
}

// inceptionFirstRecordProblems runs the first record the Chain entry of skill documents through
// roles append and roles resume, in a state home and a HOME of their own, and returns everything
// that does not hold. No problem means the record was accepted, was stored under that state home,
// and resume reports wantRole as the role that has the baton and the chain as not terminal.
func inceptionFirstRecordProblems(t *testing.T, skill, wantRole string) []string {
	t.Helper()
	stateHome := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", stateHome)

	entry := inceptionSkillSection(skill, "Chain entry")
	record, problem := inceptionExampleRecord(entry)
	if problem != "" {
		return []string{problem}
	}
	var ids struct {
		ProjectID string `json:"project_id"`
		GoalID    string `json:"goal_id"`
		ChainID   string `json:"chain_id"`
	}
	if err := json.Unmarshal([]byte(record), &ids); err != nil {
		return []string{fmt.Sprintf("the example record is not JSON: %v", err)}
	}

	var problems []string
	if ids.GoalID != ids.ChainID {
		problems = append(problems, fmt.Sprintf("goal_id %q and chain_id %q differ: the Chain entry says goal id = chain id = change-name", ids.GoalID, ids.ChainID))
	}
	appendArgs, problem := inceptionDocumentedRolesArgs(entry, "Chain entry", "append", ids.ProjectID, ids.GoalID)
	if problem != "" {
		return append(problems, problem)
	}
	appended := runRolesTest(appendArgs, record)
	if appended.code != 0 {
		return append(problems, fmt.Sprintf("`roles append` refused the example record (exit %d): %s", appended.code, strings.TrimSpace(appended.stderr)))
	}
	if !strings.Contains(appended.stdout, stateHome) {
		problems = append(problems, fmt.Sprintf("`roles append` stored the record outside the isolated state home %s: %s", stateHome, strings.TrimSpace(appended.stdout)))
	}

	resumeArgs, problem := inceptionDocumentedRolesArgs(inceptionSkillSection(skill, "Closure-Feedback"), "Closure-Feedback", "resume", ids.ProjectID, ids.GoalID)
	if problem != "" {
		return append(problems, problem)
	}
	resumed := runRolesTest(resumeArgs, "")
	if resumed.code != 0 {
		return append(problems, fmt.Sprintf("`roles resume` failed after the append (exit %d): %s", resumed.code, strings.TrimSpace(resumed.stderr)))
	}
	var state struct {
		Role        string `json:"role"`
		Interrupted bool   `json:"interrupted"`
		Terminal    bool   `json:"terminal"`
	}
	if err := json.Unmarshal([]byte(resumed.stdout), &state); err != nil {
		return append(problems, fmt.Sprintf("`roles resume` did not print JSON: %v", err))
	}
	if state.Role != wantRole {
		problems = append(problems, fmt.Sprintf("`roles resume` reports role %q as the next hop, want %q", state.Role, wantRole))
	}
	if state.Interrupted || state.Terminal {
		problems = append(problems, fmt.Sprintf("`roles resume` reports interrupted=%v terminal=%v after the first record, want a chain that is running", state.Interrupted, state.Terminal))
	}
	return problems
}

// inceptionWithExampleRecord returns skill with the example record of its Chain entry replaced by
// the same record after edit changed it.
func inceptionWithExampleRecord(t *testing.T, skill string, edit func(record map[string]any)) string {
	t.Helper()
	original, problem := inceptionExampleRecord(inceptionSkillSection(skill, "Chain entry"))
	if problem != "" {
		t.Fatal(problem)
	}
	if n := strings.Count(skill, original); n != 1 {
		t.Fatalf("the example record of the Chain entry appears %d times in the skill, want once: replacing it could edit the wrong copy", n)
	}
	var record map[string]any
	if err := json.Unmarshal([]byte(original), &record); err != nil {
		t.Fatalf("the example record of the Chain entry is not JSON: %v", err)
	}
	edit(record)
	edited, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("encoding the edited record: %v", err)
	}
	return strings.Replace(skill, original, string(edited), 1)
}

// inceptionSkillSection returns the body of the "## " section of skill whose heading starts with
// title, up to the next "## " heading outside a code block. It is empty when there is no such
// section.
func inceptionSkillSection(skill, title string) string {
	var body []string
	inSection, inFence := false, false
	for _, line := range strings.Split(skill, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
		} else if !inFence && strings.HasPrefix(line, "## ") {
			if inSection {
				break
			}
			inSection = strings.HasPrefix(line, "## "+title)
			continue
		}
		if inSection {
			body = append(body, line)
		}
	}
	return strings.Join(body, "\n")
}

// inceptionExampleRecord returns the example record of the Chain entry section entry: the lines,
// without the fences, of its only ```json code block. The block may sit anywhere in the section;
// what the test relies on is that there is exactly one, so problem names any other count instead
// of the test picking one of several blocks by position.
func inceptionExampleRecord(entry string) (record, problem string) {
	var blocks []string
	var block []string
	inBlock := false
	for _, line := range strings.Split(entry, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case !inBlock && trimmed == "```json":
			inBlock, block = true, nil
		case inBlock && trimmed == "```":
			inBlock = false
			blocks = append(blocks, strings.Join(block, "\n"))
		case inBlock:
			block = append(block, line)
		}
	}
	if len(blocks) != 1 {
		return "", fmt.Sprintf("the Chain entry documents %d example records (json code blocks), want exactly one", len(blocks))
	}
	return blocks[0], ""
}

// inceptionDocumentedRolesArgs returns the arguments, after `labdrian roles`, of the only code
// span of text (the section named section) that runs the roles verb, with the skill's {project}
// placeholder replaced by project and its {change} placeholder by change. problem names any count
// other than one, so the test never runs one of several commands chosen by position.
func inceptionDocumentedRolesArgs(text, section, verb, project, change string) (args []string, problem string) {
	spans := regexp.MustCompile("`labdrian roles ("+regexp.QuoteMeta(verb)+" [^`]*)`").FindAllStringSubmatch(text, -1)
	if len(spans) != 1 {
		return nil, fmt.Sprintf("%s documents %d `labdrian roles %s` commands, want exactly one", section, len(spans), verb)
	}
	command := strings.NewReplacer("{project}", project, "{change}", change).Replace(spans[0][1])
	return strings.Fields(command), ""
}
