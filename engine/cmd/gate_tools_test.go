package main

import (
	"regexp"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings"
)

// The settings matchers decide which tool calls reach the gate hook at all, so a matcher
// narrower than the gate would silently disable it. These tests pin the installed matchers to
// the tools the engine gives the gate (gatedEditTools): the gate has no list of its own.

func TestGatedEditToolsAreTheFourFileEditTools(t *testing.T) {
	want := []string{"Write", "Edit", "MultiEdit", "NotebookEdit"}
	if strings.Join(gatedEditTools(), ",") != strings.Join(want, ",") {
		t.Fatalf("gatedEditTools() = %v, want %v, in this order (the documentation lists them in it)", gatedEditTools(), want)
	}
	for _, tool := range gatedEditTools() {
		if !projection.GateRelevant(gatedEditTools(), tool) {
			t.Errorf("edit tool %q is not gate-relevant", tool)
		}
	}
}

func TestInstalledEditMatcherListsExactlyTheGatedEditTools(t *testing.T) {
	got := strings.Split(settings.ProjectionEditToolMatcher, "|")
	if strings.Join(got, "|") != strings.Join(gatedEditTools(), "|") {
		t.Errorf("edit matcher tools %v, gate edit tools %v", got, gatedEditTools())
	}
}

// installedMatchersCover reports whether a PreToolUse matcher of the projection family routes the
// tool name to the hook: an exact name of a list matcher, or a match of a pattern matcher.
func installedMatchersCover(name string) bool {
	for _, list := range []string{settings.ProjectionEditToolMatcher, settings.ProjectionProceduralSkillMatcher} {
		for _, tool := range strings.Split(list, "|") {
			if tool == name {
				return true
			}
		}
	}
	for _, pattern := range []string{settings.ProjectionMemoryQueryMatcher, settings.ProjectionMemoryGetMatcher, settings.ProjectionEngramReadMatcher} {
		if regexp.MustCompile(pattern).MatchString(name) {
			return true
		}
	}
	return false
}

func TestInstalledMatchersCoverEveryToolTheGateHasAnOpinionAbout(t *testing.T) {
	gated := []string{
		"Write", "Edit", "MultiEdit", "NotebookEdit",
		"mcp__longterm-mem__query", "mcp__plugin_x_longterm-mem__query", "mcp__a-b_c_longterm-mem__query",
		"mcp__longterm-mem__get", "mcp__plugin_x_longterm-mem__get",
		"mcp__engram__mem_search", "mcp__plugin_engram_engram__mem_search", "mcp__plugin_engram_engram__mem_context",
		"mcp__plugin_engram_engram__mem_get_observation", "mcp__plugin_engram_engram__mem_timeline",
		// The tools that can use a procedural skill (C23 M4 decision 5); the hook narrows Read to a
		// SKILL.md itself, from the path, before it reads anything.
		"Skill", "Read",
	}
	ignored := []string{
		"Bash", "skill", "Grep", "Glob", "mcp__longterm-memx__query", "mcp__a__longterm-mem__query", "longterm-mem__query",
		"mcp__longterm-memx__get", "mcp__longterm-mem__promote", "mcp__engramx__mem_search", "mcp__a__engram__mem_search",
		"mcp__plugin_engram_engram__mem_save", "mcp__plugin_engram_engram__mem_update", "mcp__engram__mem_session_summary", "",
	}
	for _, name := range append(append([]string{}, gated...), ignored...) {
		if projection.GateRelevant(gatedEditTools(), name) && !installedMatchersCover(name) {
			t.Errorf("gate is relevant for %q but no installed matcher covers it", name)
		}
	}
	for _, name := range gated {
		if !projection.GateRelevant(gatedEditTools(), name) {
			t.Errorf("test bug: the gate is not relevant for %q", name)
		}
	}
	// Not broader than needed either: the matchers add nothing the gate ignores.
	for _, name := range ignored {
		if installedMatchersCover(name) {
			t.Errorf("installed matchers cover %q, which the gate never checks", name)
		}
	}
}

// TestTheMemoryMatchersAreTheGatesOwnPatterns: a pattern matcher is the gate's tool-name pattern,
// so it is never narrower than what the hook re-checks (C23 M4 decision 6).
func TestTheMemoryMatchersAreTheGatesOwnPatterns(t *testing.T) {
	for matcher, want := range map[string]string{
		settings.ProjectionMemoryQueryMatcher: `^mcp__([A-Za-z0-9-]+_)*longterm-mem__query$`,
		settings.ProjectionMemoryGetMatcher:   `^mcp__([A-Za-z0-9-]+_)*longterm-mem__get$`,
		settings.ProjectionEngramReadMatcher:  `^mcp__([A-Za-z0-9-]+_)*engram__(mem_search|mem_context|mem_get_observation|mem_timeline)$`,
	} {
		if matcher != want {
			t.Errorf("matcher %q, want %q", matcher, want)
		}
	}
}
