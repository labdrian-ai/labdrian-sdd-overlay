package skills

import (
	"fmt"
	"io/fs"
	"strings"
)

// lintBaselineSkills are the global skills that predate the lint budget, by id and sorted: the
// ones BaselineLintDecision does not refuse for the legacy hard findings the budget would have
// refused them for. It is a list of ids for the lint and nothing else. It exempts no skill from
// approval, which every global skill needs for its files as they are (ClassifyApproval), and it is
// never extended at run time: a skill that is not on it meets the whole lint, so a new id here
// would be a decision to let a new skill break the budget.
var lintBaselineSkills = []string{
	"anti-generic-design",
	"branch-pr",
	"chained-pr",
	"chat-thread-analyzer",
	"cognitive-doc-design",
	"comment-writer",
	"gentle-ai-bench",
	"go-testing",
	"inception-pipeline",
	"issue-creation",
	"judgment-day",
	"knowledge-ingestion",
	"project-architect",
	"project-inception",
	"project-manifest",
	"rdd-defect-workflow",
	"requirements-from-transcripts",
	"roadmap-maker",
	"sdd-apply",
	"sdd-archive",
	"sdd-design",
	"sdd-explore",
	"sdd-init",
	"sdd-onboard",
	"sdd-propose",
	"sdd-spec",
	"sdd-tasks",
	"sdd-time-estimation",
	"sdd-verify",
	"skill-creator",
	"skill-improver",
	"skill-registry",
	"systemic-issue-triage",
	"work-unit-commits",
}

// inLintBaseline reports whether id is on lintBaselineSkills.
func inLintBaseline(id string) bool {
	for _, baseline := range lintBaselineSkills {
		if baseline == id {
			return true
		}
	}
	return false
}

// BaselineLintDecision says what the hard lint findings of a skill mean for the approval of its
// files. A skill outside the lint baseline is refused (refused is true) on any hard finding. A
// skill on it predates the lint budget (many of them fail it today), and refusing it would leave
// no way to approve the change an upstream merge makes to it, so its legacy findings (size and
// description shape, see legacyBaselineLintRules) are warned about, after the approval, and do not
// block it: warnings holds one line for each. Any other hard finding means the file is not a
// usable skill (a truncated or corrupted merge, for example) and refuses a baseline skill too.
func BaselineLintDecision(id string, hard []error) (warnings []string, refused bool) {
	if len(hard) == 0 {
		return nil, false
	}
	if !inLintBaseline(id) || !allLegacyBaselineFindings(hard) {
		return nil, true
	}
	for _, finding := range hard {
		warnings = append(warnings, baselineLintWarning(finding))
	}
	return warnings, false
}

// legacyBaselineLintRules are the hard lint rules the baseline skills already
// broke before the lint budget existed: the body budget and the description's
// length and shape. Only these become warnings for a baseline skill.
var legacyBaselineLintRules = []string{"body-hard-budget", "description-max", "description-one-line"}

// allLegacyBaselineFindings reports whether every hard finding comes from a
// legacy rule. Findings render as "[lint:<rule>] ..."; anything else, including a
// finding with no rule prefix, is structural and refuses.
func allLegacyBaselineFindings(hard []error) bool {
	for _, finding := range hard {
		legacy := false
		for _, rule := range legacyBaselineLintRules {
			if strings.HasPrefix(finding.Error(), "[lint:"+rule+"]") {
				legacy = true
				break
			}
		}
		if !legacy {
			return false
		}
	}
	return true
}

// baselineLintWarning is the stderr line for one hard lint finding of a baseline
// skill that `skills approve` records an approval for anyway: the finding as the
// lint prints it, marked as a warning and saying why it did not refuse.
func baselineLintWarning(finding error) string {
	return "warning: " + finding.Error() + " (baseline skill: approved with lint findings)"
}

// approvalRecordMode is the mode of an approval record: readable by everyone, because the record
// is committed with the skill and a temporary file's 0600 would not survive a checkout.
const approvalRecordMode fs.FileMode = 0o644

// WriteApprovalRecord writes data to path atomically: a temp file in the same
// directory, made world-readable (the record is committed with the skill, and
// the owner-only mode of a temporary file would not survive a checkout), synced,
// then renamed over path. A failure at any step removes the temp file and leaves
// path as it was.
func WriteApprovalRecord(files StagedWrites, path string, data []byte) error {
	tmp, err := writeFileAtomic(files, path, data, approvalRecordMode)
	if err != nil {
		return fmt.Errorf("writing approval record: %w", err)
	}
	if err := files.Rename(tmp, path); err != nil {
		files.Remove(tmp)
		return fmt.Errorf("writing approval record: finalizing %q: %w", path, err)
	}
	return nil
}
