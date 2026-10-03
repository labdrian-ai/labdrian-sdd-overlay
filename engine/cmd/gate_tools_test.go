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
		if projection.GateRelevant(gatedEditTools(), name) && !covered(name) {
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
