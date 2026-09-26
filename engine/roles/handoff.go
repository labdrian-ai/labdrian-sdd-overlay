package roles

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/jsonstrict"
)

// HandoffVersion is the only RoleHandoff wire version this package accepts.
const HandoffVersion = 1

// Status is the outcome of one role's work on a handoff record.
type Status string

const (
	// StatusCompleted means the role finished its work; the next record's
	// from_role equals this record's to_role.
	StatusCompleted Status = "completed"
	// StatusInterrupted means the role's work stopped before completion. The
	// next record in the chain repeats this record's from_role and to_role
	// (a resume of the same transition), and resume_reason is required.
	StatusInterrupted Status = "interrupted"
)

// Evidence is one piece of evidence a role attaches to a handoff: what kind
// it is, a reference to it, and a digest binding it to exact bytes. Evidence
// is a pointer to content, not a copy or an authority grant.
type Evidence struct {
	Kind   string `json:"kind"`
	Ref    string `json:"ref"`
	SHA256 string `json:"sha256"`
}

// Context is the free-form narrative a role hands the next role: a summary,
// decisions made, and open questions left for the next role to resolve.
type Context struct {
	Summary       string   `json:"summary"`
	Decisions     []string `json:"decisions"`
	OpenQuestions []string `json:"open_questions"`
}

// RoleHandoff is one record of a role handoff chain: data only, no
// authority. It binds to the previous record in its chain by prev_sha256 and
// to a payload by payload_sha256, but does not itself launch, dispatch, or
// grant execution of anything.
type RoleHandoff struct {
	Version       int        `json:"version"`
	ProjectID     string     `json:"project_id"`
	GoalID        string     `json:"goal_id"`
	ChainID       string     `json:"chain_id"`
	Seq           int        `json:"seq"`
	FromRole      Role       `json:"from_role"`
	ToRole        Role       `json:"to_role"`
	PrevSHA256    string     `json:"prev_sha256"`
	PayloadKind   string     `json:"payload_kind"`
	PayloadSHA256 string     `json:"payload_sha256"`
	Evidence      []Evidence `json:"evidence"`
	Context       Context    `json:"context"`
	Status        Status     `json:"status"`
	// ResumeReason must be present and non-blank when Status is
	// StatusInterrupted, and absent when Status is StatusCompleted.
	ResumeReason *string `json:"resume_reason,omitempty"`
}

var roleHandoffFields = []string{
	"version", "project_id", "goal_id", "chain_id", "seq", "from_role", "to_role",
	"prev_sha256", "payload_kind", "payload_sha256", "evidence", "context", "status",
	"resume_reason",
}

var (
	evidenceItemFields = []string{"kind", "ref", "sha256"}
	contextFields      = []string{"summary", "decisions", "open_questions"}
)

var sha256HexPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// ParseRoleHandoff strictly parses one RoleHandoff record: valid UTF-8, no
// duplicate keys at any depth, no unknown fields at any depth (exact case),
// no trailing data, the pinned version, and every field shape via Validate.
func ParseRoleHandoff(data []byte) (RoleHandoff, error) {
	if err := jsonstrict.CheckUTF8(data); err != nil {
		return RoleHandoff{}, fmt.Errorf("parse role handoff: %w", err)
	}
	if err := jsonstrict.CheckNoDuplicateKeys(data); err != nil {
		return RoleHandoff{}, fmt.Errorf("parse role handoff: %w", err)
	}
	if err := jsonstrict.CheckKnownFields(data, "role handoff", roleHandoffFields); err != nil {
		return RoleHandoff{}, fmt.Errorf("parse role handoff: %w", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return RoleHandoff{}, fmt.Errorf("parse role handoff: %w", err)
	}
	if err := checkNestedHandoffFields(raw); err != nil {
		return RoleHandoff{}, fmt.Errorf("parse role handoff: %w", err)
	}

	var h RoleHandoff
	if err := jsonstrict.DecodeStrict(data, "role handoff", &h); err != nil {
		return RoleHandoff{}, fmt.Errorf("parse role handoff: %w", err)
	}
	if err := h.Validate(); err != nil {
		return RoleHandoff{}, fmt.Errorf("parse role handoff: %w", err)
	}
	return h, nil
}

// checkNestedHandoffFields rejects unknown keys in every nested record
// object with exact-case matching, the same defense clearance.go applies:
// encoding/json matches struct fields without regard to case and the last
// matching key wins, so a case-variant key could carry a value a
// case-sensitive reader of the same bytes would never see.
func checkNestedHandoffFields(raw map[string]json.RawMessage) error {
	isNull := func(v json.RawMessage) bool { return bytes.Equal(bytes.TrimSpace(v), []byte("null")) }

	if v, ok := raw["context"]; ok && !isNull(v) {
		if err := jsonstrict.CheckKnownFields(v, "role handoff context", contextFields); err != nil {
			return fmt.Errorf("context: %w", err)
		}
	}
	v, ok := raw["evidence"]
	if !ok || isNull(v) {
		return nil
	}
	var items []json.RawMessage
	if err := json.Unmarshal(v, &items); err != nil {
		return fmt.Errorf("evidence: %w", err)
	}
	for i, item := range items {
		if err := jsonstrict.CheckKnownFields(item, "role handoff evidence item", evidenceItemFields); err != nil {
			return fmt.Errorf("evidence[%d]: %w", i, err)
		}
	}
	return nil
}

// Validate checks every field shape of h. It is first-error-wins.
func (h RoleHandoff) Validate() error {
	if h.Version != HandoffVersion {
		return fmt.Errorf("version must be %d, got %d", HandoffVersion, h.Version)
	}
	for _, f := range []struct{ name, value string }{
		{"project_id", h.ProjectID},
		{"goal_id", h.GoalID},
		{"chain_id", h.ChainID},
	} {
		if strings.TrimSpace(f.value) == "" {
			return fmt.Errorf("%s must not be blank", f.name)
		}
	}
	if h.Seq < 1 {
		return fmt.Errorf("seq must be >= 1, got %d", h.Seq)
	}
	if !IsKnownRole(h.FromRole) {
		return fmt.Errorf("from_role %q is not a known role", h.FromRole)
	}
	if !IsKnownRole(h.ToRole) {
		return fmt.Errorf("to_role %q is not a known role", h.ToRole)
	}
	if !IsValidTransition(h.FromRole, h.ToRole) {
		return fmt.Errorf("transition from %q to %q is not allowed", h.FromRole, h.ToRole)
	}
	for _, f := range []struct{ name, value string }{
		{"prev_sha256", h.PrevSHA256},
		{"payload_sha256", h.PayloadSHA256},
	} {
		if !sha256HexPattern.MatchString(f.value) {
			return fmt.Errorf("%s must be 64 lowercase hex characters, got %q", f.name, f.value)
		}
	}
	if strings.TrimSpace(h.PayloadKind) == "" {
		return fmt.Errorf("payload_kind must not be blank")
	}
	if h.Evidence == nil {
		return fmt.Errorf("evidence must be a non-null array")
	}
	for i, e := range h.Evidence {
		for _, f := range []struct{ name, value string }{
			{"kind", e.Kind},
			{"ref", e.Ref},
		} {
			if strings.TrimSpace(f.value) == "" {
				return fmt.Errorf("evidence[%d].%s must not be blank", i, f.name)
			}
		}
		if !sha256HexPattern.MatchString(e.SHA256) {
			return fmt.Errorf("evidence[%d].sha256 must be 64 lowercase hex characters, got %q", i, e.SHA256)
		}
	}
	if strings.TrimSpace(h.Context.Summary) == "" {
		return fmt.Errorf("context.summary must not be blank")
	}
	if h.Context.Decisions == nil {
		return fmt.Errorf("context.decisions must be a non-null array")
	}
	for i, d := range h.Context.Decisions {
		if strings.TrimSpace(d) == "" {
			return fmt.Errorf("context.decisions[%d] must not be blank", i)
		}
	}
	if h.Context.OpenQuestions == nil {
		return fmt.Errorf("context.open_questions must be a non-null array")
	}
	for i, q := range h.Context.OpenQuestions {
		if strings.TrimSpace(q) == "" {
			return fmt.Errorf("context.open_questions[%d] must not be blank", i)
		}
	}
	switch h.Status {
	case StatusCompleted:
		if h.ResumeReason != nil {
			return fmt.Errorf("resume_reason must be absent when status is %q", StatusCompleted)
		}
	case StatusInterrupted:
		if h.ResumeReason == nil || strings.TrimSpace(*h.ResumeReason) == "" {
			return fmt.Errorf("resume_reason must be present and non-blank when status is %q", StatusInterrupted)
		}
	default:
		return fmt.Errorf("status must be %q or %q, got %q", StatusCompleted, StatusInterrupted, h.Status)
	}
	return nil
}
