package agentgen

import (
	"fmt"
	"strings"
)

// claudeKeys are the four keys of the frontmatter of a Claude agent file, in the order project writes them.
var claudeKeys = []string{"name", "description", "model", "tools"}

// ClaudeInstructions is the body of a Claude agent file, read back from the shape project writes: the file must
// be a `---` line, the notice comment lines of a Claude-only role if any (each `#` or `# ` and text, as noticeComments
// writes them, and only before name:), the four lines `name: `, `description: `, `model: ` and `tools: ` in that order, a `---` line, a blank
// line and the body, and nothing else before the body. The file of a role that carries a notice must have comment lines,
// the first its notice header, the second `#` and the third `# ` and the role's license name; any other file must have
// none. The remaining license lines are held only to the comment form, because the function does not read the notice
// file; Check compares the whole file byte for byte. The error says what is not the generator's shape (C25 T4, L8).
func ClaudeInstructions(data []byte) (string, error) {
	rest, ok := strings.CutPrefix(string(data), "---\n")
	if !ok {
		return "", fmt.Errorf("not an agent file the generator wrote: no opening --- line")
	}
	// The notice comment lines of a Claude-only role come right after the opening line, before name:.
	var comments []string
	for {
		line, after, found := strings.Cut(rest, "\n")
		if !found || (line != "#" && !strings.HasPrefix(line, "# ")) {
			break
		}
		comments = append(comments, line)
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
	role, carriesNotice := noticeRole(name)
	if len(comments) > 0 && !carriesNotice {
		return "", fmt.Errorf("not an agent file the generator wrote: comment lines before name: in the file of role %q, which carries no license notice", name)
	}
	if carriesNotice {
		if len(comments) == 0 {
			return "", fmt.Errorf("not an agent file the generator wrote: no notice comment lines in the file of role %q, which carries a license notice", name)
		}
		if comments[0] != role.noticeHeader() {
			return "", fmt.Errorf("not an agent file the generator wrote: the first comment line is not the notice header of role %q", name)
		}
		if len(comments) < 3 || comments[1] != "#" || comments[2] != "# "+role.licenseName {
			return "", fmt.Errorf("not an agent file the generator wrote: the comment lines after the header are not the notice of role %q", name)
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
