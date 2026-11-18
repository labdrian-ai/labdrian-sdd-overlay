package capability_test

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/capability"
)

func claimsOf(t *testing.T, target string) map[capability.Capability]capability.Claim {
	t.Helper()
	d, err := capability.Declare(target)
	if err != nil {
		t.Fatal(err)
	}
	out := make(map[capability.Capability]capability.Claim)
	for _, c := range d.Claims {
		out[c.Capability] = c
	}
	return out
}

// TestCodexDeclarationIsByteIdenticalToTheGolden pins S14's "the Codex claims
// stay byte-identical": the helper that builds them was shared with OpenCode,
// so moving OpenCode off it must not move one byte of Codex's report. The
// golden is the output of `runtime capabilities --target codex` before T16.
func TestCodexDeclarationIsByteIdenticalToTheGolden(t *testing.T) {
	d, err := capability.Declare(capability.TargetCodex)
	if err != nil {
		t.Fatal(err)
	}
	got, err := capability.Report{Version: capability.ReportVersion, Declarations: []capability.Declaration{d}}.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("testdata/codex-declaration.golden")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("the Codex declaration changed:\n%s", got)
	}
}

// TestOpenCodeIsNoLongerDeclaredOnly pins S14: OpenCode has an adapter, two
// plugins and an engine source of its own, so its claims are no longer the
// shared declared-only list. Each claim the tests now back is partial and
// cites them; the rest says why it is still unsupported.
func TestOpenCodeIsNoLongerDeclaredOnly(t *testing.T) {
	claims := claimsOf(t, capability.TargetOpenCode)
	for name, want := range map[capability.Capability]capability.Status{
		capability.Installation:      capability.Partial,
		capability.Projection:        capability.Partial,
		capability.Dispatch:          capability.Partial,
		capability.Cancellation:      capability.Partial,
		capability.MemoryEnforcement: capability.Partial,
		capability.Skills:            capability.Partial,
		capability.Persistence:       capability.Unsupported,
		capability.Restart:           capability.Unsupported,
		capability.Authentication:    capability.Unsupported,
	} {
		if got := claims[name].Status; got != want {
			t.Errorf("opencode %s is %q, want %q", name, got, want)
		}
	}
	for name, c := range claims {
		if strings.Contains(c.Detail, "declared only") || strings.Contains(c.Detail, "Phase 7 writes no code") {
			t.Errorf("opencode %s still says it is declared only: %q", name, c.Detail)
		}
		if c.Status == capability.Unsupported && !strings.Contains(c.Detail, "T17") {
			t.Errorf("opencode %s is unsupported without pointing at T17: %q", name, c.Detail)
		}
	}
}

// TestOpenCodeUntestedReasonNamesWhatNeedsAModel pins the reason: the file-level
// tests and the live load test exist, and what stays untested is whatever needs a
// model session, deferred to T17.
func TestOpenCodeUntestedReasonNamesWhatNeedsAModel(t *testing.T) {
	d, err := capability.Declare(capability.TargetOpenCode)
	if err != nil {
		t.Fatal(err)
	}
	for _, phrase := range []string{"model session", "T17", "binary", "skip", "load"} {
		if !strings.Contains(d.Untested, phrase) {
			t.Errorf("untested reason %q does not state %q", d.Untested, phrase)
		}
	}
	if strings.Contains(d.Untested, "outside the regular runtime set") || strings.Contains(d.Untested, "no test exercises") {
		t.Errorf("untested reason %q still describes OpenCode as unexercised", d.Untested)
	}
}

// TestOpenCodeClaimsCiteTheTestsThatBackThem pins which existing test backs which
// claim, and the limit each claim states about the part no test reaches.
func TestOpenCodeClaimsCiteTheTestsThatBackThem(t *testing.T) {
	claims := claimsOf(t, capability.TargetOpenCode)
	for _, tc := range []struct {
		capability capability.Capability
		tests      []string
		phrases    []string
	}{
		{capability.Installation,
			[]string{
				"runtime:TestOpenCodeInstallWritesTheMemorycardPluginAndItsRecord",
				"runtime:TestOpenCodeStatusIsSupportedOnlyWhenBothPluginsAreActive",
				"runtime:TestOpenCodeLoadsTheInstalledPluginAndItWritesItsActiveMarker",
			},
			[]string{"parity", "memorycard", "plugin", "loads both installed plugins"}},
		{capability.Projection,
			[]string{
				"runtime:TestOpenCodeBridgePutsTheSessionStartContextIntoEveryModelCallOfTheSession",
				"runtime:TestOpenCodeBridgeCarriesThePromptContextInTheUsersPromptAndNeverInTheSystemPrompt",
				"runtime:TestOpenCodeBridgeKeepsTheSystemPromptByteStableAcrossPrompts",
				"cmd:TestTheSameSessionIdUnderTwoSourcesIsTwoSessions",
			},
			[]string{"--source opencode", "SessionStart", "UserPromptSubmit", "recording engine", "no test observes a model session", "T17"}},
		{capability.Dispatch,
			[]string{
				"runtime:TestOpenCodeBridgeForcesAnotherTurnWhenTheEngineBlocksTheStop",
				"runtime:TestOpenCodeBridgeLetsTheSessionStopWhenTheEngineDoesNotBlock",
			},
			[]string{"Stop", "experimental", "at each prompt whose block changed (T35)", "recording engine", "no workflow binding is exercised", "no test observes a model session", "T17"}},
		{capability.Cancellation,
			[]string{
				"hookwire:TestAdaptToolNameOpencode",
				"runtime:TestOpenCodeBridgeDeniesACallTheEngineDeniesWithToolError",
			},
			[]string{
				"Tool.Error", "reason of the engine's PreToolUse denial", "recording engine",
				"the denied call then does not run is an L10 recording, unproven with a model until T17",
				"patch to Edit (hookwire test)", "proven for Claude Code tool names, not through --source opencode or patch",
				"in-flight tool call cannot be interrupted",
			}},
		{capability.MemoryEnforcement,
			[]string{
				"cmd:TestCodexSourceProcesses",
				"hookwire:TestAdaptToolNameOpencode",
				"runtime:TestMemorycardPluginPutsTheMemorycardContextOnTheSystemPromptOfANewSession",
				"runtime:TestMemorycardPluginCapturesTheFinalTextOfASubagentOnStdin",
			},
			[]string{
				"OpencodeAdaptsBeforeGate subtest of cmd:TestCodexSourceProcesses", "execute (not mapped) stays silent",
				"sends each subagent's final text to memorycard capture (fake memorycard)",
				"No memory write is blocked", "a saved Key Learnings item and a real session are untested (T17)",
			}},
	} {
		c := claims[tc.capability]
		for _, ref := range tc.tests {
			if !slices.Contains(c.Tests, ref) {
				t.Errorf("opencode %s does not cite %s: %v", tc.capability, ref, c.Tests)
			}
		}
		for _, phrase := range tc.phrases {
			if !strings.Contains(c.Detail, phrase) {
				t.Errorf("opencode %s detail %q does not state %q", tc.capability, c.Detail, phrase)
			}
		}
	}
}

// TestOpenCodeSkillsClaimIsTheOneT14Wrote pins that the skills claim T14 wrote
// (no copy; OpenCode reads ~/.claude/skills) survived moving OpenCode off the
// shared helper.
func TestOpenCodeSkillsClaimIsTheOneT14Wrote(t *testing.T) {
	c := claimsOf(t, capability.TargetOpenCode)[capability.Skills]
	for _, ref := range []string{
		"runtime:TestOpenCodeInstallWritesNoSkillAndSaysNothingWhenThereAreNoCopies",
		"runtime:TestOpenCodeReportsLabdrianSkillsFoundWhereOpenCodeReadsThemFirst",
	} {
		if !slices.Contains(c.Tests, ref) {
			t.Errorf("opencode skills does not cite %s: %v", ref, c.Tests)
		}
	}
}

// TestOpenCodeClaimsDropTheWordingTheTestsDoNotProve pins the correction of T16:
// no claim says the denied call does not run (a recording, not a test), that the
// bridge projects at every prompt (T35 omits a repeated block), or that Key
// Learnings are captured (the test shows only the final text reaching a fake
// memorycard), and the live test is said to load both plugins.
func TestOpenCodeClaimsDropTheWordingTheTestsDoNotProve(t *testing.T) {
	d, err := capability.Declare(capability.TargetOpenCode)
	if err != nil {
		t.Fatal(err)
	}
	texts := []string{d.Untested}
	for _, c := range d.Claims {
		texts = append(texts, c.Detail)
	}
	for _, stale := range []string{
		"and the call does not run",
		"projects it at session start and at each prompt",
		"captures subagent Key Learnings",
		"loads the installed parity plugin",
		"loads the installed plugin",
		"repository bound to a workflow",
	} {
		for _, text := range texts {
			if strings.Contains(text, stale) {
				t.Errorf("opencode text %q still says %q", text, stale)
			}
		}
	}
	if !strings.Contains(d.Untested, "loads both installed plugins") {
		t.Errorf("untested reason %q does not say the live test loads both plugins", d.Untested)
	}
}

// TestClaudeSkillsClaimKeepsTheUnrelatedSkillTest pins that the claim keeps the
// test that asserts the byte-equal deploy of a skill nobody asked to touch,
// which c1a92bb6 replaced by the OpenCode references.
func TestClaudeSkillsClaimKeepsTheUnrelatedSkillTest(t *testing.T) {
	c := skillsClaim(t, capability.TargetClaude)
	if !slices.Contains(c.Tests, "installer:TestUnrelatedSkillUnchanged") {
		t.Errorf("claude skills does not cite installer:TestUnrelatedSkillUnchanged: %v", c.Tests)
	}
}

// TestNoOtherRuntimeCitesAnOpenCodeTest pins that the OpenCode tests back the
// OpenCode claims alone: until T16 four of them sat on the Claude Code skills
// claim, which said nothing about OpenCode.
func TestNoOtherRuntimeCitesAnOpenCodeTest(t *testing.T) {
	for _, d := range capability.All() {
		if d.Target == capability.TargetOpenCode {
			continue
		}
		for _, c := range d.Claims {
			for _, ref := range c.Tests {
				if strings.Contains(ref, "OpenCode") || strings.Contains(ref, "Opencode") {
					t.Errorf("%s %s cites the OpenCode test %s", d.Target, c.Capability, ref)
				}
			}
		}
	}
}
