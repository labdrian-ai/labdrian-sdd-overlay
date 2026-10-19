package roles

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/jsonstrict"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/memoryscope"
)

// The RoleHandoff wire versions this package accepts. A version 2 record is a version 1
// record that may also carry a memory_directive; a version 1 record refuses that member as
// unknown, so it parses exactly as it always did.
const (
	// HandoffVersion is version 1, the original RoleHandoff.
	HandoffVersion = 1
	// HandoffVersionV2 adds the optional memory_directive.
	HandoffVersionV2 = 2
)

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
	// MemoryDirective is the memory the handing-off role lets the next role read, a strict
	// MemoryDirective v1 object. It exists only in a version 2 record and is optional there:
	// nil means the record carries no directive, which a workflow reads as its base plan.
	MemoryDirective *memoryscope.Directive `json:"memory_directive,omitempty"`
}

// roleHandoffFields are the members of a version 1 record.
var roleHandoffFields = []string{
	"version", "project_id", "goal_id", "chain_id", "seq", "from_role", "to_role",
	"prev_sha256", "payload_kind", "payload_sha256", "evidence", "context", "status",
	"resume_reason",
}

// roleHandoffV2Fields are the members of a version 2 record: those of version 1 and the
// optional memory_directive.
var roleHandoffV2Fields = append(append([]string{}, roleHandoffFields...), "memory_directive")

var (
	evidenceItemFields = []string{"kind", "ref", "sha256"}
	contextFields      = []string{"summary", "decisions", "open_questions"}
)

// MaxRecordBytes bounds the size of one RoleHandoff JSON document that
// ParseRoleHandoff will decode, and that a chain store will read back. A record carries
// free-form narrative (a summary, decisions, open questions) and a list of evidence, none
// of which has a length of its own, so the whole document is bounded instead: it keeps a
// caller that parses records from an untrusted source (disk, stdin, another process)
// from an arbitrarily large document before any field is inspected. 4 MiB is exactly the
// stdin cap `roles append` applied before this bound existed: any record a chain already
// holds was accepted under that cap, so a smaller bound would make such a chain
// unloadable. It is still many times what a legitimate handoff needs. The bound is owned
// here, and the CLI's append verb reads stdin through it, so every record the CLI accepts
// is one a chain store can load back.
const MaxRecordBytes = 4 << 20

// ErrRecordTooLarge is returned by ParseRoleHandoff when the input exceeds
// MaxRecordBytes.
var ErrRecordTooLarge = errors.New("roles: handoff record is too large")

// ParseRoleHandoff strictly parses one RoleHandoff record: bounded overall size, valid
// UTF-8, no duplicate keys at any depth, no unknown fields at any depth (exact case),
// no trailing data, a known version, and every field shape via Validate. The members a record
// may have are those of its version: memory_directive is known only to a version 2 record, and
// is parsed there by memoryscope.ParseDirective, as strictly as a directive on its own.
func ParseRoleHandoff(data []byte) (RoleHandoff, error) {
	if len(data) > MaxRecordBytes {
		return RoleHandoff{}, fmt.Errorf("parse role handoff: %w: %d bytes, the maximum is %d", ErrRecordTooLarge, len(data), MaxRecordBytes)
	}
	if err := jsonstrict.CheckUTF8(data); err != nil {
		return RoleHandoff{}, fmt.Errorf("parse role handoff: %w", err)
	}
	if err := jsonstrict.CheckNoDuplicateKeys(data); err != nil {
		return RoleHandoff{}, fmt.Errorf("parse role handoff: %w", err)
	}
	if err := jsonstrict.CheckKnownFields(data, "role handoff", fieldsOfVersion(data)); err != nil {
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

// fieldsOfVersion returns the members a record of data's version may have. Only a record whose
// exact-case "version" member is the number 2 gets the version 2 members; every other document,
// including one that is not an object or names another version, gets the version 1 members, so
// it is refused with the words it always was.
func fieldsOfVersion(data []byte) []string {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return roleHandoffFields
	}
	var version int
	if err := json.Unmarshal(raw["version"], &version); err != nil || version != HandoffVersionV2 {
		return roleHandoffFields
	}
	return roleHandoffV2Fields
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
	if v, ok := raw["evidence"]; ok && !isNull(v) {
		var items []json.RawMessage
		if err := json.Unmarshal(v, &items); err != nil {
			return fmt.Errorf("evidence: %w", err)
		}
		for i, item := range items {
			if err := jsonstrict.CheckKnownFields(item, "role handoff evidence item", evidenceItemFields); err != nil {
				return fmt.Errorf("evidence[%d]: %w", i, err)
			}
		}
	}
	// A memory_directive member is present only in a version 2 record (the version 1 members
	// refuse it). It is parsed by the directive's own parser, null included: absent is how a
	// record says it carries no directive.
	if v, ok := raw["memory_directive"]; ok {
		if _, err := memoryscope.ParseDirective(v); err != nil {
			return fmt.Errorf("memory_directive: %w", err)
		}
	}
	return nil
}
