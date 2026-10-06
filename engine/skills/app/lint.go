package app

import (
	"fmt"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// FileReadError is a file the use case was told to read and could not.
type FileReadError struct {
	Path string
	Err  error
}

func (e *FileReadError) Error() string { return fmt.Sprintf("reading %q: %v", e.Path, e.Err) }
func (e *FileReadError) Unwrap() error { return e.Err }

// LintInput is what `skills lint <path>` is asked: which SKILL.md.
type LintInput struct {
	Path string
}

// LintResult is what the lint rules found in a skill file: Warnings are advisory and never
// block, Hard findings do.
type LintResult struct {
	Warnings []skills.Warning
	Hard     []error
}

// Passed reports whether the skill has no hard finding: warnings alone never block.
func (r LintResult) Passed() bool { return len(r.Hard) == 0 }

// LintSkill reads the file and lints it. An error is a file that could not be read, which is not
// a finding about the skill.
func LintSkill(read skills.FileReader, in LintInput) (LintResult, error) {
	data, err := read(in.Path)
	if err != nil {
		return LintResult{}, &FileReadError{Path: in.Path, Err: err}
	}
	hard, warnings := skills.LintSkillFile(data)
	return LintResult{Warnings: warnings, Hard: hard}, nil
}

// LintRuleTable is the table of the lint rules, as the style guide of skills carries it.
func LintRuleTable() string { return skills.RenderLintRules() }
