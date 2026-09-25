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
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/jsonstrict"
)

// Handoff is the approved core of the Shaper handoff wire contract, in
// version 1 or version 2. Its field order is also the canonical known-fields
// order. The two versions share every field and differ only in the shape of
// acceptance: a version 1 handoff fills Acceptance with plain criterion
// strings, and a version 2 handoff fills AcceptanceItems instead. Exactly one
// of the two is non-nil for a handoff Parse returns.
type Handoff struct {
	Version      int      `json:"version"`
	ProjectID    string   `json:"project_id"`
	GoalID       string   `json:"goal_id"`
	Architecture string   `json:"architecture"`
	Stages       []string `json:"stages"`
	// Acceptance holds the version 1 criteria; it is nil for version 2.
	Acceptance []string `json:"acceptance"`
	OutOfScope []string `json:"out_of_scope"`
	// AcceptanceItems holds the version 2 criteria, each with its planned
	// verification; it is nil for version 1. It is decoded from the same
	// "acceptance" wire field by the version 2 parser, never from its own
	// wire name.
	AcceptanceItems []AcceptanceItem `json:"-"`
}

// AcceptanceItem is one version 2 acceptance criterion together with the
// verification planned for it.
type AcceptanceItem struct {
	// Criterion is the non-blank criterion text.
	Criterion string `json:"criterion"`
	// Verification is the planned way to show the criterion holds.
	Verification AcceptanceVerification `json:"verification"`
}

// AcceptanceVerification carries exactly one of Check or Adjudication.
//
// Both are planned verification only. Nothing in this package, and nothing
// readiness consults, executes a check or performs an adjudication: their
// results are downstream fulfillment evidence, not a readiness prerequisite.
// A ready handoff therefore says only that each criterion names how it will
// be verified, never that any criterion was met, and it is not a signature:
// any process running as the same OS user can forge a clearance record.
type AcceptanceVerification struct {
	// Check is a deterministic command or test to run.
	Check string `json:"check,omitempty"`
	// Adjudication is a human adjudication path naming what must be judged.
	Adjudication string `json:"adjudication,omitempty"`
}

// handoffAllowedFields lists every known Shaper handoff wire field for
// versions 1 and 2. Adding any other field later requires a version increment
// or a separately approved additive change.
var handoffAllowedFields = []string{
	"version", "project_id", "goal_id", "architecture", "stages", "acceptance", "out_of_scope",
}

var (
	acceptanceItemFields         = []string{"criterion", "verification"}
	acceptanceVerificationFields = []string{"check", "adjudication"}
)

// handoffV2Wire is the version 2 decoding target: the Handoff wire fields
// with acceptance decoded as objects.
type handoffV2Wire struct {
	Version      int              `json:"version"`
	ProjectID    string           `json:"project_id"`
	GoalID       string           `json:"goal_id"`
	Architecture string           `json:"architecture"`
	Stages       []string         `json:"stages"`
	Acceptance   []AcceptanceItem `json:"acceptance"`
	OutOfScope   []string         `json:"out_of_scope"`
}

// Parse parses and validates one strict versioned Shaper handoff JSON
// document. It rejects duplicate or unknown fields, trailing input, and
// structurally invalid values without rewriting authored strings.
//
// A document whose version member is the JSON number 2 is parsed as version
// 2, which also rejects unknown or case-variant keys inside every acceptance
// item and its verification object. Every other document takes the version 1
// path unchanged.
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
	if isVersion2(data) {
		h, err := parseV2(data)
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
	if h.Version == 2 {
		return h.validateV2()
	}
	if h.Version != 1 {
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

// isVersion2 reports whether the top-level version member of data is exactly
// the JSON number 2. Any other value, including 2.0 or a string, is left to
// the version 1 path, which reports it exactly as before version 2 existed.
func isVersion2(data []byte) bool {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return false
	}
	return bytes.Equal(bytes.TrimSpace(fields["version"]), []byte("2"))
}

// parseV2 strictly parses a version 2 handoff whose top-level wire checks
// already passed.
func parseV2(data []byte) (Handoff, error) {
	if err := checkAcceptanceV2Fields(data); err != nil {
		return Handoff{}, err
	}
	var w handoffV2Wire
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
	}
	if err := h.Validate(); err != nil {
		return Handoff{}, err
	}
	return h, nil
}

// checkAcceptanceV2Fields rejects, with exact-case matching, any unknown key
// in an acceptance item or its verification object, a null item, a null
// member, and a verification object that does not hold exactly one member.
// encoding/json matches object keys to struct fields without regard to case,
// and the last matching key wins, so without this check a case-variant key
// such as "CHECK" could carry the value Go validates while a case-sensitive
// reader of the same bytes sees another. An absent or null acceptance array
// is left to decoding and Validate.
func checkAcceptanceV2Fields(data []byte) error {
	isNull := func(v json.RawMessage) bool { return bytes.Equal(bytes.TrimSpace(v), []byte("null")) }
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	v, ok := raw["acceptance"]
	if !ok || isNull(v) {
		return nil
	}
	var items []json.RawMessage
	if err := json.Unmarshal(v, &items); err != nil {
		return fmt.Errorf("acceptance: %w", err)
	}
	for i, item := range items {
		if isNull(item) {
			return fmt.Errorf("acceptance item %d must be an object, got null", i+1)
		}
		if err := jsonstrict.CheckKnownFields(item, "Shaper handoff acceptance item", acceptanceItemFields); err != nil {
			return fmt.Errorf("acceptance item %d: %w", i+1, err)
		}
		var members map[string]json.RawMessage
		if err := json.Unmarshal(item, &members); err != nil {
			return fmt.Errorf("acceptance item %d: %w", i+1, err)
		}
		if c, ok := members["criterion"]; ok && isNull(c) {
			return fmt.Errorf("acceptance item %d criterion must be a non-blank string, got null", i+1)
		}
		ver, ok := members["verification"]
		if !ok {
			return fmt.Errorf("acceptance item %d verification is missing", i+1)
		}
		if isNull(ver) {
			return fmt.Errorf("acceptance item %d verification must be an object, got null", i+1)
		}
		if err := jsonstrict.CheckKnownFields(ver, "Shaper handoff acceptance verification", acceptanceVerificationFields); err != nil {
			return fmt.Errorf("acceptance item %d verification: %w", i+1, err)
		}
		var methods map[string]json.RawMessage
		if err := json.Unmarshal(ver, &methods); err != nil {
			return fmt.Errorf("acceptance item %d verification: %w", i+1, err)
		}
		if len(methods) != 1 {
			return fmt.Errorf("acceptance item %d verification must carry exactly one of check or adjudication, got %d members", i+1, len(methods))
		}
		for name, value := range methods {
			if isNull(value) {
				return fmt.Errorf("acceptance item %d verification %s must be a non-blank string, got null", i+1, name)
			}
		}
	}
	return nil
}

// validateV2 applies the version 1 rules to every shared field and the
// version 2 acceptance rules: at least one item, each with a non-blank
// criterion and exactly one non-blank check or adjudication. The
// out_of_scope overlap rule applies to stages and to criterion text; check
// and adjudication text are verification plans, not scope, and are not
// compared.
func (h Handoff) validateV2() error {
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
	if err := validateStringArray("stages", h.Stages, 1); err != nil {
		return err
	}
	if h.Acceptance != nil {
		return fmt.Errorf("version 2 acceptance must be an array of criterion objects, not strings")
	}
	if h.AcceptanceItems == nil {
		return fmt.Errorf("acceptance must be a non-null array of criterion objects")
	}
	if len(h.AcceptanceItems) < 1 {
		return fmt.Errorf("acceptance must contain at least 1 item(s)")
	}
	for i, item := range h.AcceptanceItems {
		if strings.TrimSpace(item.Criterion) == "" {
			return fmt.Errorf("acceptance item %d criterion must be non-blank", i+1)
		}
		if err := item.Verification.validate(i + 1); err != nil {
			return err
		}
	}
	if err := validateStringArray("out_of_scope", h.OutOfScope, 0); err != nil {
		return err
	}
	if err := checkNoExactOverlap("stages", h.Stages, h.OutOfScope); err != nil {
		return err
	}
	return checkNoExactOverlap("acceptance criterion", acceptanceCriteria(h.AcceptanceItems), h.OutOfScope)
}

// validate requires exactly one of Check or Adjudication to be present, and
// the present one to be non-blank. item is the 1-based acceptance index.
func (v AcceptanceVerification) validate(item int) error {
	hasCheck, hasAdjudication := v.Check != "", v.Adjudication != ""
	if hasCheck == hasAdjudication {
		return fmt.Errorf("acceptance item %d verification must carry exactly one non-blank check or adjudication", item)
	}
	if hasCheck && strings.TrimSpace(v.Check) == "" {
		return fmt.Errorf("acceptance item %d verification check must be non-blank", item)
	}
	if hasAdjudication && strings.TrimSpace(v.Adjudication) == "" {
		return fmt.Errorf("acceptance item %d verification adjudication must be non-blank", item)
	}
	return nil
}

// acceptanceCriteria returns the criterion text of every item, in order.
func acceptanceCriteria(items []AcceptanceItem) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.Criterion)
	}
	return out
}
