package shaper

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/jsonstrict"
)

// ClearanceRecordVersion is the only clearance record wire version this
// package accepts.
const ClearanceRecordVersion = 1

// ClearanceDecision is the human's overall decision captured by the host.
type ClearanceDecision string

const (
	// DecisionAffirm is an affirmative semantic clearance. It is the only
	// decision Verify accepts.
	DecisionAffirm ClearanceDecision = "affirm"
	// DecisionDecline records that the human declined clearance. A declined
	// record parses and can be stored as evidence, but Verify refuses it.
	DecisionDecline ClearanceDecision = "decline"
)

// ClearanceRecord is a host-owned record of one human semantic clearance
// decision. It is kept separate from Shaper-authored JSON, and it binds the
// decision to exact content by digests only.
//
// The record is evidence of a claimed host capture. It is not a signature:
// any process running as the same OS user can write a record into the store,
// so a ready outcome built on it rests only on these digests and on the
// runtime deny guards that keep the model from writing records. A captured
// affirmative is not proof that anyone read the content, and ready never
// authorizes, permits, or dispatches work.
type ClearanceRecord struct {
	Version         int               `json:"version"`
	Subject         ClearanceSubject  `json:"subject"`
	FlagResolutions []FlagResolution  `json:"flag_resolutions"`
	Decision        ClearanceDecision `json:"decision"`
	Channel         ChannelProvenance `json:"channel"`
	// HostTime is optional and informational only; nothing verifies it.
	HostTime *HostTime `json:"host_time,omitempty"`
}

// ClearanceSubject is the bound content of a clearance record: the handoff's
// declared identity and lowercase hex SHA-256 digests of the Goal bytes, the
// plan (handoff) bytes, the worktree provenance, and the exact presented view.
// Digests detect drift only; they are not identity, authority, or a
// signature.
type ClearanceSubject struct {
	ProjectID        string `json:"project_id"`
	GoalID           string `json:"goal_id"`
	GoalSHA256       string `json:"goal_sha256"`
	HandoffSHA256    string `json:"handoff_sha256"`
	ProvenanceSHA256 string `json:"provenance_sha256"`
	ViewSHA256       string `json:"view_sha256"`
}

// ChannelProvenance names the host channel that claims to have captured the
// decision. Verified is always recorded as false: nothing proves the channel
// or the human, and the record deliberately carries no human identity.
type ChannelProvenance struct {
	// Runtime is the host runtime; only "pi" is supported.
	Runtime string `json:"runtime"`
	// Mode is the runtime's interactive mode. Only "tui" is accepted: an RPC
	// dialog is answered by the driving process, not a human, so ParseRecord,
	// and with it Verify and FileStore.Put, refuse every other mode.
	Mode string `json:"mode"`
	// Verified must be present and false.
	Verified *bool `json:"verified"`
}

// HostTime is a host-reported time, recorded as informational only.
type HostTime struct {
	Value string `json:"value"`
	// Informational must be present and true.
	Informational *bool `json:"informational"`
}

var clearanceRecordFields = []string{
	"version", "subject", "flag_resolutions", "decision", "channel", "host_time",
}

var (
	clearanceSubjectFields = []string{
		"project_id", "goal_id", "goal_sha256", "handoff_sha256", "provenance_sha256", "view_sha256",
	}
	flagResolutionFields = []string{"flag_id", "reason", "evidence"}
	channelFields        = []string{"runtime", "mode", "verified"}
	hostTimeFields       = []string{"value", "informational"}
)

var sha256HexPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// ParseRecord strictly parses one clearance record: valid UTF-8, no duplicate
// keys at any depth, no unknown fields at any depth, no trailing data, the
// pinned version, and every field shape. It accepts both decisions; Verify
// decides whether the record clears anything.
func ParseRecord(data []byte) (ClearanceRecord, error) {
	if err := jsonstrict.CheckUTF8(data); err != nil {
		return ClearanceRecord{}, fmt.Errorf("parse clearance record: %w", err)
	}
	if err := jsonstrict.CheckNoDuplicateKeys(data); err != nil {
		return ClearanceRecord{}, fmt.Errorf("parse clearance record: %w", err)
	}
	if err := jsonstrict.CheckKnownFields(data, "clearance record", clearanceRecordFields); err != nil {
		return ClearanceRecord{}, fmt.Errorf("parse clearance record: %w", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return ClearanceRecord{}, fmt.Errorf("parse clearance record: %w", err)
	}
	if ht, ok := raw["host_time"]; ok && bytes.Equal(bytes.TrimSpace(ht), []byte("null")) {
		return ClearanceRecord{}, fmt.Errorf("parse clearance record: host_time must be an object when present")
	}
	if err := checkNestedRecordFields(raw); err != nil {
		return ClearanceRecord{}, fmt.Errorf("parse clearance record: %w", err)
	}

	var r ClearanceRecord
	if err := jsonstrict.DecodeStrict(data, "clearance record", &r); err != nil {
		return ClearanceRecord{}, fmt.Errorf("parse clearance record: %w", err)
	}
	if err := r.Validate(); err != nil {
		return ClearanceRecord{}, fmt.Errorf("parse clearance record: %w", err)
	}
	return r, nil
}

// checkNestedRecordFields rejects unknown keys in every nested record object
// with exact-case matching. encoding/json matches object keys to struct
// fields without regard to case, and the last matching key wins, so without
// this check a case-variant key such as "FLAG_ID" could carry the value Go
// verifies while a case-sensitive reader of the same bytes sees "flag_id".
// Absent or null members are left to decoding and Validate.
func checkNestedRecordFields(raw map[string]json.RawMessage) error {
	isNull := func(v json.RawMessage) bool { return bytes.Equal(bytes.TrimSpace(v), []byte("null")) }
	for _, obj := range []struct {
		key    string
		name   string
		fields []string
	}{
		{"subject", "clearance record subject", clearanceSubjectFields},
		{"channel", "clearance record channel", channelFields},
		{"host_time", "clearance record host_time", hostTimeFields},
	} {
		v, ok := raw[obj.key]
		if !ok || isNull(v) {
			continue
		}
		if err := jsonstrict.CheckKnownFields(v, obj.name, obj.fields); err != nil {
			return fmt.Errorf("%s: %w", obj.key, err)
		}
	}
	v, ok := raw["flag_resolutions"]
	if !ok || isNull(v) {
		return nil
	}
	var items []json.RawMessage
	if err := json.Unmarshal(v, &items); err != nil {
		return fmt.Errorf("flag_resolutions: %w", err)
	}
	for i, item := range items {
		if err := jsonstrict.CheckKnownFields(item, "clearance record flag resolution", flagResolutionFields); err != nil {
			return fmt.Errorf("flag_resolutions[%d]: %w", i, err)
		}
	}
	return nil
}

// Validate checks every field shape of r. It is first-error-wins.
func (r ClearanceRecord) Validate() error {
	if r.Version != ClearanceRecordVersion {
		return fmt.Errorf("version must be %d, got %d", ClearanceRecordVersion, r.Version)
	}
	s := r.Subject
	for _, f := range []struct{ name, value string }{
		{"subject.project_id", s.ProjectID},
		{"subject.goal_id", s.GoalID},
	} {
		if strings.TrimSpace(f.value) == "" {
			return fmt.Errorf("%s must not be blank", f.name)
		}
	}
	for _, f := range []struct{ name, value string }{
		{"subject.goal_sha256", s.GoalSHA256},
		{"subject.handoff_sha256", s.HandoffSHA256},
		{"subject.provenance_sha256", s.ProvenanceSHA256},
		{"subject.view_sha256", s.ViewSHA256},
	} {
		if !sha256HexPattern.MatchString(f.value) {
			return fmt.Errorf("%s must be 64 lowercase hex characters, got %q", f.name, f.value)
		}
	}
	if r.FlagResolutions == nil {
		return fmt.Errorf("flag_resolutions must be a non-null array")
	}
	seen := make(map[string]bool, len(r.FlagResolutions))
	for i, res := range r.FlagResolutions {
		for _, f := range []struct{ name, value string }{
			{"flag_id", res.FlagID},
			{"reason", res.Reason},
			{"evidence", res.Evidence},
		} {
			if strings.TrimSpace(f.value) == "" {
				return fmt.Errorf("flag_resolutions[%d].%s must not be blank", i, f.name)
			}
		}
		if seen[res.FlagID] {
			return fmt.Errorf("flag_resolutions[%d] resolves flag %q more than once", i, res.FlagID)
		}
		seen[res.FlagID] = true
	}
	if r.Decision != DecisionAffirm && r.Decision != DecisionDecline {
		return fmt.Errorf("decision must be %q or %q, got %q", DecisionAffirm, DecisionDecline, r.Decision)
	}
	c := r.Channel
	if c.Runtime != "pi" {
		return fmt.Errorf("channel.runtime must be %q, got %q", "pi", c.Runtime)
	}
	if c.Mode != "tui" {
		return fmt.Errorf("channel.mode must be %q: an RPC or non-interactive dialog is answered by a process, not a human, got %q", "tui", c.Mode)
	}
	if c.Verified == nil || *c.Verified {
		return fmt.Errorf("channel.verified must be present and false")
	}
	if ht := r.HostTime; ht != nil {
		if strings.TrimSpace(ht.Value) == "" {
			return fmt.Errorf("host_time.value must not be blank")
		}
		if ht.Informational == nil || !*ht.Informational {
			return fmt.Errorf("host_time.informational must be present and true")
		}
	}
	return nil
}

// PresentedView is exactly what the host shows the human before a clearance
// decision: the Goal bytes, the plan bytes, the Goal scope and non_goals, the
// handoff out_of_scope limits, for handoff version 2 each acceptance
// criterion with its planned check or adjudication, and the flags raised for
// review.
type PresentedView struct {
	GoalBytes    []byte
	PlanBytes    []byte
	GoalScope    string
	GoalNonGoals []string
	OutOfScope   []string
	// Acceptance is nil for handoff version 1, whose view is rendered
	// exactly as before version 2 existed. When non-nil the view is
	// rendered as view version 2 with an acceptance section.
	Acceptance []AcceptanceItem
	Flags      []Flag
	// V3 holds the version 3 plan fields, paired with their corresponding
	// Goal fields; it is nil for version 1 and 2, whose view stays
	// byte-identical. When non-nil the view is headed view version 3 and
	// adds the six fields after the acceptance section.
	V3 *PresentedViewV3
}

// PresentedViewV3 is the version 3 addition to PresentedView: the plan's
// roles, tests, risks, estimates, memory_scope, and delivery_limit, shown
// beside the Goal's memory_scope and delivery_boundary for human judgment.
type PresentedViewV3 struct {
	Roles     []Role
	Tests     []string
	Risks     []Risk
	Estimates []Estimate
	// MemoryScope is the handoff's memory_scope, shown next to
	// GoalMemoryScope.
	MemoryScope     string
	GoalMemoryScope string
	// DeliveryLimit is the handoff's delivery_limit, shown next to
	// GoalDeliveryBoundary.
	DeliveryLimit        string
	GoalDeliveryBoundary string
}

// RenderView renders v as deterministic bytes. Every value is length
// prefixed, so no two distinct views render identically. The host displays
// these bytes verbatim and never re-renders them, so the view digest binds
// the decision to what was shown. RenderView itself neither checks nor
// rewrites content: Evaluate refuses a view holding a terminal control or
// invisible formatting rune (ReasonViewUnpresentable) before rendering it,
// and Verify and CheckRecordBinding refuse such view bytes.
//
// A view with a non-nil Acceptance is headed view version 2 and adds, after
// out_of_scope, one criterion per item followed by its verification_check or
// verification_adjudication, so the human sees exactly which verification is
// planned. Planned verification is shown, never executed.
//
// A view with a non-nil V3 is headed view version 3 instead, and adds, after
// the acceptance section, the six version 3 fields: memory_scope paired with
// goal_memory_scope, delivery_limit paired with goal_delivery_boundary, then
// roles, tests, risks, and estimates. Planned tests and estimates are shown,
// never executed or treated as a calendar commitment.
func RenderView(v PresentedView) []byte {
	var b bytes.Buffer
	switch {
	case v.V3 != nil:
		b.WriteString("labdrian shaper clearance view 3\n")
	case v.Acceptance != nil:
		b.WriteString("labdrian shaper clearance view 2\n")
	default:
		b.WriteString("labdrian shaper clearance view 1\n")
	}
	writeViewValue(&b, "goal", v.GoalBytes)
	writeViewValue(&b, "plan", v.PlanBytes)
	writeViewValue(&b, "goal_scope", []byte(v.GoalScope))
	writeViewList(&b, "goal_non_goals", v.GoalNonGoals)
	writeViewList(&b, "out_of_scope", v.OutOfScope)
	if v.Acceptance != nil {
		fmt.Fprintf(&b, "acceptance %d\n", len(v.Acceptance))
		for _, item := range v.Acceptance {
			writeViewValue(&b, "criterion", []byte(item.Criterion))
			if item.Verification.Check != "" {
				writeViewValue(&b, "verification_check", []byte(item.Verification.Check))
			}
			if item.Verification.Adjudication != "" {
				writeViewValue(&b, "verification_adjudication", []byte(item.Verification.Adjudication))
			}
		}
	}
	if v.V3 != nil {
		writeViewV3(&b, v.V3)
	}
	fmt.Fprintf(&b, "flags %d\n", len(v.Flags))
	for _, f := range v.Flags {
		writeViewValue(&b, "flag_id", []byte(f.ID))
		writeViewValue(&b, "flag_kind", []byte(f.Kind))
		writeViewValue(&b, "flag_field", []byte(f.Field))
		fmt.Fprintf(&b, "flag_index %d\n", f.Index)
		writeViewValue(&b, "flag_item", []byte(f.Item))
	}
	return b.Bytes()
}

func writeViewValue(b *bytes.Buffer, name string, value []byte) {
	fmt.Fprintf(b, "%s %d\n", name, len(value))
	b.Write(value)
	b.WriteByte('\n')
}

func writeViewList(b *bytes.Buffer, name string, values []string) {
	fmt.Fprintf(b, "%s %d\n", name, len(values))
	for _, value := range values {
		writeViewValue(b, "item", []byte(value))
	}
}

// writeViewV3 renders the six version 3 fields: memory_scope paired with
// goal_memory_scope, delivery_limit paired with goal_delivery_boundary, then
// roles, tests, risks, and estimates, every string value length-prefixed.
func writeViewV3(b *bytes.Buffer, v *PresentedViewV3) {
	writeViewValue(b, "memory_scope", []byte(v.MemoryScope))
	writeViewValue(b, "goal_memory_scope", []byte(v.GoalMemoryScope))
	writeViewValue(b, "delivery_limit", []byte(v.DeliveryLimit))
	writeViewValue(b, "goal_delivery_boundary", []byte(v.GoalDeliveryBoundary))
	fmt.Fprintf(b, "roles %d\n", len(v.Roles))
	for _, r := range v.Roles {
		writeViewValue(b, "role", []byte(r.Role))
		writeViewValue(b, "responsibility", []byte(r.Responsibility))
	}
	writeViewList(b, "tests", v.Tests)
	fmt.Fprintf(b, "risks %d\n", len(v.Risks))
	for _, r := range v.Risks {
		writeViewValue(b, "risk", []byte(r.Risk))
		writeViewValue(b, "mitigation", []byte(r.Mitigation))
	}
	fmt.Fprintf(b, "estimates %d\n", len(v.Estimates))
	for _, e := range v.Estimates {
		writeViewValue(b, "stage", []byte(e.Stage))
		fmt.Fprintf(b, "low_minutes %d\n", e.LowMinutes)
		fmt.Fprintf(b, "high_minutes %d\n", e.HighMinutes)
	}
}

// ViewDigest is the lowercase hex SHA-256 of rendered view bytes. It detects
// drift only and is not a signature.
func ViewDigest(view []byte) string {
	return sha256Hex(view)
}

// ProvenanceSHA256 is the lowercase hex SHA-256 of a length-prefixed encoding
// of s's provenance: both source paths and the bound worktree toplevel, git
// dir, and common dir. HEAD is not part of it. It detects drift only.
func (s Subject) ProvenanceSHA256() string {
	var b bytes.Buffer
	b.WriteString("labdrian shaper clearance provenance 1\n")
	for _, f := range []struct{ name, value string }{
		{"handoff_source_path", s.HandoffSourcePath},
		{"goal_source_path", s.GoalSourcePath},
		{"toplevel", s.Worktree.Toplevel},
		{"git_dir", s.Worktree.GitDir},
		{"common_dir", s.Worktree.CommonDir},
	} {
		writeViewValue(&b, f.name, []byte(f.value))
	}
	return sha256Hex(b.Bytes())
}

// Verify is the only constructor of VerifiedClearance. It strictly parses
// recordBytes and checks it against a freshly evaluated subject, its flags,
// and the exact view bytes presented to the human. It refuses, first error
// wins, on any parse failure or unknown field, a decline, view bytes holding
// a terminal control or invisible formatting rune (unpresentable), any
// subject or view digest mismatch, a flag without a resolution, or a resolution for a flag
// that was not raised. It never returns a clearance together with an error.
//
// A clearance built here can let Evaluate reach ready only when no other
// blocker remains. It is not a signature: any process running as the same OS
// user can forge a record, so this checks binding and completeness, not who
// wrote it. It grants no execution authority.
func Verify(recordBytes []byte, subject Subject, flags []Flag, view []byte) (*VerifiedClearance, error) {
	r, err := ParseRecord(recordBytes)
	if err != nil {
		return nil, fmt.Errorf("verify clearance: %w", err)
	}
	if r.Decision != DecisionAffirm {
		return nil, fmt.Errorf("verify clearance: decision is %q; a decline never clears", r.Decision)
	}
	if err := checkRecordBinding(r, subject, flags, view); err != nil {
		return nil, fmt.Errorf("verify clearance: %w", err)
	}
	return &VerifiedClearance{
		verified:    true,
		subject:     subject,
		viewSHA256:  ViewDigest(view),
		resolutions: append([]FlagResolution(nil), r.FlagResolutions...),
	}, nil
}

// CheckRecordBinding applies Verify's parse, presentable view, subject
// digest, view digest, and flag resolution checks to recordBytes, but accepts either decision, so a
// host can store a decline as evidence under the same binding rules. It
// returns the parsed record and never a clearance: only Verify clears.
func CheckRecordBinding(recordBytes []byte, subject Subject, flags []Flag, view []byte) (ClearanceRecord, error) {
	r, err := ParseRecord(recordBytes)
	if err != nil {
		return ClearanceRecord{}, fmt.Errorf("check clearance binding: %w", err)
	}
	if err := checkRecordBinding(r, subject, flags, view); err != nil {
		return ClearanceRecord{}, fmt.Errorf("check clearance binding: %w", err)
	}
	return r, nil
}

// checkRecordBinding refuses an unpresentable view, then requires every
// recorded subject digest and the view digest to equal the fresh values, and
// exactly one resolution per flag.
func checkRecordBinding(r ClearanceRecord, subject Subject, flags []Flag, view []byte) error {
	if err := checkViewBytesPresentable(view); err != nil {
		return err
	}
	viewSHA := ViewDigest(view)
	for _, f := range []struct{ name, recorded, fresh string }{
		{"project_id", r.Subject.ProjectID, subject.ProjectID},
		{"goal_id", r.Subject.GoalID, subject.GoalID},
		{"goal_sha256", r.Subject.GoalSHA256, subject.GoalSHA256},
		{"handoff_sha256", r.Subject.HandoffSHA256, subject.HandoffSHA256},
		{"provenance_sha256", r.Subject.ProvenanceSHA256, subject.ProvenanceSHA256()},
		{"view_sha256", r.Subject.ViewSHA256, viewSHA},
	} {
		if f.recorded != f.fresh {
			return fmt.Errorf("subject.%s mismatch: record %q, fresh %q", f.name, f.recorded, f.fresh)
		}
	}
	return checkResolutionsExact(r.FlagResolutions, flags)
}

// checkResolutionsExact requires exactly one resolution per flag and no
// resolution for a flag outside flags.
func checkResolutionsExact(resolutions []FlagResolution, flags []Flag) error {
	resolved := make(map[string]bool, len(resolutions))
	for _, res := range resolutions {
		if resolved[res.FlagID] {
			return fmt.Errorf("flag %q is resolved more than once", res.FlagID)
		}
		resolved[res.FlagID] = true
	}
	raised := make(map[string]bool, len(flags))
	for _, f := range flags {
		raised[f.ID] = true
		if !resolved[f.ID] {
			return fmt.Errorf("flag %q is unresolved", f.ID)
		}
	}
	for _, res := range resolutions {
		if !raised[res.FlagID] {
			return fmt.Errorf("resolution names unknown flag %q", res.FlagID)
		}
	}
	return nil
}
