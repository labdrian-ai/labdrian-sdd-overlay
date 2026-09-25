// Package shaper defines the versioned, machine-only Shaper handoff record
// that describes a bounded plan handed from exploration/prototyping to
// execution. Structural validity, checked here, is purely deterministic: it
// says nothing about semantic consistency, readiness, or execution
// authority. In particular, the out-of-scope overlap rule detects literal,
// byte-identical overlap only; it proves nothing about differently worded
// items agreeing, about an item being semantically in or out of scope, or
// about two plan statements contradicting one another.
package shaper

import (
	"fmt"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/jsonstrict"
)

// Handoff is the approved v1 core of the Shaper handoff wire contract. Its
// field order is also the canonical known-fields order.
type Handoff struct {
	Version      int      `json:"version"`
	ProjectID    string   `json:"project_id"`
	GoalID       string   `json:"goal_id"`
	Architecture string   `json:"architecture"`
	Stages       []string `json:"stages"`
	Acceptance   []string `json:"acceptance"`
	OutOfScope   []string `json:"out_of_scope"`
}

// handoffAllowedFields lists every known Shaper handoff wire field for
// version 1. Adding any other field later requires a version increment or a
// separately approved additive change.
var handoffAllowedFields = []string{
	"version", "project_id", "goal_id", "architecture", "stages", "acceptance", "out_of_scope",
}

// Parse parses and validates one strict versioned Shaper handoff JSON
// document. It rejects duplicate or unknown fields, trailing input, and
// structurally invalid values without rewriting authored strings.
func Parse(data []byte) (Handoff, error) {
	if err := jsonstrict.CheckUTF8(data); err != nil {
		return Handoff{}, fmt.Errorf("parse shaper handoff: %w", err)
	}
	if err := jsonstrict.CheckNoDuplicateKeys(data); err != nil {
		return Handoff{}, fmt.Errorf("parse shaper handoff: %w", err)
	}
	if err := jsonstrict.CheckKnownFields(data, "Shaper handoff", handoffAllowedFields); err != nil {
		return Handoff{}, fmt.Errorf("parse shaper handoff: %w", err)
	}

	var h Handoff
	if err := jsonstrict.DecodeStrict(data, "Shaper handoff", &h); err != nil {
		return Handoff{}, fmt.Errorf("parse shaper handoff: %w", err)
	}
	if err := h.Validate(); err != nil {
		return Handoff{}, fmt.Errorf("parse shaper handoff: %w", err)
	}
	return h, nil
}

// Validate checks the version and deterministic structural requirements of a
// Shaper handoff, first error wins. It does not interpret natural-language
// meaning, infer permissions, or prove semantic scope consistency.
func (h Handoff) Validate() error {
	if h.Version != 1 {
		return fmt.Errorf("unsupported version %d, want 1", h.Version)
	}
	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "project_id", value: h.ProjectID},
		{name: "goal_id", value: h.GoalID},
		{name: "architecture", value: h.Architecture},
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
		{name: "stages", values: h.Stages, minItems: 1},
		{name: "acceptance", values: h.Acceptance, minItems: 1},
		{name: "out_of_scope", values: h.OutOfScope, minItems: 0},
	} {
		if err := validateStringArray(field.name, field.values, field.minItems); err != nil {
			return err
		}
	}

	for _, field := range []struct {
		name   string
		values []string
	}{
		{name: "stages", values: h.Stages},
		{name: "acceptance", values: h.Acceptance},
	} {
		if err := checkNoExactOverlap(field.name, field.values, h.OutOfScope); err != nil {
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

// checkNoExactOverlap rejects any item of values that is byte-identical to
// an item of outOfScope. No trimming, case folding, substring, or other
// normalization is applied: this detects literal overlap only and proves
// nothing about semantic scope.
func checkNoExactOverlap(name string, values []string, outOfScope []string) error {
	for i, value := range values {
		for _, excluded := range outOfScope {
			if value == excluded {
				return fmt.Errorf("%s item %d %q is byte-identical to an out_of_scope item", name, i+1, value)
			}
		}
	}
	return nil
}
