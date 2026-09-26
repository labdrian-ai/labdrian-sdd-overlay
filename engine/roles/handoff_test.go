package roles

import (
	"strings"
	"testing"
)

const validDigest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const otherDigest = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

func validHandoffJSON() string {
	return `{
  "version": 1,
  "project_id": "proj-1",
  "goal_id": "goal-1",
  "chain_id": "chain-1",
  "seq": 2,
  "from_role": "builder",
  "to_role": "reviewer",
  "prev_sha256": "` + validDigest + `",
  "payload_kind": "diff",
  "payload_sha256": "` + otherDigest + `",
  "evidence": [
    {"kind": "test", "ref": "go test ./...", "sha256": "` + validDigest + `"}
  ],
  "context": {
    "summary": "implemented the feature",
    "decisions": ["used option A"],
    "open_questions": []
  },
  "status": "completed"
}`
}

func TestParseRoleHandoffAcceptsValidRecord(t *testing.T) {
	h, err := ParseRoleHandoff([]byte(validHandoffJSON()))
	if err != nil {
		t.Fatalf("ParseRoleHandoff() = %v, want nil", err)
	}
	if h.Version != HandoffVersion {
		t.Fatalf("Version = %d, want %d", h.Version, HandoffVersion)
	}
	if h.FromRole != RoleBuilder || h.ToRole != RoleReviewer {
		t.Fatalf("FromRole/ToRole = %q/%q, want builder/reviewer", h.FromRole, h.ToRole)
	}
	if h.Status != StatusCompleted {
		t.Fatalf("Status = %q, want completed", h.Status)
	}
	if h.ResumeReason != nil {
		t.Fatalf("ResumeReason = %v, want nil for a completed record", h.ResumeReason)
	}
}

func TestParseRoleHandoffRejectsUnknownField(t *testing.T) {
	data := strings.Replace(validHandoffJSON(), `"status": "completed"`, `"status": "completed", "extra_field": true`, 1)
	if _, err := ParseRoleHandoff([]byte(data)); err == nil {
		t.Fatalf("ParseRoleHandoff() = nil, want error for unknown field")
	}
}

func TestParseRoleHandoffRejectsDuplicateKey(t *testing.T) {
	data := `{"version":1,"version":1,"project_id":"p","goal_id":"g","chain_id":"c","seq":1,"from_role":"shaper","to_role":"estimator","prev_sha256":"` + EmptyChainDigest + `","payload_kind":"x","payload_sha256":"` + validDigest + `","evidence":[],"context":{"summary":"s","decisions":[],"open_questions":[]},"status":"completed"}`
	if _, err := ParseRoleHandoff([]byte(data)); err == nil {
		t.Fatalf("ParseRoleHandoff() = nil, want error for duplicate key")
	}
}

func TestParseRoleHandoffRejectsUnknownNestedField(t *testing.T) {
	data := strings.Replace(validHandoffJSON(), `"kind": "test"`, `"kind": "test", "bogus": 1`, 1)
	if _, err := ParseRoleHandoff([]byte(data)); err == nil {
		t.Fatalf("ParseRoleHandoff() = nil, want error for unknown nested evidence field")
	}
}

func TestParseRoleHandoffRejectsExactCaseVariantNestedField(t *testing.T) {
	// encoding/json matches struct fields case-insensitively; a case-variant
	// key must still be rejected by the exact-case nested check.
	data := strings.Replace(validHandoffJSON(), `"summary": "implemented the feature"`, `"Summary": "implemented the feature"`, 1)
	if _, err := ParseRoleHandoff([]byte(data)); err == nil {
		t.Fatalf("ParseRoleHandoff() = nil, want error for case-variant nested key")
	}
}

func TestRoleHandoffValidate(t *testing.T) {
	base := func() RoleHandoff {
		h, err := ParseRoleHandoff([]byte(validHandoffJSON()))
		if err != nil {
			t.Fatalf("setup: ParseRoleHandoff() = %v", err)
		}
		return h
	}

	t.Run("valid record passes", func(t *testing.T) {
		if err := base().Validate(); err != nil {
			t.Fatalf("Validate() = %v, want nil", err)
		}
	})

	t.Run("wrong version rejected", func(t *testing.T) {
		h := base()
		h.Version = 2
		if err := h.Validate(); err == nil {
			t.Fatalf("Validate() = nil, want error for wrong version")
		}
	})

	t.Run("blank project id rejected", func(t *testing.T) {
		h := base()
		h.ProjectID = "  "
		if err := h.Validate(); err == nil {
			t.Fatalf("Validate() = nil, want error for blank project_id")
		}
	})

	t.Run("seq below one rejected", func(t *testing.T) {
		h := base()
		h.Seq = 0
		if err := h.Validate(); err == nil {
			t.Fatalf("Validate() = nil, want error for seq < 1")
		}
	})

	t.Run("unknown from_role rejected", func(t *testing.T) {
		h := base()
		h.FromRole = Role("astronaut")
		if err := h.Validate(); err == nil {
			t.Fatalf("Validate() = nil, want error for unknown from_role")
		}
	})

	t.Run("invalid transition rejected", func(t *testing.T) {
		h := base()
		h.FromRole = RolePrototyper
		h.ToRole = RoleEstimator
		if err := h.Validate(); err == nil {
			t.Fatalf("Validate() = nil, want error for invalid transition")
		}
	})

	t.Run("malformed prev_sha256 rejected", func(t *testing.T) {
		h := base()
		h.PrevSHA256 = "not-hex"
		if err := h.Validate(); err == nil {
			t.Fatalf("Validate() = nil, want error for malformed prev_sha256")
		}
	})

	t.Run("uppercase hex digest rejected", func(t *testing.T) {
		h := base()
		h.PrevSHA256 = strings.ToUpper(validDigest)
		if err := h.Validate(); err == nil {
			t.Fatalf("Validate() = nil, want error for uppercase digest")
		}
	})

	t.Run("blank payload_kind rejected", func(t *testing.T) {
		h := base()
		h.PayloadKind = ""
		if err := h.Validate(); err == nil {
			t.Fatalf("Validate() = nil, want error for blank payload_kind")
		}
	})

	t.Run("nil evidence rejected", func(t *testing.T) {
		h := base()
		h.Evidence = nil
		if err := h.Validate(); err == nil {
			t.Fatalf("Validate() = nil, want error for nil evidence")
		}
	})

	t.Run("blank evidence field rejected", func(t *testing.T) {
		h := base()
		h.Evidence = []Evidence{{Kind: "", Ref: "r", SHA256: validDigest}}
		if err := h.Validate(); err == nil {
			t.Fatalf("Validate() = nil, want error for blank evidence.kind")
		}
	})

	t.Run("invalid evidence digest rejected", func(t *testing.T) {
		h := base()
		h.Evidence = []Evidence{{Kind: "test", Ref: "r", SHA256: "short"}}
		if err := h.Validate(); err == nil {
			t.Fatalf("Validate() = nil, want error for invalid evidence digest")
		}
	})

	t.Run("blank context summary rejected", func(t *testing.T) {
		h := base()
		h.Context.Summary = "  "
		if err := h.Validate(); err == nil {
			t.Fatalf("Validate() = nil, want error for blank context.summary")
		}
	})

	t.Run("nil decisions rejected", func(t *testing.T) {
		h := base()
		h.Context.Decisions = nil
		if err := h.Validate(); err == nil {
			t.Fatalf("Validate() = nil, want error for nil context.decisions")
		}
	})

	t.Run("blank decision item rejected", func(t *testing.T) {
		h := base()
		h.Context.Decisions = []string{""}
		if err := h.Validate(); err == nil {
			t.Fatalf("Validate() = nil, want error for blank decisions item")
		}
	})

	t.Run("nil open_questions rejected", func(t *testing.T) {
		h := base()
		h.Context.OpenQuestions = nil
		if err := h.Validate(); err == nil {
			t.Fatalf("Validate() = nil, want error for nil context.open_questions")
		}
	})

	t.Run("unknown status rejected", func(t *testing.T) {
		h := base()
		h.Status = Status("paused")
		if err := h.Validate(); err == nil {
			t.Fatalf("Validate() = nil, want error for unknown status")
		}
	})

	t.Run("resume_reason on completed rejected", func(t *testing.T) {
		h := base()
		reason := "network was down"
		h.ResumeReason = &reason
		if err := h.Validate(); err == nil {
			t.Fatalf("Validate() = nil, want error for resume_reason present on a completed record")
		}
	})

	t.Run("interrupted without resume_reason rejected", func(t *testing.T) {
		h := base()
		h.Status = StatusInterrupted
		h.ResumeReason = nil
		if err := h.Validate(); err == nil {
			t.Fatalf("Validate() = nil, want error for interrupted without resume_reason")
		}
	})

	t.Run("interrupted with blank resume_reason rejected", func(t *testing.T) {
		h := base()
		h.Status = StatusInterrupted
		blank := "   "
		h.ResumeReason = &blank
		if err := h.Validate(); err == nil {
			t.Fatalf("Validate() = nil, want error for blank resume_reason")
		}
	})

	t.Run("interrupted with resume_reason accepted", func(t *testing.T) {
		h := base()
		h.Status = StatusInterrupted
		reason := "network was down"
		h.ResumeReason = &reason
		if err := h.Validate(); err != nil {
			t.Fatalf("Validate() = %v, want nil for interrupted with resume_reason", err)
		}
	})
}

func TestParseRoleHandoffRejectsInterruptedWithoutResumeReasonEndToEnd(t *testing.T) {
	data := strings.Replace(validHandoffJSON(), `"status": "completed"`, `"status": "interrupted"`, 1)
	if _, err := ParseRoleHandoff([]byte(data)); err == nil {
		t.Fatalf("ParseRoleHandoff() = nil, want error: interrupted record needs resume_reason")
	}
}

func TestParseRoleHandoffAcceptsInterruptedWithResumeReason(t *testing.T) {
	data := strings.Replace(validHandoffJSON(), `"status": "completed"`, `"status": "interrupted", "resume_reason": "network was down"`, 1)
	h, err := ParseRoleHandoff([]byte(data))
	if err != nil {
		t.Fatalf("ParseRoleHandoff() = %v, want nil", err)
	}
	if h.ResumeReason == nil || *h.ResumeReason != "network was down" {
		t.Fatalf("ResumeReason = %v, want %q", h.ResumeReason, "network was down")
	}
}
