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
// also declared untested: it is installed here but outside the regular runtime
// set, and no test exercises it (one recorded live probe aside).
//
// Skills, added in Phase 8, is the exception to "declared only": every runtime
// states what its own tests prove about carrying skills into it, and all four are
// partial. The global tier is deployed by labdrian apply (the overlay script),
// not by the engine, so each claim says so and says what that tier's tests are.
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
			supported(Projection,
				"install-hooks installs the UserPromptSubmit hook (projection hook --event UserPromptSubmit) into Claude Code settings.json, and the hook builds the bound workflow's context on every prompt. An existing install reports partial until install-hooks is re-run, and Claude Code loads hook changes only after a restart. If the binding or workflow cannot be followed, the hook projects nothing and warns. Tests feed the hook JSON and read the settings file; a real session receiving the context is not part of them.",
				"cmd:TestCheckProjectionHooks",
				"cmd:TestPhase7Acceptance",
				"cmd:TestProjectionHookOutputShape",
				"cmd:TestProjectionHookProjectsEachOpenStatus",
				"projection:TestProjectContextOfARunningWorkflowExactly",
				"runtime:TestClaudeInstallWritesTheProjectionHookFamily",
				"runtime:TestClaudeStatusIsPartialUntilInstallHooksIsRerunForTheProjectionFamily",
				"settings:TestInstall_AddsTheProjectionFamily",
			),
			partial(Dispatch,
				"The engine never starts a session by design. A repository can be bound to a workflow (stored on disk, shared by every worktree), and the hook projects that workflow into a session on every prompt. Limits: a repository is bound only by an explicit workflow bind, the projection reaches a session at its next prompt and after a Claude Code restart that loads the installed hooks, and no test observes a real session being steered.",
				"cmd:TestPhase7Acceptance",
				"cmd:TestProjectionHookProjectsEachOpenStatus",
				"cmd:TestWorkflowBindBindsAnActiveWorkflow",
				"cmd:TestWorkflowBindingIsSharedAcrossWorktreesAndInvocations",
				"projection:TestBindWritesTheBindingMarshalProduces",
				"runtime:TestClaudeInstallWritesTheProjectionHookFamily",
			),
			partial(Cancellation,
				"Pausing a bound workflow makes the PreToolUse gate deny Write, Edit, MultiEdit, and NotebookEdit at the next tool call, and the projected context tells the session it is paused; a closed workflow stops being projected and gated. install-hooks installs the gate. Limits: an in-flight tool call cannot be interrupted, Bash is never gated, the gate denies nothing when the binding or the workflow cannot be followed, hooks load after a Claude Code restart, and no test observes a real session being denied.",
				"cmd:TestPhase7Acceptance",
				"cmd:TestPreToolUseAllowsEditsOnceTheWorkflowIsResumedOrWhileItRuns",
				"cmd:TestPreToolUseDeniesTheEditToolsOfAPausedWorkflow",
				"cmd:TestPreToolUseNeverDeniesOtherToolsOfAPausedWorkflow",
				"cmd:TestPreToolUseNeverUnbindsAClosedWorkflow",
				"projection:TestGateDeniesTheEditToolsOfAPausedWorkflow",
				"projection:TestInstalledEditMatcherListsExactlyTheGatedEditTools",
				"settings:TestInstall_AddsTheProjectionFamily",
			),
			supported(Persistence,
				"The workflow log and the session binding are files under the XDG state home, outside every worktree, written atomically, so both survive process restarts and a new process reads the state a previous one left. Session transcripts are not managed.",
				"cmd:TestPhase6Acceptance_Restart",
				"cmd:TestPhase7Acceptance",
				"cmd:TestWorkflowBindingIsSharedAcrossWorktreesAndInvocations",
				"projection:TestAFreshStoreReadsWhatAnotherWrote",
			),
			supported(Restart,
				"A new process reads the binding and the workflow log from disk on every prompt, so a new session, in any worktree, gets the same context with no re-bind step; tested across separate processes. install-hooks installs the hook, and Claude Code loads hook changes only after a restart, so install and update report restart_required. Configuration reloads stay restart-gated, and a real session re-binding is not part of the tests.",
				"cmd:TestPhase7Acceptance",
				"cmd:TestProjectionHookOutputIsDeterministicAcrossCallsAndSessions",
				"cmd:TestProjectionHookOutputIsIdenticalAcrossProcesses",
				"cmd:TestProjectionHookSeesTheSameWorkflowFromEveryWorktree",
				"runtime:TestClaudeInstallWritesTheProjectionHookFamily",
			),
			partial(Authentication,
				"The presence prober reports whether Claude Code's credentials file exists, by stat only (runtime probe --target claude, and the observations a workflow records). It never reads the file and cannot prove the credentials are valid or that a session is authenticated. No lifecycle operation requires authentication.",
				"capability:TestPresenceProberReportsEachFileSignalFromTheFixtureHome",
				"capability:TestPresenceProberSourceOnlyStats",
				"cmd:TestPhase7Acceptance",
				"cmd:TestRuntimeProbeDefaultsToAllTargetsAndReportsPresenceOnly",
				"cmd:TestRuntimeProbeSourceOpensNoFile",
			),
			partial(MemoryEnforcement,
				"The PreToolUse gate denies a longterm-mem query whose project differs from the memory plan's, and every query when the plan has no project. Not enforced, because the tool input cannot verify these: a get call (it carries no project), Engram tools, and the mapping between the plan's sources and a query's sources. Writes are never blocked. install-hooks installs the gate with a matcher no narrower than the gate's tool-name pattern; hooks load after a restart; no test observes a real session being denied.",
				"cmd:TestPhase7Acceptance",
				"cmd:TestPreToolUseChecksTheProjectOfALongtermMemQuery",
				"cmd:TestPreToolUseDeniesEveryQueryWhenThePlanHasNoProject",
				"cmd:TestPreToolUseHookRunsAsASeparateProcess",
				"cmd:TestPreToolUseNeverTouchesTheMemoryToolsThatCarryNoProject",
				"projection:TestGateComparesTheQueryProjectWithThePlanProject",
				"projection:TestGateRecognizesOnlyTheLongtermMemQueryToolName",
				"projection:TestInstalledMatchersCoverEveryToolTheGateHasAnOpinionAbout",
				"settings:TestInstall_AddsTheProjectionFamily",
			),
			partial(Skills,
				"Project tier: skills install and adopt write .claude/skills and .agents/skills, replacing only files they own by hash; project-register, revise, and retire manage agent-written skills; locks serialize writers. Global tier: skills add needs an approval record; labdrian apply deploys global skills, not the engine, tested only on a fixture overlay and a sandbox home, never the real skills. Unused-skill detection is report-only and covers the project tier only. No test observes a session loading a skill.",
				"installer:TestApply_AgentsLandInNativeAgentDirs",
				"installer:TestUnrelatedSkillUnchanged",
				"skills:TestAddCore_AcceptsAGlobalSkillWithAMatchingRecord",
				"skills:TestAddCore_RefusesAGlobalSkillWithoutAValidApproval",
				"skills:TestAdoptVerb_TakesOwnershipOfAnExistingInstallAndThenInstallLeavesItBe",
				"skills:TestInstallCLI_ARunOverAHandEditedFileIsRefusedAndNothingIsWritten",
				"skills:TestInstallCLI_CopiesTheSkillIntoBothRuntimesAndSaysSo",
				"skills:TestInstall_AbsentTargetsAreCreatedInBothRuntimesAndRecorded",
				"skills:TestPlanAndExecuteProjectRetireRemovesTargetsAndLockEntry",
				"skills:TestPlanProjectRevise_HashMatchBumpsRevisionAndHash",
				"skills:TestProjectLockE2E_InstallsAndRegistrationsShareOneProjectWithoutLosingAnything",
				"skills:TestRegistryLockE2E_ConcurrentVerbsNeverLoseAnUpdate",
				"skills:TestRenderProjectRegisterCore_WritesEveryTargetAndPrintsTrustNote",
			),
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
		}, declaredOnlyClaims("Codex", presenceProbeDetail("Codex"),
			partial(Skills,
				"skills install, adopt, and project-register write project skills to .agents/skills. A live check on 2026-09-28 recorded Codex reading a project skill's body there; that is recorded evidence, not a test. Global skills are deployed to ~/.codex/skills by labdrian apply, not the engine, tested only on a fixture overlay and a sandbox home, never the real skills. No test observes Codex loading a skill, and the global tier has no unused-skill detector.",
				"installer:TestApply_AgentsLandInNativeAgentDirs",
				"installer:TestUnrelatedSkillUnchanged",
				"skills:TestInstallCLI_CopiesTheSkillIntoBothRuntimesAndSaysSo",
				"skills:TestInstall_AbsentTargetsAreCreatedInBothRuntimesAndRecorded",
				"skills:TestRenderProjectRegisterCore_WritesEveryTargetAndPrintsTrustNote",
			))...),
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
		}, declaredOnlyClaims("Pi", presenceProbeDetail("Pi"),
			partial(Skills,
				"Skills reach Pi as the labdrian-pi package: pipkg builds it from the registry's Pi-targeted skills, leaving out approval records and writers' temporary files, and install registers it. The adapter tests replace the pi CLI with a recording stub, so the real pi never loads the package. skills install and project-register never write .pi/skills; project-register says Pi loads .agents/skills only after project trust. No test observes Pi loading a skill.",
				"pipkg:TestPipkgBuild_DoesNotProjectAWritersTemporaryFile",
				"pipkg:TestPipkgBuild_DoesNotProjectTheApprovalRecord",
				"pipkg:TestPipkgBuild_SelectsPiTargetedSkills",
				"runtime:TestPiAdapter_ApplyInstallSyncCheck_WiredToPipkg",
				"skills:TestProjectTargetsMatchContractTable",
				"skills:TestRenderProjectRegisterCore_WritesEveryTargetAndPrintsTrustNote",
			))...),
	},
	{
		Target:   TargetOpenCode,
		Untested: "OpenCode is outside the regular runtime set here and no test exercises it",
		Claims: append([]Claim{
			partial(Installation,
				"Install, update, uninstall, and status are tested at file level against a temporary config root. Status reaches supported only after a running OpenCode plugin writes its active marker; tests write that marker by hand, so plugin loading by a real OpenCode is unverified.",
				"runtime:TestOpenCodeInstallUpdatePreservesLoadedMarkerUntilRestart",
				"runtime:TestOpenCodeInstallWritesPluginConfigAndRestartRequiredStatus",
				"runtime:TestOpenCodeStatusRejectsTamperedPromptConfig",
				"runtime:TestOpenCodeStatusSupportedWhenActiveMarkerMatchesHash",
				"runtime:TestOpenCodeUninstallRemovesPluginAndConfig",
			),
		}, declaredOnlyClaims("OpenCode", "Not implemented: the engine does not check whether OpenCode has credentials, and the presence prober has no credentials check for it.",
			partial(Skills,
				"labdrian apply copies global skills into OpenCode's skills directory, tested only on a fixture overlay and a sandbox home, never the real skills. A live OpenCode 1.18.31 session on 2026-09-30 discovered project skills under .agents/skills and .claude/skills (recorded, not tested); the global tier was not observed live, its names also existing in several global directories. No test observes OpenCode loading a skill, and no test connects the project tier to OpenCode.",
				"installer:TestApply_AgentsLandInNativeAgentDirs",
				"installer:TestUnrelatedSkillUnchanged",
			))...),
	},
}

// declaredOnlyClaims returns the claims after installation for a runtime that
// Phase 7 declares without implementing: the seven Phase 7 capabilities, every
// one unsupported with the limit written, and then the runtime's own skills claim,
// which is the one capability Phase 8 states from the tests that exist. runtime is
// the display name used in the sentences, and authDetail is the authentication
// limit, which differs by runtime because only some runtimes have a credentials
// presence check.
func declaredOnlyClaims(runtime, authDetail string, skills Claim) []Claim {
	const scope = " This runtime is declared only: Phase 7 writes no code for it."
	return []Claim{
		unsupported(Projection, "No code projects the active workflow (Goal, Profile, stage) into a "+runtime+" session."+scope),
		unsupported(Dispatch, "Not implemented. The engine never starts sessions by design, and dispatch means projecting the active workflow into an existing session, which does not exist for "+runtime+"."+scope),
		unsupported(Cancellation, "Not implemented: no workflow transition reaches a "+runtime+" session. Hard cancel of an in-flight tool call is out of scope."+scope),
		unsupported(Persistence, "Not implemented: no binding between a "+runtime+" session and a workflow is stored."+scope),
		unsupported(Restart, "Not implemented: a new "+runtime+" session does not re-bind to an on-disk workflow."+scope),
		unsupported(Authentication, authDetail+scope),
		unsupported(MemoryEnforcement, "Not implemented: no memory plan is projected into or enforced in a "+runtime+" session."+scope),
		skills,
	}
}

// presenceProbeDetail is the authentication limit of a declared-only runtime
// that the presence prober has a credentials signal for. The signal exists, but
// it is not part of an implementation for the runtime: nothing reads the file,
// nothing checks the runtime is authenticated, and nothing acts on the result.
func presenceProbeDetail(runtime string) string {
	return "The presence prober reports whether " + runtime + "'s credentials file exists, by stat only (runtime probe), " +
		"but that check is not part of an implementation for " + runtime + ": nothing reads the file, and a present file " +
		"does not prove " + runtime + " is authenticated."
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
