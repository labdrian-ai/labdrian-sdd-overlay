// Package goal defines a versioned, runtime-neutral representation of user
// intent. A Goal is declarative data; it does not grant permission or dispatch
// work.
package goal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/fieldrule"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/jsonstrict"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/memoryscope"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/sha256hex"
)

// Goal is the versioned declarative intent contract. Its field order is also
// the canonical JSON serialization order.
type Goal struct {
	Version   int    `json:"version"`
	ProjectID string `json:"project_id"`
	// GoalID distinguishes Goals within ProjectID in versions 2 and 3; it is not
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
	// MemoryScope is the free-text memory scope of versions 1 and 2; it is ""
	// for version 3, whose Marshal omits it.
	MemoryScope string `json:"memory_scope,omitempty"`
	// MemoryDirective is the version 3 typed memory narrower, a strict
	// MemoryDirective v1 object; it is nil for versions 1 and 2, which narrow
	// nothing.
	MemoryDirective  *memoryscope.Directive `json:"memory_directive,omitempty"`
	RuntimeScope     string                 `json:"runtime_scope"`
	DeliveryBoundary string                 `json:"delivery_boundary"`
}

// Parse parses and validates one strict versioned Goal JSON document. It
// rejects duplicate or unknown fields, trailing input, and structurally
// invalid Goal values without rewriting authored strings. A document whose
// version member is the JSON number 3 is parsed as version 3, whose
// memory_directive is parsed by memoryscope.ParseDirective; every other
// document takes the version 1 and 2 path unchanged.
func Parse(data []byte) (Goal, error) {
	if err := jsonstrict.CheckUTF8(data); err != nil {
		return Goal{}, fmt.Errorf("parse goal: %w", err)
	}
	if err := jsonstrict.CheckNoDuplicateKeys(data); err != nil {
		return Goal{}, fmt.Errorf("parse goal: %w", err)
	}
	v3 := isVersion3(data)
	allowedFields := goalAllowedFields
	if v3 {
		allowedFields = goalAllowedFieldsV3
	}
	if err := jsonstrict.CheckKnownFields(data, "Goal", allowedFields); err != nil {
		return Goal{}, fmt.Errorf("parse goal: %w", err)
	}
	var directive *memoryscope.Directive
	if v3 {
		d, err := parseDirectiveField(data)
		if err != nil {
			return Goal{}, fmt.Errorf("parse goal: %w", err)
		}
		directive = d
	}

	var g Goal
	if err := jsonstrict.DecodeStrict(data, "Goal", &g); err != nil {
		return Goal{}, fmt.Errorf("parse goal: %w", err)
	}
	g.MemoryDirective = directive
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

// goalAllowedFields lists every known Goal wire field of versions 1 and 2.
// goal_id is only meaningful in version 2; Parse rejects the key in
// any form (including "" or null) alongside version 1, and Validate rejects a
// non-empty GoalID on a version-1 Goal.
var goalAllowedFields = []string{
	"version", "project_id", "goal_id", "objective", "scope", "constraints",
	"non_goals", "acceptance_criteria", "memory_scope", "runtime_scope",
	"delivery_boundary",
}

// goalAllowedFieldsV3 lists every wire field of a version 3 Goal: version 2's
// fields with memory_scope replaced by memory_directive. Parse selects it only
// when the top-level version member is exactly the JSON number 3, so a version
// 1 or 2 document carrying memory_directive, and a version 3 document carrying
// memory_scope, are each refused as an unknown field.
var goalAllowedFieldsV3 = []string{
	"version", "project_id", "goal_id", "objective", "scope", "constraints",
	"non_goals", "acceptance_criteria", "memory_directive", "runtime_scope",
	"delivery_boundary",
}

// isVersion3 reports whether the top-level version member of data is exactly
// the JSON number 3. Any other value takes the version 1 and 2 path, which
// reports it exactly as before version 3 existed.
func isVersion3(data []byte) bool {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return false
	}
	return bytes.Equal(bytes.TrimSpace(fields["version"]), []byte("3"))
}

// parseDirectiveField strictly parses the memory_directive member of a
// version 3 Goal with memoryscope.ParseDirective, so its nested keys get the
// same exact-case unknown-field check as the Goal's own. An absent or null
// member returns nil, left to Validate to refuse.
func parseDirectiveField(data []byte) (*memoryscope.Directive, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	raw, ok := fields["memory_directive"]
	if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, nil
	}
	d, err := memoryscope.ParseDirective(raw)
	if err != nil {
		return nil, fmt.Errorf("memory_directive: %w", err)
	}
	return &d, nil
}

// Validate checks the version and deterministic structural requirements of a
// Goal. It does not interpret natural-language meaning or infer permissions.
func (g Goal) Validate() error {
	if g.Version < 1 || g.Version > 3 {
		return fmt.Errorf("unsupported version %d, want 1, 2, or 3", g.Version)
	}
	if g.Version == 1 && g.GoalID != "" {
		return fmt.Errorf("goal_id is only valid in version 2")
	}
	if g.Version >= 2 && strings.TrimSpace(g.GoalID) == "" {
		return fmt.Errorf("goal_id must be a non-blank string")
	}
	if err := checkNonBlank(
		namedValue{name: "project_id", value: g.ProjectID},
		namedValue{name: "objective", value: g.Objective},
		namedValue{name: "scope", value: g.Scope},
	); err != nil {
		return err
	}
	if err := g.validateMemory(); err != nil {
		return err
	}
	if err := checkNonBlank(
		namedValue{name: "runtime_scope", value: g.RuntimeScope},
		namedValue{name: "delivery_boundary", value: g.DeliveryBoundary},
	); err != nil {
		return err
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
		if err := fieldrule.CheckStringArray(field.name, field.values, field.minItems); err != nil {
			return err
		}
	}
	return nil
}

// namedValue is one named string a Goal requires to be non-blank.
type namedValue struct {
	name  string
	value string
}

// checkNonBlank returns the error of the first field that is blank.
func checkNonBlank(fields ...namedValue) error {
	for _, f := range fields {
		if strings.TrimSpace(f.value) == "" {
			return fmt.Errorf("%s must be a non-blank string", f.name)
		}
	}
	return nil
}

// validateMemory keeps each memory field to its version: versions 1 and 2
// require a non-blank memory_scope and carry no directive; version 3 requires
// a valid memory_directive (memoryscope's own Validate) and no memory_scope.
func (g Goal) validateMemory() error {
	if g.Version != 3 {
		if g.MemoryDirective != nil {
			return fmt.Errorf("memory_directive is only valid in version 3")
		}
		return checkNonBlank(namedValue{name: "memory_scope", value: g.MemoryScope})
	}
	if g.MemoryScope != "" {
		return fmt.Errorf("memory_scope is only valid in version 1 or 2")
	}
	if g.MemoryDirective == nil {
		return fmt.Errorf("memory_directive must be a non-null object")
	}
	if err := g.MemoryDirective.Validate(); err != nil {
		return fmt.Errorf("memory_directive: %w", err)
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

// Digest returns the lowercase hex SHA-256 of g's canonical Marshal encoding.
// Marshal validates g and produces a deterministic, field-ordered encoding, so
// the digest changes if and only if g's contents change, whatever bytes the
// Goal was parsed from. A Goal that Marshal refuses has no digest, and the
// error is Marshal's own.
func (g Goal) Digest() (string, error) {
	data, err := g.Marshal()
	if err != nil {
		return "", err
	}
	return sha256hex.Sum(data), nil
}
