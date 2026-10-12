package skillstale

import "strings"

// skillBody is the text of a skill file below its front matter. A file with no front matter, or whose front
// matter never closes, is all body.
func skillBody(data []byte) string {
	text := strings.TrimPrefix(string(data), "\ufeff")
	lines := strings.Split(text, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return text
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			return strings.Join(lines[i+1:], "\n")
		}
	}
	return text
}

// fencedCommands lists, in order, the first word of every line inside a fenced code block of text, without a
// leading "$" and without comment lines. A command that occurs twice is listed twice.
func fencedCommands(text string) []string {
	var commands []string
	var fence string
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if fence == "" {
			if run := openingFence(trimmed); run != "" {
				fence = run
			}
			continue
		}
		if closesFence(trimmed, fence) {
			fence = ""
			continue
		}
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "$"))
		fields := strings.Fields(trimmed)
		if len(fields) > 0 {
			commands = append(commands, strings.Trim(fields[0], "`;,"))
		}
	}
	return commands
}

func openingFence(line string) string {
	if !strings.HasPrefix(line, "```") {
		return ""
	}
	count := 0
	for count < len(line) && line[count] == '`' {
		count++
	}
	if count < 3 {
		return ""
	}
	return strings.Repeat("`", count)
}

func closesFence(line, fence string) bool {
	return strings.HasPrefix(line, fence) && strings.TrimSpace(strings.TrimPrefix(line, fence)) == ""
}
