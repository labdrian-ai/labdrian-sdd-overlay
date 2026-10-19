package shaper

// Version 4 of the handoff: version 3 with memory_scope, a free-form string,
// replaced by memory_directive, a strict MemoryDirective v1 object
// (memoryscope.ParseDirective). Every other version 3 rule applies unchanged;
// the dispatch on the version is Parse, in shaper.go.

import (
	"fmt"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/jsonstrict"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/memoryscope"
)

// handoffAllowedFieldsV4 lists every known wire field for version 4: version
// 3's fields with memory_scope replaced by memory_directive. Parse selects it
// only when the top-level version is exactly 4, so a version 1 to 3 document
// carrying memory_directive, and a version 4 document carrying memory_scope,
// are each rejected as unknown.
var handoffAllowedFieldsV4 = []string{
	"version", "project_id", "goal_id", "architecture", "stages", "acceptance", "out_of_scope",
	"roles", "tests", "risks", "estimates", "memory_directive", "delivery_limit",
}

// handoffV4Wire is the version 4 decoding target: handoffV3Wire with
// memory_directive in place of memory_scope.
type handoffV4Wire struct {
	Version         int                    `json:"version"`
	ProjectID       string                 `json:"project_id"`
	GoalID          string                 `json:"goal_id"`
	Architecture    string                 `json:"architecture"`
	Stages          []string               `json:"stages"`
	Acceptance      []AcceptanceItem       `json:"acceptance"`
	OutOfScope      []string               `json:"out_of_scope"`
	Roles           []Role                 `json:"roles"`
	Tests           []string               `json:"tests"`
	Risks           []Risk                 `json:"risks"`
	Estimates       []Estimate             `json:"estimates"`
	MemoryDirective *memoryscope.Directive `json:"memory_directive"`
	DeliveryLimit   string                 `json:"delivery_limit"`
}

// parseV4 strictly parses a version 4 handoff whose top-level wire checks
// already passed: version 3's nested checks, then the memory directive through
// memoryscope.ParseDirective, whose exact-case known-field check also covers
// the directive's keys.
func parseV4(data []byte) (Handoff, error) {
	if err := checkAcceptanceV2Fields(data); err != nil {
		return Handoff{}, err
	}
	if err := checkV3NestedFields(data); err != nil {
		return Handoff{}, err
	}
	directive, err := memoryscope.ParseDirectiveMember(data, "memory_directive")
	if err != nil {
		return Handoff{}, err
	}
	var w handoffV4Wire
	if err := jsonstrict.DecodeStrict(data, "Shaper handoff", &w); err != nil {
		return Handoff{}, err
	}
	h := Handoff{
		Version:         w.Version,
		ProjectID:       w.ProjectID,
		GoalID:          w.GoalID,
		Architecture:    w.Architecture,
		Stages:          w.Stages,
		OutOfScope:      w.OutOfScope,
		AcceptanceItems: w.Acceptance,
		Roles:           w.Roles,
		Tests:           w.Tests,
		Risks:           w.Risks,
		Estimates:       w.Estimates,
		MemoryDirective: directive,
		DeliveryLimit:   w.DeliveryLimit,
	}
	if err := h.Validate(); err != nil {
		return Handoff{}, err
	}
	return h, nil
}

// validateV4 applies every version 3 rule, with a valid memory_directive
// (memoryscope's own Validate) required in place of memory_scope.
func (h Handoff) validateV4() error {
	return h.validatePlan(func() error {
		if h.MemoryScope != "" {
			return fmt.Errorf("memory_scope is only valid in version 3")
		}
		if h.MemoryDirective == nil {
			return fmt.Errorf("memory_directive must be a non-null object")
		}
		if err := h.MemoryDirective.Validate(); err != nil {
			return fmt.Errorf("memory_directive: %w", err)
		}
		return nil
	})
}

// formatDirective renders d on one line, in its field order, for the presented
// view: for example "version=1 scope=goal sources=[engram,longterm-mem]
// write=none". The rendering is deterministic, and an empty source list shows
// as "sources=[]".
func formatDirective(d memoryscope.Directive) string {
	sources := make([]string, len(d.Sources))
	for i, s := range d.Sources {
		sources[i] = string(s)
	}
	return fmt.Sprintf("version=%d scope=%s sources=[%s] write=%s", d.Version, d.Scope, strings.Join(sources, ","), d.Write)
}
