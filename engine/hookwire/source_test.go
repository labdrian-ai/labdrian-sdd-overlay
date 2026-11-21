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
			if err == nil || !strings.Contains(err.Error(), "(supported: claude, codex, opencode)") {
				t.Fatalf("unknown source: %v; want error naming claude, codex, opencode", err)
			}
		})
	}
}

func TestAdaptToolNameOpencode(t *testing.T) {
	for _, tt := range []struct{ name, in, want string }{
		{"shell is Bash", "shell", "Bash"},
		{"patch is Edit", "patch", "Edit"},
		{"server_tool is an mcp name", "memorycard_mem_search", "mcp__memorycard__mem_search"},
		{"a hyphenated server keeps its hyphen", "longterm-mem_query", "mcp__longterm-mem__query"},
		{"the server ends at the first underscore", "long_term_query", "mcp__long__term_query"},
		{"execute is not mapped", "execute", "execute"},
		{"a built-in with an underscore is not an mcp name", "experimental_image_generate", "experimental_image_generate"},
		{"read is Read", "read", "Read"},
		{"skill is Skill", "skill", "Skill"},
		{"glob passes unchanged", "glob", "glob"},
		{"grep passes unchanged", "grep", "grep"},
		{"webfetch passes unchanged", "webfetch", "webfetch"},
		{"websearch passes unchanged", "websearch", "websearch"},
		{"edit passes unchanged", "edit", "edit"},
		{"write passes unchanged", "write", "write"},
		{"subagent passes unchanged", "subagent", "subagent"},
		{"an already mapped name passes", "mcp__engram__mem_save", "mcp__engram__mem_save"},
		{"Claude spellings pass", "Bash", "Bash"},
		{"empty", "", ""},
		{"no server part", "_tool", "_tool"},
		{"no tool part", "server_", "server_"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := AdaptToolName("opencode", tt.in)
			if err != nil || got != tt.want {
				t.Fatalf("AdaptToolName(opencode, %q) = %q, %v; want %q, nil", tt.in, got, err, tt.want)
			}
		})
	}
}

// claude and codex keep their old maps: an opencode name is opaque to them.
func TestAdaptToolNameOpencodeNamesAreOpaqueToOtherSources(t *testing.T) {
	for _, source := range []string{"claude", "codex"} {
		for _, name := range []string{"shell", "patch", "read", "skill", "memorycard_mem_search", "execute"} {
			got, err := AdaptToolName(source, name)
			if err != nil || got != name {
				t.Errorf("AdaptToolName(%q, %q) = %q, %v; want unchanged", source, name, got, err)
			}
		}
	}
}
