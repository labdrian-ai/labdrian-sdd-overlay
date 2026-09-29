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
// Phase 7 gives Claude Code the full implementation and declares the other
// three runtimes only, so Codex, Pi, and OpenCode state what is proven today
// (installation) and an honest unsupported, with the limit written, for the
// rest. OpenCode is also declared untested: it is not used on this machine
// and cannot be exercised here.
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
			unsupported(Projection, "Not implemented yet: no hook projects the active workflow (Goal, Profile, stage) into a Claude Code session."),
			unsupported(Dispatch, "Not implemented yet. The engine never starts a session by design; dispatch will mean projecting the active workflow into a session the user already opened, and no projection exists."),
			unsupported(Cancellation, "Not implemented yet: pausing, abandoning, or closing a workflow does not change what a session is told or allowed to do. Hard cancel of an in-flight tool call is out of scope."),
			unsupported(Persistence, "Not implemented yet: no binding between a session and a workflow is stored, so nothing ties a session to an on-disk workflow."),
			unsupported(Restart, "Not implemented yet: a new session does not re-bind to an on-disk workflow. The adapter reports restart_required only after configuration changes."),
			unsupported(Authentication, "Not implemented yet: the engine does not check whether Claude Code has credentials."),
			unsupported(MemoryEnforcement, "Not implemented yet: memory plans are resolved by 'memory plan', but nothing projects them into a session or enforces them."),
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
	const scope = " Phase 7 implements it for Claude Code only."
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
