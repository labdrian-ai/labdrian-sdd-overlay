package core

import (
	"fmt"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/capability"
)

// Target names a runtime an overlay can be applied to, or a request for all of them.
type Target string

// The runtimes are the ones capability declares: their names are its, so there is one list of
// them. TargetAll is not a runtime but a request for every registered one.
const (
	TargetClaude   Target = capability.TargetClaude
	TargetOpenCode Target = capability.TargetOpenCode
	TargetCodex    Target = capability.TargetCodex
	TargetPi       Target = capability.TargetPi
	TargetAll      Target = "all"
)

// CapabilityStatus is how far a runtime honours what the overlay asks of it. The values are
// printed and parsed by people and scripts, so they are stable.
type CapabilityStatus string

const (
	CapabilitySupported       CapabilityStatus = "supported"
	CapabilityPartial         CapabilityStatus = "partial"
	CapabilityUnsupported     CapabilityStatus = "unsupported"
	CapabilityRestartRequired CapabilityStatus = "restart_required"
)

// Action is a step of the lifecycle of the overlay in a runtime.
type Action string

const (
	ActionApply     Action = "apply"
	ActionInstall   Action = "install"
	ActionStatus    Action = "status"
	ActionSyncCheck Action = "sync-check"
	ActionUpdate    Action = "update"
	ActionRollback  Action = "rollback"
	ActionUninstall Action = "uninstall"
)

// LifecycleResult is what an adapter answers for one Action: the status and the words that
// explain it, with the reasons that led to a status that is not supported.
type LifecycleResult struct {
	Target  Target
	Action  Action
	Status  CapabilityStatus
	Message string
	Reasons []string
}

// String renders the result as the one line the command prints.
func (r LifecycleResult) String() string {
	base := ""
	if r.Message == "" {
		base = fmt.Sprintf("[%s] %s: %s", r.Target, r.Action, r.Status)
	} else {
		base = fmt.Sprintf("[%s] %s: %s — %s", r.Target, r.Action, r.Status, r.Message)
	}
	if len(r.Reasons) == 0 {
		return base
	}
	return base + " — reasons: " + strings.Join(r.Reasons, "; ")
}

// NewLifecycleResult builds the answer of target for action.
func NewLifecycleResult(target Target, action Action, status CapabilityStatus, message string, reasons []string) LifecycleResult {
	return LifecycleResult{Target: target, Action: action, Status: status, Message: message, Reasons: reasons}
}

// Adapter is the port every runtime implements: one method for each Action, each answering with
// the result of its step. The adapters live in engine/runtime and are built by a Registry.
type Adapter interface {
	Target() Target
	Apply() LifecycleResult
	Install() LifecycleResult
	Status() LifecycleResult
	SyncCheck() LifecycleResult
	Update() LifecycleResult
	Rollback() LifecycleResult
	Uninstall() LifecycleResult
}
