package gate

// RewriteSkillPaths (labdrian-orchestrator-opencode T43 commit a, S8 L117 b): on a runtime whose roles cannot
// use the skill tool, the skills a launch prompt lists must be absolute SKILL.md paths. It reads the section
// under the first exact injection header and turns each bare skill id into its path. It is not Rewrite: the
// gate of Claude Code never calls it.

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/contract"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skillid"
)

// maxQuotedLine bounds how much of a refused line the sentence quotes.
const maxQuotedLine = 240

// RewriteSkillPaths returns prompt with each bare skill id under the first exact line of
// contract.DefaultInjectionPoint, up to the next line that starts with '#', replaced by
// <skillsRoot>/<id>/SKILL.md when that file exists; an absolute path to an existing file and a blank line
// stay as they are. It refuses, with one sentence that names the line (counted from 1) and says what to write
// (isFile is the port that tells an existing regular file; the gate reads no disk), a line that starts with
// the header text but is not the header alone, an id with no SKILL.md under skillsRoot, an absolute path to
// no file and any other text; a refusal returns no prompt. A skill the prompt then names in prose outside the
// section is added to it as its path (addProseMentions). A prompt with neither the header nor a prose
// mention is returned unchanged.
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
		case skillid.Valid(entry):
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
// mention, unless the section already holds it by id (a bare id, or an absolute path whose file is SKILL.md
// and whose directory is named for the id). Fences are read as CommonMark opens them (fenceOpening). The
// disk is asked once for each distinct candidate word. A section made here follows the line ending of the
// prompt. With no mention the text is joined as it came.
func addProseMentions(lines []string, start, end int, skillsRoot string, isFile func(string) bool) string {
	listed := map[string]bool{}
	for i := start; i >= 0 && i < end; i++ {
		entry := strings.TrimSpace(lines[i])
		if skillid.Valid(entry) {
			listed[entry] = true
		} else if filepath.IsAbs(entry) && filepath.Base(entry) == "SKILL.md" {
			listed[filepath.Base(filepath.Dir(filepath.Clean(entry)))] = true
		}
	}
	asked := map[string]bool{}
	var added []string
	var fenceChar byte
	fenceLen := 0
	for i, line := range lines {
		if fenceLen > 0 {
			if fenceCloses(line, fenceChar, fenceLen) {
				fenceLen = 0
			}
			continue
		}
		if char, n := fenceOpening(line); n > 0 {
			fenceChar, fenceLen = char, n
			continue
		}
		if i >= start && i < end && start >= 0 {
			continue
		}
		for _, word := range wordsOf(line) {
			if !skillid.Valid(word) || !strings.Contains(word, "-") || listed[word] {
				continue
			}
			isSkill, known := asked[word]
			if !known {
				isSkill = isFile(filepath.Join(skillsRoot, word, "SKILL.md"))
				asked[word] = isSkill
			}
			if !isSkill {
				continue
			}
			listed[word] = true
			added = append(added, filepath.Join(skillsRoot, word, "SKILL.md"))
		}
	}
	if len(added) == 0 {
		return strings.Join(lines, "\n")
	}
	if start < 0 {
		ending, trim := "\n", "\n"
		if len(lines) > 1 && strings.HasSuffix(lines[0], "\r") {
			ending, trim = "\r\n", "\r\n"
		}
		body := strings.TrimRight(strings.Join(lines, "\n"), trim)
		return body + ending + ending + contract.DefaultInjectionPoint + ending + strings.Join(added, ending) + ending
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

// fenceOpening is the character and length of the fence line opens, after leading spaces and tabs: three or
// more backticks or three or more tildes. The length is 0 when line opens none.
func fenceOpening(line string) (byte, int) {
	line = strings.TrimLeft(line, " \t")
	if line == "" || line[0] != '`' && line[0] != '~' {
		return 0, 0
	}
	n := len(line) - len(strings.TrimLeft(line, line[:1]))
	if n < 3 {
		return 0, 0
	}
	return line[0], n
}

// fenceCloses is whether line closes a fence of char and length n: after leading spaces and tabs, a run of
// char at least n long followed only by spaces, tabs or a carriage return.
func fenceCloses(line string, char byte, n int) bool {
	line = strings.TrimLeft(line, " \t")
	run := len(line) - len(strings.TrimLeft(line, string(char)))
	return run >= n && strings.Trim(line[run:], " \t\r") == ""
}

// wordsOf is the maximal runs of characters of a skill id, letters, digits, hyphen and underscore, of line.
func wordsOf(line string) []string {
	var words []string
	from := -1
	for i := 0; i <= len(line); i++ {
		if i < len(line) && skillid.WordChar(line[i]) {
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

// quoteLine is the line quoted for a sentence, cut at maxQuotedLine runes.
func quoteLine(line string) string {
	runes := []rune(line)
	if len(runes) > maxQuotedLine {
		line = string(runes[:maxQuotedLine]) + "..."
	}
	return `"` + line + `"`
}
