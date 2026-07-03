package runtime

// ClaudeAdapter is the runtime adapter foundation for Claude CLI.
//
// Full mutation/install lifecycle support is not yet implemented in this PR,
// but the adapter intentionally exists to preserve the three-runtime architecture.
type ClaudeAdapter struct {
	target Target
}

func NewClaudeAdapter() ClaudeAdapter {
	return ClaudeAdapter{target: TargetClaude}
}

func (a ClaudeAdapter) Target() Target         { return a.target }
func (a ClaudeAdapter) Apply() LifecycleResult { return a.result(ActionApply) }
func (a ClaudeAdapter) Install() LifecycleResult {
	return a.result(ActionInstall)
}
func (a ClaudeAdapter) Status() LifecycleResult    { return a.result(ActionStatus) }
func (a ClaudeAdapter) SyncCheck() LifecycleResult { return a.result(ActionSyncCheck) }
func (a ClaudeAdapter) Update() LifecycleResult    { return a.result(ActionUpdate) }
func (a ClaudeAdapter) Rollback() LifecycleResult  { return a.result(ActionRollback) }
func (a ClaudeAdapter) Uninstall() LifecycleResult { return a.result(ActionUninstall) }

func (a ClaudeAdapter) result(action Action) LifecycleResult {
	return NewLifecycleResult(a.target, action, CapabilityUnsupported, "runtime adapter foundation present; Claude implementation is scheduled for a later PR slice", nil)
}
