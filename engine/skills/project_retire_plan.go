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
	if in.ReadFile == nil || in.ReadDir == nil || in.Stat == nil {
		return ProjectPlan{}, fmt.Errorf("project-retire: read, directory, and stat probes are required")
	}
	if in.ResolvePath == nil {
		return ProjectPlan{}, fmt.Errorf("project-retire: no symlink resolver was injected")
	}
	if !filepath.IsAbs(in.ProjectRoot) {
		return ProjectPlan{}, fmt.Errorf("project-retire: --project-root %q must be absolute", in.ProjectRoot)
	}
	root := filepath.Clean(in.ProjectRoot)
	info, err := in.Stat(root)
	if err != nil {
		return ProjectPlan{}, fmt.Errorf("project-retire: --project-root %q must exist: %v", in.ProjectRoot, err)
	}
	if !info.IsDir() {
		return ProjectPlan{}, fmt.Errorf("project-retire: --project-root %q must be a directory", in.ProjectRoot)
	}
	if in.ID == "" {
		return ProjectPlan{}, fmt.Errorf("project-retire: skill id must not be empty")
	}
	if !in.LockExists {
		return ProjectPlan{}, fmt.Errorf("project-retire: skill %q is human-owned (not-in-lock)", in.ID)
	}

	lock, err := ParseProjectLock(in.LockData)
	if err != nil {
		return ProjectPlan{}, fmt.Errorf("project-retire: %v", err)
	}
	entryIndex := -1
	var entry ProjectLockEntry
	for i, candidate := range lock.Skills {
		if candidate.ID == in.ID {
			entryIndex = i
			entry = candidate
			break
		}
	}
	if entryIndex < 0 {
		return ProjectPlan{}, fmt.Errorf("project-retire: skill %q is human-owned (not-in-lock)", in.ID)
	}

	ownership := EvaluateOwnership(root, entry, in.ReadFile, in.ReadDir, in.ResolvePath)
	if !ownership.AgentOwned {
		return ProjectPlan{}, fmt.Errorf("project-retire: skill %q is human-owned (%s)", in.ID, ownership.Reason)
	}
	if err := VerifyAbsorbedInto(in.AbsorbedInto, in.Registry, lock); err != nil {
		return ProjectPlan{}, fmt.Errorf("project-retire: %v", err)
	}

	registerInput := RegisterInput{ProjectRoot: root, ResolvePath: in.ResolvePath}
	seen := make(map[string]string, len(entry.Targets)+1)
	deleteWrites := make([]ProjectWrite, 0, len(entry.Targets))
	deletes := make([]string, 0, len(entry.Targets))
	for _, target := range entry.Targets {
		abs, resolved, err := resolveWritePath(registerInput, root, target)
		if err != nil {
			return ProjectPlan{}, fmt.Errorf("project-retire: target %q: %v", target, err)
		}
		if other, ok := seen[resolved]; ok {
			return ProjectPlan{}, fmt.Errorf("project-retire: targets %q and %q resolve to the same file", other, target)
		}
		seen[resolved] = target

		data, err := in.ReadFile(abs)
		if err != nil {
			return ProjectPlan{}, fmt.Errorf("project-retire: reading target %q: %v", target, err)
		}
		info, err := in.Stat(abs)
		if err != nil {
			return ProjectPlan{}, fmt.Errorf("project-retire: inspecting target %q: %v", target, err)
		}
		if info.IsDir() {
			return ProjectPlan{}, fmt.Errorf("project-retire: target %q is a directory", target)
		}
		rel := filepath.ToSlash(target)
		deletes = append(deletes, rel)
		deleteWrites = append(deleteWrites, ProjectWrite{
			Rel:    rel,
			Abs:    abs,
			Mode:   info.Mode().Perm(),
			Backup: cloneProjectBytes(data),
		})
	}

	lockAbs, resolvedLock, err := resolveWritePath(registerInput, root, ProjectLockRelPath)
	if err != nil {
		return ProjectPlan{}, fmt.Errorf("project-retire: lock destination: %v", err)
	}
	if other, ok := seen[resolvedLock]; ok {
		return ProjectPlan{}, fmt.Errorf("project-retire: targets %q and %q resolve to the same file", other, ProjectLockRelPath)
	}
	lockInfo, err := in.Stat(lockAbs)
	if err != nil {
		return ProjectPlan{}, fmt.Errorf("project-retire: inspecting project lock: %v", err)
	}

	remaining := make([]ProjectLockEntry, 0, len(lock.Skills)-1)
	remaining = append(remaining, lock.Skills[:entryIndex]...)
	remaining = append(remaining, lock.Skills[entryIndex+1:]...)
	lock.Skills = remaining
	lockData, err := SerializeProjectLock(lock)
	if err != nil {
		return ProjectPlan{}, fmt.Errorf("project-retire: %v", err)
	}

	return ProjectPlan{
		ID:           in.ID,
		Revision:     entry.Revision,
		AbsorbedInto: in.AbsorbedInto,
		Deletes:      deletes,
		DeleteWrites: deleteWrites,
		Lock: ProjectWrite{
			Rel:    ProjectLockRelPath,
			Abs:    lockAbs,
			Data:   lockData,
			Mode:   lockInfo.Mode().Perm(),
			Backup: cloneProjectBytes(in.LockData),
		},
	}, nil
}
