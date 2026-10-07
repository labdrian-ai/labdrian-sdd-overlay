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
	// DivApprovalMissing reports a global skill with no approval record whose
	// SKILL.md is not the grandfathered baseline's.
	DivApprovalMissing DivergenceClass = "APPROVAL_MISSING"
	// DivApprovalStale reports a global skill whose record digest no longer
	// matches its SKILL.md bytes.
	DivApprovalStale DivergenceClass = "APPROVAL_STALE"
	// DivApprovalMalformed reports a global skill whose record does not parse
	// strictly, or names a different skill.
	DivApprovalMalformed DivergenceClass = "APPROVAL_MALFORMED"
	// DivApprovalUnverifiable reports a global skill whose SKILL.md or record
	// exists but cannot be read, so the approval cannot be checked at all.
	DivApprovalUnverifiable DivergenceClass = "APPROVAL_UNVERIFIABLE"
)

// ApprovalVerdict is the outcome of applying the approval requirement to one
// global skill. When OK is false, Class and Detail describe the refusal;
// Detail names the skill, the state and the exact command that fixes it.
type ApprovalVerdict struct {
	OK bool
	// Grandfathered is true when OK holds only because the skill is in the
	// baseline and its bytes still equal the pinned digest (no record needed).
	Grandfathered bool
	Class         DivergenceClass
	Detail        string
}

// approveHint is the exact fixing command every refusal ends with. The
// approver label is the one thing the engine cannot fill in for the human.
func approveHint(id string) string {
	return fmt.Sprintf("labdrian skills approve --id %s --approver <name>", id)
}

// EvaluateApproval applies the approval requirement for a global skill:
//
//   - a valid record satisfies it;
//   - an absent record satisfies it only when the skill is in the grandfathered
//     baseline and its SKILL.md still has the pinned digest;
//   - a stale or malformed record never satisfies it, baseline or not: a record
//     that is present must be valid, so a governance file is never silently
//     ignored.
//
// recordPath is used only to make the refusal message actionable. It is pure:
// no filesystem access. `skills add` and `skills validate` share it, so they
// can never disagree about what is approved.
func EvaluateApproval(id, recordPath string, skillMD []byte, status ApprovalStatus) ApprovalVerdict {
	return EvaluateApprovalAgainst(FixedBaseline, id, recordPath, skillMD, status)
}

// EvaluateApprovalAgainst is EvaluateApproval with the baseline the caller names; a nil baseline is the
// fixed one.
func EvaluateApprovalAgainst(baseline BaselineLookup, id, recordPath string, skillMD []byte, status ApprovalStatus) ApprovalVerdict {
	baseline = baseline.OrFixed()
	switch status.State {
	case ApprovalValid:
		return ApprovalVerdict{OK: true}

	case ApprovalAbsent:
		digest := SkillDigest(skillMD)
		pinned, inBaseline := baseline(id)
		if inBaseline && pinned == digest {
			return ApprovalVerdict{OK: true, Grandfathered: true}
		}
		detail := fmt.Sprintf("skill %q: no approval record at %s; a human must review the exact SKILL.md bytes and run: %s",
			id, filepath.ToSlash(recordPath), approveHint(id))
		if inBaseline {
			detail = fmt.Sprintf("skill %q: no approval record at %s, and SKILL.md differs from the grandfathered baseline (baseline digest %s, file digest %s); a human must review the changed bytes and run: %s",
				id, filepath.ToSlash(recordPath), pinned, digest, approveHint(id))
		}
		return ApprovalVerdict{Class: DivApprovalMissing, Detail: detail}

	case ApprovalStale:
		return ApprovalVerdict{
			Class: DivApprovalStale,
			Detail: fmt.Sprintf("skill %q: approval record %s is stale (%s); a human must review the changed bytes and run: %s",
				id, filepath.ToSlash(recordPath), status.Detail, approveHint(id)),
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
// Global = Approved + Grandfathered + SkillFileMissing + the number of
// approval divergences CheckApprovals returned. Approved and Grandfathered
// count the entries satisfied by a valid record and by the baseline
// respectively; SkillFileMissing counts the entries set aside because their
// SKILL.md does not exist, which the manifest and on-disk cross-checks report.
type ApprovalSummary struct {
	Global           int
	Approved         int
	Grandfathered    int
	SkillFileMissing int
}

// CheckApprovals applies the approval requirement to every global-scope entry
// in reg, reading SKILL.md and the record beside it under sourceRoot. It is a
// full scan: it never stops at the first divergence. Project-scope entries stay
// autonomous and are skipped.
//
// An entry whose SKILL.md does not exist is set aside here, counted in
// SkillFileMissing so the summary still adds up: every full `skills validate`
// run already reports it, as MISSING_ON_DISK when the manifest has its row and
// as a registry/manifest divergence when it does not, and reporting it a second
// time as an approval finding would only add noise. Any other read failure, of
// the skill or of its record, is DivApprovalUnverifiable rather than a guess in
// either direction.
//
// The entry's Path names its directory under sourceRoot and is the skill id the
// record must carry (AddEntry always registers Path == ID).
func CheckApprovals(reg Registry, sourceRoot string, records ApprovalRecordStore) ([]Divergence, ApprovalSummary) {
	return CheckApprovalsAgainst(FixedBaseline, reg, sourceRoot, records)
}

// CheckApprovalsAgainst is CheckApprovals with the baseline the caller names; a nil baseline is the
// fixed one.
func CheckApprovalsAgainst(baseline BaselineLookup, reg Registry, sourceRoot string, records ApprovalRecordStore) ([]Divergence, ApprovalSummary) {
	baseline = baseline.OrFixed()
	var divs []Divergence
	var sum ApprovalSummary
	for _, e := range reg.Skills {
		if e.Install.DefaultScope != "global" {
			continue
		}
		sum.Global++

		skillPath := SkillMDPath(sourceRoot, e.Path)
		skillMD, err := records.ReadSkill(sourceRoot, e.Path)
		if err != nil {
			if isAbsent(err) {
				sum.SkillFileMissing++
				continue
			}
			divs = append(divs, Divergence{
				Class:  DivApprovalUnverifiable,
				Path:   e.Path,
				Detail: fmt.Sprintf("skill %q: cannot read %s to verify its approval: %v", e.Path, filepath.ToSlash(skillPath), err),
			})
			continue
		}

		status, err := ReadApprovalStatus(sourceRoot, e.Path, skillMD, records)
		if err != nil {
			divs = append(divs, Divergence{Class: DivApprovalUnverifiable, Path: e.Path, Detail: err.Error()})
			continue
		}

		verdict := EvaluateApprovalAgainst(baseline, e.Path, ApprovalRecordPath(sourceRoot, e.Path), skillMD, status)
		switch {
		case !verdict.OK:
			divs = append(divs, Divergence{Class: verdict.Class, Path: e.Path, Detail: verdict.Detail})
		case verdict.Grandfathered:
			sum.Grandfathered++
		default:
			sum.Approved++
		}
	}
	return divs, sum
}
