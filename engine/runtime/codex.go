package runtime

type CodexCapabilities struct {
	RuntimeDetected            bool
	DeterministicScopedRewrite bool
	ScopedSubagentTaskLaunches bool
	VerifiedRuntimeVersion     string
}

type CodexAdapter struct {
	capabilities CodexCapabilities
}

func NewCodexAdapter(capabilities CodexCapabilities) CodexAdapter {
	return CodexAdapter{capabilities: capabilities}
}

func (a CodexAdapter) Target() Target             { return TargetCodex }
func (a CodexAdapter) Apply() LifecycleResult     { return a.evaluate(ActionApply) }
func (a CodexAdapter) Install() LifecycleResult   { return a.evaluate(ActionInstall) }
func (a CodexAdapter) Status() LifecycleResult    { return a.evaluate(ActionStatus) }
func (a CodexAdapter) SyncCheck() LifecycleResult { return a.evaluate(ActionSyncCheck) }
func (a CodexAdapter) Update() LifecycleResult    { return a.evaluate(ActionUpdate) }
func (a CodexAdapter) Rollback() LifecycleResult  { return a.evaluate(ActionRollback) }
func (a CodexAdapter) Uninstall() LifecycleResult { return a.evaluate(ActionUninstall) }

func (a CodexAdapter) evaluate(action Action) LifecycleResult {
	reasons := a.missingReasons()
	if !a.capabilities.RuntimeDetected {
		return NewLifecycleResult(TargetCodex, action, CapabilityUnsupported, "Codex runtime not detected", reasons)
	}
	if len(reasons) > 0 {
		return NewLifecycleResult(TargetCodex, action, CapabilityPartial, "Codex support is conditional", reasons)
	}
	version := a.capabilities.VerifiedRuntimeVersion
	if version == "" {
		version = "verified runtime"
	}
	return NewLifecycleResult(TargetCodex, action, CapabilitySupported, "Codex deterministic scoped injection verified for "+version, nil)
}

func (a CodexAdapter) missingReasons() []string {
	var reasons []string
	if !a.capabilities.RuntimeDetected {
		reasons = append(reasons, "Codex runtime not detected")
	}
	if !a.capabilities.DeterministicScopedRewrite {
		reasons = append(reasons, "deterministic scoped subagent/task rewrite not verified")
	}
	if !a.capabilities.ScopedSubagentTaskLaunches {
		reasons = append(reasons, "scoped subagent/task launches not verified")
	}
	return reasons
}
