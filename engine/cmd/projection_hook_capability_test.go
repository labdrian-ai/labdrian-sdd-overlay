package main

import (
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/capability"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/enginetest"
)

// limitsLine is the capability line the engine declares for target, built from the declaration as the
// projection builds it, so the test never writes the limits by hand.
func limitsLine(t *testing.T, target string) string {
	t.Helper()
	d, err := capability.Declare(target)
	if err != nil {
		t.Fatal(err)
	}
	var parts []string
	for _, c := range d.Claims {
		if c.Status != capability.Supported {
			parts = append(parts, string(c.Capability)+"="+string(c.Status))
		}
	}
	return "capability limits (" + target + "): " + strings.Join(parts, ", ")
}

// A bound session is told the limits of the runtime that runs it: OpenCode's own for --source opencode,
// Claude Code's for no source, --source claude and --source codex, as before (S10, L117).
func TestThePromptProjectionNamesTheCapabilityLimitsOfTheRuntimeThatAsks(t *testing.T) {
	w := newPhase7World(t, enginetest.Binary(t))
	w.bindRunning("proj-1", "wf-1", "odd", "authorize")
	input := hookInput(t, w.repo, "", "please continue with the task")

	cases := []struct {
		args   []string
		target string
	}{
		{nil, capability.TargetClaude},
		{[]string{"--source", "claude"}, capability.TargetClaude},
		{[]string{"--source", "codex"}, capability.TargetClaude},
		{[]string{"--source", "opencode"}, capability.TargetOpenCode},
	}
	for _, c := range cases {
		args := append([]string{"projection", "hook", "--event", "UserPromptSubmit"}, c.args...)
		ctx := decodeHookOutput(t, w.engine(w.dir, input, args...).asHook()).Context
		got := contextLineWithPrefix(ctx, "capability limits (")
		if want := limitsLine(t, c.target); got != want {
			t.Errorf("%v: the projected context says %q, want %q", c.args, got, want)
		}
	}
}
