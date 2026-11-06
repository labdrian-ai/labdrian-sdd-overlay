package skills

import (
	"strings"
	"testing"
)

// The skill adhd-output is how this overlay shapes replies for an owner who has ADHD and
// dyslexia. It is a skill, the same vehicle as the other communication rules, not a hook and
// not a phase contract. These pins hold the rules the owner asked to keep: drop one and the
// pin fails.

const adhdOutputSkillRel = "skills/adhd-output/SKILL.md"

// adhdOutputRule is one rule and a fragment that only that rule carries.
type adhdOutputRule struct {
	what     string
	fragment string
}

func adhdOutputRules() []adhdOutputRule {
	return []adhdOutputRule{
		{"lead with one doable action", "The first line is one action the owner can do now."},
		{"number steps and separate them", "Leave a blank line between steps."},
		{"end on one short next action", "under two minutes"},
		{"finish the first issue before a second", "Offer a second issue as its own question."},
		{"restate state every turn", "Restate where the work stands, every turn."},
		{"give time in minutes or hours", `Never "a bit" or "soon"`},
		{"name what now works", "Name what now works, in concrete terms."},
		{"state cause and fix", "State the cause and the fix."},
		{"cap visible lists without cutting analysis", "This cap is presentation only."},
		{"skip preamble and closers", "No preamble, no recap, no closer."},
		{"keep paragraphs short", "No wall of text."},
		{"put paths and commands in code spans", "Put each path, command, and identifier in a code span."},
		{"add headers to a long explanation", "add headers so it can be skimmed"},
		{"avoid idioms and unmarked lookalikes", "say digit zero or letter O"},
		{"start on every session", "On at the start of every session,"},
		{"start on again next session", "The next session starts on."},
		{"turn off in one line", "ADHD output is off."},
		{"let safety beat brevity", "Safety beats brevity."},
		{"stop a debug spiral", "The last three turns are still broken"},
		{"ask one question instead of guessing", "One short question. Do not guess."},
		{"let the task win when a rule would erase the answer", "The task wins."},
		{"let the harness outrank the skill", "The harness wins."},
		{"pass the skill to agents the owner reads, not to machine replies", "Do not pass it to `reviewer` or `verifier`."},
		{"credit the upstream skill", "https://github.com/ayghri/i-have-adhd"},
		{"credit the tool kit", "The Adult ADHD Tool Kit by J. Russell Ramsay and Anthony L. Rostain"},
	}
}

// adhdOutputGaps reports every rule whose fragment text lacks.
func adhdOutputGaps(text string) []string {
	var gaps []string
	for _, rule := range adhdOutputRules() {
		if !strings.Contains(text, rule.fragment) {
			gaps = append(gaps, rule.what)
		}
	}
	return gaps
}

func TestAdhdOutputSkillCarriesTheRulesAndTheAttribution(t *testing.T) {
	root := skillsRepoRoot(t)
	whole := readRepoFile(t, root, adhdOutputSkillRel)
	frontmatter, body, err := SplitSkillFile([]byte(whole))
	if err != nil {
		t.Fatalf("splitting the skill: %v", err)
	}
	fm := parseFrontmatterFields(frontmatter)
	if fm.Name != "adhd-output" {
		t.Errorf("name = %q, want adhd-output", fm.Name)
	}
	if fm.License != "MIT" {
		t.Errorf("license = %q, want MIT", fm.License)
	}
	if fm.MetaAuthor != "labdrian" || fm.MetaVersion != "1.0" {
		t.Errorf("author %q version %q, want labdrian 1.0", fm.MetaAuthor, fm.MetaVersion)
	}
	for _, phrase := range []string{"Trigger: ", "every user-facing reply", "Always on", "stop adhd mode", "normal mode"} {
		if !strings.Contains(fm.Description, phrase) {
			t.Errorf("description %q lacks %q", fm.Description, phrase)
		}
	}
	for _, phrase := range []string{"i have adhd", "say `adhd mode`"} {
		if strings.Contains(fm.Description, phrase) || strings.Contains(body, phrase) {
			t.Errorf("the skill still waits for the owner to say %q", phrase)
		}
	}
	if strings.Contains(frontmatter, "disable-model-invocation") {
		t.Error("the model must load the skill on every user-facing reply, without a phrase")
	}
	hard, warnings := LintSkillFile([]byte(whole))
	for _, finding := range hard {
		t.Errorf("hard lint finding: %v", finding)
	}
	for _, warning := range warnings {
		// The ten rules, the overrides and the dyslexia lines fit the hard body budget and
		// not the advisory one. Any other warning is a defect.
		if warning.Rule == "body-recommended" {
			continue
		}
		t.Errorf("lint warning [%s]: %s", warning.Rule, warning.Msg)
	}
	if gaps := adhdOutputGaps(body); len(gaps) != 0 {
		t.Errorf("the skill is missing rules: %s", strings.Join(gaps, "; "))
	}
	license := readRepoFile(t, root, "skills/adhd-output/LICENSE")
	if !strings.Contains(license, "MIT License") || !strings.Contains(license, "Copyright (c) 2026 Ayoub Ghriss") {
		t.Error("LICENSE must keep the upstream MIT notice and copyright")
	}
}

// A pin that still passes after its fragment is deleted does not hold the rule.
func TestAdhdOutputPinsFailWhenTheRuleIsRemoved(t *testing.T) {
	_, body, err := SplitSkillFile([]byte(readRepoFile(t, skillsRepoRoot(t), adhdOutputSkillRel)))
	if err != nil {
		t.Fatalf("splitting the skill: %v", err)
	}
	for _, rule := range adhdOutputRules() {
		t.Run(rule.what, func(t *testing.T) {
			if !strings.Contains(body, rule.fragment) {
				t.Fatalf("the skill lacks %q, so the pin cannot be load-bearing", rule.fragment)
			}
			stripped := strings.ReplaceAll(body, rule.fragment, "")
			var held bool
			for _, gap := range adhdOutputGaps(stripped) {
				if gap == rule.what {
					held = true
				}
			}
			if !held {
				t.Fatalf("removing %q left the pin %q green", rule.fragment, rule.what)
			}
		})
	}
}

func TestAdhdOutputIsRegisteredForEveryRuntimeAndDeployed(t *testing.T) {
	root := skillsRepoRoot(t)
	registry := readRepoFile(t, root, "skills.registry.yaml")
	entry := "  - id: adhd-output\n    path: adhd-output\n    source:\n      type: custom\n    install:\n      defaultScope: global\n      targets:\n        - claude\n        - opencode\n        - codex\n        - pi\n    lifecycle:\n      updateStrategy: overlay-only\n"
	if !strings.Contains(registry, entry) {
		t.Error("skills.registry.yaml must register adhd-output as a global custom skill for claude, opencode, codex and pi")
	}
	manifest := readRepoFile(t, root, "overlay.manifest")
	for _, row := range []string{"adhd-output/SKILL.md custom\n", "adhd-output/LICENSE custom\n"} {
		if !strings.Contains(manifest, row) {
			t.Errorf("overlay.manifest lacks %q", row)
		}
	}
	resolver := readRepoFile(t, root, "skills/_shared/skill-resolver.md")
	for _, line := range []string{
		"`adhd-output` is always on for user-facing replies.",
		"Do not pass it to `reviewer` or `verifier`.",
	} {
		if !strings.Contains(resolver, line) {
			t.Errorf("skill-resolver must say %q", line)
		}
	}
}
