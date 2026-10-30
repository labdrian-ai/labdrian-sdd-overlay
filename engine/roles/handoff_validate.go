package roles

import (
	"fmt"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/sha256hex"
)

// Validate checks every field shape of h. It is first-error-wins: the checks run in the order
// of the record, and the first problem any of them finds is the one returned. Each check is a
// method of its own, named for what it holds the record to.
func (h RoleHandoff) Validate() error {
	for _, check := range []func() error{
		h.checkVersion,
		h.checkIdentity,
		h.checkTransition,
		h.checkBindings,
		h.checkEvidence,
		h.checkContext,
		h.checkStatus,
		h.checkMemoryDirective,
	} {
		if err := check(); err != nil {
			return err
		}
	}
	return nil
}

// checkVersion requires one of the wire versions this package accepts.
func (h RoleHandoff) checkVersion() error {
	if h.Version != HandoffVersion && h.Version != HandoffVersionV2 {
		return fmt.Errorf("version must be %d or %d, got %d", HandoffVersion, HandoffVersionV2, h.Version)
	}
	return nil
}

// checkIdentity requires the project, the goal and the chain to be named, and the record to be
// numbered from 1.
func (h RoleHandoff) checkIdentity() error {
	if err := requireNotBlank(
		field{"project_id", h.ProjectID},
		field{"goal_id", h.GoalID},
		field{"chain_id", h.ChainID},
	); err != nil {
		return err
	}
	if h.Seq < 1 {
		return fmt.Errorf("seq must be >= 1, got %d", h.Seq)
	}
	return nil
}

// checkTransition requires both roles to be known, in the order from then to, and the step
// from one to the other to be one the roles allow, or the completed exemption that skips the reviewer
// (SkipsReviewer).
func (h RoleHandoff) checkTransition() error {
	if !IsKnownRole(h.FromRole) {
		return fmt.Errorf("from_role %q is not a known role", h.FromRole)
	}
	if !IsKnownRole(h.ToRole) {
		return fmt.Errorf("to_role %q is not a known role", h.ToRole)
	}
	if !IsValidTransition(h.FromRole, h.ToRole) && !SkipsReviewer(h) {
		return fmt.Errorf("transition from %q to %q is not allowed", h.FromRole, h.ToRole)
	}
	return nil
}

// checkBindings requires what the record binds to: the digest of the previous record, the digest
// of the payload, and the kind of the payload.
func (h RoleHandoff) checkBindings() error {
	if err := requireDigest(
		field{"prev_sha256", h.PrevSHA256},
		field{"payload_sha256", h.PayloadSHA256},
	); err != nil {
		return err
	}
	return requireNotBlank(field{"payload_kind", h.PayloadKind})
}

// checkEvidence requires the evidence to be a present array whose items each name a kind and a
// reference and carry a digest. The first item with a problem is the one reported.
func (h RoleHandoff) checkEvidence() error {
	if h.Evidence == nil {
		return fmt.Errorf("evidence must be a non-null array")
	}
	for i, e := range h.Evidence {
		item := fmt.Sprintf("evidence[%d]", i)
		if err := requireNotBlank(
			field{item + ".kind", e.Kind},
			field{item + ".ref", e.Ref},
		); err != nil {
			return err
		}
		if err := requireDigest(field{item + ".sha256", e.SHA256}); err != nil {
			return err
		}
	}
	return nil
}

// checkContext requires the summary, and the decisions and open questions as present arrays
// whose items are not blank.
func (h RoleHandoff) checkContext() error {
	if err := requireNotBlank(field{"context.summary", h.Context.Summary}); err != nil {
		return err
	}
	if err := requireNonBlankItems("context.decisions", h.Context.Decisions); err != nil {
		return err
	}
	return requireNonBlankItems("context.open_questions", h.Context.OpenQuestions)
}

// checkStatus requires a known status and the resume reason that goes with it: absent on a
// completed record, present and not blank on an interrupted one.
func (h RoleHandoff) checkStatus() error {
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

// checkMemoryDirective requires a directive to be on a version 2 record only, and to be a valid
// MemoryDirective by memoryscope's own rules. No directive is valid at either version.
func (h RoleHandoff) checkMemoryDirective() error {
	if h.MemoryDirective == nil {
		return nil
	}
	if h.Version != HandoffVersionV2 {
		return fmt.Errorf("memory_directive requires version %d, got version %d", HandoffVersionV2, h.Version)
	}
	if err := h.MemoryDirective.Validate(); err != nil {
		return fmt.Errorf("memory_directive: %w", err)
	}
	return nil
}

// field is a string of a record with the name its message gives it.
type field struct{ name, value string }

// requireNotBlank returns the error for the first of fields that is empty or only white space.
func requireNotBlank(fields ...field) error {
	for _, f := range fields {
		if strings.TrimSpace(f.value) == "" {
			return fmt.Errorf("%s must not be blank", f.name)
		}
	}
	return nil
}

// requireDigest returns the error for the first of fields that is not a SHA-256 digest.
func requireDigest(fields ...field) error {
	for _, f := range fields {
		if !sha256hex.IsValid(f.value) {
			return fmt.Errorf("%s must be 64 lowercase hex characters, got %q", f.name, f.value)
		}
	}
	return nil
}

// requireNonBlankItems requires items to be a present array and returns the error for the first
// item that is empty or only white space, named by its index from 0.
func requireNonBlankItems(name string, items []string) error {
	if items == nil {
		return fmt.Errorf("%s must be a non-null array", name)
	}
	for i, item := range items {
		if strings.TrimSpace(item) == "" {
			return fmt.Errorf("%s[%d] must not be blank", name, i)
		}
	}
	return nil
}
