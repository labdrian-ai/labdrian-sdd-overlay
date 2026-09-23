// Package goal defines a versioned, runtime-neutral representation of user
// intent. A Goal is declarative data; it does not grant permission or dispatch
// work.
package goal

import (
	"fmt"
	"strings"
)

// Goal is the versioned declarative intent contract. Its field order is also
// the canonical JSON serialization order.
type Goal struct {
	Version            int      `json:"version"`
	ProjectID          string   `json:"project_id"`
	Objective          string   `json:"objective"`
	Scope              string   `json:"scope"`
	Constraints        []string `json:"constraints"`
	NonGoals           []string `json:"non_goals"`
	AcceptanceCriteria []string `json:"acceptance_criteria"`
	MemoryScope        string   `json:"memory_scope"`
	RuntimeScope       string   `json:"runtime_scope"`
	DeliveryBoundary   string   `json:"delivery_boundary"`
}

// Validate checks the version and deterministic structural requirements of a
// Goal. It does not interpret natural-language meaning or infer permissions.
func (g Goal) Validate() error {
	if g.Version != 1 {
		return fmt.Errorf("unsupported version %d, want 1", g.Version)
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
