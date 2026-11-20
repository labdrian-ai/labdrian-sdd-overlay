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
// to write. A skill the prompt names in prose outside the section is then added to it (addProseMentions).
// (isFile is the port that tells an existing regular file; the gate reads no disk): a line that starts with the header text but is not the header alone, an id with no SKILL.md under
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
	start := -1
	if hasExactHeader(prompt, header) {
		for i, line := range lines {
			if strings.TrimSpace(line) == header {
				start = i + 1
				break
			}
		}
	}
	end := start
	for end >= 0 && end < len(lines) && !strings.HasPrefix(lines[end], "#") {
		end++
	}
	for i := start; i >= 0 && i < end; i++ {
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
	return addProseMentions(lines, start, end, skillsRoot, isFile), nil
}

// addProseMentions is the text of lines (the section under the header is lines[start:end], start -1 when
// the prompt has none) with the SKILL.md path of each skill the prose names added to that section, or in a
// section of its own after the prompt when it has none. A mention is a whole word, bounded by characters
// outside [A-Za-z0-9_-], of the form of a skill id with a hyphen whose <skillsRoot>/<id>/SKILL.md is a file,
// found outside the section and outside fenced code. Each path is added once, in the order of first
// mention, unless the section already holds it. With no mention the text is joined as it came.
func addProseMentions(lines []string, start, end int, skillsRoot string, isFile func(string) bool) string {
	listed := map[string]bool{}
	for i := start; i >= 0 && i < end; i++ {
		listed[strings.TrimSpace(lines[i])] = true
	}
	var added []string
	fenced := false
	for i, line := range lines {
		if strings.HasPrefix(line, "```") {
			fenced = !fenced
			continue
		}
		if fenced || i >= start && i < end && start >= 0 {
			continue
		}
		for _, word := range wordsOf(line) {
			if !proseIDForm.MatchString(word) || !strings.Contains(word, "-") {
				continue
			}
			path := filepath.Join(skillsRoot, word, "SKILL.md")
			if listed[path] || !isFile(path) {
				continue
			}
			listed[path] = true
			added = append(added, path)
		}
	}
	if len(added) == 0 {
		return strings.Join(lines, "\n")
	}
	if start < 0 {
		body := strings.TrimRight(strings.Join(lines, "\n"), "\n")
		return body + "\n\n" + contract.DefaultInjectionPoint + "\n" + strings.Join(added, "\n") + "\n"
	}
	anchor := start - 1
	for i := start; i < end; i++ {
		if strings.TrimSpace(lines[i]) != "" {
			anchor = i
		}
	}
	eol := ""
	if strings.HasSuffix(lines[anchor], "\r") {
		eol = "\r"
	}
	out := append([]string{}, lines[:anchor+1]...)
	for _, path := range added {
		out = append(out, path+eol)
	}
	return strings.Join(append(out, lines[anchor+1:]...), "\n")
}

// proseIDForm is the form of a skill id a prose word must have to be a mention.
var proseIDForm = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// wordsOf is the maximal runs of characters of a skill id, letters, digits, hyphen and underscore, of line.
func wordsOf(line string) []string {
	var words []string
	from := -1
	for i := 0; i <= len(line); i++ {
		if i < len(line) && wordChar(line[i]) {
			if from < 0 {
				from = i
			}
		} else if from >= 0 {
			words = append(words, line[from:i])
			from = -1
		}
	}
	return words
}

// wordChar is whether b continues a word: the characters of a skill id, uppercase letters and the underscore.
func wordChar(b byte) bool {
	return b == '_' || b == '-' || b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}

// quoteLine is the line quoted for a sentence, cut at maxQuotedLine runes.
func quoteLine(line string) string {
	runes := []rune(line)
	if len(runes) > maxQuotedLine {
		line = string(runes[:maxQuotedLine]) + "..."
	}
	return `"` + line + `"`
}
