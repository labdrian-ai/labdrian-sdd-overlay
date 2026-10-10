// Package workflowprofile defines declarative, runtime-neutral workflow profiles.
// Profiles describe work organization; they do not authorize or execute work.
package workflowprofile

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrUnknownProfile = errors.New("unknown workflow profile")
	ErrInvalidProfile = errors.New("invalid workflow profile")
)

// Stage is one named workflow step. Dependencies must refer to earlier stages.
type Stage struct {
	Name      string   `json:"name"`
	DependsOn []string `json:"depends_on"`
}

// MemoryScope is how far a profile's default memory read reaches: none reads nothing, goal is
// bounded to one goal within a project, and project spans the whole project.
type MemoryScope string

const (
	MemoryScopeNone    MemoryScope = "none"
	MemoryScopeGoal    MemoryScope = "goal"
	MemoryScopeProject MemoryScope = "project"
)

// MemorySource is one memory backend a profile's default may read from.
type MemorySource string

const (
	MemorySourceEngram           MemorySource = "engram"
	MemorySourceLongtermMem      MemorySource = "longterm-mem"
	MemorySourceProceduralSkills MemorySource = "procedural-skills"
)

// MemoryDefault is the ceiling of what a request under a profile can read from memory: the widest
// scope and the sources the profile's memory_policy prose allows. A request may only narrow it
// (engine/memoryscope refuses any widening). It never grants a write.
type MemoryDefault struct {
	Scope   MemoryScope
	Sources []MemorySource
}

// WorkflowProfile is the seven-field declarative Phase 2 contract, plus the typed data that the
// prose fields stand for: the memory default behind memory_policy and the review dependency behind
// review_policy. The typed data is not part of the contract's wire form (json:"-"), so a profile
// still serializes exactly its seven fields; the prose says what the policy is for a reader, and
// the typed fields say it for a program, so that no program decides either by the profile's name.
type WorkflowProfile struct {
	Name           string   `json:"name"`
	Stages         []Stage  `json:"stages"`
	Roles          []string `json:"roles"`
	Checks         []string `json:"checks"`
	MemoryPolicy   string   `json:"memory_policy"`
	ReviewPolicy   string   `json:"review_policy"`
	DeliveryPolicy string   `json:"delivery_policy"`

	// MemoryDefault is the default memory read of the profile (see MemoryDefault).
	MemoryDefault MemoryDefault `json:"-"`
	// ReliesOnGentleReview says that the profile's review_policy inherits Gentle AI's
	// receipt-driven development (RDD) review, so a workflow of it records whether that review is
	// available. The profile that declares "no Gentle/RDD dependency" is the one that does not.
	// The flag is explicit rather than derived from the policy prose, so rewording a policy cannot
	// silently change which dependencies a workflow records; a test fails when prose and flag
	// disagree.
	ReliesOnGentleReview bool `json:"-"`
}

// definition is one built-in profile as declared: the profile itself, and the checks of it that
// Validate does not insist on (the others are mandatory). It is the one place a profile's stages
// and checks are written; Resolve serves them and Validate compares a profile with them.
type definition struct {
	profile        WorkflowProfile
	optionalChecks []string
}

// catalog is every built-in profile, by name.
var catalog = index(
	definition{profile: WorkflowProfile{
		Name:           "odd",
		Stages:         chain("authorize", "explore", "resolve-uncertainty", "classify", "track-if-substantial", "implement-task-by-task", "close"),
		Roles:          []string{"human", "coordinator", "explorer", "implementer", "verifier"},
		Checks:         []string{"TDD only when configured", "applicable functional checks", "coordinator spot-check"},
		MemoryPolicy:   "durable task ledger and Engram mirror for substantial work; store evidence as well as status; may read project evidence from Engram, long-term memory, and procedural skills",
		ReviewPolicy:   "inherit user-owned RDD switch; candidate consent remains separate",
		DeliveryPolicy: "one work unit per task; Conventional Commit only within authorization and repo policy; push/PR/merge remain separate decisions",
		// The policy names all three stores directly; a single feature narrows to scope goal.
		MemoryDefault:        MemoryDefault{Scope: MemoryScopeProject, Sources: []MemorySource{MemorySourceEngram, MemorySourceLongtermMem, MemorySourceProceduralSkills}},
		ReliesOnGentleReview: true,
	}},
	definition{profile: WorkflowProfile{
		Name:           "sdd",
		Stages:         chain("dispatcher-selected-planning-phases", "tasks", "apply", "verify", "archive"),
		Roles:          []string{"human", "orchestrator", "phase-subagents"},
		Checks:         []string{"native dispatcher/dependencies", "configured TDD and apply checks", "record verify report before archive; findings do not automatically block archive"},
		MemoryPolicy:   "use only the store declared/resolved for the change; do not infer or mix stores",
		ReviewPolicy:   "inherit RDD and per-candidate consent; no inferred approval",
		DeliveryPolicy: "strategy declared for the change; default proposal ask-on-risk; remote delivery follows ordinary policy",
		// A change's artifacts are project-scoped (topic keys are sdd/{change-name}/..., not tied to
		// one goal), and the ceiling is the single store SDD resolves to by default, Engram, so the
		// unnarrowed plan never mixes stores.
		MemoryDefault:        MemoryDefault{Scope: MemoryScopeProject, Sources: []MemorySource{MemorySourceEngram}},
		ReliesOnGentleReview: true,
	}},
	definition{profile: WorkflowProfile{
		Name:           "standalone-minimal",
		Stages:         chain("authorize", "bound-scope", "execute-one-bounded-sequential-path", "check", "report"),
		Roles:          []string{"human", "sequential-executor"},
		Checks:         []string{"relevant available checks", "preserve unavailable/unverified; never synthesize PASS"},
		MemoryPolicy:   "no persistence required; allow only a store explicitly configured by the caller",
		ReviewPolicy:   "no Gentle/RDD dependency; state when native review is unavailable",
		DeliveryPolicy: "local by default; no implicit commit, push, PR, or release",
		// No default source is assumed.
		MemoryDefault:        MemoryDefault{Scope: MemoryScopeNone, Sources: []MemorySource{}},
		ReliesOnGentleReview: false,
	}},
	definition{profile: WorkflowProfile{
		Name:           "maintenance",
		Stages:         chain("inspect", "isolate-smallest-change", "repair", "validate", "report"),
		Roles:          []string{"human", "investigator", "maintainer", "verifier"},
		Checks:         []string{"before/after evidence", "focused checks", "expand with impact, not ceremony"},
		MemoryPolicy:   "record substantial work units; do not promote transient incidents to reusable memory; may read project evidence from Engram, long-term memory, and procedural skills",
		ReviewPolicy:   "inherit RDD; use applicable candidate review only when enabled",
		DeliveryPolicy: "bounded local change; delivery follows repository policy, with no automatic publication",
		// The policy names all three stores directly; one bounded unit narrows to scope goal.
		MemoryDefault:        MemoryDefault{Scope: MemoryScopeProject, Sources: []MemorySource{MemorySourceEngram, MemorySourceLongtermMem, MemorySourceProceduralSkills}},
		ReliesOnGentleReview: true,
	}, optionalChecks: []string{"expand with impact, not ceremony"}},
	definition{profile: WorkflowProfile{
		Name:           "incident-recovery",
		Stages:         chain("preserve-evidence", "classify", "identify-supported-recovery", "obtain-required-authorization", "recover", "verify", "report"),
		Roles:          []string{"human-incident-owner", "investigator", "recovery-operator", "verifier"},
		Checks:         []string{"capture initial state", "use only supported audited recovery", "confirm final state", "stop when evidence is missing"},
		MemoryPolicy:   "case-bounded evidence; exclude secrets/raw logs; preserve verifiable references; may read the case's Engram evidence and procedural skills",
		ReviewPolicy:   "does not bypass RDD or maintenance authorization; use only supported native recovery",
		DeliveryPolicy: "recover only authorized scope; no publication or opportunistic scope expansion",
		// "Case-bounded" means one goal (the incident case), and the policy names exactly those two
		// stores, not the project's broad long-term memory.
		MemoryDefault:        MemoryDefault{Scope: MemoryScopeGoal, Sources: []MemorySource{MemorySourceEngram, MemorySourceProceduralSkills}},
		ReliesOnGentleReview: true,
	}},
)

func index(definitions ...definition) map[string]definition {
	byName := make(map[string]definition, len(definitions))
	for _, d := range definitions {
		byName[d.profile.Name] = d
	}
	return byName
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
	d, ok := catalog[name]
	if !ok {
		return WorkflowProfile{}, fmt.Errorf("%w: %q", ErrUnknownProfile, name)
	}
	return clone(d.profile), nil
}

func clone(profile WorkflowProfile) WorkflowProfile {
	profile.Stages = append([]Stage(nil), profile.Stages...)
	for i := range profile.Stages {
		profile.Stages[i].DependsOn = append([]string(nil), profile.Stages[i].DependsOn...)
	}
	profile.Roles = append([]string(nil), profile.Roles...)
	profile.Checks = append([]string(nil), profile.Checks...)
	profile.MemoryDefault.Sources = append([]MemorySource{}, profile.MemoryDefault.Sources...)
	return profile
}

// CheckStagePrefix says whether recorded, the names of the stages a workflow has recorded, is a
// prefix of the profile's declared stage order: recorded[i] is Stages[i].Name for every i, and there
// are no more of them than the profile declares. It is the one rule for what a log may say about its
// stages; the error names the first place the log departs from the order.
func (p WorkflowProfile) CheckStagePrefix(recorded []string) error {
	if len(recorded) > len(p.Stages) {
		return fmt.Errorf("recorded %d stages exceeds profile %q's %d declared stages", len(recorded), p.Name, len(p.Stages))
	}
	for i, stage := range recorded {
		if p.Stages[i].Name != stage {
			return fmt.Errorf("recorded stage %d is %q, want %q per profile %q's declared order", i, stage, p.Stages[i].Name, p.Name)
		}
	}
	return nil
}

// NextStage is the stage declared after the recorded ones, given how many are recorded; it says
// false when every declared stage is recorded. It does not look at which stages they are: that is
// CheckStagePrefix's question.
func (p WorkflowProfile) NextStage(recorded int) (string, bool) {
	if recorded >= len(p.Stages) {
		return "", false
	}
	return p.Stages[recorded].Name, true
}

// Validate rejects missing contract fields, invalid dependencies, and a stage sequence or set of
// mandatory checks that differs from the catalog's for the profile's name. It validates data only;
// it authorizes nothing.
func Validate(profile WorkflowProfile) error {
	d, ok := catalog[profile.Name]
	if !ok {
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
	expected := d.profile.Stages
	if len(profile.Stages) != len(expected) {
		return fmt.Errorf("%w: profile %q has an incomplete or unexpected stage sequence", ErrInvalidProfile, profile.Name)
	}
	for i, required := range expected {
		if profile.Stages[i].Name != required.Name {
			return fmt.Errorf("%w: profile %q stage %d must be %q", ErrInvalidProfile, profile.Name, i+1, required.Name)
		}
		if i > 0 && !has(profile.Stages[i].DependsOn, expected[i-1].Name) {
			return fmt.Errorf("%w: stage %q must depend on %q", ErrInvalidProfile, required.Name, expected[i-1].Name)
		}
	}
	for _, required := range d.profile.Checks {
		if has(d.optionalChecks, required) {
			continue
		}
		if !has(profile.Checks, required) {
			return fmt.Errorf("%w: profile %q is missing mandatory check %q", ErrInvalidProfile, profile.Name, required)
		}
	}
	return nil
}

func has(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
