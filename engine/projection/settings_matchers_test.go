package projection_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings"
)

// The settings matchers decide which tool calls reach the gate hook at all, so a
// matcher narrower than the gate would silently disable it. These tests pin the
// installed matchers to the gate's own tool set.

func TestInstalledEditMatcherListsExactlyTheGatedEditTools(t *testing.T) {
	got := strings.Split(settings.ProjectionEditToolMatcher, "|")
	want := projection.EditTools()
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("edit matcher tools %v, gate edit tools %v", got, want)
	}
}

func TestInstalledMatchersCoverEveryToolTheGateHasAnOpinionAbout(t *testing.T) {
	memory := regexp.MustCompile(settings.ProjectionMemoryQueryMatcher)
	covered := func(name string) bool {
		for _, tool := range strings.Split(settings.ProjectionEditToolMatcher, "|") {
			if tool == name {
				return true
			}
		}
		return memory.MatchString(name)
	}
	for _, name := range []string{
		"Write", "Edit", "MultiEdit", "NotebookEdit",
		"mcp__longterm-mem__query", "mcp__plugin_x_longterm-mem__query", "mcp__a-b_c_longterm-mem__query",
		"Bash", "Read", "Grep", "mcp__longterm-mem__get", "mcp__longterm-memx__query", "mcp__a__longterm-mem__query",
		"longterm-mem__query", "mcp__plugin_engram_engram__mem_save", "",
	} {
		if projection.GateRelevant(name) && !covered(name) {
			t.Errorf("gate is relevant for %q but no installed matcher covers it", name)
		}
	}
	// Not broader than needed either: the matchers add nothing the gate ignores.
	for _, name := range []string{"Bash", "Read", "mcp__longterm-mem__get", "mcp__longterm-memx__query"} {
		if covered(name) {
			t.Errorf("installed matchers cover %q, which the gate never checks", name)
		}
	}
}
