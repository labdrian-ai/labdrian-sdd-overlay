// Package runtime defines target-aware overlay lifecycle adapters.
package runtime

import (
	"fmt"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/propagator"
)

type Target string

const (
	TargetClaude   Target = "claude"
	TargetOpenCode Target = "opencode"
	TargetCodex    Target = "codex"
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

type ContractPhases struct {
	AppliesTo      []string
	Excluded       []string
	InjectionPoint string
}

func LoadContractPhases(content string) (ContractPhases, error) {
	phases, err := propagator.ParseFrontmatter(content)
	if err != nil {
		return ContractPhases{}, err
	}
	return ContractPhases{
		AppliesTo:      phases.AppliesTo,
		Excluded:       phases.Excluded,
		InjectionPoint: phases.InjectionPoint,
	}, nil
}

func (p ContractPhases) AppliesToPhase(phase string) bool {
	return contains(p.AppliesTo, phase)
}

func (p ContractPhases) ExcludesPhase(phase string) bool {
	return contains(p.Excluded, phase)
}

func MutatePrompt(prompt, phase, contractPath string, phases ContractPhases) (string, bool) {
	switch {
	case phases.AppliesToPhase(phase):
		mutated := InjectPrompt(prompt, contractPath, injectionHeader(phases))
		return mutated, mutated != prompt
	case phases.ExcludesPhase(phase):
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

func ParseTarget(raw string) (Target, error) {
	switch Target(strings.TrimSpace(raw)) {
	case TargetClaude:
		return TargetClaude, nil
	case TargetOpenCode:
		return TargetOpenCode, nil
	case TargetCodex:
		return TargetCodex, nil
	case TargetAll:
		return TargetAll, nil
	default:
		return "", fmt.Errorf("unknown target %q", raw)
	}
}

func ExpandTarget(target Target) []Target {
	if target != TargetAll {
		return []Target{target}
	}
	return []Target{TargetClaude, TargetOpenCode, TargetCodex}
}

func NewFoundationAdapter(target Target) Adapter {
	if target == TargetClaude {
		return NewClaudeAdapter()
	}
	if target == TargetOpenCode {
		return NewOpenCodeAdapter(DefaultOpenCodeConfigRoot())
	}
	if target == TargetCodex {
		return NewCodexAdapter(CodexCapabilities{})
	}
	return foundationAdapter{target: target}
}

type foundationAdapter struct {
	target Target
}

func (a foundationAdapter) Target() Target             { return a.target }
func (a foundationAdapter) Apply() LifecycleResult     { return a.result(ActionApply) }
func (a foundationAdapter) Install() LifecycleResult   { return a.result(ActionInstall) }
func (a foundationAdapter) Status() LifecycleResult    { return a.result(ActionStatus) }
func (a foundationAdapter) SyncCheck() LifecycleResult { return a.result(ActionSyncCheck) }
func (a foundationAdapter) Update() LifecycleResult    { return a.result(ActionUpdate) }
func (a foundationAdapter) Rollback() LifecycleResult  { return a.result(ActionRollback) }
func (a foundationAdapter) Uninstall() LifecycleResult { return a.result(ActionUninstall) }

func (a foundationAdapter) result(action Action) LifecycleResult {
	return NewLifecycleResult(a.target, action, CapabilityUnsupported, "runtime adapter foundation present; target implementation is scheduled for a later PR slice", nil)
}

func injectionHeader(phases ContractPhases) string {
	if phases.InjectionPoint != "" {
		return phases.InjectionPoint
	}
	return "## Skills to load before work"
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
