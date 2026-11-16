package hookwire

import (
	"fmt"
	"strings"
)

// AdaptToolName translates a runtime's tool name to the vocabulary the policies
// already know. Codex's two sanitized longterm-mem read names differ, and
// OpenCode v2 spells its tools its own way (adaptOpencode). It is deliberately
// not a general underscore replacement: every other name is opaque.
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
	case "opencode":
		return adaptOpencode(name), nil
	default:
		return "", fmt.Errorf("unsupported --source %q (supported: claude, codex, opencode)", source)
	}
}

// adaptOpencode maps OpenCode v2's tool names (measured in T2, L10): shell is
// Bash, patch is Edit, and an MCP tool is "<server>_<tool>", which becomes
// "mcp__<server>__<tool>". The Code Mode wrapper "execute" is not mapped, so no
// policy ever matches it (its inner MCP calls fire their own hook). Every other
// name passes unchanged.
//
// An MCP name is told apart from a built-in by shape: the one built-in that
// contains an underscore (experimental_image_generate) is excluded by name, a
// name already in the mcp__ spelling passes, and the server is the part before
// the first underscore, so a server name must not contain one (hyphens are fine).
func adaptOpencode(name string) string {
	switch name {
	case "shell":
		return "Bash"
	case "patch":
		return "Edit"
	case "experimental_image_generate":
		return name
	}
	if strings.HasPrefix(name, "mcp__") {
		return name
	}
	server, tool, found := strings.Cut(name, "_")
	if !found || server == "" || tool == "" {
		return name
	}
	return "mcp__" + server + "__" + tool
}
