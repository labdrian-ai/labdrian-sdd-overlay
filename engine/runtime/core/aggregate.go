package core

// Perform asks adapter to do action and answers what it answered. An action the lifecycle does
// not have is answered as unsupported, in the words of a status, and never reaches the adapter.
func Perform(adapter Adapter, action Action) LifecycleResult {
	switch action {
	case ActionApply:
		return adapter.Apply()
	case ActionInstall:
		return adapter.Install()
	case ActionStatus:
		return adapter.Status()
	case ActionSyncCheck:
		return adapter.SyncCheck()
	case ActionUpdate:
		return adapter.Update()
	case ActionRollback:
		return adapter.Rollback()
	case ActionUninstall:
		return adapter.Uninstall()
	default:
		return NewLifecycleResult(adapter.Target(), ActionStatus, CapabilityUnsupported, "unknown runtime action", nil)
	}
}

// AggregateStatus says whether a run of action over several targets failed, from the result each
// target answered. One failing target fails the run; a target that did not fail does not hide
// another that did.
//
// For a status, a target fails unless it is supported, with one exception: a codex that is only
// partial does not fail a run over several targets, because its activation cannot be proven yet
// and a partial answer is all it can give. Asked alone, codex partial fails like any other target.
// For every other action, a target fails when it is partial or unsupported; restart_required is
// the normal answer of an install or an update that took effect.
func AggregateStatus(action Action, results []LifecycleResult) bool {
	several := len(results) > 1
	for _, r := range results {
		if action == ActionStatus {
			switch {
			case r.Status == CapabilityRestartRequired, r.Status == CapabilityUnsupported:
				return true
			case r.Status == CapabilityPartial && !(several && r.Target == TargetCodex):
				return true
			}
			continue
		}
		if r.Status == CapabilityPartial || r.Status == CapabilityUnsupported {
			return true
		}
	}
	return false
}

// ComponentFailed says whether the result of the one component an action ran on fails the run.
// A component is not a target and is never aggregated with one: for a status it fails unless it is
// supported, and for any other action when it is partial or unsupported.
func ComponentFailed(action Action, result LifecycleResult) bool {
	if action == ActionStatus {
		return result.Status != CapabilitySupported
	}
	return result.Status == CapabilityUnsupported || result.Status == CapabilityPartial
}
