// Package goal defines a versioned, runtime-neutral representation of user
// intent. A Goal is declarative data; it does not grant permission or dispatch
// work.
package goal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

// Goal is the versioned declarative intent contract. Its field order is also
// the canonical JSON serialization order.
type Goal struct {
	Version   int    `json:"version"`
	ProjectID string `json:"project_id"`
	// GoalID distinguishes Goals within ProjectID in version 2; it is not globally unique.
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
	if !utf8.Valid(data) {
		return Goal{}, fmt.Errorf("parse goal: input is not valid UTF-8")
	}
	if err := checkDuplicateJSONKeys(data); err != nil {
		return Goal{}, fmt.Errorf("parse goal: %w", err)
	}
	if err := checkGoalFieldNames(data); err != nil {
		return Goal{}, fmt.Errorf("parse goal: %w", err)
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	var g Goal
	if err := dec.Decode(&g); err != nil {
		return Goal{}, fmt.Errorf("parse goal: %w", err)
	}
	if err := dec.Decode(new(json.RawMessage)); err != io.EOF {
		return Goal{}, fmt.Errorf("parse goal: trailing data after the Goal value")
	}
	if err := g.Validate(); err != nil {
		return Goal{}, fmt.Errorf("parse goal: %w", err)
	}
	return g, nil
}

func checkGoalFieldNames(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	version := 0
	if rawVersion, ok := fields["version"]; ok {
		_ = json.Unmarshal(rawVersion, &version)
	}
	for field := range fields {
		switch field {
		case "version", "project_id", "objective", "scope", "constraints", "non_goals", "acceptance_criteria", "memory_scope", "runtime_scope", "delivery_boundary":
		case "goal_id":
			if version != 2 {
				return fmt.Errorf("unknown Goal field %q", field)
			}
		default:
			return fmt.Errorf("unknown Goal field %q", field)
		}
	}
	return nil
}

// checkDuplicateJSONKeys validates the JSON token structure while rejecting
// duplicate keys in every object, before decoding into a Go struct can discard
// duplicate member values.
func checkDuplicateJSONKeys(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := scanJSONValue(dec); err != nil {
		if err == io.EOF {
			return fmt.Errorf("empty JSON document")
		}
		return err
	}
	if _, err := dec.Token(); err != io.EOF {
		if err == nil {
			return fmt.Errorf("trailing data after JSON value")
		}
		return fmt.Errorf("trailing data after JSON value: %w", err)
	}
	return nil
}

func scanJSONValue(dec *json.Decoder) error {
	token, err := dec.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}

	switch delim {
	case '{':
		seen := make(map[string]struct{})
		for dec.More() {
			keyToken, err := dec.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("object member name is not a string")
			}
			if _, exists := seen[key]; exists {
				return fmt.Errorf("duplicate JSON object key %q", key)
			}
			seen[key] = struct{}{}
			if err := scanJSONValue(dec); err != nil {
				return err
			}
		}
		end, err := dec.Token()
		if err != nil {
			return err
		}
		if end != json.Delim('}') {
			return fmt.Errorf("invalid JSON object termination")
		}
	case '[':
		for dec.More() {
			if err := scanJSONValue(dec); err != nil {
				return err
			}
		}
		end, err := dec.Token()
		if err != nil {
			return err
		}
		if end != json.Delim(']') {
			return fmt.Errorf("invalid JSON array termination")
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delim)
	}
	return nil
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
