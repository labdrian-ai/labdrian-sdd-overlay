package codexhooks

import (
	"regexp"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/hookwire"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
)

// ADR decisions 1 and 2: canonical Codex server names are alphanumeric/underscore.
// Compare the installed matcher against the real policy after boundary adaptation.
func TestM12bIndependentMatcherPolicyScope(t *testing.T) {
	matcher := regexp.MustCompile(memoryMatcher)
	count := 0
	for _, prefix := range []string{"", "plugin_", "plugin_engram_", "A_1_", "a__", "_", "__", "plugin_longterm_mem_"} {
		for _, server := range []string{"engram", "engramx", "longterm_mem", "longterm_memx", "context7"} {
			for _, tool := range []string{"mem_search", "mem_context", "mem_get_observation", "mem_timeline", "mem_save", "mem_update", "query", "get", "promote", "query2", "mem_search_extra", ""} {
				name := "mcp__" + prefix + server + "__" + tool
				for _, raw := range []string{name, "x" + name, name + "x", name + "\n"} {
					adapted, err := hookwire.AdaptToolName("codex", raw)
					if err != nil {
						t.Fatal(err)
					}
					if got, want := matcher.MatchString(raw), projection.GateRelevant(nil, adapted); got != want {
						t.Fatalf("%q => %q matcher=%v policy=%v", raw, adapted, got, want)
					}
					count++
				}
			}
		}
	}
	for _, name := range strings.Fields("Bash apply_patch Skill Read spawn_agent") {
		if matcher.MatchString(name) {
			t.Errorf("out-of-scope tool matched: %s", name)
		}
	}
	t.Logf("%d canonical/near-canonical names compared", count)
}
