package hookwire

import (
	"strings"
	"testing"
)

func TestAdaptToolName(t *testing.T) {
	names := []string{
		"mcp__longterm_mem__query", "mcp__longterm_mem__get",
		"mcp__engram__mem_search", "mcp__engram__mem_context",
		"mcp__engram__mem_get_observation", "mcp__engram__mem_timeline", "mcp__engram__mem_save",
		"Bash", "apply_patch", "spawn_agent", "", "unknown",
		"mcp__longterm_mem__promote", "mcp__longterm_mem__query2",
		"mcp__plugin_longterm_mem__get", "mcp__longterm-mem__query",
	}
	for _, source := range []string{"claude", "codex"} {
		for _, name := range names {
			t.Run(source+"/"+name, func(t *testing.T) {
				want := name
				if source == "codex" {
					switch name {
					case "mcp__longterm_mem__query":
						want = "mcp__longterm-mem__query"
					case "mcp__longterm_mem__get":
						want = "mcp__longterm-mem__get"
					}
				}
				got, err := AdaptToolName(source, name)
				if err != nil || got != want {
					t.Fatalf("AdaptToolName(%q, %q) = %q, %v; want %q, nil", source, name, got, err, want)
				}
			})
		}
	}
	for _, source := range []string{"", "other", "Codex"} {
		t.Run("unknown/"+source, func(t *testing.T) {
			_, err := AdaptToolName(source, "Bash")
			if err == nil || !strings.Contains(err.Error(), "claude, codex") {
				t.Fatalf("unknown source: %v; want error naming claude, codex", err)
			}
		})
	}
}
