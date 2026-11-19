package gate

// RewriteSkillPaths (labdrian-orchestrator-opencode T43 commit a, S8 L117 b): on a runtime whose roles cannot
// use the skill tool, the skills a launch prompt lists must be absolute SKILL.md paths. It reads the section
// under the first exact injection header and turns each bare skill id into its path. It is not Rewrite: the
// gate of Claude Code never calls it.

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/contract"
)

// skillIDForm is the form of a skill id: the names of the directories of skills/.
var skillIDForm = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// maxQuotedLine bounds how much of a refused line the sentence quotes.
const maxQuotedLine = 240

// RewriteSkillPaths returns prompt with each bare skill id under the first exact line of
// contract.DefaultInjectionPoint, up to the next line that starts with '#', replaced by
// <skillsRoot>/<id>/SKILL.md when that file exists. An absolute path to an existing file and a blank line
// stay as they are. A prompt with neither the header nor a line that starts with the header text is returned
// unchanged. Anything else is refused with one sentence that names the line (counted from 1) and says what
// to write (isFile is the port that tells an existing regular file; the gate reads no disk): a line that starts with the header text but is not the header alone, an id with no SKILL.md under
// skillsRoot, an absolute path to no file, any other text. A refusal returns no prompt.
func RewriteSkillPaths(prompt, skillsRoot string, isFile func(path string) bool) (string, error) {
	header := contract.DefaultInjectionPoint
	lines := strings.Split(prompt, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != header && strings.HasPrefix(trimmed, header) {
			return "", fmt.Errorf("line %d %s is not the header alone: write %q alone on its line, then one skill id or absolute SKILL.md path per line", i+1, quoteLine(trimmed), header)
		}
	}
	if !hasExactHeader(prompt, header) {
		return prompt, nil
	}
	start := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == header {
			start = i + 1
			break
		}
	}
	for i := start; i < len(lines) && !strings.HasPrefix(lines[i], "#"); i++ {
		entry := strings.TrimSpace(lines[i])
		switch {
		case entry == "":
		case filepath.IsAbs(entry):
			if !isFile(entry) {
				return "", fmt.Errorf("line %d %s is an absolute path to no file: write the absolute path of an existing SKILL.md or a skill id of %s", i+1, quoteLine(entry), skillsRoot)
			}
		case skillIDForm.MatchString(entry):
			path := filepath.Join(skillsRoot, entry, "SKILL.md")
			if !isFile(path) {
				return "", fmt.Errorf("line %d %s names no skill: write a skill id of %s (a directory with a SKILL.md) or the absolute path of its SKILL.md", i+1, quoteLine(entry), skillsRoot)
			}
			lines[i] = strings.Replace(lines[i], entry, path, 1)
		default:
			return "", fmt.Errorf("line %d %s is neither a skill id nor an absolute path: a skill id is lowercase letters, digits and hyphens; write one skill id or absolute SKILL.md path per line", i+1, quoteLine(entry))
		}
	}
	return strings.Join(lines, "\n"), nil
}

// quoteLine is the line quoted for a sentence, cut at maxQuotedLine runes.
func quoteLine(line string) string {
	runes := []rune(line)
	if len(runes) > maxQuotedLine {
		line = string(runes[:maxQuotedLine]) + "..."
	}
	return `"` + line + `"`
}
