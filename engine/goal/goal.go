// Package goal defines a versioned, runtime-neutral representation of user
// intent. A Goal is declarative data; it does not grant permission or dispatch
// work.
package goal

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/jsonstrict"
)

// Goal is the versioned declarative intent contract. Its field order is also
// the canonical JSON serialization order.
type Goal struct {
	Version   int    `json:"version"`
	ProjectID string `json:"project_id"`
	// GoalID distinguishes Goals within ProjectID in version 2; it is not
	// globally unique. It is an opaque caller-supplied string: Validate requires
	// at least one rune that is not Unicode whitespace (unicode.IsSpace) and
	// imposes no character-set or path format. Values decoded by Parse are valid
	// UTF-8 and are kept verbatim; consumers must compare GoalID exactly.
	GoalID             string   `json:"goal_id,omitempty"`
	Objective          string   `json:"objective"`
	Scope              string   `json:"scope"`
	Constraints        []string `json:"constraints"`
	NonGoals           []string `json:"non_goals"`
	AcceptanceCriteria []string `json:"acceptance_criteria"`
	MemoryScope        string   `json:"memory_scope"`
	RuntimeScope       string   `json:"runtime_scope"`
	DeliveryBoundary   string   `json:"delivery_boundary"`
}

// Parse parses and validates one strict versioned Goal JSON document. It
// rejects duplicate or unknown fields, trailing input, and structurally
// invalid Goal values without rewriting authored strings.
func Parse(data []byte) (Goal, error) {
	if err := jsonstrict.CheckUTF8(data); err != nil {
		return Goal{}, fmt.Errorf("parse goal: %w", err)
	}
	if err := jsonstrict.CheckNoDuplicateKeys(data); err != nil {
		return Goal{}, fmt.Errorf("parse goal: %w", err)
	}
	if err := jsonstrict.CheckKnownFields(data, "Goal", goalAllowedFields); err != nil {
		return Goal{}, fmt.Errorf("parse goal: %w", err)
	}

	var g Goal
	if err := jsonstrict.DecodeStrict(data, "Goal", &g); err != nil {
		return Goal{}, fmt.Errorf("parse goal: %w", err)
	}
	if g.Version == 1 {
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(data, &raw); err != nil {
			return Goal{}, fmt.Errorf("parse goal: %w", err)
		}
		if _, present := raw["goal_id"]; present {
			return Goal{}, fmt.Errorf("parse goal: goal_id is only valid in version 2")
		}
	}
	if err := g.Validate(); err != nil {
		return Goal{}, fmt.Errorf("parse goal: %w", err)
	}
	return g, nil
}

// goalAllowedFields lists every known Goal wire field across supported
// versions. goal_id is only meaningful in version 2; Parse rejects the key in
// any form (including "" or null) alongside version 1, and Validate rejects a
// non-empty GoalID on a version-1 Goal.
var goalAllowedFields = []string{
	"version", "project_id", "goal_id", "objective", "scope", "constraints",
	"non_goals", "acceptance_criteria", "memory_scope", "runtime_scope",
	"delivery_boundary",
}

// Validate checks the version and deterministic structural requirements of a
// Goal. It does not interpret natural-language meaning or infer permissions.
func (g Goal) Validate() error {
	if g.Version != 1 && g.Version != 2 {
		return fmt.Errorf("unsupported version %d, want 1 or 2", g.Version)
	}
	if g.Version == 1 && g.GoalID != "" {
		return fmt.Errorf("goal_id is only valid in version 2")
	}
	if g.Version == 2 && strings.TrimSpace(g.GoalID) == "" {
		return fmt.Errorf("goal_id must be a non-blank string")
	}
	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "project_id", value: g.ProjectID},
		{name: "objective", value: g.Objective},
		{name: "scope", value: g.Scope},
		{name: "memory_scope", value: g.MemoryScope},
		{name: "runtime_scope", value: g.RuntimeScope},
		{name: "delivery_boundary", value: g.DeliveryBoundary},
	} {
		if strings.TrimSpace(field.value) == "" {
			return fmt.Errorf("%s must be a non-blank string", field.name)
		}
	}
	for _, field := range []struct {
		name     string
		values   []string
		minItems int
	}{
		{name: "constraints", values: g.Constraints},
		{name: "non_goals", values: g.NonGoals},
		{name: "acceptance_criteria", values: g.AcceptanceCriteria, minItems: 1},
	} {
		if err := validateStringArray(field.name, field.values, field.minItems); err != nil {
			return err
		}
	}
	return nil
}

// Marshal returns the canonical JSON representation of a valid Goal: fields
// in contract order, two-space indentation, and exactly one trailing newline.
func (g Goal) Marshal() ([]byte, error) {
	if err := g.Validate(); err != nil {
		return nil, fmt.Errorf("marshal goal: %w", err)
	}
	data, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal goal: %w", err)
	}
	return append(data, '\n'), nil
}

func validateStringArray(name string, values []string, minItems int) error {
	if values == nil {
		return fmt.Errorf("%s must be a non-null string array", name)
	}
	if len(values) < minItems {
		return fmt.Errorf("%s must contain at least %d item(s)", name, minItems)
	}
	for i, value := range values {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s item %d must be non-blank", name, i+1)
		}
	}
	return nil
}
