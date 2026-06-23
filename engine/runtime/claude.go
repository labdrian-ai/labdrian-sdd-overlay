package runtime

type ClaudeAdapter struct{}

func NewClaudeAdapter() ClaudeAdapter {
	return ClaudeAdapter{}
}

func (a ClaudeAdapter) Target() Target             { return TargetClaude }
func (a ClaudeAdapter) Apply() LifecycleResult     { return a.result(ActionApply) }
func (a ClaudeAdapter) Install() LifecycleResult   { return a.result(ActionInstall) }
func (a ClaudeAdapter) Status() LifecycleResult    { return a.result(ActionStatus) }
func (a ClaudeAdapter) SyncCheck() LifecycleResult { return a.result(ActionSyncCheck) }
func (a ClaudeAdapter) Update() LifecycleResult    { return a.result(ActionUpdate) }
func (a ClaudeAdapter) Rollback() LifecycleResult  { return a.result(ActionRollback) }
func (a ClaudeAdapter) Uninstall() LifecycleResult { return a.result(ActionUninstall) }

func (a ClaudeAdapter) result(action Action) LifecycleResult {
	return NewLifecycleResult(
		TargetClaude,
		action,
		CapabilityPartial,
		"Claude deterministic scoping is managed by legacy hook commands; use overlay install-hooks, status-hooks, and uninstall-hooks for real hook lifecycle state",
		nil,
	)
}
