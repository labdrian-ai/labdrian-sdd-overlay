package app

import (
	"errors"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

func reads(files map[string]string) skills.FileReader {
	return func(name string) ([]byte, error) {
		if content, ok := files[name]; ok {
			return []byte(content), nil
		}
		return nil, errors.New("open " + name + ": no such file or directory")
	}
}

const cleanSkill = "---\nname: demo\ndescription: A concise procedural skill for demo.\nlicense: MIT\nmetadata:\n  author: tester\n  version: \"1.0\"\n---\n## Activation Contract\nLoad this skill for its documented procedure.\n\n## Hard Rules\n- Keep the procedure explicit.\n\n## Execution Steps\n1. Follow the procedure.\n"

func TestLintSkillPassesACleanFile(t *testing.T) {
	got, err := LintSkill(reads(map[string]string{"a.md": cleanSkill}), LintInput{Path: "a.md"})
	if err != nil || !got.Passed() || len(got.Hard) != 0 {
		t.Fatalf("LintSkill = %+v, %v, want a pass", got, err)
	}
}

func TestLintSkillFailsAFileWithAHardFinding(t *testing.T) {
	got, err := LintSkill(reads(map[string]string{"a.md": strings.TrimPrefix(cleanSkill, "---\n")}), LintInput{Path: "a.md"})
	if err != nil {
		t.Fatalf("a skill that fails the lint is a result, not an error: %v", err)
	}
	if got.Passed() || len(got.Hard) == 0 {
		t.Errorf("LintSkill = %+v, want a hard finding", got)
	}
}

func TestLintSkillSaysAFileThatCannotBeRead(t *testing.T) {
	_, err := LintSkill(reads(nil), LintInput{Path: "missing.md"})
	var unreadable *FileReadError
	if !errors.As(err, &unreadable) || unreadable.Path != "missing.md" {
		t.Fatalf("err = %v, want a FileReadError naming missing.md", err)
	}
	if want := `reading "missing.md": open missing.md: no such file or directory`; err.Error() != want {
		t.Errorf("err = %q, want %q", err, want)
	}
}

func TestLintSkillWarningsAloneDoNotBlock(t *testing.T) {
	r := LintResult{Warnings: []skills.Warning{{Rule: "x", Msg: "y"}}}
	if !r.Passed() {
		t.Error("a result with warnings and no hard finding must pass")
	}
}

func TestLintRuleTableIsTheDomainsOne(t *testing.T) {
	if LintRuleTable() != skills.RenderLintRules() || !strings.HasPrefix(LintRuleTable(), "| ID | Severity | Check |") {
		t.Error("the rule table is not the one the style guide carries")
	}
}
