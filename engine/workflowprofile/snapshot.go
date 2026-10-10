package workflowprofile

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// Snapshot is a workflow profile as a workflow's log records it: everything the profile declares
// that a program acts on, the seven fields of the contract and the typed data behind the memory
// and review policies, so that a workflow can be carried on without the catalog that served it. It
// is the profile in its wire form; WorkflowProfile is the same data to work with. The two convert
// without loss (Profile, WorkflowProfile.Snapshot).
//
// The encoding of a snapshot is part of the format of the workflow log. Its fields are written in
// the order they are declared here, as encoding/json writes a struct, and a list is never null: an
// empty one is []. Digest is the SHA-256 of that encoding.
type Snapshot struct {
	Name                 string        `json:"name"`
	Stages               []Stage       `json:"stages"`
	Roles                []string      `json:"roles"`
	Checks               []string      `json:"checks"`
	MemoryPolicy         string        `json:"memory_policy"`
	ReviewPolicy         string        `json:"review_policy"`
	DeliveryPolicy       string        `json:"delivery_policy"`
	MemoryDefault        MemoryDefault `json:"memory_default"`
	ReliesOnGentleReview bool          `json:"relies_on_gentle_review"`
}

// Snapshot is the profile in the form a workflow log records it: a copy, so that nothing done to
// the profile reaches it, with every list present (an empty one is empty, not nil).
func (p WorkflowProfile) Snapshot() Snapshot {
	s := Snapshot{
		Name:                 p.Name,
		Stages:               make([]Stage, len(p.Stages)),
		Roles:                append([]string{}, p.Roles...),
		Checks:               append([]string{}, p.Checks...),
		MemoryPolicy:         p.MemoryPolicy,
		ReviewPolicy:         p.ReviewPolicy,
		DeliveryPolicy:       p.DeliveryPolicy,
		MemoryDefault:        MemoryDefault{Scope: p.MemoryDefault.Scope, Sources: append([]MemorySource{}, p.MemoryDefault.Sources...)},
		ReliesOnGentleReview: p.ReliesOnGentleReview,
	}
	for i, stage := range p.Stages {
		s.Stages[i] = Stage{Name: stage.Name, DependsOn: append([]string{}, stage.DependsOn...)}
	}
	return s
}

// Profile is the profile the snapshot records, as a copy of it. It does not validate: Validate
// says whether the snapshot is well formed, and the profile of one that is not is as malformed as
// it was.
func (s Snapshot) Profile() WorkflowProfile {
	return clone(WorkflowProfile{
		Name:                 s.Name,
		Stages:               s.Stages,
		Roles:                s.Roles,
		Checks:               s.Checks,
		MemoryPolicy:         s.MemoryPolicy,
		ReviewPolicy:         s.ReviewPolicy,
		DeliveryPolicy:       s.DeliveryPolicy,
		MemoryDefault:        s.MemoryDefault,
		ReliesOnGentleReview: s.ReliesOnGentleReview,
	})
}

// Digest is the SHA-256 of the snapshot's encoding (see Snapshot), in lowercase hex. Two snapshots
// have one digest if and only if every field of them is the same, so it says whether a profile is
// the one a log recorded.
func (s Snapshot) Digest() (string, error) {
	data, err := json.Marshal(s)
	if err != nil {
		return "", fmt.Errorf("workflow profile snapshot digest: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// Validate says whether the snapshot is well formed, on its own: it asks nothing of the catalog, so
// a profile that is in no catalog is valid, and one that differs from the catalog's of the same
// name is valid too (that is drift, not malformation). It requires the seven fields to be
// populated, the stages to be named once each and to depend only on earlier ones, the lists to be
// present (an empty list is [], and a null one is refused, so that one profile has one encoding),
// and the memory default to be made of a scope and sources that exist.
func (s Snapshot) Validate() error {
	if strings.TrimSpace(s.Name) == "" {
		return fmt.Errorf("%w: the name is blank", ErrInvalidProfile)
	}
	if err := checkShape(s.Profile()); err != nil {
		return err
	}
	for i, stage := range s.Stages {
		if stage.DependsOn == nil {
			return fmt.Errorf("%w: stage %d (%q) has a null dependency list, want a list", ErrInvalidProfile, i, stage.Name)
		}
	}
	if s.MemoryDefault.Sources == nil {
		return fmt.Errorf("%w: the memory default has a null list of sources, want a list", ErrInvalidProfile)
	}
	switch s.MemoryDefault.Scope {
	case MemoryScopeNone, MemoryScopeGoal, MemoryScopeProject:
	default:
		return fmt.Errorf("%w: the memory scope %q is not one of %q, %q, %q", ErrInvalidProfile, s.MemoryDefault.Scope, MemoryScopeNone, MemoryScopeGoal, MemoryScopeProject)
	}
	for _, source := range s.MemoryDefault.Sources {
		switch source {
		case MemorySourceEngram, MemorySourceLongtermMem, MemorySourceProceduralSkills:
		default:
			return fmt.Errorf("%w: the memory source %q is not one of %q, %q, %q", ErrInvalidProfile, source, MemorySourceEngram, MemorySourceLongtermMem, MemorySourceProceduralSkills)
		}
	}
	return nil
}

// DifferingFields names the fields in which the snapshot and other differ, by their names in the
// encoding and in the order of it: name, stages, roles, checks, memory_policy, review_policy,
// delivery_policy, memory_default and relies_on_gentle_review. It is empty when they are equal.
func (s Snapshot) DifferingFields(other Snapshot) []string {
	var differing []string
	note := func(field string, differs bool) {
		if differs {
			differing = append(differing, field)
		}
	}
	note("name", s.Name != other.Name)
	note("stages", !slices.EqualFunc(s.Stages, other.Stages, func(a, b Stage) bool {
		return a.Name == b.Name && slices.Equal(a.DependsOn, b.DependsOn)
	}))
	note("roles", !slices.Equal(s.Roles, other.Roles))
	note("checks", !slices.Equal(s.Checks, other.Checks))
	note("memory_policy", s.MemoryPolicy != other.MemoryPolicy)
	note("review_policy", s.ReviewPolicy != other.ReviewPolicy)
	note("delivery_policy", s.DeliveryPolicy != other.DeliveryPolicy)
	note("memory_default", s.MemoryDefault.Scope != other.MemoryDefault.Scope || !slices.Equal(s.MemoryDefault.Sources, other.MemoryDefault.Sources))
	note("relies_on_gentle_review", s.ReliesOnGentleReview != other.ReliesOnGentleReview)
	return differing
}
