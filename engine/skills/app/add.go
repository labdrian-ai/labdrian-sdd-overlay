package app

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// ErrRefWithoutRepo is a --ref given for an entry that has no --repo: a ref is only valid for an
// external entry, and one given to a custom entry would be dropped in silence.
var ErrRefWithoutRepo = errors.New("--ref requires --repo (ref is only valid for external entries)")

// AddInput is what `skills add` is asked: the skill to register, where the registry, the manifest
// and the skills tree are, and, for a skill that lives in another repository, where from.
type AddInput struct {
	RegistryPath string
	ManifestPath string
	SourceRoot   string
	ID           string
	// Repo and Ref make the entry external; Ref is only valid with a Repo.
	Repo string
	Ref  string
}

// AddPorts is what add reads and writes through.
type AddPorts struct {
	// Registries reads the registry and encodes the one it writes.
	Registries skills.RegistryRepository
	// Files reads the manifest.
	Files skills.FileReader
	// Stat says whether the SKILL.md of the skill is there.
	Stat skills.FileStatter
	// Approvals reads the evidence of the approval of the skill: its files, which are linted and
	// judged from one read, and the record beside them.
	Approvals skills.ApprovalRecordStore
	// Staged writes the manifest and the registry.
	Staged skills.StagedWrites
}

// AddResult is what add did, or how far it got: UnreadWarning says what the reader left out of
// the registry it read, and is set even when add then refused, except when the refusal is that
// very thing (the refusal says it).
type AddResult struct {
	ID            string
	UnreadWarning string
}

// SkillNotFoundError is a skill whose SKILL.md is not in the skills tree.
type SkillNotFoundError struct {
	ID   string
	Path string
	Err  error
}

func (e *SkillNotFoundError) Error() string {
	return fmt.Sprintf("skill %q: SKILL.md not found at %q: %v", e.ID, e.Path, e.Err)
}
func (e *SkillNotFoundError) Unwrap() error { return e.Err }

// SkillReadError is a skill whose files are there and could not be read; Path is its SKILL.md.
type SkillReadError struct {
	ID   string
	Path string
	Err  error
}

func (e *SkillReadError) Error() string { return fmt.Sprintf("reading skill %q: %v", e.Path, e.Err) }
func (e *SkillReadError) Unwrap() error { return e.Err }

// LintRefusal is a skill that the lint rules refuse: Findings are its hard findings, in order.
type LintRefusal struct{ Findings []error }

func (e *LintRefusal) Error() string {
	return fmt.Sprintf("the skill has %d hard lint finding(s)", len(e.Findings))
}

// NotApprovedError is a global skill whose files are not approved: Class says how, and Detail
// what the record or its absence says.
type NotApprovedError struct {
	Class  skills.DivergenceClass
	Detail string
}

func (e *NotApprovedError) Error() string { return fmt.Sprintf("[%s] %s", e.Class, e.Detail) }

// AddSkill registers a skill in the registry and in the manifest: it reads the registry, adds the
// entry (pure), checks the skill, and writes both files, manifest first (ADR-9). Every check is
// made before the first write, so that a refusal leaves both files as they were:
//
//   - the id is a valid slug and is not registered (skills.AddEntry), and the registry is one
//     that can be written back whole;
//   - <SourceRoot>/<id>/SKILL.md exists and has no hard lint finding;
//   - the skill has a valid approval record for its exact files, every one of them (every skill
//     add registers is global, so a human has approved it);
//   - the registry as it is written reads back as it is, and the manifest it would be written
//     with agrees with it.
//
// An error is a *IDRequiredError, ErrRefWithoutRepo, a
// *RegistryError, a *SkillNotFoundError, a *SkillReadError, a *LintRefusal, a *NotApprovedError,
// an *EncodeError, a *ReparseError, ErrRoundTripMismatch, a *ManifestReadError, a
// *ManifestSyntaxError, a *DivergenceError or a *skills.StagedWriteError; the result says what
// was read before it.
func AddSkill(p AddPorts, in AddInput) (AddResult, error) {
	var res AddResult
	if in.ID == "" {
		return res, &IDRequiredError{Verb: "add"}
	}
	if in.Ref != "" && in.Repo == "" {
		return res, ErrRefWithoutRepo
	}
	reg, warning, err := readRegistry(p.Registries, in.RegistryPath)
	if err != nil {
		return res, err
	}
	// A registry the reader left fields out of cannot be written back whole. The refusal says
	// what was left out, so the warning that says it is not also set: a person is told once.
	if err := reg.CheckWritable(); err != nil {
		return res, err
	}
	res.UnreadWarning = warning
	newReg, err := skills.AddEntry(reg, in.ID, in.Repo, in.Ref)
	if err != nil {
		return res, err
	}

	// <SourceRoot>/<id>/SKILL.md must exist (R-060).
	skillPath := in.SourceRoot + string(filepath.Separator) + in.ID + string(filepath.Separator) + "SKILL.md"
	if _, err := p.Stat(skillPath); err != nil {
		return res, &SkillNotFoundError{ID: in.ID, Path: skillPath, Err: err}
	}
	entries, err := p.Approvals.ReadSkillTree(in.SourceRoot, in.ID)
	if err != nil {
		return res, &SkillReadError{ID: in.ID, Path: skillPath, Err: err}
	}
	skillMD, found := skills.SkillMDEntry(entries)
	if !found {
		// Something is at the path of SKILL.md (it was stat'ed) that is not a file in the tree: a
		// directory, for one.
		return res, &SkillReadError{ID: in.ID, Path: skillPath, Err: errors.New("the skill directory holds no file named SKILL.md")}
	}
	// Warnings are advisory; a hard finding refuses the add. A SKILL.md that is not a regular
	// file has no bytes to lint, and the approval below says why it cannot be approved.
	if skillMD.Type.IsRegular() {
		if hard, _ := skills.LintSkillFile(skillMD.Data); len(hard) > 0 {
			return res, &LintRefusal{Findings: hard}
		}
	}
	// Every skill add registers is global tier, so a human-approval record bound to the exact
	// files, all of them, is required. Project-tier skills never reach this verb: they register
	// through project-register and stay autonomous.
	approval, err := skills.ReadApprovalStatus(in.SourceRoot, in.ID, entries, p.Approvals)
	if err != nil {
		return res, err
	}
	if verdict := skills.EvaluateApproval(in.ID, skills.ApprovalRecordPath(in.SourceRoot, in.ID), approval); !verdict.OK {
		return res, &NotApprovedError{Class: verdict.Class, Detail: verdict.Detail}
	}

	regBytes, reread, err := encodeVerified(p.Registries, newReg)
	if err != nil {
		return res, err
	}
	manifest, err := p.Files(in.ManifestPath)
	if err != nil {
		return res, &ManifestReadError{Path: in.ManifestPath, Err: err}
	}
	newManifest := skills.ManifestWithSkill(manifest, in.ID)
	if err := crossCheck(reread, newManifest); err != nil {
		return res, err
	}
	if err := skills.CommitStaged(p.Staged, skills.OverlayFileMode,
		skills.StagedFile{Name: "manifest", Path: in.ManifestPath, Data: newManifest},
		skills.StagedFile{Name: "registry", Path: in.RegistryPath, Data: regBytes}); err != nil {
		return res, err
	}
	res.ID = in.ID
	return res, nil
}
