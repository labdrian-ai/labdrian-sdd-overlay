package app

import (
	"errors"
	"fmt"
	"io/fs"
	"path"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// ProjectLockReadError is a project lock that could not be read, or that a verb needs and the
// project has none of. The verb refuses: a lock that exists and cannot be read must never be
// replaced by an empty one, which would disown everything recorded in it.
type ProjectLockReadError struct{ Err error }

func (e *ProjectLockReadError) Error() string {
	return fmt.Sprintf("reading project lock %q: %v", skills.ProjectLockRelPath, e.Err)
}
func (e *ProjectLockReadError) Unwrap() error { return e.Err }

// ExecutionError is a plan that could not be carried out: the files it staged and renamed were put
// back as far as they could be. Unrestored is every path that could not be (repo-relative, in the
// order the rollback found them) and is empty when everything was; the adapter tells them before
// the error itself.
type ExecutionError struct {
	Err        error
	Unrestored []string
}

// executionFailure is the ExecutionError of err, the failure of an executor of the domain: the
// paths its rollback could not restore are the ones its typed error names.
func executionFailure(err error) *ExecutionError {
	failure := &ExecutionError{Err: err}
	var incomplete *skills.RollbackIncompleteError
	if errors.As(err, &incomplete) {
		failure.Unrestored = incomplete.Unrestored
	}
	return failure
}

func (e *ExecutionError) Error() string { return e.Err.Error() }
func (e *ExecutionError) Unwrap() error { return e.Err }

// readOptionalProjectLock reads the lock of the project at root for a verb that creates it when
// the project has none: exists is false for a project with no lock yet, and any other failure is a
// *ProjectLockReadError.
func readOptionalProjectLock(locks skills.ProjectLockStore, root string) (data []byte, exists bool, err error) {
	data, err = locks.ReadLock(root)
	switch {
	case err == nil:
		return data, true, nil
	case errors.Is(err, fs.ErrNotExist):
		return nil, false, nil
	}
	return nil, false, &ProjectLockReadError{Err: err}
}

// readRequiredProjectLock reads the lock of the project at root for a verb that works on what is
// recorded in it: a project with no lock is refused like one whose lock cannot be read.
func readRequiredProjectLock(locks skills.ProjectLockStore, root string) ([]byte, error) {
	data, err := locks.ReadLock(root)
	if err != nil {
		return nil, &ProjectLockReadError{Err: err}
	}
	return data, nil
}

// ProjectPorts is what the verbs that register skills in a project read and write through. The
// registry is read by register, retire and status; revise reads none.
type ProjectPorts struct {
	// Registries reads the registry.
	Registries skills.RegistryRepository
	// Files reads the draft, and the files of the project that ownership is proved from.
	Files skills.FileReader
	// Locks reads the project lock.
	Locks skills.ProjectLockStore
	// Project probes the project and carries out the plan.
	Project skills.ProjectFS
}

// DraftReadError is a draft that could not be read.
type DraftReadError struct {
	Path string
	Err  error
}

func (e *DraftReadError) Error() string { return fmt.Sprintf("reading draft %q: %v", e.Path, e.Err) }
func (e *DraftReadError) Unwrap() error { return e.Err }

// PlanFailure is a plan the domain refused to make: the draft is not one it registers, the project
// is not one it writes into, the skill is not the agent's. Nothing was written. Err is the refusal
// of the planner, in its own words.
type PlanFailure struct{ Err error }

func (e *PlanFailure) Error() string { return e.Err.Error() }
func (e *PlanFailure) Unwrap() error { return e.Err }

// ProjectLockParseError is a project lock that was read and is not a lock the domain accepts.
type ProjectLockParseError struct{ Err error }

func (e *ProjectLockParseError) Error() string { return e.Err.Error() }
func (e *ProjectLockParseError) Unwrap() error { return e.Err }

// SkillNotInLockError is a skill that was asked about and the project lock does not record.
type SkillNotInLockError struct{ ID string }

func (e *SkillNotInLockError) Error() string {
	return fmt.Sprintf("project-status: skill %q is not in the project lock", e.ID)
}

// relPaths lists the repo-relative paths of writes, which are forward-slashed so that each is
// usable as a git pathspec.
func relPaths(writes []skills.ProjectWrite) []string {
	rels := make([]string, len(writes))
	for i, w := range writes {
		rels[i] = w.Rel
	}
	return rels
}

// ---- project-register --------------------------------------------------------------------

// ProjectRegisterInput is what `skills project-register` is asked.
type ProjectRegisterInput struct {
	// ProjectRoot is the project the skill is registered in: absolute.
	ProjectRoot string
	// Candidate is the key of the procedural candidate the draft comes from.
	Candidate string
	// RegistryPath is the overlay registry the identity check runs against.
	RegistryPath string
	// DraftPath is the draft SKILL.md.
	DraftPath string
	// DryRun plans and writes nothing.
	DryRun bool
}

// ProjectRegisterResult is what register did, or how far it got: UnreadWarning says what the
// reader left out of the registry it read, and is set even when register then refused.
type ProjectRegisterResult struct {
	UnreadWarning string
	// Planned lists the paths of the plan in commit order (the skills, then the lock last); it is
	// set by a dry run only.
	Planned []string
	// Wrote lists the paths written, in the same order.
	Wrote []string
	// SHA256 is the digest of the SKILL.md written, and Revision its revision.
	SHA256   string
	Revision int
}

// ProjectRegister registers a draft as a procedural skill of the project: it reads the registry,
// the draft and the project lock, plans every write before making one, and carries the plan out all
// or nothing. An error is a *RegistryError, a *DraftReadError, a *ProjectLockReadError, a
// *PlanFailure or an *ExecutionError; where it is not an *ExecutionError nothing was written.
func ProjectRegister(p ProjectPorts, in ProjectRegisterInput) (ProjectRegisterResult, error) {
	var res ProjectRegisterResult
	reg, warning, err := readRegistry(p.Registries, in.RegistryPath)
	if err != nil {
		return res, err
	}
	res.UnreadWarning = warning
	draft, err := p.Files(in.DraftPath)
	if err != nil {
		return res, &DraftReadError{Path: in.DraftPath, Err: err}
	}
	// The lock is optional: the first registration in a project creates it. One that exists and
	// cannot be read must never be replaced by an empty one, which would drop every skill already
	// registered.
	lockData, lockExists, err := readOptionalProjectLock(p.Locks, in.ProjectRoot)
	if err != nil {
		return res, err
	}
	plan, err := skills.PlanProjectRegister(skills.RegisterInput{
		ProjectRoot: in.ProjectRoot, DraftPath: in.DraftPath, DraftData: draft, CandidateKey: in.Candidate,
		Registry: reg, LockData: lockData, LockExists: lockExists,
		Stat: p.Project.Stat, ResolvePath: p.Project.ResolvePath,
	})
	if err != nil {
		return res, &PlanFailure{Err: err}
	}
	order := relPaths(skills.ProjectCommitOrder(plan))
	if in.DryRun {
		res.Planned = order
		return res, nil
	}
	if err := executeProject(plan, p.Project, skills.ExecuteProjectPlan); err != nil {
		return res, err
	}
	res.Wrote, res.SHA256, res.Revision = order, plan.SHA256, plan.Revision
	return res, nil
}

// executeProject carries a plan out through the executor of the domain. What the executor could not
// put back is in the error it returns; this hands it over typed, and prints nothing.
func executeProject(plan skills.ProjectPlan, fsys skills.ProjectFS, execute func(skills.ProjectPlan, skills.ProjectFS) error) error {
	if err := execute(plan, fsys); err != nil {
		return executionFailure(err)
	}
	return nil
}

// ---- project-revise ------------------------------------------------------------------------

// ProjectReviseInput is what `skills project-revise` is asked.
type ProjectReviseInput struct {
	ProjectRoot string
	Candidate   string
	DraftPath   string
	DryRun      bool
}

// ProjectReviseResult is what revise did.
type ProjectReviseResult struct {
	// Planned lists the paths of the plan in commit order; set by a dry run only.
	Planned []string
	// Wrote lists the paths written, in the same order.
	Wrote    []string
	SHA256   string
	Revision int
}

// ProjectRevise replaces the SKILL.md of a skill the agent registered, once the project lock proves
// the agent still owns it: a skill a person edited, deleted or added a file to is not touched. The
// project lock must be there. The errors are those of ProjectRegister, without the registry's.
func ProjectRevise(p ProjectPorts, in ProjectReviseInput) (ProjectReviseResult, error) {
	var res ProjectReviseResult
	draft, err := p.Files(in.DraftPath)
	if err != nil {
		return res, &DraftReadError{Path: in.DraftPath, Err: err}
	}
	lockData, err := readRequiredProjectLock(p.Locks, in.ProjectRoot)
	if err != nil {
		return res, err
	}
	plan, err := skills.PlanProjectRevise(skills.ReviseInput{
		ProjectRoot: in.ProjectRoot, DraftPath: in.DraftPath, DraftData: draft, CandidateKey: in.Candidate,
		LockData: lockData, LockExists: true,
		ReadFile: p.Files, ReadDir: p.Project.ReadDir, Stat: p.Project.Stat, ResolvePath: p.Project.ResolvePath,
	})
	if err != nil {
		return res, &PlanFailure{Err: err}
	}
	order := relPaths(skills.ProjectCommitOrder(plan))
	if in.DryRun {
		res.Planned = order
		return res, nil
	}
	if err := executeProject(plan, p.Project, skills.ExecuteProjectRevisePlan); err != nil {
		return res, err
	}
	res.Wrote, res.SHA256, res.Revision = order, plan.SHA256, plan.Revision
	return res, nil
}

// ---- project-retire ------------------------------------------------------------------------

// ProjectRetireInput is what `skills project-retire` is asked.
type ProjectRetireInput struct {
	ProjectRoot  string
	RegistryPath string
	// ID is the skill to retire.
	ID string
	// Reason and AbsorbedInto are the metadata the retirement carries; AbsorbedInto, when it names
	// a target, is proved to exist before anything is deleted.
	Reason       string
	AbsorbedInto string
	DryRun       bool
}

// ProjectRetireResult is what retire did, or how far it got.
type ProjectRetireResult struct {
	UnreadWarning string
	// Planned lists the paths of the plan in commit order (the deletions, then the lock); set by a
	// dry run only.
	Planned []string
	// Removed lists the skill files that were removed.
	Removed []string
}

// ProjectRetire removes a skill the agent registered from the project, files and lock entry, once
// the lock proves the agent still owns it. It never infers a retirement. The errors are those of
// ProjectRegister; the lock must be there.
func ProjectRetire(p ProjectPorts, in ProjectRetireInput) (ProjectRetireResult, error) {
	var res ProjectRetireResult
	reg, warning, err := readRegistry(p.Registries, in.RegistryPath)
	if err != nil {
		return res, err
	}
	res.UnreadWarning = warning
	root := in.ProjectRoot
	lockData, err := readRequiredProjectLock(p.Locks, root)
	if err != nil {
		return res, err
	}
	plan, err := skills.PlanProjectRetire(skills.RetireInput{
		ProjectRoot: root, ID: in.ID, Reason: in.Reason, AbsorbedInto: in.AbsorbedInto,
		Registry: reg, LockData: lockData, LockExists: true,
		ReadFile: p.Files, ReadDir: p.Project.ReadDir, Stat: p.Project.Stat, ResolvePath: p.Project.ResolvePath,
	})
	if err != nil {
		return res, &PlanFailure{Err: err}
	}
	if in.DryRun {
		res.Planned = relPaths(skills.ProjectRetireCommitOrder(plan))
		return res, nil
	}
	if err := executeProject(plan, p.Project, skills.ExecuteProjectRetirePlan); err != nil {
		return res, err
	}
	res.Removed = relPaths(plan.DeleteWrites)
	return res, nil
}

// ---- project-status ------------------------------------------------------------------------

// ProjectStatusInput is what `skills project-status` is asked.
type ProjectStatusInput struct {
	ProjectRoot  string
	RegistryPath string
	// ID narrows the answer to one skill of the lock; empty asks for every one.
	ID string
}

// ProjectSkillStatus is what is known of one skill the project lock records.
type ProjectSkillStatus struct {
	ID       string
	Revision int
	// AgentOwned says the agent still owns the skill; when it does not, Reason says why (a
	// hash-mismatch, a missing file, an entry that is not the agent's).
	AgentOwned bool
	Reason     string
	// SupersededBy is the path of the global skill that covers the same identity, or empty.
	SupersededBy string
}

// ProjectStatusResult is the answer of project-status.
type ProjectStatusResult struct {
	UnreadWarning string
	Skills        []ProjectSkillStatus
}

// ProjectStatus reports who owns each skill the project lock records, and which global skill
// supersedes it, matched by the id of the skill and by the last slug of its candidate key. It reads
// and writes nothing. The errors are a *RegistryError, a *ProjectLockReadError, a
// *ProjectLockParseError and a *SkillNotInLockError.
func ProjectStatus(p ProjectPorts, in ProjectStatusInput) (ProjectStatusResult, error) {
	var res ProjectStatusResult
	root := in.ProjectRoot
	reg, warning, err := readRegistry(p.Registries, in.RegistryPath)
	if err != nil {
		return res, err
	}
	res.UnreadWarning = warning
	data, err := readRequiredProjectLock(p.Locks, root)
	if err != nil {
		return res, err
	}
	lock, err := skills.ParseProjectLock(data)
	if err != nil {
		return res, &ProjectLockParseError{Err: err}
	}
	entries := lock.Skills
	if in.ID != "" {
		entries = nil
		for _, entry := range lock.Skills {
			if entry.ID == in.ID {
				entries = []skills.ProjectLockEntry{entry}
				break
			}
		}
		if len(entries) == 0 {
			return res, &SkillNotInLockError{ID: in.ID}
		}
	}
	for _, entry := range entries {
		ownership := skills.EvaluateOwnership(root, entry, p.Files, p.Project.ReadDir, p.Project.ResolvePath)
		res.Skills = append(res.Skills, ProjectSkillStatus{
			ID: entry.ID, Revision: entry.Revision,
			AgentOwned: ownership.AgentOwned, Reason: ownership.Reason,
			SupersededBy: supersededBy(reg, entry),
		})
	}
	return res, nil
}

// supersededBy is the first global registry path that matches the project entry's id or the last
// slug of its candidate key. MatchCandidate is an existence and identity lookup here: it does not
// claim that the global skill covers the project skill's content.
func supersededBy(reg skills.Registry, entry skills.ProjectLockEntry) string {
	if matched, skillPath := skills.MatchCandidate(reg, entry.ID); matched {
		return skillPath
	}
	if matched, skillPath := skills.MatchCandidate(reg, path.Base(entry.Candidate)); matched {
		return skillPath
	}
	return ""
}
