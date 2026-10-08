// Package runtime defines target-aware overlay lifecycle adapters.
package runtime

import (
	"fmt"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/capability"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/contract"
)

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

type CapabilityStatus string

const (
	CapabilitySupported       CapabilityStatus = "supported"
	CapabilityPartial         CapabilityStatus = "partial"
	CapabilityUnsupported     CapabilityStatus = "unsupported"
	CapabilityRestartRequired CapabilityStatus = "restart_required"
)

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

type LifecycleResult struct {
	Target  Target
	Action  Action
	Status  CapabilityStatus
	Message string
	Reasons []string
}

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

func NewLifecycleResult(target Target, action Action, status CapabilityStatus, message string, reasons []string) LifecycleResult {
	return LifecycleResult{Target: target, Action: action, Status: status, Message: message, Reasons: reasons}
}

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

func MutatePrompt(prompt, phase, contractPath string, c contract.Contract) (string, bool) {
	switch {
	case c.AppliesToPhase(phase):
		mutated := InjectPrompt(prompt, contractPath, c.Header())
		return mutated, mutated != prompt
	case c.ExcludesPhase(phase):
		mutated := StripPrompt(prompt, contractPath)
		return mutated, mutated != prompt
	default:
		return prompt, false
	}
}

func InjectPrompt(prompt, contractPath, injectionHeader string) string {
	entry := CanonicalEntry(contractPath)
	if HasExactEntry(prompt, contractPath) {
		return prompt
	}
	if HasExactHeader(prompt, injectionHeader) {
		lines := strings.Split(prompt, "\n")
		out := make([]string, 0, len(lines)+1)
		for _, line := range lines {
			out = append(out, line)
			if strings.TrimSpace(line) == injectionHeader {
				out = append(out, entry)
			}
		}
		return strings.Join(out, "\n")
	}
	sep := "\n"
	if !strings.HasSuffix(prompt, "\n") {
		sep = "\n\n"
	} else if !strings.HasSuffix(prompt, "\n\n") {
		sep = "\n"
	}
	return prompt + sep + injectionHeader + "\n" + entry + "\n"
}

func StripPrompt(prompt, contractPath string) string {
	if !HasExactEntry(prompt, contractPath) {
		return prompt
	}
	entry := CanonicalEntry(contractPath)
	lines := strings.Split(prompt, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == entry {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func CanonicalEntry(contractPath string) string {
	return contractPath
}

func HasExactEntry(prompt, contractPath string) bool {
	entry := CanonicalEntry(contractPath)
	for _, line := range strings.Split(prompt, "\n") {
		if strings.TrimSpace(line) == entry {
			return true
		}
	}
	return false
}

func HasExactHeader(prompt, injectionHeader string) bool {
	for _, line := range strings.Split(prompt, "\n") {
		if strings.TrimSpace(line) == injectionHeader {
			return true
		}
	}
	return false
}
