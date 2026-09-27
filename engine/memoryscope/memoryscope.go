// Package memoryscope defines a typed MemoryDirective and resolves one or
// more directives into a read-only query plan. It executes nothing: the
// actual memory query or write belongs to a runtime adapter outside this
// package (Phase 7). This package never imports longterm-mem internals,
// keeping the memory-layer boundary intact.
package memoryscope

import (
	"fmt"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/jsonstrict"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflowprofile"
)

// DirectiveVersion is the only MemoryDirective wire version this package
// accepts.
const DirectiveVersion = 1

// Scope is how far a memory directive is allowed to read: none reads
// nothing, goal is bounded to one Goal within a project, and project spans
// the whole project. Scopes are ordered none < goal < project.
type Scope string

const (
	ScopeNone    Scope = "none"
	ScopeGoal    Scope = "goal"
	ScopeProject Scope = "project"
)

// scopeOrder gives every known scope its position in the none < goal <
// project order used to detect widening.
var scopeOrder = map[Scope]int{
	ScopeNone:    0,
	ScopeGoal:    1,
	ScopeProject: 2,
}

// Source is one member of the closed set of memory backends a directive may
// read from.
type Source string

const (
	SourceEngram           Source = "engram"
	SourceLongtermMem      Source = "longterm-mem"
	SourceProceduralSkills Source = "procedural-skills"
)

var knownSources = map[Source]bool{
	SourceEngram:           true,
	SourceLongtermMem:      true,
	SourceProceduralSkills: true,
}

// Directive is one MemoryDirective v1 record: a declared scope, a
// duplicate-free subset of the closed source set, and a write mode that is
// always "none". Reading memory never grants writing it.
type Directive struct {
	Version int      `json:"version"`
	Scope   Scope    `json:"scope"`
	Sources []Source `json:"sources"`
	Write   string   `json:"write"`
}

var directiveFields = []string{"version", "scope", "sources", "write"}

// ParseDirective strictly parses one MemoryDirective record: valid UTF-8, no
// duplicate keys, no unknown fields (exact case), no trailing data, the
// pinned version, and every field shape via Validate.
func ParseDirective(data []byte) (Directive, error) {
	if err := jsonstrict.CheckUTF8(data); err != nil {
		return Directive{}, fmt.Errorf("parse memory directive: %w", err)
	}
	if err := jsonstrict.CheckNoDuplicateKeys(data); err != nil {
		return Directive{}, fmt.Errorf("parse memory directive: %w", err)
	}
	if err := jsonstrict.CheckKnownFields(data, "memory directive", directiveFields); err != nil {
		return Directive{}, fmt.Errorf("parse memory directive: %w", err)
	}

	var d Directive
	if err := jsonstrict.DecodeStrict(data, "memory directive", &d); err != nil {
		return Directive{}, fmt.Errorf("parse memory directive: %w", err)
	}
	if err := d.Validate(); err != nil {
		return Directive{}, fmt.Errorf("parse memory directive: %w", err)
	}
	return d, nil
}

// Validate checks every field shape of d. It is first-error-wins.
func (d Directive) Validate() error {
	if d.Version != DirectiveVersion {
		return fmt.Errorf("version must be %d, got %d", DirectiveVersion, d.Version)
	}
	if _, ok := scopeOrder[d.Scope]; !ok {
		return fmt.Errorf("scope must be %q, %q, or %q, got %q", ScopeNone, ScopeGoal, ScopeProject, d.Scope)
	}
	if d.Sources == nil {
		return fmt.Errorf("sources must be a non-null array")
	}
	seen := make(map[Source]bool, len(d.Sources))
	for _, s := range d.Sources {
		if !knownSources[s] {
			return fmt.Errorf("unknown source %q", s)
		}
		if seen[s] {
			return fmt.Errorf("duplicate source %q", s)
		}
		seen[s] = true
	}
	if d.Scope == ScopeNone && len(d.Sources) != 0 {
		return fmt.Errorf("scope %q requires empty sources, got %v", ScopeNone, d.Sources)
	}
	if d.Write != "none" {
		return fmt.Errorf(`write must be exactly "none", got %q`, d.Write)
	}
	return nil
}

// odd's memory_policy (engine/workflowprofile.go): "durable task ledger and
// Engram mirror for substantial work; store evidence as well as status" —
// the task ledger and its Engram mirror are scoped to one feature (a goal),
// not the whole project.
var oddDefault = Directive{Version: DirectiveVersion, Scope: ScopeGoal, Sources: []Source{SourceEngram}, Write: "none"}

// sdd's memory_policy: "use only the store declared/resolved for the
// change; do not infer or mix stores" — a change's artifacts are
// project-scoped (topic keys are sdd/{change-name}/..., not tied to one
// Goal), and Engram is the store SDD resolves to by default when a backend
// is available.
var sddDefault = Directive{Version: DirectiveVersion, Scope: ScopeProject, Sources: []Source{SourceEngram}, Write: "none"}

// standalone-minimal's memory_policy: "no persistence required; allow only
// a store explicitly configured by the caller" — no default source is
// assumed.
var standaloneMinimalDefault = Directive{Version: DirectiveVersion, Scope: ScopeNone, Sources: []Source{}, Write: "none"}

// maintenance's memory_policy: "record substantial work units; do not
// promote transient incidents to reusable memory" — one bounded unit is a
// goal, recorded through Engram only; "do not promote ... to reusable
// memory" excludes procedural-skills.
var maintenanceDefault = Directive{Version: DirectiveVersion, Scope: ScopeGoal, Sources: []Source{SourceEngram}, Write: "none"}

// incident-recovery's memory_policy: "case-bounded evidence; exclude
// secrets/raw logs; preserve verifiable references" — "case-bounded" means
// one goal (the incident case), through Engram only.
var incidentRecoveryDefault = Directive{Version: DirectiveVersion, Scope: ScopeGoal, Sources: []Source{SourceEngram}, Write: "none"}

var profileDefaults = map[string]Directive{
	"odd":                oddDefault,
	"sdd":                sddDefault,
	"standalone-minimal": standaloneMinimalDefault,
	"maintenance":        maintenanceDefault,
	"incident-recovery":  incidentRecoveryDefault,
}

// DefaultFor returns the default memory directive for one of the five
// workflow profiles, derived from that profile's existing memory_policy
// prose (see the comments above each default). An unknown profile name is
// refused.
func DefaultFor(profileName string) (Directive, error) {
	if _, err := workflowprofile.Resolve(profileName); err != nil {
		return Directive{}, err
	}
	d, ok := profileDefaults[profileName]
	if !ok {
		return Directive{}, fmt.Errorf("no memory directive default registered for profile %q", profileName)
	}
	return d, nil
}

// Filters narrows a Plan's read to one project and, for scope goal, one
// Goal within it.
type Filters struct {
	ProjectID string `json:"project_id,omitempty"`
	GoalID    string `json:"goal_id,omitempty"`
}

// planAuthority states that a Plan grants no execution: it is a read-only
// description of what a runtime adapter would be allowed to query, not a
// query itself.
const planAuthority = "this plan executes no query and grants no memory write; executing it is a runtime adapter's responsibility outside this package (Phase 7)"

// Plan is the effective, executable-nothing description of what memory a
// caller may read: scope, sources, identifying filters, a write mode that
// is always "none", and a no-authority statement.
type Plan struct {
	Scope     Scope    `json:"scope"`
	Sources   []Source `json:"sources"`
	Filters   Filters  `json:"filters"`
	Write     string   `json:"write"`
	Authority string   `json:"authority"`
}

// Resolve narrows base by narrowers, applied in order, into one query Plan.
// Each narrower must not exceed the running scope (none < goal < project)
// and must use a subset of the running sources; either violation refuses
// with a named reason. The effective directive is the most narrowed one:
// base if there are no narrowers, otherwise the last narrower applied.
// projectID and goalID come from the caller (typically a strictly parsed
// Goal v2); they populate Plan.Filters and are validated against the
// effective scope: project_id is required for scope goal and project,
// goal_id is only accepted for scope goal, and neither is accepted for
// scope none.
func Resolve(base Directive, projectID, goalID string, narrowers ...Directive) (Plan, error) {
	if err := base.Validate(); err != nil {
		return Plan{}, fmt.Errorf("base directive: %w", err)
	}
	running := base
	for i, narrower := range narrowers {
		if err := narrower.Validate(); err != nil {
			return Plan{}, fmt.Errorf("narrower[%d]: %w", i, err)
		}
		if scopeOrder[narrower.Scope] > scopeOrder[running.Scope] {
			return Plan{}, fmt.Errorf("narrower[%d] widens scope from %s to %s", i, running.Scope, narrower.Scope)
		}
		runningSources := toSourceSet(running.Sources)
		for _, s := range narrower.Sources {
			if !runningSources[s] {
				return Plan{}, fmt.Errorf("narrower[%d] adds source %s not present in the current scope", i, s)
			}
		}
		running = narrower
	}
	return newPlan(running, projectID, goalID)
}

func newPlan(effective Directive, projectID, goalID string) (Plan, error) {
	var filters Filters
	switch effective.Scope {
	case ScopeNone:
		if projectID != "" {
			return Plan{}, fmt.Errorf("project_id must not be supplied for scope %q", ScopeNone)
		}
		if goalID != "" {
			return Plan{}, fmt.Errorf("goal_id must not be supplied for scope %q", ScopeNone)
		}
	case ScopeGoal:
		if strings.TrimSpace(projectID) == "" {
			return Plan{}, fmt.Errorf("project_id is required for scope %q", ScopeGoal)
		}
		filters.ProjectID = projectID
		filters.GoalID = goalID
	case ScopeProject:
		if strings.TrimSpace(projectID) == "" {
			return Plan{}, fmt.Errorf("project_id is required for scope %q", ScopeProject)
		}
		if goalID != "" {
			return Plan{}, fmt.Errorf("goal_id must not be supplied for scope %q", ScopeProject)
		}
		filters.ProjectID = projectID
	default:
		return Plan{}, fmt.Errorf("unknown scope %q", effective.Scope)
	}
	sources := append([]Source(nil), effective.Sources...)
	return Plan{
		Scope:     effective.Scope,
		Sources:   sources,
		Filters:   filters,
		Write:     "none",
		Authority: planAuthority,
	}, nil
}

func toSourceSet(sources []Source) map[Source]bool {
	set := make(map[Source]bool, len(sources))
	for _, s := range sources {
		set[s] = true
	}
	return set
}
