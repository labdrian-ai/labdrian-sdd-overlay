package skills

import (
	"fmt"
	"path/filepath"
)

// Divergence classes for the approval check. They extend the registry/manifest
// classes in validate.go and the on-disk classes in ondisk.go and print through
// the same `[CLASS] path: detail` line, so `skills validate` reports them beside
// the others in one full scan.
const (
	// DivApprovalMissing reports a global skill with no approval record.
	DivApprovalMissing DivergenceClass = "APPROVAL_MISSING"
	// DivApprovalStale reports a global skill whose record no longer approves
	// what the skill is: a file of the skill changed, was added or was removed
	// after the approval, or the record is of the version that covered SKILL.md
	// alone.
	DivApprovalStale DivergenceClass = "APPROVAL_STALE"
	// DivApprovalMalformed reports a global skill whose record does not parse
	// strictly, or names a different skill.
	DivApprovalMalformed DivergenceClass = "APPROVAL_MALFORMED"
	// DivApprovalUnverifiable reports a global skill whose files or record exist
	// but cannot be read, or whose directory holds something an approval cannot
	// cover (a link, a pipe), so the approval cannot be checked at all.
	DivApprovalUnverifiable DivergenceClass = "APPROVAL_UNVERIFIABLE"
)

// ApprovalVerdict is the outcome of applying the approval requirement to one
// global skill. When OK is false, Class and Detail describe the refusal;
// Detail names the skill, the state and the exact command that fixes it.
type ApprovalVerdict struct {
	OK     bool
	Class  DivergenceClass
	Detail string
}

// approveHint is the exact fixing command every refusal ends with. The
// approver label is the one thing the engine cannot fill in for the human.
func approveHint(id string) string {
	return fmt.Sprintf("labdrian skills approve --id %s --approver <name>", id)
}

// EvaluateApproval applies the approval requirement for a global skill: a valid
// record satisfies it, and nothing else does. A skill is not exempt for what it
// was or for who wrote it, and a record that is present but not valid is never
// silently ignored.
//
// recordPath is used only to make the refusal message actionable. It is pure:
// no filesystem access. `skills add` and `skills validate` share it, so they
// can never disagree about what is approved.
func EvaluateApproval(id, recordPath string, status ApprovalStatus) ApprovalVerdict {
	switch status.State {
	case ApprovalValid:
		return ApprovalVerdict{OK: true}

	case ApprovalAbsent:
		return ApprovalVerdict{
			Class: DivApprovalMissing,
			Detail: fmt.Sprintf("skill %q: no approval record at %s; a human must review every file of the skill and run: %s",
				id, filepath.ToSlash(recordPath), approveHint(id)),
		}

	case ApprovalStale:
		return ApprovalVerdict{
			Class: DivApprovalStale,
			Detail: fmt.Sprintf("skill %q: approval record %s is stale (%s); a human must review every file of the skill and run: %s",
				id, filepath.ToSlash(recordPath), status.Detail, approveHint(id)),
		}

	case ApprovalUnverifiable:
		return ApprovalVerdict{
			Class: DivApprovalUnverifiable,
			Detail: fmt.Sprintf("skill %q: its approval cannot be verified (%s); an approval covers regular files only, so replace what is not one with a regular file, or remove it",
				id, status.Detail),
		}

	default:
		return ApprovalVerdict{
			Class: DivApprovalMalformed,
			Detail: fmt.Sprintf("skill %q: approval record %s is malformed (%s); fix or delete it, then run: %s",
				id, filepath.ToSlash(recordPath), status.Detail, approveHint(id)),
		}
	}
}

// ApprovalSummary accounts for every global-scope registry entry exactly once:
// Global = Approved + SkillFileMissing + the number of approval divergences
// CheckApprovals returned. Approved counts the entries satisfied by a valid
// record; SkillFileMissing counts the entries set aside because their SKILL.md
// does not exist, which the manifest and on-disk cross-checks report.
type ApprovalSummary struct {
	Global           int
	Approved         int
	SkillFileMissing int
}

// CheckApprovals applies the approval requirement to every global-scope entry
// in reg, reading the files of the skill and the record beside them under
// sourceRoot. It is a full scan: it never stops at the first divergence.
// Project-scope entries stay autonomous and are skipped.
//
// An entry whose SKILL.md does not exist is set aside here, counted in
// SkillFileMissing so the summary still adds up: every full `skills validate`
// run already reports it, as MISSING_ON_DISK when the manifest has its row and
// as a registry/manifest divergence when it does not, and reporting it a second
// time as an approval finding would only add noise. Any other read failure, of
// the skill or of its record, is DivApprovalUnverifiable rather than a guess in
// either direction, and so is a skill whose directory holds something an
// approval cannot cover.
//
// The entry's Path names its directory under sourceRoot and is the skill id the
// record must carry (AddEntry always registers Path == ID).
func CheckApprovals(reg Registry, sourceRoot string, records ApprovalRecordStore) ([]Divergence, ApprovalSummary) {
	var divs []Divergence
	var sum ApprovalSummary
	for _, e := range reg.Skills {
		if e.Install.DefaultScope != "global" {
			continue
		}
		sum.Global++

		entries, err := records.ReadSkillTree(sourceRoot, e.Path)
		if err != nil {
			if isAbsent(err) {
				sum.SkillFileMissing++
				continue
			}
			divs = append(divs, Divergence{
				Class:  DivApprovalUnverifiable,
				Path:   e.Path,
				Detail: fmt.Sprintf("skill %q: cannot read %s to verify its approval: %v", e.Path, filepath.ToSlash(SkillDirPath(sourceRoot, e.Path)), err),
			})
			continue
		}
		if _, found := SkillMDEntry(entries); !found {
			sum.SkillFileMissing++
			continue
		}

		status, err := ReadApprovalStatus(sourceRoot, e.Path, entries, records)
		if err != nil {
			divs = append(divs, Divergence{Class: DivApprovalUnverifiable, Path: e.Path, Detail: err.Error()})
			continue
		}

		if verdict := EvaluateApproval(e.Path, ApprovalRecordPath(sourceRoot, e.Path), status); !verdict.OK {
			divs = append(divs, Divergence{Class: verdict.Class, Path: e.Path, Detail: verdict.Detail})
		} else {
			sum.Approved++
		}
	}
	return divs, sum
}
