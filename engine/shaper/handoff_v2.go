package shaper

// Version 2 of the handoff: the acceptance of a version 1 handoff is a list of
// criterion strings, and from version 2 each criterion is an object that also
// names how it will be verified. Version 3 (handoff_v3.go) builds on the rules
// here; the dispatch on the version is Parse, in shaper.go.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/fieldrule"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/jsonstrict"
)

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
	return h.validateAcceptanceCore()
}

// validateAcceptanceCore is the version 2 rule set shared by validateV2 and
// validateV3: version 1's shared fields, plus the version 2 acceptance
// object shape and its out_of_scope overlap check.
func (h Handoff) validateAcceptanceCore() error {
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
	if err := fieldrule.CheckStringArray("stages", h.Stages, 1); err != nil {
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
	if err := fieldrule.CheckStringArray("out_of_scope", h.OutOfScope, 0); err != nil {
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
