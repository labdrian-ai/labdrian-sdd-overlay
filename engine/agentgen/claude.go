package agentgen

import (
	"fmt"
	"strings"
)

// claudeKeys are the four keys of the frontmatter of a Claude agent file, in the order project writes them.
var claudeKeys = []string{"name", "description", "model", "tools"}

// ClaudeInstructions is the body of a Claude agent file, the exact inverse of the Claude file of project: the file must
// be a `---` line, the notice comment lines of a Claude-only role if any (each `#` or `# ` and text, as noticeComments
// writes them, and only before name:; they are accepted only in the file of a Claude-only role that carries a notice,
// and only when the first of them is that role's notice header), the four lines `name: `, `description: `, `model: ` and `tools: ` in that order, a `---` line, a blank
// line and the body, and nothing else before the body. The error says what is not the generator's shape (C25 T4, L8).
func ClaudeInstructions(data []byte) (string, error) {
	rest, ok := strings.CutPrefix(string(data), "---\n")
	if !ok {
		return "", fmt.Errorf("not an agent file the generator wrote: no opening --- line")
	}
	// The notice comment lines of a Claude-only role come right after the opening line, before name:.
	var firstComment string
	for {
		line, after, found := strings.Cut(rest, "\n")
		if !found || (line != "#" && !strings.HasPrefix(line, "# ")) {
			break
		}
		if firstComment == "" {
			firstComment = line // never empty: a comment line holds at least "#"
		}
		rest = after
	}
	var name string
	for _, key := range claudeKeys {
		line, after, found := strings.Cut(rest, "\n")
		if !found || !strings.HasPrefix(line, key+": ") {
			return "", fmt.Errorf("not an agent file the generator wrote: expected the key %q next, in the order %s", key, strings.Join(claudeKeys, ", "))
		}
		if key == "name" {
			name = strings.TrimPrefix(line, "name: ")
		}
		rest = after
	}
	if firstComment != "" {
		role, ok := noticeRole(name)
		if !ok {
			return "", fmt.Errorf("not an agent file the generator wrote: comment lines before name: in the file of role %q, which carries no license notice", name)
		}
		if firstComment != role.noticeHeader() {
			return "", fmt.Errorf("not an agent file the generator wrote: the first comment line is not the notice header of role %q", name)
		}
	}
	rest, ok = strings.CutPrefix(rest, "---\n")
	if !ok {
		return "", fmt.Errorf("not an agent file the generator wrote: no closing --- line after the key tools")
	}
	body, ok := strings.CutPrefix(rest, "\n")
	if !ok {
		return "", fmt.Errorf("not an agent file the generator wrote: no blank line after the closing --- line")
	}
	return body, nil
}
