package runtime

// CodexAdapter is the runtime adapter foundation for Codex CLI.
//
// Full mutation/install lifecycle support is not yet implemented in this PR,
// but the adapter intentionally exists to preserve the three-runtime architecture.
type CodexAdapter struct {
	target Target
}

func NewCodexAdapter() CodexAdapter {
	return CodexAdapter{target: TargetCodex}
}

func (a CodexAdapter) Target() Target             { return a.target }
func (a CodexAdapter) Apply() LifecycleResult     { return a.result(ActionApply) }
func (a CodexAdapter) Install() LifecycleResult   { return a.result(ActionInstall) }
func (a CodexAdapter) Status() LifecycleResult    { return a.result(ActionStatus) }
func (a CodexAdapter) SyncCheck() LifecycleResult { return a.result(ActionSyncCheck) }
func (a CodexAdapter) Update() LifecycleResult    { return a.result(ActionUpdate) }
func (a CodexAdapter) Rollback() LifecycleResult  { return a.result(ActionRollback) }
func (a CodexAdapter) Uninstall() LifecycleResult { return a.result(ActionUninstall) }

func (a CodexAdapter) result(action Action) LifecycleResult {
	return NewLifecycleResult(a.target, action, CapabilityUnsupported, "runtime adapter foundation present; Codex implementation is scheduled for a later PR slice", nil)
}
