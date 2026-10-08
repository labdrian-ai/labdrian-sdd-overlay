package app

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// InstallInput is what `skills install` and `skills adopt` are asked: which registry says what
// each project admits, where the skills are copied from, and which directory is the project.
type InstallInput struct {
	// RegistryPath is the registry that admits skills to projects.
	RegistryPath string
	// SourceRoot is the skills tree the admitted skills are read from.
	SourceRoot string
	// ProjectID is the id the person gave to the project, empty when they gave none: the identity
	// port answers then.
	ProjectID skills.ProjectID
	// ProjectRoot is the directory the skills go into: absolute, and the one the caller locked.
	ProjectRoot string
}

// InstallPorts is what install and adopt read and write through.
type InstallPorts struct {
	// Registries reads the registry.
	Registries skills.RegistryRepository
	// Identity says which project ProjectRoot is.
	Identity skills.ProjectIdentity
	// Tree reads the source of a skill.
	Tree skills.SkillTree
	// Locks reads the project lock.
	Locks skills.ProjectLockStore
	// Files reads the files of the project the plan looks at.
	Files skills.FileReader
	// Project probes the project and carries out the plan.
	Project skills.ProjectFS
}

// InstallResult is what install or adopt did, or how far it got: UnreadWarning says what the
// reader left out of the registry it read, and is set even when the verb then refused.
type InstallResult struct {
	UnreadWarning string
	// ProjectID is the project the verb worked for, once it knew it.
	ProjectID skills.ProjectID
	// NoneAdmitted says the registry admits no project-scoped skill to the project: there was
	// nothing to do, and nothing was done.
	NoneAdmitted bool
	// Skills is what was done to each skill, in registry order.
	Skills []skills.InstallOutcome
	// Notes are what is worth telling that is no refusal.
	Notes []string
}

// IdentityFailure says in which way the project could not be named.
type IdentityFailure int

const (
	// IdentityNotWired: no identity port was given, so no project can be named.
	IdentityNotWired IdentityFailure = iota + 1
	// IdentityUnknown: the port could not tell, which is not the same as having no answer.
	IdentityUnknown
	// IdentityNoAnswer: no source of identity could name the project.
	IdentityNoAnswer
)

// IdentityError is a project that could not be named: a verb that cannot name its project writes
// nothing, rather than naming it by a rule of its own.
type IdentityError struct {
	Failure IdentityFailure
	Verb    string
	// Dir is the directory that was asked about.
	Dir string
	// Err is the failure of the port, for IdentityUnknown.
	Err error
}

func (e *IdentityError) Unwrap() error { return e.Err }

func (e *IdentityError) Error() string {
	switch e.Failure {
	case IdentityNotWired:
		return fmt.Sprintf("skills %s: no project identity is wired", e.Verb)
	case IdentityUnknown:
		return fmt.Sprintf("skills %s: resolving project identity: %v", e.Verb, e.Err)
	default:
		return fmt.Sprintf("skills %s: no source of project identity could name the project in %s; give --project-id", e.Verb, e.Dir)
	}
}

// PlanError is a registry that admits a skill whose paths would leave the directories they belong
// in (R-055): nothing is planned.
type PlanError struct {
	Verb string
	Err  error
}

func (e *PlanError) Error() string { return fmt.Sprintf("planning %s: %v", e.Verb, e.Err) }
func (e *PlanError) Unwrap() error { return e.Err }

// SourceRootRequiredError is an install or adopt that admits a skill and was given no source root
// to read it from. It is a missing input, and is told as one; a registry that admits nothing needs
// no source root.
type SourceRootRequiredError struct{ Verb string }

func (e *SourceRootRequiredError) Error() string {
	return fmt.Sprintf("skills %s: no source root is given to read the admitted skills from", e.Verb)
}

// SourcesMissingError is an admitted skill whose source directory is not there. Every one is
// listed, not the first: the person fixes them in one go.
type SourcesMissingError struct{ Missing []skills.CopyOp }

func (e *SourcesMissingError) Error() string {
	return fmt.Sprintf("%d source director(ies) missing", len(e.Missing))
}

// SourceReadError is the source of a skill that is there and could not be read.
type SourceReadError struct {
	ID, Dir string
	Err     error
}

func (e *SourceReadError) Error() string {
	return fmt.Sprintf("skill %s: reading its source %s: %v", e.ID, e.Dir, e.Err)
}
func (e *SourceReadError) Unwrap() error { return e.Err }

// PlanRefusal is a verb that the ownership rules refused, every reason given and not the first:
// nothing was written.
type PlanRefusal struct {
	Verb    string
	Reasons []string
}

func (e *PlanRefusal) Error() string {
	return fmt.Sprintf("skills %s: refused (%d)", e.Verb, len(e.Reasons))
}

// InstallProject copies the skills the registry admits to the project into it, and records them
// in the project lock, all or nothing (skills.PlanInstallOwnership decides, skills.ExecuteInstallPlan
// carries out). An error is an *IdentityError, a *RegistryError, a *SourceRootRequiredError, a *PlanError, a
// *SourcesMissingError, a *SourceReadError, a *ProjectLockReadError, a *PlanRefusal or an
// *ExecutionError; where it is not an *ExecutionError nothing was written.
func InstallProject(p InstallPorts, in InstallInput) (InstallResult, error) {
	return runPlanned("install", skills.PlanInstallOwnership, p, in)
}

// AdoptProject records the skills of the project that are already there exactly as the source has
// them, so that install owns them from now on. It writes the project lock and no skill file. The
// errors are those of InstallProject.
func AdoptProject(p InstallPorts, in InstallInput) (InstallResult, error) {
	return runPlanned("adopt", skills.PlanAdopt, p, in)
}

// runPlanned is a verb that is a plan followed by its execution: a refusal gives every reason and
// writes nothing; otherwise the plan is executed all or nothing.
func runPlanned(verb string, plan func(skills.InstallInput) (skills.InstallPlan, []string), p InstallPorts, in InstallInput) (InstallResult, error) {
	var res InstallResult
	root := filepath.Clean(in.ProjectRoot)

	projectID, err := identify(verb, p.Identity, in.ProjectRoot, in.ProjectID)
	if err != nil {
		return res, err
	}
	res.ProjectID = projectID

	reg, warning, err := readRegistry(p.Registries, in.RegistryPath)
	if err != nil {
		return res, err
	}
	res.UnreadWarning = warning

	ops, err := admittedSources(verb, reg, projectID, in.SourceRoot, root)
	if err != nil {
		return res, err
	}
	if len(ops) == 0 {
		res.NoneAdmitted = true
		return res, nil
	}
	sources, err := readSources(p, ops)
	if err != nil {
		return res, err
	}

	// The lock is optional: the first install in a project creates it.
	lockData, lockExists, err := readOptionalProjectLock(p.Locks, root)
	if err != nil {
		return res, err
	}

	planned, refusals := plan(skills.InstallInput{
		ProjectRoot: root,
		ProjectID:   projectID,
		Skills:      sources,
		LockData:    lockData,
		LockExists:  lockExists,
		ReadFile:    p.Files,
		Stat:        p.Project.Stat,
		ResolvePath: p.Project.ResolvePath,
		ReadDir:     p.Project.ReadDir,
	})
	if len(refusals) > 0 {
		return res, &PlanRefusal{Verb: verb, Reasons: refusals}
	}
	if err := skills.ExecuteInstallPlan(planned, root, p.Project); err != nil {
		return res, executionFailure(err)
	}
	res.Skills, res.Notes = planned.Skills, planned.Notes
	return res, nil
}

// admittedSources is what the registry admits to the project: the copies the install would make.
// A planner that refuses a path is a *PlanError; an admitted skill with no source root to read it
// from is a *SourceRootRequiredError.
func admittedSources(verb string, reg skills.Registry, projectID skills.ProjectID, sourceRoot, root string) ([]skills.CopyOp, error) {
	ops, err := skills.PlanInstall(reg, projectID, sourceRoot, root)
	if errors.Is(err, skills.ErrNoSourceRoot) {
		return nil, &SourceRootRequiredError{Verb: verb}
	}
	if err != nil {
		return nil, &PlanError{Verb: verb, Err: err}
	}
	return ops, nil
}

// readSources reads the source of every skill the copies name. Every source must be there before
// any is read (R-053): the ones that are not are a *SourcesMissingError, and the first that cannot
// be read is a *SourceReadError.
func readSources(p InstallPorts, ops []skills.CopyOp) ([]skills.InstallSkill, error) {
	var missing []skills.CopyOp
	for _, op := range ops {
		if info, err := p.Project.Stat(op.Src); err != nil || !info.IsDir() {
			missing = append(missing, op)
		}
	}
	if len(missing) > 0 {
		return nil, &SourcesMissingError{Missing: missing}
	}
	sources := make([]skills.InstallSkill, 0, len(ops))
	for _, op := range ops {
		files, err := p.Tree.ReadSkillSource(op.Src)
		if err != nil {
			return nil, &SourceReadError{ID: op.SkillID, Dir: op.Src, Err: err}
		}
		sources = append(sources, skills.InstallSkill{ID: op.SkillID, Files: files})
	}
	return sources, nil
}

// identify asks the identity port which project dir is, with the id the person gave.
func identify(verb string, identity skills.ProjectIdentity, dir string, explicit skills.ProjectID) (skills.ProjectID, error) {
	if identity == nil {
		return "", &IdentityError{Failure: IdentityNotWired, Verb: verb, Dir: dir}
	}
	id, ok, err := identity.Identify(skills.ProjectQuery{Dir: dir, Explicit: explicit})
	switch {
	case err != nil:
		return "", &IdentityError{Failure: IdentityUnknown, Verb: verb, Dir: dir, Err: err}
	case !ok:
		return "", &IdentityError{Failure: IdentityNoAnswer, Verb: verb, Dir: dir}
	}
	return id, nil
}
