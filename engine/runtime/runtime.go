// Package runtime defines target-aware overlay lifecycle adapters.
package runtime

import (
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/capability"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/contract"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/core"
)

// Transitional: the vocabulary, the Adapter port and the prompt rules now live in runtime/core.
// The names below forward to it so the adapters, the command and the tests can be switched one
// family at a time; the commit that has switched the last of them deletes this file.

type Target = core.Target

// The runtimes are written from the capability vocabulary, as the core writes them, so the scan
// of this package that every Target constant is a capability name keeps reading something.
const (
	TargetClaude   Target = capability.TargetClaude
	TargetOpenCode Target = capability.TargetOpenCode
	TargetCodex    Target = capability.TargetCodex
	TargetPi       Target = capability.TargetPi
	TargetAll      Target = core.TargetAll
)

type CapabilityStatus = core.CapabilityStatus

const (
	CapabilitySupported       = core.CapabilitySupported
	CapabilityPartial         = core.CapabilityPartial
	CapabilityUnsupported     = core.CapabilityUnsupported
	CapabilityRestartRequired = core.CapabilityRestartRequired
)

type Action = core.Action

const (
	ActionApply     = core.ActionApply
	ActionInstall   = core.ActionInstall
	ActionStatus    = core.ActionStatus
	ActionSyncCheck = core.ActionSyncCheck
	ActionUpdate    = core.ActionUpdate
	ActionRollback  = core.ActionRollback
	ActionUninstall = core.ActionUninstall
)

type LifecycleResult = core.LifecycleResult

type Adapter = core.Adapter

func NewLifecycleResult(target Target, action Action, status CapabilityStatus, message string, reasons []string) LifecycleResult {
	return core.NewLifecycleResult(target, action, status, message, reasons)
}

func MutatePrompt(prompt, phase, contractPath string, c contract.Contract) (string, bool) {
	return core.MutatePrompt(prompt, phase, contractPath, c)
}

func InjectPrompt(prompt, contractPath, injectionHeader string) string {
	return core.InjectPrompt(prompt, contractPath, injectionHeader)
}

func StripPrompt(prompt, contractPath string) string { return core.StripPrompt(prompt, contractPath) }

func CanonicalEntry(contractPath string) string { return core.CanonicalEntry(contractPath) }

func HasExactEntry(prompt, contractPath string) bool {
	return core.HasExactEntry(prompt, contractPath)
}

func HasExactHeader(prompt, injectionHeader string) bool {
	return core.HasExactHeader(prompt, injectionHeader)
}
