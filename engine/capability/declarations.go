package capability

import (
	"fmt"
	"strings"
)

// declarations is the one table of what each runtime declares, listed in
// Targets() order. It is the only place a claim is written; edit it here.
//
// To upgrade a claim, change its status, name the tests that prove it, and
// add or update those tests in the same commit. Two guards keep the table
// honest: Validate refuses a supported or partial claim that names no test,
// and TestDeclaredEvidenceExists (through CheckEvidence) refuses a reference
// to a test that does not exist. A claim is never upgraded on the strength of
// a runtime being known to work: only a named, existing test counts.
//
// Phase 7 builds the Claude Code implementation task by task, and a Claude
// Code claim is upgraded in the commit that adds the tests proving it, so the
// table states only what the tests at that commit prove. Codex, Pi, and
// OpenCode are declared only: they state what is proven today (installation)
// and an honest unsupported, with the limit written, for the rest. OpenCode is
// also declared untested: it is not used on this machine and cannot be
// exercised here.
var declarations = []Declaration{
	{
		Target: TargetClaude,
		Claims: []Claim{
			supported(Installation,
				"Install, update, uninstall, and status of the overlay's lifecycle hook families in Claude Code settings.json. Hook changes load only after a Claude Code restart, so install and update report restart_required.",
				"runtime:TestClaudeInstallWritesLifecycleHooksAndReportsSupportedStatus",
				"runtime:TestClaudeStatusFailsWhenSettingsIsMalformed",
				"runtime:TestClaudeStatusRequiresFullLifecycleState",
				"runtime:TestClaudeUninstallRemovesOwnedHooksAndReturnsUnhealthyStatus",
				"runtime:TestClaudeUpdateRefreshesLifecycleAndKeepsSupportedStatus",
			),
			partial(Projection,
				"The UserPromptSubmit hook (projection hook --event UserPromptSubmit) builds the bound workflow's context (workflow, profile, status, stages, memory plan); tests feed it hook JSON. Limit: install-hooks does not install the hook into Claude Code settings yet, and hook changes need a Claude Code restart, so no session receives the context.",
				"cmd:TestProjectionHookOutputShape",
				"cmd:TestProjectionHookProjectsEachOpenStatus",
				"projection:TestProjectContextOfARunningWorkflowExactly",
			),
			partial(Dispatch,
				"The engine never starts a session by design. A repository can be bound to a workflow (stored on disk, shared by every worktree), and the hook projects that workflow into a session on every prompt. Limit: install-hooks does not install the hook into Claude Code settings yet, so no session is steered.",
				"cmd:TestProjectionHookProjectsEachOpenStatus",
				"cmd:TestWorkflowBindBindsAnActiveWorkflow",
				"cmd:TestWorkflowBindingIsSharedAcrossWorktreesAndInvocations",
				"projection:TestBindWritesTheBindingMarshalProduces",
			),
			unsupported(Cancellation, "Not implemented yet: the hook tells a session when its workflow is paused (a notice) or closed (a note, and the binding is removed), but no gate denies any tool, so a session can still act, and the hook is not installed yet. Hard cancel of an in-flight tool call is out of scope."),
			supported(Persistence,
				"The workflow log and the session binding are files under the XDG state home, outside every worktree, written atomically, so both survive process restarts and a new process reads the state a previous one left. Session transcripts are not managed.",
				"cmd:TestPhase6Acceptance_Restart",
				"cmd:TestWorkflowBindingIsSharedAcrossWorktreesAndInvocations",
				"projection:TestAFreshStoreReadsWhatAnotherWrote",
			),
			partial(Restart,
				"A new process reads the binding and the workflow log from disk on every prompt, so a new session, in any worktree, gets the same context with no re-bind step; tested across separate processes. Limit: install-hooks does not install the hook into Claude Code settings yet, and hook changes need a restart, so no real session re-binds yet.",
				"cmd:TestProjectionHookOutputIsDeterministicAcrossCallsAndSessions",
				"cmd:TestProjectionHookOutputIsIdenticalAcrossProcesses",
				"cmd:TestProjectionHookSeesTheSameWorkflowFromEveryWorktree",
			),
			unsupported(Authentication, "Not implemented yet: the engine does not check whether Claude Code has credentials."),
			unsupported(MemoryEnforcement, "Not implemented yet: the hook's context states the memory plan (scope, sources, filters, write none), but nothing enforces it: no tool call is checked against the plan, and the hook is not installed yet."),
		},
	},
	{
		Target: TargetCodex,
		Claims: append([]Claim{
			partial(Installation,
				"Install, update, and uninstall manage one lifecycle manifest (labdrian-runtime-lifecycle.json) and leave unrelated files alone. Status never exceeds partial: whether a running Codex process activates or reloads the manifest cannot be verified.",
				"runtime:TestCodexInstallRejectsUnownedOrMalformedExistingManifest",
				"runtime:TestCodexInstallWritesManifestAndPreservesUnrelatedFiles",
				"runtime:TestCodexStatusReportsPartialWithActivationUncertainty",
				"runtime:TestCodexUninstallRemovesManifestWithoutTouchingUnrelatedFiles",
				"runtime:TestCodexUpdateRefreshesManagedManifest",
			),
		}, declaredOnlyClaims("Codex")...),
	},
	{
		Target: TargetPi,
		Claims: append([]Claim{
			supported(Installation,
				"Install builds the labdrian-pi package and registers it with the pi CLI, uninstall runs pi remove, and status reports supported only when all five owned entries are proven. Every test replaces the pi CLI with a recording stub, so the real pi binary is never run.",
				"runtime:TestPiAdapter_ApplyInstallSyncCheck_WiredToPipkg",
				"runtime:TestPiAdapter_InstallNoShellInjection",
				"runtime:TestPiAdapter_StatusTriangulatesAllOwnedEntries",
				"runtime:TestPiAdapter_UninstallUsesRemoveNotUninstall",
			),
		}, declaredOnlyClaims("Pi")...),
	},
	{
		Target:   TargetOpenCode,
		Untested: "OpenCode is not used on this machine and cannot be exercised here",
		Claims: append([]Claim{
			partial(Installation,
				"Install, update, uninstall, and status are tested at file level against a temporary config root. Status reaches supported only after a running OpenCode plugin writes its active marker; tests write that marker by hand, so plugin loading by a real OpenCode is unverified.",
				"runtime:TestOpenCodeInstallUpdatePreservesLoadedMarkerUntilRestart",
				"runtime:TestOpenCodeInstallWritesPluginConfigAndRestartRequiredStatus",
				"runtime:TestOpenCodeStatusRejectsTamperedPromptConfig",
				"runtime:TestOpenCodeStatusSupportedWhenActiveMarkerMatchesHash",
				"runtime:TestOpenCodeUninstallRemovesPluginAndConfig",
			),
		}, declaredOnlyClaims("OpenCode")...),
	},
}

// declaredOnlyClaims returns the seven claims after installation for a
// runtime that Phase 7 declares without implementing: every one unsupported,
// with the limit written. runtime is the display name used in the sentences.
func declaredOnlyClaims(runtime string) []Claim {
	const scope = " This runtime is declared only: Phase 7 writes no code for it."
	return []Claim{
		unsupported(Projection, "No code projects the active workflow (Goal, Profile, stage) into a "+runtime+" session."+scope),
		unsupported(Dispatch, "Not implemented. The engine never starts sessions by design, and dispatch means projecting the active workflow into an existing session, which does not exist for "+runtime+"."+scope),
		unsupported(Cancellation, "Not implemented: no workflow transition reaches a "+runtime+" session. Hard cancel of an in-flight tool call is out of scope."+scope),
		unsupported(Persistence, "Not implemented: no binding between a "+runtime+" session and a workflow is stored."+scope),
		unsupported(Restart, "Not implemented: a new "+runtime+" session does not re-bind to an on-disk workflow."+scope),
		unsupported(Authentication, "Not implemented: the engine does not check whether "+runtime+" has credentials."+scope),
		unsupported(MemoryEnforcement, "Not implemented: no memory plan is projected into or enforced in a "+runtime+" session."+scope),
	}
}

func supported(c Capability, detail string, tests ...string) Claim {
	return Claim{Capability: c, Status: Supported, Tests: tests, Detail: detail}
}

func partial(c Capability, detail string, tests ...string) Claim {
	return Claim{Capability: c, Status: Partial, Tests: tests, Detail: detail}
}

func unsupported(c Capability, detail string) Claim {
	return Claim{Capability: c, Status: Unsupported, Detail: detail}
}

// Declare returns the declaration for one target. The result is a copy:
// editing it never changes the table.
func Declare(target string) (Declaration, error) {
	for _, d := range declarations {
		if d.Target == target {
			return d.clone(), nil
		}
	}
	return Declaration{}, fmt.Errorf("unknown target %q (expected %s)", target, strings.Join(Targets(), ", "))
}

// All returns every declaration in Targets() order. The result is a copy:
// editing it never changes the table.
func All() []Declaration {
	out := make([]Declaration, len(declarations))
	for i, d := range declarations {
		out[i] = d.clone()
	}
	return out
}

// clone returns a deep copy of d in which every Tests slice is non-nil, so a
// consumer can range over it without a nil check and a report decoded from
// JSON compares equal to the declaration it was printed from.
func (d Declaration) clone() Declaration {
	out := d
	out.Claims = make([]Claim, len(d.Claims))
	for i, c := range d.Claims {
		c.Tests = append(make([]string, 0, len(c.Tests)), c.Tests...)
		out.Claims[i] = c
	}
	return out
}
