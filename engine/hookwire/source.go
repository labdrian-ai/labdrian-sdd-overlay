package hookwire

import "fmt"

// AdaptToolName translates a runtime's tool name to the vocabulary the policies
// already know. Only Codex's two sanitized longterm-mem read names differ. It is
// deliberately not a general underscore replacement: every other name is opaque.
func AdaptToolName(source, name string) (string, error) {
	switch source {
	case "claude":
		return name, nil
	case "codex":
		switch name {
		case "mcp__longterm_mem__query":
			return "mcp__longterm-mem__query", nil
		case "mcp__longterm_mem__get":
			return "mcp__longterm-mem__get", nil
		}
		return name, nil
	default:
		return "", fmt.Errorf("unsupported --source %q (supported: claude, codex)", source)
	}
}
