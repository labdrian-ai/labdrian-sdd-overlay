// Package workflowprofile defines declarative, runtime-neutral workflow profiles.
// Profiles describe work organization; they do not authorize or execute work.
package workflowprofile

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrUnknownProfile         = errors.New("unknown workflow profile")
	ErrUnsupportedCombination = errors.New("unsupported profile and compatibility combination")
	ErrInvalidProfile         = errors.New("invalid workflow profile")
	ErrInvalidResumeState     = errors.New("invalid workflow resume state")
)

// Stage is one named workflow step. Dependencies must refer to earlier stages.
type Stage struct {
	Name      string   `json:"name"`
	DependsOn []string `json:"depends_on"`
}

// WorkflowProfile is the seven-field declarative Phase 2 contract.
type WorkflowProfile struct {
	Name           string   `json:"name"`
	Stages         []Stage  `json:"stages"`
	Roles          []string `json:"roles"`
	Checks         []string `json:"checks"`
	MemoryPolicy   string   `json:"memory_policy"`
	ReviewPolicy   string   `json:"review_policy"`
	DeliveryPolicy string   `json:"delivery_policy"`
}

// CompatibilityMode is a distinct axis from a workflow profile. No combination
// matrix is established in Phase 2, so explicit combinations fail closed.
type CompatibilityMode string

const (
	CompatibilityStandalone       CompatibilityMode = "standalone"
	CompatibilityGentleCompatible CompatibilityMode = "gentle-compatible"
)

// ResumeState is caller-supplied progress evidence; validation does not persist it.
type ResumeState struct {
	ProfileName     string   `json:"profile_name"`
	CompletedStages []string `json:"completed_stages"`
	CurrentStage    string   `json:"current_stage"`
}

var profiles = map[string]WorkflowProfile{
	"odd": {
		Name:           "odd",
		Stages:         chain("authorize", "explore", "resolve-uncertainty", "classify", "track-if-substantial", "implement-task-by-task", "close"),
		Roles:          []string{"human", "coordinator", "explorer", "implementer", "verifier"},
		Checks:         []string{"TDD only when configured", "applicable functional checks", "coordinator spot-check"},
		MemoryPolicy:   "durable task ledger and Engram mirror for substantial work; store evidence as well as status",
		ReviewPolicy:   "inherit user-owned RDD switch; candidate consent remains separate",
		DeliveryPolicy: "one work unit per task; Conventional Commit only within authorization and repo policy; push/PR/merge remain separate decisions",
	},
	"sdd": {
		Name:           "sdd",
		Stages:         chain("dispatcher-selected-planning-phases", "tasks", "apply", "verify", "archive"),
		Roles:          []string{"human", "orchestrator", "phase-subagents"},
		Checks:         []string{"native dispatcher/dependencies", "configured TDD and apply checks", "record verify report before archive; findings do not automatically block archive"},
		MemoryPolicy:   "use only the store declared/resolved for the change; do not infer or mix stores",
		ReviewPolicy:   "inherit RDD and per-candidate consent; no inferred approval",
		DeliveryPolicy: "strategy declared for the change; default proposal ask-on-risk; remote delivery follows ordinary policy",
	},
	"standalone-minimal": {
		Name:           "standalone-minimal",
		Stages:         chain("authorize", "bound-scope", "execute-one-bounded-sequential-path", "check", "report"),
		Roles:          []string{"human", "sequential-executor"},
		Checks:         []string{"relevant available checks", "preserve unavailable/unverified; never synthesize PASS"},
		MemoryPolicy:   "no persistence required; allow only a store explicitly configured by the caller",
		ReviewPolicy:   "no Gentle/RDD dependency; state when native review is unavailable",
		DeliveryPolicy: "local by default; no implicit commit, push, PR, or release",
	},
	"maintenance": {
		Name:           "maintenance",
		Stages:         chain("inspect", "isolate-smallest-change", "repair", "validate", "report"),
		Roles:          []string{"human", "investigator", "maintainer", "verifier"},
		Checks:         []string{"before/after evidence", "focused checks", "expand with impact, not ceremony"},
		MemoryPolicy:   "record substantial work units; do not promote transient incidents to reusable memory",
		ReviewPolicy:   "inherit RDD; use applicable candidate review only when enabled",
		DeliveryPolicy: "bounded local change; delivery follows repository policy, with no automatic publication",
	},
	"incident-recovery": {
		Name:           "incident-recovery",
		Stages:         chain("preserve-evidence", "classify", "identify-supported-recovery", "obtain-required-authorization", "recover", "verify", "report"),
		Roles:          []string{"human-incident-owner", "investigator", "recovery-operator", "verifier"},
		Checks:         []string{"capture initial state", "use only supported audited recovery", "confirm final state", "stop when evidence is missing"},
		MemoryPolicy:   "case-bounded evidence; exclude secrets/raw logs; preserve verifiable references",
		ReviewPolicy:   "does not bypass RDD or maintenance authorization; use only supported native recovery",
		DeliveryPolicy: "recover only authorized scope; no publication or opportunistic scope expansion",
	},
}

func chain(names ...string) []Stage {
	stages := make([]Stage, len(names))
	for i, name := range names {
		var depends []string
		if i > 0 {
			depends = []string{names[i-1]}
		}
		stages[i] = Stage{Name: name, DependsOn: depends}
	}
	return stages
}

// Resolve returns a detached copy of a built-in profile. Unknown names fail closed.
func Resolve(name string) (WorkflowProfile, error) {
	profile, ok := profiles[name]
	if !ok {
		return WorkflowProfile{}, fmt.Errorf("%w: %q", ErrUnknownProfile, name)
	}
	return clone(profile), nil
}

// ResolveForCompatibility refuses combinations until an explicit support matrix
// exists; neither axis is inferred from the other.
func ResolveForCompatibility(profileName, compatibility string) (WorkflowProfile, error) {
	_, err := Resolve(profileName)
	if err != nil {
		return WorkflowProfile{}, err
	}
	mode := CompatibilityMode(compatibility)
	if mode != CompatibilityStandalone && mode != CompatibilityGentleCompatible {
		return WorkflowProfile{}, fmt.Errorf("%w: unknown compatibility mode %q", ErrUnsupportedCombination, compatibility)
	}
	return WorkflowProfile{}, fmt.Errorf("%w: profile %q with compatibility mode %q", ErrUnsupportedCombination, profileName, mode)
}

func clone(profile WorkflowProfile) WorkflowProfile {
	profile.Stages = append([]Stage(nil), profile.Stages...)
	for i := range profile.Stages {
		profile.Stages[i].DependsOn = append([]string(nil), profile.Stages[i].DependsOn...)
	}
	profile.Roles = append([]string(nil), profile.Roles...)
	profile.Checks = append([]string(nil), profile.Checks...)
	return profile
}

// Validate rejects missing contract fields, invalid dependencies, and missing
// profile-specific mandatory steps/gates. It validates data only; it authorizes nothing.
func Validate(profile WorkflowProfile) error {
	if _, ok := profiles[profile.Name]; !ok {
		return fmt.Errorf("%w: unknown name %q", ErrInvalidProfile, profile.Name)
	}
	if len(profile.Stages) == 0 || len(profile.Roles) == 0 || len(profile.Checks) == 0 ||
		strings.TrimSpace(profile.MemoryPolicy) == "" || strings.TrimSpace(profile.ReviewPolicy) == "" || strings.TrimSpace(profile.DeliveryPolicy) == "" {
		return fmt.Errorf("%w: all seven contract fields must be populated", ErrInvalidProfile)
	}
	indices := make(map[string]int, len(profile.Stages))
	for i, stage := range profile.Stages {
		if strings.TrimSpace(stage.Name) == "" {
			return fmt.Errorf("%w: stage %d has a blank name", ErrInvalidProfile, i)
		}
		if _, duplicate := indices[stage.Name]; duplicate {
			return fmt.Errorf("%w: duplicate stage %q", ErrInvalidProfile, stage.Name)
		}
		indices[stage.Name] = i
		for _, dependency := range stage.DependsOn {
			dependencyIndex, exists := indices[dependency]
			if !exists || dependencyIndex >= i {
				return fmt.Errorf("%w: stage %q depends on missing or unordered stage %q", ErrInvalidProfile, stage.Name, dependency)
			}
		}
	}
	expectedStages := requiredStages(profile.Name)
	if len(profile.Stages) != len(expectedStages) {
		return fmt.Errorf("%w: profile %q has an incomplete or unexpected stage sequence", ErrInvalidProfile, profile.Name)
	}
	for i, required := range expectedStages {
		if profile.Stages[i].Name != required {
			return fmt.Errorf("%w: profile %q stage %d must be %q", ErrInvalidProfile, profile.Name, i+1, required)
		}
		if i > 0 && !has(profile.Stages[i].DependsOn, expectedStages[i-1]) {
			return fmt.Errorf("%w: stage %q must depend on %q", ErrInvalidProfile, required, expectedStages[i-1])
		}
	}
	if profile.Name == "sdd" && !stageBefore(profile.Stages, "verify", "archive") {
		return fmt.Errorf("%w: SDD verify must precede archive", ErrInvalidProfile)
	}
	for _, required := range requiredChecks(profile.Name) {
		if !has(profile.Checks, required) {
			return fmt.Errorf("%w: profile %q is missing mandatory check %q", ErrInvalidProfile, profile.Name, required)
		}
	}
	return nil
}

func requiredStages(name string) []string {
	switch name {
	case "odd":
		return []string{"authorize", "explore", "resolve-uncertainty", "classify", "track-if-substantial", "implement-task-by-task", "close"}
	case "sdd":
		return []string{"dispatcher-selected-planning-phases", "tasks", "apply", "verify", "archive"}
	case "standalone-minimal":
		return []string{"authorize", "bound-scope", "execute-one-bounded-sequential-path", "check", "report"}
	case "maintenance":
		return []string{"inspect", "isolate-smallest-change", "repair", "validate", "report"}
	case "incident-recovery":
		return []string{"preserve-evidence", "classify", "identify-supported-recovery", "obtain-required-authorization", "recover", "verify", "report"}
	default:
		return nil
	}
}

func requiredChecks(name string) []string {
	switch name {
	case "odd":
		return []string{"TDD only when configured", "applicable functional checks", "coordinator spot-check"}
	case "sdd":
		return []string{"native dispatcher/dependencies", "configured TDD and apply checks", "record verify report before archive; findings do not automatically block archive"}
	case "standalone-minimal":
		return []string{"relevant available checks", "preserve unavailable/unverified; never synthesize PASS"}
	case "maintenance":
		return []string{"before/after evidence", "focused checks"}
	case "incident-recovery":
		return []string{"capture initial state", "use only supported audited recovery", "confirm final state", "stop when evidence is missing"}
	default:
		return nil
	}
}

func stageBefore(stages []Stage, first, second string) bool {
	firstIndex, secondIndex := -1, -1
	for i, stage := range stages {
		if stage.Name == first {
			firstIndex = i
		}
		if stage.Name == second {
			secondIndex = i
		}
	}
	return firstIndex >= 0 && secondIndex > firstIndex
}

func has(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

// ValidateResume validates caller-provided progress without persisting or
// resuming execution. Every completed stage must be earlier than current stage.
func ValidateResume(profile WorkflowProfile, state ResumeState) error {
	if err := Validate(profile); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidResumeState, err)
	}
	if state.ProfileName != profile.Name || state.CurrentStage == "" {
		return fmt.Errorf("%w: missing or mismatched profile/current stage", ErrInvalidResumeState)
	}
	indices := make(map[string]int, len(profile.Stages))
	for i, stage := range profile.Stages {
		indices[stage.Name] = i
	}
	currentIndex, ok := indices[state.CurrentStage]
	if !ok {
		return fmt.Errorf("%w: unknown current stage %q", ErrInvalidResumeState, state.CurrentStage)
	}
	completed := make(map[string]bool, len(state.CompletedStages))
	for _, name := range state.CompletedStages {
		index, exists := indices[name]
		if !exists || index >= currentIndex || completed[name] {
			return fmt.Errorf("%w: invalid completed stage %q", ErrInvalidResumeState, name)
		}
		completed[name] = true
	}
	for i := 0; i < currentIndex; i++ {
		if !completed[profile.Stages[i].Name] {
			return fmt.Errorf("%w: stage %q is missing from completed prefix", ErrInvalidResumeState, profile.Stages[i].Name)
		}
	}
	for _, dependency := range profile.Stages[currentIndex].DependsOn {
		if !completed[dependency] {
			return fmt.Errorf("%w: current stage %q lacks completed dependency %q", ErrInvalidResumeState, state.CurrentStage, dependency)
		}
	}
	return nil
}
