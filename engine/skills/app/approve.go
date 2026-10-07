package app

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// ApproveInput is what `skills approve` is asked: which skill, who approves it, and the skills
// tree it is in. There is deliberately no default approver: no engine convention records a human
// identity, and a value taken from the environment would read as a person even when an agent ran
// the command. There is no default source root either (the R-002 precedent of validate).
type ApproveInput struct {
	ID string
	// Approver is the label of the person; ApproverGiven says one was named at all, which a blank
	// label is not told apart from by its text.
	Approver      string
	ApproverGiven bool
	SourceRoot    string
}

// ApprovePorts is what approve reads and writes through.
type ApprovePorts struct {
	// Files reads the SKILL.md of the skill.
	Files skills.FileReader
	// Approvals reads the record that is there already.
	Approvals skills.ApprovalRecordStore
	// Staged writes the record.
	Staged skills.StagedWrites
	// Baseline says which skills are grandfathered; nil is the baseline the domain pins.
	Baseline skills.BaselineLookup
	// Now returns the approval time as an RFC 3339 UTC timestamp (YYYY-MM-DDTHH:MM:SSZ). A nil Now
	// refuses the approval: the verb never invents a timestamp.
	Now func() string
}

// ApproveResult is what approve did. Verdict is "approved" when a record was written and
// "unchanged" when a valid one already covered these bytes, in which case it is left exactly as
// it is, so that the original approver and time stay the record of who approved them. Warnings
// are the legacy lint findings of a baseline skill that did not stop its approval.
type ApproveResult struct {
	Verdict    string
	ID         string
	Digest     string
	RecordPath string
	Warnings   []string
}

// ApproveRefusal is an approval that was refused, or that failed: Reason says why, in the words
// of the verb, and Err is the failure behind it when there is one.
type ApproveRefusal struct {
	Reason string
	Err    error
}

func (e *ApproveRefusal) Error() string { return "skills approve: " + e.Reason }
func (e *ApproveRefusal) Unwrap() error { return e.Err }

// The refusals that need no detail of their own.
var (
	ErrApproveIDRequired       = &ApproveRefusal{Reason: "requires --id <skill-id>"}
	ErrApproveApproverRequired = &ApproveRefusal{Reason: "requires --approver <label> (a name for the record; the engine cannot verify it)"}
	ErrApproveSourceRequired   = &ApproveRefusal{Reason: "requires --source-root <skills-dir>"}
	ErrApproveNoClock          = &ApproveRefusal{Reason: "no clock configured: the approval time cannot be recorded"}
)

// ApproveSkill records a human approval of <SourceRoot>/<id>/SKILL.md: a typed record bound to the
// SHA-256 of the exact file bytes, written next to the skill. `skills add` requires such a record
// for a global skill.
//
// The engine cannot prove that a human ran this verb: the approver label is whatever the caller
// passes, and any process that can write the file can write a record. What the use case
// guarantees, and what is tested, is that the record matches the exact bytes of the skill it sits
// next to.
//
// Everything is checked before anything is written: the id is a valid slug, the approver is a
// non-blank single-line label, the source root and the clock are given, the SKILL.md exists and
// passes the same hard lint `skills add` enforces (warnings never block), and an existing record
// is readable (an unreadable one is refused rather than overwritten blind). A skill in the
// approval baseline (by id, whatever its bytes now are) is not refused for its legacy hard lint
// findings (skills.BaselineLintDecision): they are returned as warnings once the approval has
// happened, never for one that was refused or failed.
//
// Re-approving identical bytes is idempotent. Approving changed bytes, or replacing a stale or
// malformed record, writes a new record atomically. An error is an *ApproveRefusal or a
// *LintRefusal; nothing was written.
func ApproveSkill(p ApprovePorts, in ApproveInput) (ApproveResult, error) {
	var res ApproveResult
	refuse := func(err error, format string, args ...any) (ApproveResult, error) {
		return res, &ApproveRefusal{Reason: fmt.Sprintf(format, args...), Err: err}
	}
	switch {
	case in.ID == "":
		return res, ErrApproveIDRequired
	case !skills.IsSlug(in.ID):
		return refuse(nil, "id %q: invalid slug (must match ^[a-z0-9][a-z0-9-]*$)", in.ID)
	case !in.ApproverGiven:
		return res, ErrApproveApproverRequired
	}
	approver := strings.TrimSpace(in.Approver)
	if problem := skills.ApproverLabelError(approver); problem != "" {
		return refuse(nil, "approver %s", problem)
	}
	switch {
	case in.SourceRoot == "":
		return res, ErrApproveSourceRequired
	case p.Now == nil:
		return res, ErrApproveNoClock
	}

	skillPath := filepath.Join(in.SourceRoot, in.ID, "SKILL.md")
	skillData, err := p.Files(skillPath)
	if err != nil {
		return refuse(err, "skill %q: SKILL.md not found or unreadable at %q: %v", in.ID, skillPath, err)
	}
	hard, _ := skills.LintSkillFile(skillData)
	warnings, refused := skills.BaselineLintDecisionAgainst(p.Baseline.OrFixed(), in.ID, hard)
	if refused {
		return res, &LintRefusal{Findings: hard}
	}

	digest := skills.SkillDigest(skillData)
	recordPath := skills.ApprovalRecordPath(in.SourceRoot, in.ID)
	status, err := skills.ReadApprovalStatus(in.SourceRoot, in.ID, skillData, p.Approvals)
	if err != nil {
		return refuse(err, "%v", err)
	}
	verdict := "approved"
	if status.State == skills.ApprovalValid {
		verdict = "unchanged"
	} else {
		record, err := skills.SerializeApprovalRecord(skills.ApprovalRecord{
			Skill:      in.ID,
			SHA256:     digest,
			ApprovedAt: p.Now(),
			Approver:   approver,
		})
		if err != nil {
			return refuse(err, "%v", err)
		}
		if err := skills.WriteApprovalRecord(p.Staged, recordPath, record); err != nil {
			return refuse(err, "skill %q: %v", in.ID, err)
		}
	}
	return ApproveResult{Verdict: verdict, ID: in.ID, Digest: digest, RecordPath: recordPath, Warnings: warnings}, nil
}
