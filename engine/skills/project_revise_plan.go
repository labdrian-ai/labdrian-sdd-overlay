package skills

import (
	"fmt"
	"path/filepath"
)

// PlanProjectRevise plans an in-place project-tier revision. It first finds the
// skill id in the project lock and runs EvaluateOwnership against the current
// target bytes and directory entries. A mismatch is a hard human-ownership
// refusal: no new bytes are staged, no lock entry is changed, and the reason
// from EvaluateOwnership is preserved for the CLI/status surface.
//
// Once ownership is proved, the revision follows the same preparation seam as
// registration — stamp -> lint -> hash — and captures every existing target
// and the lock as backups. ExecuteProjectPlan then supplies the same staged
// temp-file, ordered rename, and rollback behavior used by registration.
func PlanProjectRevise(in ReviseInput) (ProjectPlan, error) {
	if err := requireReviseProbes(in); err != nil {
		return ProjectPlan{}, err
	}
	root, err := requireProjectRoot(projectReviseVerb, in.ProjectRoot, in.Stat)
	if err != nil {
		return ProjectPlan{}, err
	}
	if err := requireDraftOutsideRoot(projectReviseVerb, in.DraftPath, root, in.ResolvePath); err != nil {
		return ProjectPlan{}, err
	}
	if err := requireNoCarriageReturn(projectReviseVerb, in.DraftPath, in.DraftData); err != nil {
		return ProjectPlan{}, err
	}
	frontmatter, err := readDraftFrontmatter(projectReviseVerb, in.DraftData)
	if err != nil {
		return ProjectPlan{}, err
	}
	id := parseFrontmatterFields(frontmatter).Name
	if err := checkSkillID(projectReviseVerb, id); err != nil {
		return ProjectPlan{}, err
	}
	if err := ValidateCandidateKey(in.CandidateKey); err != nil {
		return ProjectPlan{}, fmt.Errorf("%s: %v", projectReviseVerb, err)
	}
	lock, entryIndex, entry, err := requireRecordedSkill(projectReviseVerb, in.LockExists, in.LockData, id)
	if err != nil {
		return ProjectPlan{}, err
	}
	probes := ownershipProbes{readFile: in.ReadFile, readDir: in.ReadDir, resolvePath: in.ResolvePath}
	if err := requireAgentOwned(projectReviseVerb, root, id, entry, probes); err != nil {
		return ProjectPlan{}, err
	}

	// This is deliberately the same preparation call used by registration:
	// provenance stamp, hard lint on the stamped bytes, then hash.
	stamped, sum, err := prepareProjectSkill(in.DraftData, in.CandidateKey)
	if err != nil {
		return ProjectPlan{}, fmt.Errorf("%s: %v", projectReviseVerb, err)
	}

	guard := writeGuard{root: root, resolve: in.ResolvePath}
	writes, seen, err := planReviseTargets(guard, in.ReadFile, entry, stamped)
	if err != nil {
		return ProjectPlan{}, err
	}
	revised := entry
	revised.Revision++
	revised.SHA256 = sum
	revised.Targets = append([]string(nil), entry.Targets...)
	lockWrite, err := planReviseLock(guard, seen, in.LockData, lock, entryIndex, revised)
	if err != nil {
		return ProjectPlan{}, err
	}

	return ProjectPlan{
		ID:       id,
		SHA256:   sum,
		Revision: revised.Revision,
		Writes:   writes,
		Lock:     lockWrite,
	}, nil
}

// requireReviseProbes refuses a planner that was given nothing to look at the file system with.
func requireReviseProbes(in ReviseInput) error {
	if in.ReadFile == nil || in.ReadDir == nil || in.Stat == nil {
		return fmt.Errorf("%s: read, directory, and stat probes are required", projectReviseVerb)
	}
	if in.ResolvePath == nil {
		return fmt.Errorf("%s: no symlink resolver was injected", projectReviseVerb)
	}
	return nil
}

// planReviseTargets plans the new bytes of every target the lock records for the skill, each with
// the bytes it holds now as its backup. It also returns, per RESOLVED destination, the target that
// reached it, so the lock cannot alias a skill file.
func planReviseTargets(guard writeGuard, readFile func(string) ([]byte, error), entry ProjectLockEntry, stamped []byte) ([]ProjectWrite, map[string]string, error) {
	seen := make(map[string]string, len(entry.Targets)+1)
	writes := make([]ProjectWrite, 0, len(entry.Targets))
	for _, target := range entry.Targets {
		abs, resolved, err := guard.destination(target)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: destination %q: %v", projectReviseVerb, target, err)
		}
		if other, ok := seen[resolved]; ok {
			return nil, nil, fmt.Errorf("%s: destinations %q and %q resolve to the same file, so one would silently overwrite the other", projectReviseVerb, other, target)
		}
		seen[resolved] = target
		backup, err := readFile(abs)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: reading existing target %q: %v", projectReviseVerb, target, err)
		}
		writes = append(writes, ProjectWrite{
			Rel:    filepath.ToSlash(target),
			Abs:    abs,
			Data:   stamped,
			Mode:   ProjectFileMode,
			Backup: cloneProjectBytes(backup),
		})
	}
	return writes, seen, nil
}

// planReviseLock is the lock with the entry at entryIndex replaced by its revision. It passes the
// same guard as any destination and may not alias a skill file.
func planReviseLock(guard writeGuard, seen map[string]string, lockData []byte, lock ProjectLock, entryIndex int, revised ProjectLockEntry) (ProjectWrite, error) {
	lockAbs, resolvedLock, err := guard.destination(ProjectLockRelPath)
	if err != nil {
		return ProjectWrite{}, fmt.Errorf("%s: destination %q: %v", projectReviseVerb, ProjectLockRelPath, err)
	}
	if other, ok := seen[resolvedLock]; ok {
		return ProjectWrite{}, fmt.Errorf("%s: destinations %q and %q resolve to the same file, so one would silently overwrite the other", projectReviseVerb, other, ProjectLockRelPath)
	}

	lock.Skills[entryIndex] = revised
	revisedData, err := SerializeProjectLock(lock)
	if err != nil {
		return ProjectWrite{}, fmt.Errorf("%s: %v", projectReviseVerb, err)
	}
	return ProjectWrite{
		Rel:    ProjectLockRelPath,
		Abs:    lockAbs,
		Data:   revisedData,
		Mode:   ProjectFileMode,
		Backup: cloneProjectBytes(lockData),
	}, nil
}
