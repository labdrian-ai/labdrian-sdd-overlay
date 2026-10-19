package shaper

// Version 3 of the handoff: version 2 plus roles, tests, risks, estimates, a
// memory scope and a delivery limit. It uses the acceptance rules of version 2
// (handoff_v2.go); the dispatch on the version is Parse, in shaper.go.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/fieldrule"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/jsonstrict"
)

// Role is one version 3 role with its free-form responsibility. Phase 4
// types the role vocabulary later; here role is only required to be
// non-blank and unique among its siblings.
type Role struct {
	Role           string `json:"role"`
	Responsibility string `json:"responsibility"`
}

// Risk is one version 3 risk with its planned mitigation.
type Risk struct {
	Risk       string `json:"risk"`
	Mitigation string `json:"mitigation"`
}

// Estimate is one version 3 per-stage agent-effort estimate, in whole
// minutes. It estimates the agent effort to execute one Goal stage,
// excluding plan preparation and human labor; it is not a calendar ETA.
type Estimate struct {
	// Stage must equal, byte for byte, exactly one Handoff.Stages item.
	Stage       string `json:"stage"`
	LowMinutes  int    `json:"low_minutes"`
	HighMinutes int    `json:"high_minutes"`
}

// handoffAllowedFieldsV3 lists every known wire field for version 3: version
// 2's fields plus the six version 3 additions. A version 1 or 2 document
// carrying any of these six fields is rejected as unknown, since Parse
// selects this list only when the top-level version is exactly 3.
var handoffAllowedFieldsV3 = []string{
	"version", "project_id", "goal_id", "architecture", "stages", "acceptance", "out_of_scope",
	"roles", "tests", "risks", "estimates", "memory_scope", "delivery_limit",
}

var (
	roleFields     = []string{"role", "responsibility"}
	riskFields     = []string{"risk", "mitigation"}
	estimateFields = []string{"stage", "low_minutes", "high_minutes"}
)

// handoffV3Wire is the version 3 decoding target: handoffV2Wire's fields plus
// the six version 3 additions.
type handoffV3Wire struct {
	Version       int              `json:"version"`
	ProjectID     string           `json:"project_id"`
	GoalID        string           `json:"goal_id"`
	Architecture  string           `json:"architecture"`
	Stages        []string         `json:"stages"`
	Acceptance    []AcceptanceItem `json:"acceptance"`
	OutOfScope    []string         `json:"out_of_scope"`
	Roles         []Role           `json:"roles"`
	Tests         []string         `json:"tests"`
	Risks         []Risk           `json:"risks"`
	Estimates     []Estimate       `json:"estimates"`
	MemoryScope   string           `json:"memory_scope"`
	DeliveryLimit string           `json:"delivery_limit"`
}

// parseV3 strictly parses a version 3 handoff whose top-level wire checks
// already passed: version 2's acceptance shape plus roles, tests, risks, and
// estimates, each checked for unknown or case-variant nested keys before
// decoding.
func parseV3(data []byte) (Handoff, error) {
	if err := checkAcceptanceV2Fields(data); err != nil {
		return Handoff{}, err
	}
	if err := checkV3NestedFields(data); err != nil {
		return Handoff{}, err
	}
	var w handoffV3Wire
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
		MemoryScope:     w.MemoryScope,
		DeliveryLimit:   w.DeliveryLimit,
	}
	if err := h.Validate(); err != nil {
		return Handoff{}, err
	}
	return h, nil
}

// checkV3NestedFields rejects, with exact-case matching, any unknown key in a
// roles or risks item, and a null item in roles, risks, or estimates,
// mirroring checkAcceptanceV2Fields for the version 3 additions. estimates
// items carry no free-form field, so no case-variant check applies beyond
// their own known-field set. An absent or null roles, risks, or estimates
// array is left to decoding and Validate.
func checkV3NestedFields(data []byte) error {
	isNull := func(v json.RawMessage) bool { return bytes.Equal(bytes.TrimSpace(v), []byte("null")) }
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	for _, group := range []struct {
		key    string
		name   string
		fields []string
	}{
		{"roles", "Shaper handoff role", roleFields},
		{"risks", "Shaper handoff risk", riskFields},
		{"estimates", "Shaper handoff estimate", estimateFields},
	} {
		v, ok := raw[group.key]
		if !ok || isNull(v) {
			continue
		}
		var items []json.RawMessage
		if err := json.Unmarshal(v, &items); err != nil {
			return fmt.Errorf("%s: %w", group.key, err)
		}
		for i, item := range items {
			if isNull(item) {
				return fmt.Errorf("%s item %d must be an object, got null", group.key, i+1)
			}
			if err := jsonstrict.CheckKnownFields(item, group.name, group.fields); err != nil {
				return fmt.Errorf("%s item %d: %w", group.key, i+1, err)
			}
		}
	}
	return nil
}

// validateV3 applies validateAcceptanceCore, then the six version 3
// additions: at least one role with a unique non-blank role and a non-blank
// responsibility; at least one non-blank planned test; a non-null risks
// array whose entries, if any, carry a non-blank risk and mitigation; exactly
// one positive-minute, low-at-most-high estimate per stage; and non-blank
// memory_scope and delivery_limit. The out_of_scope overlap rule extends to
// tests and role responsibilities.
func (h Handoff) validateV3() error {
	return h.validatePlan(func() error {
		if h.MemoryDirective != nil {
			return fmt.Errorf("memory_directive is only valid in version 4")
		}
		if strings.TrimSpace(h.MemoryScope) == "" {
			return fmt.Errorf("memory_scope must be a non-blank string")
		}
		return nil
	})
}

// validatePlan holds the rules versions 3 and 4 share, in order, with memory
// checking the version's own memory field where version 3 checks
// memory_scope.
func (h Handoff) validatePlan(memory func() error) error {
	if err := h.validateAcceptanceCore(); err != nil {
		return err
	}
	if err := validateRoles(h.Roles); err != nil {
		return err
	}
	if err := fieldrule.CheckStringArray("tests", h.Tests, 1); err != nil {
		return err
	}
	if err := validateRisks(h.Risks); err != nil {
		return err
	}
	if err := validateEstimates(h.Stages, h.Estimates); err != nil {
		return err
	}
	if err := memory(); err != nil {
		return err
	}
	if strings.TrimSpace(h.DeliveryLimit) == "" {
		return fmt.Errorf("delivery_limit must be a non-blank string")
	}
	if err := checkNoExactOverlap("tests", h.Tests, h.OutOfScope); err != nil {
		return err
	}
	return checkNoExactOverlap("roles responsibility", roleResponsibilities(h.Roles), h.OutOfScope)
}

// validateRoles requires at least one role, each with a non-blank role and
// responsibility, and no two roles sharing the same byte-identical role
// text.
func validateRoles(roles []Role) error {
	if roles == nil {
		return fmt.Errorf("roles must be a non-null array")
	}
	if len(roles) < 1 {
		return fmt.Errorf("roles must contain at least 1 item(s)")
	}
	seen := make(map[string]bool, len(roles))
	for i, r := range roles {
		if strings.TrimSpace(r.Role) == "" {
			return fmt.Errorf("roles item %d role must be non-blank", i+1)
		}
		if strings.TrimSpace(r.Responsibility) == "" {
			return fmt.Errorf("roles item %d responsibility must be non-blank", i+1)
		}
		if seen[r.Role] {
			return fmt.Errorf("roles item %d role %q duplicates an earlier role", i+1, r.Role)
		}
		seen[r.Role] = true
	}
	return nil
}

// validateRisks requires a non-null risks array; it may be empty, but every
// entry must carry a non-blank risk and mitigation.
func validateRisks(risks []Risk) error {
	if risks == nil {
		return fmt.Errorf("risks must be a non-null array")
	}
	for i, r := range risks {
		if strings.TrimSpace(r.Risk) == "" {
			return fmt.Errorf("risks item %d risk must be non-blank", i+1)
		}
		if strings.TrimSpace(r.Mitigation) == "" {
			return fmt.Errorf("risks item %d mitigation must be non-blank", i+1)
		}
	}
	return nil
}

// validateEstimates requires exactly one estimate per stage, each stage
// referenced by its exact text, with positive low_minutes and high_minutes
// and low_minutes <= high_minutes. A count mismatch, an estimate stage that
// matches no handoff stage, and a stage referenced by more than one estimate
// are each reported explicitly.
func validateEstimates(stages []string, estimates []Estimate) error {
	if estimates == nil {
		return fmt.Errorf("estimates must be a non-null array")
	}
	if len(estimates) != len(stages) {
		return fmt.Errorf("estimates must contain exactly one item per stage, got %d for %d stage(s)", len(estimates), len(stages))
	}
	stageSet := make(map[string]bool, len(stages))
	for _, s := range stages {
		stageSet[s] = true
	}
	seen := make(map[string]bool, len(estimates))
	for i, e := range estimates {
		if !stageSet[e.Stage] {
			return fmt.Errorf("estimates item %d stage %q does not match any stage", i+1, e.Stage)
		}
		if seen[e.Stage] {
			return fmt.Errorf("estimates item %d stage %q duplicates an earlier estimate", i+1, e.Stage)
		}
		seen[e.Stage] = true
		if e.LowMinutes <= 0 {
			return fmt.Errorf("estimates item %d low_minutes must be a positive integer, got %d", i+1, e.LowMinutes)
		}
		if e.HighMinutes <= 0 {
			return fmt.Errorf("estimates item %d high_minutes must be a positive integer, got %d", i+1, e.HighMinutes)
		}
		if e.LowMinutes > e.HighMinutes {
			return fmt.Errorf("estimates item %d low_minutes %d must be <= high_minutes %d", i+1, e.LowMinutes, e.HighMinutes)
		}
	}
	return nil
}

// roleResponsibilities returns the responsibility text of every role, in
// order.
func roleResponsibilities(roles []Role) []string {
	out := make([]string, 0, len(roles))
	for _, r := range roles {
		out = append(out, r.Responsibility)
	}
	return out
}
