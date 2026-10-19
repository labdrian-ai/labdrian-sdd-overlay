package roles

import (
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/memoryscope"
)

// TestRoleHandoffValidateSaysFirstWhatItFindsFirst pins what Validate answers for each rule it
// holds, word for word, and the order in which it looks: a record with two problems is refused for
// the one that comes first. The checks of Validate are separate functions, and this test is what
// says they still run in the order they always did and say what they always said.
func TestRoleHandoffValidateSaysFirstWhatItFindsFirst(t *testing.T) {
	base := func(t *testing.T) RoleHandoff {
		t.Helper()
		h, err := ParseRoleHandoff([]byte(validHandoffJSON()))
		if err != nil {
			t.Fatalf("setup: ParseRoleHandoff() = %v", err)
		}
		return h
	}
	upper := strings.ToUpper(validDigest)
	reason := "network was down"
	blank := "   "
	goodEvidence := Evidence{Kind: "test", Ref: "go test ./...", SHA256: validDigest}

	for _, tc := range []struct {
		name   string
		mutate func(*RoleHandoff)
		want   string
	}{
		// One rule at a time, in the order of the record.
		{"version", func(h *RoleHandoff) { h.Version = 3 }, "version must be 1 or 2, got 3"},
		{"project_id", func(h *RoleHandoff) { h.ProjectID = "  " }, "project_id must not be blank"},
		{"goal_id", func(h *RoleHandoff) { h.GoalID = "" }, "goal_id must not be blank"},
		{"chain_id", func(h *RoleHandoff) { h.ChainID = "\t" }, "chain_id must not be blank"},
		{"seq", func(h *RoleHandoff) { h.Seq = 0 }, "seq must be >= 1, got 0"},
		{"from_role", func(h *RoleHandoff) { h.FromRole = Role("astronaut") }, `from_role "astronaut" is not a known role`},
		{"to_role", func(h *RoleHandoff) { h.ToRole = Role("astronaut") }, `to_role "astronaut" is not a known role`},
		{"transition", func(h *RoleHandoff) { h.FromRole, h.ToRole = RolePrototyper, RoleEstimator }, `transition from "prototyper" to "estimator" is not allowed`},
		{"prev_sha256", func(h *RoleHandoff) { h.PrevSHA256 = "not-hex" }, `prev_sha256 must be 64 lowercase hex characters, got "not-hex"`},
		{"payload_sha256", func(h *RoleHandoff) { h.PayloadSHA256 = upper }, `payload_sha256 must be 64 lowercase hex characters, got "` + upper + `"`},
		{"payload_kind", func(h *RoleHandoff) { h.PayloadKind = " " }, "payload_kind must not be blank"},
		{"evidence null", func(h *RoleHandoff) { h.Evidence = nil }, "evidence must be a non-null array"},
		{"evidence kind", func(h *RoleHandoff) { h.Evidence = []Evidence{{Kind: "", Ref: "r", SHA256: validDigest}} }, "evidence[0].kind must not be blank"},
		{"evidence ref", func(h *RoleHandoff) { h.Evidence = []Evidence{{Kind: "k", Ref: " ", SHA256: validDigest}} }, "evidence[0].ref must not be blank"},
		{"evidence sha256", func(h *RoleHandoff) { h.Evidence = []Evidence{{Kind: "k", Ref: "r", SHA256: "short"}} }, `evidence[0].sha256 must be 64 lowercase hex characters, got "short"`},
		{"evidence is named by its index", func(h *RoleHandoff) { h.Evidence = []Evidence{goodEvidence, {Kind: "", Ref: "r", SHA256: validDigest}} }, "evidence[1].kind must not be blank"},
		{"summary", func(h *RoleHandoff) { h.Context.Summary = "  " }, "context.summary must not be blank"},
		{"decisions null", func(h *RoleHandoff) { h.Context.Decisions = nil }, "context.decisions must be a non-null array"},
		{"decisions item", func(h *RoleHandoff) { h.Context.Decisions = []string{"ok", ""} }, "context.decisions[1] must not be blank"},
		{"open_questions null", func(h *RoleHandoff) { h.Context.OpenQuestions = nil }, "context.open_questions must be a non-null array"},
		{"open_questions item", func(h *RoleHandoff) { h.Context.OpenQuestions = []string{" "} }, "context.open_questions[0] must not be blank"},
		{"status", func(h *RoleHandoff) { h.Status = Status("paused") }, `status must be "completed" or "interrupted", got "paused"`},
		{"resume_reason on a completed record", func(h *RoleHandoff) { h.ResumeReason = &reason }, `resume_reason must be absent when status is "completed"`},
		{"interrupted without a reason", func(h *RoleHandoff) { h.Status, h.ResumeReason = StatusInterrupted, nil }, `resume_reason must be present and non-blank when status is "interrupted"`},
		{"interrupted with a blank reason", func(h *RoleHandoff) { h.Status, h.ResumeReason = StatusInterrupted, &blank }, `resume_reason must be present and non-blank when status is "interrupted"`},

		// Two problems at once: the first in this order is the one said.
		{"version before the ids", func(h *RoleHandoff) { h.Version, h.ProjectID = 3, "" }, "version must be 1 or 2, got 3"},
		{"project_id before goal_id", func(h *RoleHandoff) { h.ProjectID, h.GoalID = "", "" }, "project_id must not be blank"},
		{"goal_id before chain_id", func(h *RoleHandoff) { h.GoalID, h.ChainID = "", "" }, "goal_id must not be blank"},
		{"ids before seq", func(h *RoleHandoff) { h.ChainID, h.Seq = "", 0 }, "chain_id must not be blank"},
		{"seq before the roles", func(h *RoleHandoff) { h.Seq, h.FromRole = 0, Role("astronaut") }, "seq must be >= 1, got 0"},
		{"from_role before to_role", func(h *RoleHandoff) { h.FromRole, h.ToRole = Role("astronaut"), Role("astronaut") }, `from_role "astronaut" is not a known role`},
		{"the roles before the transition", func(h *RoleHandoff) { h.ToRole, h.FromRole = Role("astronaut"), RoleDelivery }, `to_role "astronaut" is not a known role`},
		{"the transition before the digests", func(h *RoleHandoff) { h.FromRole, h.ToRole, h.PrevSHA256 = RolePrototyper, RoleEstimator, "x" }, `transition from "prototyper" to "estimator" is not allowed`},
		{"prev_sha256 before payload_sha256", func(h *RoleHandoff) { h.PrevSHA256, h.PayloadSHA256 = "x", "y" }, `prev_sha256 must be 64 lowercase hex characters, got "x"`},
		{"payload_sha256 before payload_kind", func(h *RoleHandoff) { h.PayloadSHA256, h.PayloadKind = "y", "" }, `payload_sha256 must be 64 lowercase hex characters, got "y"`},
		{"payload_kind before evidence", func(h *RoleHandoff) { h.PayloadKind, h.Evidence = "", nil }, "payload_kind must not be blank"},
		{"evidence before the summary", func(h *RoleHandoff) { h.Evidence, h.Context.Summary = nil, "" }, "evidence must be a non-null array"},
		{"a bad evidence item before the summary", func(h *RoleHandoff) {
			h.Evidence, h.Context.Summary = []Evidence{{Kind: "", Ref: "r", SHA256: validDigest}}, ""
		}, "evidence[0].kind must not be blank"},
		{"the first bad evidence item, and in it kind before ref before sha256", func(h *RoleHandoff) {
			h.Evidence = []Evidence{goodEvidence, {Kind: "", Ref: "", SHA256: "x"}, {Kind: "", Ref: "r", SHA256: validDigest}}
		}, "evidence[1].kind must not be blank"},
		{"ref before sha256 in an evidence item", func(h *RoleHandoff) { h.Evidence = []Evidence{{Kind: "k", Ref: "", SHA256: "x"}} }, "evidence[0].ref must not be blank"},
		{"summary before decisions", func(h *RoleHandoff) { h.Context.Summary, h.Context.Decisions = "", nil }, "context.summary must not be blank"},
		{"decisions null before decisions items", func(h *RoleHandoff) { h.Context.Decisions, h.Context.OpenQuestions = nil, nil }, "context.decisions must be a non-null array"},
		{"decisions items before open_questions", func(h *RoleHandoff) { h.Context.Decisions, h.Context.OpenQuestions = []string{""}, nil }, "context.decisions[0] must not be blank"},
		{"open_questions before the status", func(h *RoleHandoff) { h.Context.OpenQuestions, h.Status = []string{""}, Status("paused") }, "context.open_questions[0] must not be blank"},
		{"the status before the memory directive", func(h *RoleHandoff) {
			h.Status, h.MemoryDirective = Status("paused"), &memoryscope.Directive{Version: 1, Scope: memoryscope.ScopeNone, Sources: []memoryscope.Source{}, Write: "none"}
		}, `status must be "completed" or "interrupted", got "paused"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := base(t)
			tc.mutate(&h)
			err := h.Validate()
			if err == nil {
				t.Fatalf("Validate() = nil, want %q", tc.want)
			}
			if err.Error() != tc.want {
				t.Errorf("Validate() = %q, want %q", err.Error(), tc.want)
			}
		})
	}

	t.Run("an interrupted record with a reason passes", func(t *testing.T) {
		h := base(t)
		h.Status, h.ResumeReason = StatusInterrupted, &reason
		if err := h.Validate(); err != nil {
			t.Errorf("Validate() = %v, want nil", err)
		}
	})
}
