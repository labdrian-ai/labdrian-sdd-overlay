package skills

import (
	"fmt"
	"path/filepath"
)

// VerifyAbsorbedInto verifies only that a consolidation target exists at one
// of the two supported tiers. A global target is looked up through
// MatchCandidate, which is deliberately used here as an existence lookup; it
// does not prove that the target covers the retiring skill's content. A
// project target is an exact project-lock entry. An empty target means that
// this retirement is not a consolidation and therefore needs no lookup.
func VerifyAbsorbedInto(target string, registry Registry, lock ProjectLock) error {
	if target == "" {
		return nil
	}
	if matched, _ := MatchCandidate(registry, target); matched {
		return nil
	}
	for _, entry := range lock.Skills {
		if entry.ID == target {
			return nil
		}
	}
	return fmt.Errorf("absorbed-into target %q is not verified in overlay registry or project lock", target)
}

// PlanProjectRetire plans removal of one agent-owned project-tier skill. It
// proves ownership from the lock hash and target directory shape before
// capturing deletion backups, removes only the selected lock entry, and keeps
// the empty lock file in the plan so a later git revert can restore it
// symmetrically. The plan is pure: all filesystem facts arrive through the
// injected probes.
func PlanProjectRetire(in RetireInput) (ProjectPlan, error) {
	if err := requireRetireProbes(in); err != nil {
		return ProjectPlan{}, err
	}
	root, err := requireProjectRoot(projectRetireVerb, in.ProjectRoot, in.Stat)
	if err != nil {
		return ProjectPlan{}, err
	}
	if in.ID == "" {
		return ProjectPlan{}, fmt.Errorf("%s: skill id must not be empty", projectRetireVerb)
	}
	lock, entryIndex, entry, err := requireRecordedSkill(projectRetireVerb, in.LockExists, in.LockData, in.ID)
	if err != nil {
		return ProjectPlan{}, err
	}
	probes := ownershipProbes{readFile: in.ReadFile, readDir: in.ReadDir, resolvePath: in.ResolvePath}
	if err := requireAgentOwned(projectRetireVerb, root, in.ID, entry, probes); err != nil {
		return ProjectPlan{}, err
	}
	if err := VerifyAbsorbedInto(in.AbsorbedInto, in.Registry, lock); err != nil {
		return ProjectPlan{}, fmt.Errorf("%s: %v", projectRetireVerb, err)
	}

	guard := writeGuard{root: root, resolve: in.ResolvePath}
	deleteWrites, seen, err := planRetireDeletions(guard, in, entry)
	if err != nil {
		return ProjectPlan{}, err
	}
	lockWrite, err := planRetireLock(guard, in, lock, entryIndex, seen)
	if err != nil {
		return ProjectPlan{}, err
	}

	return ProjectPlan{
		ID:           in.ID,
		Revision:     entry.Revision,
		AbsorbedInto: in.AbsorbedInto,
		Deletes:      relsOf(deleteWrites),
		DeleteWrites: deleteWrites,
		Lock:         lockWrite,
	}, nil
}

// requireRetireProbes refuses a planner that was given nothing to look at the file system with.
func requireRetireProbes(in RetireInput) error {
	if in.ReadFile == nil || in.ReadDir == nil || in.Stat == nil {
		return fmt.Errorf("%s: read, directory, and stat probes are required", projectRetireVerb)
	}
	if in.ResolvePath == nil {
		return fmt.Errorf("%s: no symlink resolver was injected", projectRetireVerb)
	}
	return nil
}

// planRetireDeletions plans the removal of every target the lock records for the skill, each with
// the bytes it holds now as its backup and the mode it has now. A target that is a directory is
// refused. It also returns, per RESOLVED destination, the target that reached it, so the lock
// cannot alias a skill file.
func planRetireDeletions(guard writeGuard, in RetireInput, entry ProjectLockEntry) ([]ProjectWrite, map[string]string, error) {
	seen := make(map[string]string, len(entry.Targets)+1)
	deleteWrites := make([]ProjectWrite, 0, len(entry.Targets))
	for _, target := range entry.Targets {
		abs, resolved, err := guard.destination(target)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: target %q: %v", projectRetireVerb, target, err)
		}
		if other, ok := seen[resolved]; ok {
			return nil, nil, fmt.Errorf("%s: targets %q and %q resolve to the same file", projectRetireVerb, other, target)
		}
		seen[resolved] = target

		data, err := in.ReadFile(abs)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: reading target %q: %v", projectRetireVerb, target, err)
		}
		info, err := in.Stat(abs)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: inspecting target %q: %v", projectRetireVerb, target, err)
		}
		if info.IsDir() {
			return nil, nil, fmt.Errorf("%s: target %q is a directory", projectRetireVerb, target)
		}
		deleteWrites = append(deleteWrites, ProjectWrite{
			Rel:    filepath.ToSlash(target),
			Abs:    abs,
			Mode:   info.Mode().Perm(),
			Backup: cloneProjectBytes(data),
		})
	}
	return deleteWrites, seen, nil
}

// planRetireLock is the lock without the retired entry. It passes the same guard as any
// destination, may not alias a skill file, and must exist: it stays in the plan, empty if the
// skill was the last, so a later git revert can restore it symmetrically.
func planRetireLock(guard writeGuard, in RetireInput, lock ProjectLock, entryIndex int, seen map[string]string) (ProjectWrite, error) {
	lockAbs, resolvedLock, err := guard.destination(ProjectLockRelPath)
	if err != nil {
		return ProjectWrite{}, fmt.Errorf("%s: lock destination: %v", projectRetireVerb, err)
	}
	if other, ok := seen[resolvedLock]; ok {
		return ProjectWrite{}, fmt.Errorf("%s: targets %q and %q resolve to the same file", projectRetireVerb, other, ProjectLockRelPath)
	}
	lockInfo, err := in.Stat(lockAbs)
	if err != nil {
		return ProjectWrite{}, fmt.Errorf("%s: inspecting project lock: %v", projectRetireVerb, err)
	}

	remaining := make([]ProjectLockEntry, 0, len(lock.Skills)-1)
	remaining = append(remaining, lock.Skills[:entryIndex]...)
	remaining = append(remaining, lock.Skills[entryIndex+1:]...)
	lock.Skills = remaining
	lockData, err := SerializeProjectLock(lock)
	if err != nil {
		return ProjectWrite{}, fmt.Errorf("%s: %v", projectRetireVerb, err)
	}
	return ProjectWrite{
		Rel:    ProjectLockRelPath,
		Abs:    lockAbs,
		Data:   lockData,
		Mode:   lockInfo.Mode().Perm(),
		Backup: cloneProjectBytes(in.LockData),
	}, nil
}
