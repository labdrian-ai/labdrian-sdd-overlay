package agentgen

import (
	"fmt"
	"strings"
)

// claudeKeys are the four keys of the frontmatter of a Claude agent file, in the order project writes them.
var claudeKeys = []string{"name", "description", "model", "tools"}

// ClaudeInstructions is the body of a Claude agent file, the exact inverse of the Claude file of project: the file must
// be a `---` line, the four lines `name: `, `description: `, `model: ` and `tools: ` in that order, a `---` line, a blank
// line and the body, and nothing else before the body. The error says what is not the generator's shape (C25 T4, L8).
func ClaudeInstructions(data []byte) (string, error) {
	rest, ok := strings.CutPrefix(string(data), "---\n")
	if !ok {
		return "", fmt.Errorf("not an agent file the generator wrote: no opening --- line")
	}
	for _, key := range claudeKeys {
		line, after, found := strings.Cut(rest, "\n")
		if !found || !strings.HasPrefix(line, key+": ") {
			return "", fmt.Errorf("not an agent file the generator wrote: expected the key %q next, in the order %s", key, strings.Join(claudeKeys, ", "))
		}
		rest = after
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
