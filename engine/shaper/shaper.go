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
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/fieldrule"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/jsonstrict"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/memoryscope"
)

// The Shaper handoff versions Parse accepts, each the one before it with one
// change.
const (
	// HandoffVersionCriteria fills acceptance with plain criterion strings.
	HandoffVersionCriteria = 1
	// HandoffVersionVerified fills acceptance with items, each a criterion
	// with its planned verification.
	HandoffVersionVerified = 2
	// HandoffVersionPlan adds the plan: roles, tests, risks, estimates,
	// memory_scope, and delivery_limit.
	HandoffVersionPlan = 3
	// HandoffVersionDirective replaces memory_scope with memory_directive, a
	// strict MemoryDirective v1 object.
	HandoffVersionDirective = 4
)

// Handoff is the approved core of the Shaper handoff wire contract, in
// versions 1 to 4; the fields after OutOfScope belong to the later versions.
// Its field order is also the canonical known-fields order. Version 1 fills
// Acceptance with plain criterion strings; version 2 fills AcceptanceItems
// instead, so exactly one of the two is non-nil for a handoff Parse returns.
// Version 3 adds the plan (Roles, Tests, Risks, Estimates, MemoryScope, and
// DeliveryLimit), and version 4 replaces MemoryScope with MemoryDirective.
type Handoff struct {
	Version      int      `json:"version"`
	ProjectID    string   `json:"project_id"`
	GoalID       string   `json:"goal_id"`
	Architecture string   `json:"architecture"`
	Stages       []string `json:"stages"`
	// Acceptance holds the version 1 criteria; it is nil for version 2 to 4.
	Acceptance []string `json:"acceptance"`
	OutOfScope []string `json:"out_of_scope"`
	// AcceptanceItems holds the version 2 to 4 criteria, each with its
	// planned verification; it is nil for version 1. It is decoded from the
	// same "acceptance" wire field by the version 2 to 4 parsers, never from
	// its own wire name.
	AcceptanceItems []AcceptanceItem `json:"-"`
	// Roles holds the version 3 and 4 role list; it is nil for version 1 and
	// 2.
	Roles []Role `json:"-"`
	// Tests holds the version 3 and 4 planned test list; it is nil for
	// version 1 and 2.
	Tests []string `json:"-"`
	// Risks holds the version 3 and 4 risk list; it is nil for version 1 and
	// 2.
	Risks []Risk `json:"-"`
	// Estimates holds the version 3 and 4 per-stage estimate list; it is nil
	// for version 1 and 2.
	Estimates []Estimate `json:"-"`
	// MemoryScope is the version 3 memory scope; it is "" for version 1, 2,
	// and 4.
	MemoryScope string `json:"-"`
	// MemoryDirective is the version 4 typed memory narrower, a strict
	// MemoryDirective v1 object that replaces memory_scope; it is nil for
	// version 1 to 3, which narrow nothing.
	MemoryDirective *memoryscope.Directive `json:"-"`
	// DeliveryLimit is the version 3 and 4 delivery limit; it is "" for
	// version 1 and 2.
	DeliveryLimit string `json:"-"`
}

// handoffAllowedFields lists every known Shaper handoff wire field for
// versions 1 and 2. Adding any other field later requires a version increment
// or a separately approved additive change.
var handoffAllowedFields = []string{
	"version", "project_id", "goal_id", "architecture", "stages", "acceptance", "out_of_scope",
}

// Parse parses and validates one strict versioned Shaper handoff JSON
// document. It rejects duplicate or unknown fields, trailing input, and
// structurally invalid values without rewriting authored strings.
//
// A document whose version member is the JSON number 2 is parsed as version
// 2, which also rejects unknown or case-variant keys inside every acceptance
// item and its verification object. A document whose version member is the
// JSON number 3 is parsed as version 3, version 2 plus roles, tests, risks,
// estimates, memory_scope, and delivery_limit, applying the same strict
// nested-key checks to every new nested object. A document whose version
// member is the JSON number 4 is parsed as version 4, version 3 with
// memory_scope replaced by memory_directive, a strict MemoryDirective v1
// object. Every other document takes the version 1 path unchanged.
func Parse(data []byte) (Handoff, error) {
	if err := jsonstrict.CheckUTF8(data); err != nil {
		return Handoff{}, fmt.Errorf("parse shaper handoff: %w", err)
	}
	if err := jsonstrict.CheckNoDuplicateKeys(data); err != nil {
		return Handoff{}, fmt.Errorf("parse shaper handoff: %w", err)
	}
	version := wireVersion(data)
	allowedFields := handoffAllowedFields
	switch version {
	case HandoffVersionPlan:
		allowedFields = handoffAllowedFieldsV3
	case HandoffVersionDirective:
		allowedFields = handoffAllowedFieldsV4
	}
	if err := jsonstrict.CheckKnownFields(data, "Shaper handoff", allowedFields); err != nil {
		return Handoff{}, fmt.Errorf("parse shaper handoff: %w", err)
	}
	var parseVersion func([]byte) (Handoff, error)
	switch version {
	case HandoffVersionDirective:
		parseVersion = parseV4
	case HandoffVersionPlan:
		parseVersion = parseV3
	case HandoffVersionVerified:
		parseVersion = parseV2
	}
	if parseVersion != nil {
		h, err := parseVersion(data)
		if err != nil {
			return Handoff{}, fmt.Errorf("parse shaper handoff: %w", err)
		}
		return h, nil
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
	switch h.Version {
	case HandoffVersionDirective:
		return h.validateV4()
	case HandoffVersionPlan:
		return h.validateV3()
	case HandoffVersionVerified:
		return h.validateV2()
	case HandoffVersionCriteria:
		// Checked below.
	default:
		return fmt.Errorf("unsupported version %d, want 1", h.Version)
	}
	if h.AcceptanceItems != nil {
		return fmt.Errorf("version 1 acceptance must be a string array; acceptance items require version 2")
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
		if err := fieldrule.CheckStringArray(field.name, field.values, field.minItems); err != nil {
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

// wireVersion is the version Parse dispatches on: the top-level, exact-case
// version member of data when it is a JSON integer written as Go's strconv.Itoa
// writes it, and 0 otherwise. Any value that names no version 2 to 4,
// including 2.0, 02, or a string, is left to the version 1 path, which reports
// it exactly as before the later versions existed.
func wireVersion(data []byte) int {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return 0
	}
	raw := string(bytes.TrimSpace(fields["version"]))
	version, err := strconv.Atoi(raw)
	if err != nil || strconv.Itoa(version) != raw {
		return 0
	}
	return version
}
