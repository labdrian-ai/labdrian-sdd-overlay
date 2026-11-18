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
// and TestDeclaredEvidenceExists (CheckEvidence over capabilitytest.NewCatalog) refuses a reference
// to a test that does not exist. A claim is never upgraded on the strength of
// a runtime being known to work: only a named, existing test counts.
//
// Phase 7 builds the Claude Code implementation task by task, and a Claude
// Code claim is upgraded in the commit that adds the tests proving it, so the
// table states only what the tests at that commit prove. Codex and Pi are
// declared only: they state what is proven today (installation) and an honest
// unsupported, with the limit written, for the rest, except for Codex's partial
// MCP memory gate (C23 M12b). OpenCode has claims of its own (T16): its adapter,
// both plugins and the engine source are tested at file level, under node and
// against a real opencode binary when there is one, and it is declared untested
// only for what needs a model session, which T17 observes.
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
				"Install, update, uninstall, and status of the overlay's lifecycle hook families in Claude Code settings.json, among them the Stop hook that asks for each thing the first commit with unfinished review steps misses, once per session and commit, and the builder guard on the Agent tool that denies a launch without a task the shaper designed (a speed bump), and the \"agent\": \"foreman\" and \"outputStyle\": \"labdrian\" keys, each owned by value, never overwriting another. Install and update report restart_required.",
				"cmd:TestMergeSettingsKeepsAForeignOutputStyleAndSaysSoOnStderr",
				"cmd:TestUninstallHooksRemovesTheOutputStyleKeyOnlyWhenItIsLabdrian",
				"runtime:TestClaudeInstallAndUpdateCarryTheNoticeOfAForeignMainAgentValue",
				"runtime:TestClaudeInstallAndUpdateKeepAForeignOutputStyleAndSayIt",
				"runtime:TestClaudeInstallAndUpdateWriteTheOutputStyleKey",
				"runtime:TestClaudeInstallWritesLifecycleHooksAndReportsSupportedStatus",
				"runtime:TestClaudeInstallWritesTheBuilderGuard",
				"runtime:TestClaudeInstallWritesTheReviewStopHook",
				"runtime:TestClaudeStatusFailsWhenSettingsIsMalformed",
				"runtime:TestClaudeStatusIsPartialAndNamesAForeignMainAgentValue",
				"runtime:TestClaudeStatusIsPartialAndNamesAForeignOutputStyleValue",
				"runtime:TestClaudeStatusIsPartialUntilInstallHooksIsRerunForTheBuilderGuard",
				"runtime:TestClaudeStatusIsPartialUntilInstallHooksIsRerunForTheReviewStopHook",
				"runtime:TestClaudeStatusIsPartialWhenTheBuilderGuardEntryDrifted",
				"runtime:TestClaudeStatusIsPartialWhenTheMainAgentKeyIsMissing",
				"runtime:TestClaudeStatusIsPartialWhenTheOutputStyleKeyIsMissing",
				"runtime:TestClaudeStatusIsPartialWhenTheReviewStopHookEntryDrifted",
				"runtime:TestClaudeStatusRequiresFullLifecycleState",
				"runtime:TestClaudeUninstallRemovesOwnedHooksAndReturnsUnhealthyStatus",
				"runtime:TestClaudeUninstallRemovesTheBuilderGuardAndKeepsForeignAgentEntries",
				"runtime:TestClaudeUninstallRemovesTheOutputStyleKeyOnlyWhenItIsLabdrian",
				"runtime:TestClaudeUninstallRemovesTheReviewStopHookAndKeepsForeignStopEntries",
				"runtime:TestClaudeUpdateRefreshesLifecycleAndKeepsSupportedStatus",
				"status:TestAForeignOutputStyleIsReportedAsAConflict",
				"status:TestTheBuilderGuardNamesWhatIsMissingOrDriftedAndTheLimitOfAGuard",
				"status:TestTheReviewStopHookNamesWhatIsMissingOrDriftedAndAsksForARestart",
			),
			supported(Projection,
				"install-hooks installs the UserPromptSubmit hook (projection hook --event UserPromptSubmit) into Claude Code settings.json, and the hook builds the bound workflow's context on every prompt. An existing install reports partial until install-hooks is re-run, and Claude Code loads hook changes only after a restart. If the binding or workflow cannot be followed, the hook projects nothing and warns. Tests feed the hook JSON and read the settings file; a real session receiving the context is not part of them.",
				"cmd:TestPhase7Acceptance",
				"cmd:TestProjectionHookOutputShape",
				"cmd:TestProjectionHookProjectsEachOpenStatus",
				"cmd:TestSessionStartGivesTheInstructionsOfTheBoundWorkflow",
				"projection:TestProjectContextOfARunningWorkflowExactly",
				"runtime:TestClaudeInstallWritesTheProjectionHookFamily",
				"runtime:TestClaudeStatusIsPartialUntilInstallHooksIsRerunForTheProjectionFamily",
				"settings/settingsfile:TestInstall_AddsTheProjectionFamily",
				"status:TestTheProjectionHooksNameWhatIsMissingOrDriftedAndAskForARestart",
			),
			partial(Dispatch,
				"The engine never starts a session by design. A repository can be bound to a workflow (stored on disk, shared by every worktree), and the hook projects that workflow into a session on every prompt. Limits: a repository is bound only by an explicit workflow bind, the projection reaches a session at its next prompt and after a Claude Code restart that loads the installed hooks, and no test observes a real session being steered.",
				"cmd:TestPhase7Acceptance",
				"cmd:TestProjectionHookProjectsEachOpenStatus",
				"cmd:TestWorkflowBindBindsAnActiveWorkflow",
				"cmd:TestWorkflowBindingIsSharedAcrossWorktreesAndInvocations",
				"projection/fsstore:TestBindWritesTheBindingMarshalProduces",
				"runtime:TestClaudeInstallWritesTheProjectionHookFamily",
			),
			partial(Cancellation,
				"Pausing a bound workflow makes the PreToolUse gate deny Write, Edit, MultiEdit, and NotebookEdit at the next tool call, and the projected context tells the session it is paused; a closed workflow stops being projected and gated. install-hooks installs the gate. Limits: an in-flight tool call cannot be interrupted, Bash is never gated, the gate denies nothing when the binding or the workflow cannot be followed, hooks load after a Claude Code restart, and no test observes a real session being denied.",
				"cmd:TestInstalledEditMatcherListsExactlyTheGatedEditTools",
				"cmd:TestPhase7Acceptance",
				"cmd:TestPreToolUseAllowsEditsOnceTheWorkflowIsResumedOrWhileItRuns",
				"cmd:TestPreToolUseDeniesTheEditToolsOfAPausedWorkflow",
				"cmd:TestPreToolUseNeverDeniesOtherToolsOfAPausedWorkflow",
				"cmd:TestPreToolUseNeverUnbindsAClosedWorkflow",
				"projection:TestGateDeniesTheEditToolsOfAPausedWorkflow",
				"settings/settingsfile:TestInstall_AddsTheProjectionFamily",
			),
			supported(Persistence,
				"The workflow log and the session binding are files under the XDG state home, outside every worktree, written atomically, so both survive process restarts and a new process reads the state a previous one left. Session transcripts are not managed.",
				"cmd:TestPhase6Acceptance_Restart",
				"cmd:TestPhase7Acceptance",
				"cmd:TestWorkflowBindingIsSharedAcrossWorktreesAndInvocations",
				"projection/fsstore:TestAFreshStoreReadsWhatAnotherWrote",
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
				"capability/presence:TestPresenceProberReportsEachFileSignalFromTheFixtureHome",
				"capability/presence:TestPresenceProberSourceOnlyStats",
				"cmd:TestPhase7Acceptance",
				"cmd:TestRuntimeProbeDefaultsToAllTargetsAndReportsPresenceOnly",
				"cmd:TestRuntimeProbeSourceOpensNoFile",
			),
			partial(MemoryEnforcement,
				"The PreToolUse gate denies a memory read of a source the plan excludes (Engram reads, longterm-mem query and get, a procedural skill of the project lock), and all with no project; queries and Engram searches must name the plan's project, goal-scoped queries its goal. Not enforced: a skill file read through Bash, the project of a read by id. Writes are never blocked. install-hooks installs matchers no narrower than its tool-name patterns; hooks need a restart; no test observes a real session being denied.",
				"cmd:TestInstalledMatchersCoverEveryToolTheGateHasAnOpinionAbout",
				"cmd:TestPhase7Acceptance",
				"cmd:TestPreToolUseChecksTheGoalOfALongtermMemQueryUnderScopeGoal",
				"cmd:TestPreToolUseChecksTheProjectOfALongtermMemQuery",
				"cmd:TestPreToolUseChecksTheProjectOfAnEngramReadAgainstItsEngramName",
				"cmd:TestPreToolUseDeniesEveryQueryWhenThePlanHasNoProject",
				"cmd:TestPreToolUseDeniesTheReadsOfASourceThePlanExcludes",
				"cmd:TestPreToolUseGatesTheProceduralSkillsOfTheLock",
				"cmd:TestPreToolUseHookRunsAsASeparateProcess",
				"projection:TestGateComparesTheQueryProjectWithThePlanProject",
				"projection:TestGateNeverTouchesAMemoryWrite",
				"projection:TestGateRecognizesTheMemoryReadsByTheirNamesAndNothingLooser",
				"settings/settingsfile:TestInstall_AddsTheProjectionFamily",
			),
			partial(Skills,
				"Project tier: skills install and adopt write .claude/skills and .agents/skills, replacing only files they own by hash; project-register, revise, and retire manage agent-written skills; locks serialize writers. Global tier: skills add needs an approval record; labdrian apply deploys global skills, not the engine, tested only on a fixture overlay and a sandbox home, never the real skills. Unused-skill detection is report-only and covers the project tier only. No test observes a session loading a skill.",
				"cmd:TestAdoptTakesOwnershipOfAnExistingInstallAndThenInstallLeavesItBe",
				"cmd:TestProjectRegisterTellsEachPathThenTheDigestTheRevisionAndTheTrustNote",
				"installer:TestApply_AgentsLandInNativeAgentDirs",
				"installer:TestUnrelatedSkillUnchanged",
				"skills/app:TestAddRegistersTheSkillInTheRegistryAndTheManifest",
				"skills/app:TestAddWritesNothingWhenItRefuses",
				"skills/app:TestAnInstallOverAHandEditedFileIsRefusedWithEveryReasonAndNothingIsWritten",
				"skills/app:TestInstallCopiesTheSkillIntoBothRuntimesAndRecordsIt",
				"skills:TestInstall_AbsentTargetsAreCreatedInBothRuntimesAndRecorded",
				"skills:TestPlanAndExecuteProjectRetireRemovesTargetsAndLockEntry",
				"skills:TestPlanProjectRevise_HashMatchBumpsRevisionAndHash",
				"skills:TestProjectLockE2E_InstallsAndRegistrationsShareOneProjectWithoutLosingAnything",
				"skills:TestRegistryLockE2E_ConcurrentVerbsNeverLoseAnUpdate",
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
				"skills install, adopt, and project-register write project skills to .agents/skills. A live check on 2026-09-28 recorded Codex reading a project skill's body there; that is recorded evidence, not a test. Global skills are deployed to ~/.codex/skills by labdrian apply, not the engine, tested only on a fixture overlay and a sandbox home, never the real skills. Role agents go to ~/.codex/agents the same way. No test observes Codex loading a skill or an agent, and the global tier has no unused-skill detector.",
				"cmd:TestProjectRegisterTellsEachPathThenTheDigestTheRevisionAndTheTrustNote",
				"installer:TestApply_AgentsLandInNativeAgentDirs",
				"installer:TestCodexAgentsLandInTheAgentsDirectoryOfCodexAndNowhereElse",
				"installer:TestUnrelatedSkillUnchanged",
				"skills/app:TestInstallCopiesTheSkillIntoBothRuntimesAndRecordsIt",
				"skills:TestInstall_AbsentTargetsAreCreatedInBothRuntimesAndRecorded",
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
				"Skills reach Pi as the labdrian-pi package: pipkg builds it from the registry's Pi-targeted skills, leaving out approval records and writers' temporary files, and install registers it. The adapter tests replace the pi CLI with a recording stub, so the real pi never loads the package. skills install and project-register never write .pi/skills; project-register says Pi loads .agents/skills only after project trust. Install also links the package's agents. No test observes Pi loading a skill or an agent.",
				"cmd:TestProjectRegisterTellsEachPathThenTheDigestTheRevisionAndTheTrustNote",
				"pipkg:TestPipkgBuild_CopiesEveryPiAgentOfTheOverlay",
				"pipkg:TestPipkgBuild_DoesNotProjectAWritersTemporaryFile",
				"pipkg:TestPipkgBuild_DoesNotProjectTheApprovalRecord",
				"pipkg:TestPipkgBuild_SelectsPiTargetedSkills",
				"runtime:TestPiAdapter_ApplyInstallSyncCheck_WiredToPipkg",
				"runtime:TestPiAdapter_InstallLinksEveryAgentOfThePackage",
				"skills:TestProjectTargetsMatchContractTable",
			))...),
	},
	{
		Target:   TargetOpenCode,
		Untested: "No test drives a model session. The adapter, both plugins and the engine source run at file level and under node against a recording engine (skipped without node), and one test loads both installed plugins into the real opencode binary (skipped without it). What needs a model session stays untested: context reaching a session, a captured Key Learnings item, the prompt and execute.after shapes, compaction (T17).",
		Claims: []Claim{
			partial(Installation,
				"Install, update, uninstall and status are tested at file level against a temporary config root: three owned opencode.json entries, one AGENTS.md block, the parity plugin and the memorycard plugin. Status reaches supported only when both plugins wrote their active marker; a test with the real opencode binary loads both installed plugins (it skips without the binary). A restart is owed after install and update.",
				"cmd:TestRuntimeCLIOpenCodeInstallStatusUninstallInASandboxHome",
				"runtime:TestOpenCodeAGENTSIsNotRewrittenWhenCurrent",
				"runtime:TestOpenCodeAGENTSKeepsEveryByteOutsideTheBlockAndReplacesItInPlace",
				"runtime:TestOpenCodeAGENTSOfTheShippedOverlayHoldsTheOwnerRulesAndTheStyleBody",
				"runtime:TestOpenCodeAGENTSWithBrokenMarkersIsRefusedAndKept",
				"runtime:TestOpenCodeDropsTheStyleFrontmatterInEveryShape",
				"runtime:TestOpenCodeInstallKeepsEveryForeignKeyAndWritesABackupOnce",
				"runtime:TestOpenCodeInstallKeepsTheBytesAroundAContainerItHasToCreate",
				"runtime:TestOpenCodeInstallLeavesAForeignDefaultAgentAndSaysSo",
				"runtime:TestOpenCodeInstallLeavesAForeignMCPEntryAndSaysSo",
				"runtime:TestOpenCodeInstallOverACheckoutWithNoInstructionSourcesIsTheBaseResultWithANote",
				"runtime:TestOpenCodeInstallRefusesAFileItCannotParseAndChangesNothing",
				"runtime:TestOpenCodeInstallRefusesDuplicateKeysAndTrailingContent",
				"runtime:TestOpenCodeInstallSplicesOnlyTheOwnedEntriesIntoTheOwnersBytes",
				"runtime:TestOpenCodeInstallTwiceWritesNothingTheSecondTime",
				"runtime:TestOpenCodeInstallUpdatePreservesLoadedMarkerUntilRestart",
				"runtime:TestOpenCodeInstallWritesAGENTSFromTheRulesTheStyleBodyAndTheMemorycardBlock",
				"runtime:TestOpenCodeInstallWritesNoSkillAndSaysNothingWhenThereAreNoCopies",
				"runtime:TestOpenCodeInstallWritesPluginConfigAndRestartRequiredStatus",
				"runtime:TestOpenCodeInstallWritesTheMemorycardPluginAndItsRecord",
				"runtime:TestOpenCodeInstallWritesTheV2ConfigWithForemanAndTheMCPServers",
				"runtime:TestOpenCodeLeavesAMemorycardPluginItDidNotWriteAndSaysSo",
				"runtime:TestOpenCodeLoadsTheInstalledPluginAndItWritesItsActiveMarker",
				"runtime:TestOpenCodeMemorycardBlockNamesTheToolsAndNeverTheOldStore",
				"runtime:TestOpenCodeRefusedAGENTSDoesNotStopTheConfigAndNamesItsReason",
				"runtime:TestOpenCodeRefusedConfigDoesNotStopAGENTSNorTheSkillsReport",
				"runtime:TestOpenCodeReportsLabdrianSkillsFoundWhereOpenCodeReadsThemFirst",
				"runtime:TestOpenCodeStatusIsPartialWhileAGENTSIsMissingOrStale",
				"runtime:TestOpenCodeStatusIsPartialWhileTheOwnedEntriesAreMissing",
				"runtime:TestOpenCodeStatusIsSupportedOnlyWhenBothPluginsAreActive",
				"runtime:TestOpenCodeStatusRejectsTamperedPromptConfig",
				"runtime:TestOpenCodeStatusSupportedWhenActiveMarkerMatchesHash",
				"runtime:TestOpenCodeUninstallGivesBackAnAbsentFileAbsentAndKeepsAForeignAgent",
				"runtime:TestOpenCodeUninstallNamesBothCleanupsThatFailed",
				"runtime:TestOpenCodeUninstallOfAFileItCannotParseSaysSoAndKeepsIt",
				"runtime:TestOpenCodeUninstallRemovesOnlyTheBlockAndGivesBackTheFile",
				"runtime:TestOpenCodeUninstallRemovesOnlyTheOwnedEntries",
				"runtime:TestOpenCodeUninstallRemovesPluginAndConfig",
				"runtime:TestOpenCodeUninstallRemovesTheMemorycardPluginAndRecordAndNothingElseInItsDirectory",
				"runtime:TestOpenCodeUpdateRewritesAChangedMemorycardPluginAndTheProgramItRuns",
				"runtime:TestOpenCodeWritesTheAbsolutePathOfTheServersItFinds",
			),
			// OpenCode has its own claims since T16 (S14), so it no longer takes the
			// declared-only list that Codex and Pi still share. A claim is partial
			// only when a test named here backs it; what needs a model session is
			// unsupported or says so, and points at T17.
			partial(Projection,
				"The parity plugin runs projection hook --source opencode: SessionStart context goes on the system prompt of every model call of the session, byte-stable, and UserPromptSubmit context joins the user's prompt text. Tests run the plugin under node against a recording engine (skipped without node). The prompt and execute.after shapes come from the binary only; no test observes a model session receiving the context (T17).",
				"cmd:TestTheSameSessionIdUnderTwoSourcesIsTwoSessions",
				"runtime:TestOpenCodeBridgeAddsNothingToTheModelCallWhenTheEngineHasNoBlock",
				"runtime:TestOpenCodeBridgeCarriesThePromptContextInTheUsersPromptAndNeverInTheSystemPrompt",
				"runtime:TestOpenCodeBridgeKeepsTheSystemPromptByteStableAcrossPrompts",
				"runtime:TestOpenCodeBridgeLeavesAPromptAloneWhenTheEngineHasNothingToAdd",
				"runtime:TestOpenCodeBridgeLogsOncePerProcessAnAnswerItCannotPutInAPromptOfAnUnexpectedShape",
				"runtime:TestOpenCodeBridgePutsTheSessionStartContextIntoEveryModelCallOfTheSession",
				"runtime:TestOpenCodeBridgeSessionEventsSurviveAnEngineThatFails",
			),
			partial(Dispatch,
				"The engine never starts sessions. The same bridge asks the engine at session start and at each prompt whose block changed (T35), and the Stop hook (experimental.idle, an experimental OpenCode API) forces another turn when the engine blocks the stop. Tests run it under node against a recording engine that answers as told, so no workflow binding is exercised; no test observes a model session being steered (T17).",
				"runtime:TestOpenCodeBridgeAsksTheStopOfMainSessionsOnly",
				"runtime:TestOpenCodeBridgeForcesAnotherTurnWhenTheEngineBlocksTheStop",
				"runtime:TestOpenCodeBridgeKeepsTheStopGuardAcrossThePromptItCaused",
				"runtime:TestOpenCodeBridgeLetsTheSessionStopWhenTheEngineDoesNotBlock",
				"runtime:TestOpenCodeBridgePutsTheSessionStartContextIntoEveryModelCallOfTheSession",
			),
			partial(Cancellation,
				"The bridge throws Tool.Error with the reason of the engine's PreToolUse denial (node, recording engine); that the denied call then does not run is an L10 recording, unproven with a model until T17. The engine maps patch to Edit (hookwire test); the paused-workflow edit gate is proven for Claude Code tool names, not through --source opencode or patch. An in-flight tool call cannot be interrupted, and shell is never gated.",
				"cmd:TestPreToolUseDeniesTheEditToolsOfAPausedWorkflow",
				"hookwire:TestAdaptToolNameOpencode",
				"runtime:TestOpenCodeBridgeDeniesACallTheEngineDeniesWithToolError",
				"runtime:TestOpenCodeBridgeLetsACallThroughWhenTheEngineFailsAndSaysSoOnce",
			),
			unsupported(Persistence,
				"Not implemented: the plugin stores no binding between an OpenCode session and a workflow. The engine's binding belongs to the repository, and no test ties it to a model session (T17)."),
			unsupported(Restart,
				"Not implemented: no test shows a new OpenCode session re-binding to an on-disk workflow. The bridge asks the engine at each session start, which only a model session can show (T17)."),
			unsupported(Authentication,
				"Not implemented: the engine does not check whether OpenCode has credentials, and the presence prober has no credentials check for it. A provider login is observed only with a model session (T17)."),
			partial(MemoryEnforcement,
				"Gate via --source opencode: the OpencodeAdaptsBeforeGate and OpencodeMemorycardReads subtests of cmd:TestCodexSourceProcesses answer longterm-mem and memorycard reads as Claude Code; execute (not mapped) stays silent; a bridge test pins an execute and its inner memorycard call. The plugin puts context on new main sessions and sends each subagent's final text to memorycard capture (fake memorycard). No memory write is blocked; a saved Key Learnings item and a real session are untested (T17).",
				"cmd:TestCodexSourceProcesses",
				"hookwire:TestAdaptToolNameOpencode",
				"runtime:TestMemorycardPluginAsksNothingInAChildSession",
				"runtime:TestMemorycardPluginCapturesTheFinalTextOfASubagentOnStdin",
				"runtime:TestMemorycardPluginPutsTheMemorycardContextOnTheSystemPromptOfANewSession",
				"runtime:TestOpenCodeBridgeDeniesACallTheEngineDeniesWithToolError",
				"runtime:TestOpenCodeBridgeJudgesTheInnerMemorycardCallOfACodeModeExecute",
			),
			partial(Skills,
				"labdrian apply copies no skill for OpenCode, which reads the global .claude/skills: a copy in its own directory would shadow that one. Install, update and status only report labdrian skills found where OpenCode reads first, and never write or remove one (tested on a fixture overlay and a sandbox home, never the real skills). A live OpenCode 1.18.31 session on 2026-09-30 discovered project skills under .agents/skills and .claude/skills (recorded, not tested). No test observes OpenCode loading a skill.",
				"installer:TestApply_AgentsLandInNativeAgentDirs",
				"installer:TestSyncCheck_OpenCodeChecksItsAgentsWithNoSkillsDirectory",
				"installer:TestUnrelatedSkillUnchanged",
				"runtime:TestOpenCodeInstallWritesNoSkillAndSaysNothingWhenThereAreNoCopies",
				"runtime:TestOpenCodeReportsLabdrianSkillsFoundWhereOpenCodeReadsThemFirst",
				"runtime:TestOpenCodeStatusReportsShadowingSkillsWhateverItsOutcome",
			),
		},
	},
}

// declaredOnlyClaims returns the claims after installation for a runtime that
// Phase 7 declares without implementing: the seven Phase 7 capabilities, every
// one unsupported with the limit written, and then the runtime's own skills claim,
// which is the one capability Phase 8 states from the tests that exist. runtime is
// the display name used in the sentences, and authDetail is the authentication
// limit, which differs by runtime because only some runtimes have a credentials
// presence check. C23 M12b upgrades only Codex memory enforcement, using the
// same ordered declaration and proof scheme as every other claim.
func declaredOnlyClaims(runtime, authDetail string, skills Claim) []Claim {
	const scope = " This runtime is declared only: Phase 7 writes no code for it."
	memory := unsupported(MemoryEnforcement, memoryEnforcementDetail(runtime, scope))
	if runtime == "Codex" {
		memory = partial(MemoryEnforcement, memoryEnforcementDetail(runtime, scope),
			"cmd:TestCodexSourceProcesses",
			"cmd:TestMemoryWiringAcceptance",
		)
	}
	return []Claim{
		unsupported(Projection, "No code projects the active workflow (Goal, Profile, stage) into a "+runtime+" session."+scope),
		unsupported(Dispatch, "Not implemented. The engine never starts sessions by design, and dispatch means projecting the active workflow into an existing session, which does not exist for "+runtime+"."+scope),
		unsupported(Cancellation, "Not implemented: no workflow transition reaches a "+runtime+" session. Hard cancel of an in-flight tool call is out of scope."+scope),
		unsupported(Persistence, "Not implemented: no binding between a "+runtime+" session and a workflow is stored."+scope),
		unsupported(Restart, "Not implemented: a new "+runtime+" session does not re-bind to an on-disk workflow."+scope),
		unsupported(Authentication, authDetail+scope),
		memory,
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

// memoryEnforcementDetail bounds the Codex MCP gate and the text of Pi, the one
// other declared-only runtime (OpenCode states its own claim since T16).
func memoryEnforcementDetail(runtime, scope string) string {
	if runtime == "Codex" {
		return "PreToolUse checks plan sources for MCP Engram reads, memorycard reads and longterm-mem query/get; searches/queries check project, goal-scoped queries check goal. install-hooks installs --source codex; trust each new or changed handler in the Codex TUI. Limits: Codex fails open on hook failure; no procedural skills, Bash or apply_patch enforcement; no project check for reads by id; agent_id ignored. PostToolUse records successful MCP reads only. Proven by engine hook processes, not live validation."
	}
	return "Not implemented: no memory plan is projected into or enforced in a " + runtime + " session." + scope
}
