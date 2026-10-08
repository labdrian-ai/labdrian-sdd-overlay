package skills

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/pathguard"
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
	if in.ReadFile == nil || in.ReadDir == nil || in.Stat == nil {
		return ProjectPlan{}, fmt.Errorf("project-revise: read, directory, and stat probes are required")
	}
	if in.ResolvePath == nil {
		return ProjectPlan{}, fmt.Errorf("project-revise: no symlink resolver was injected")
	}
	if !filepath.IsAbs(in.ProjectRoot) {
		return ProjectPlan{}, fmt.Errorf("project-revise: --project-root %q must be absolute", in.ProjectRoot)
	}
	root := filepath.Clean(in.ProjectRoot)
	info, err := in.Stat(root)
	if err != nil {
		return ProjectPlan{}, fmt.Errorf("project-revise: --project-root %q must exist: %v", in.ProjectRoot, err)
	}
	if !info.IsDir() {
		return ProjectPlan{}, fmt.Errorf("project-revise: --project-root %q must be a directory", in.ProjectRoot)
	}

	draft := filepath.Clean(in.DraftPath)
	if !filepath.IsAbs(draft) {
		return ProjectPlan{}, fmt.Errorf("project-revise: the <draft-file> argument %q must be an absolute path", in.DraftPath)
	}
	if pathguard.WithinRoot(root, draft) {
		return ProjectPlan{}, fmt.Errorf("project-revise: draft %q must lie outside the project root %q", in.DraftPath, root)
	}
	if inside, err := pathguard.ResolvedWithinRootUsing(in.ResolvePath, root, draft); err != nil {
		return ProjectPlan{}, fmt.Errorf("project-revise: resolving draft %q: %v", in.DraftPath, err)
	} else if inside {
		return ProjectPlan{}, fmt.Errorf("project-revise: draft %q resolves inside the project root %q and must lie outside it", in.DraftPath, root)
	}
	if strings.ContainsRune(string(in.DraftData), '\r') {
		return ProjectPlan{}, fmt.Errorf("project-revise: draft %q must not contain carriage returns", in.DraftPath)
	}

	frontmatter, _, err := SplitSkillFile(in.DraftData)
	if err != nil {
		return ProjectPlan{}, fmt.Errorf("project-revise: draft frontmatter: %v", err)
	}
	if err := checkFrontmatterAllowlist(frontmatter); err != nil {
		return ProjectPlan{}, fmt.Errorf("%s", strings.Replace(err.Error(), "project-register:", "project-revise:", 1))
	}
	id := parseFrontmatterFields(frontmatter).Name
	if id == "" {
		return ProjectPlan{}, fmt.Errorf("project-revise: the draft frontmatter declares no name, so the skill id is empty")
	}
	if !slugRe.MatchString(id) {
		return ProjectPlan{}, fmt.Errorf("project-revise: id %q does not match the skill identifier pattern", id)
	}
	if got := NormalizeSlug(id); got != id {
		return ProjectPlan{}, fmt.Errorf("project-revise: id %q is not normalized, want %q", id, got)
	}
	if err := ValidateCandidateKey(in.CandidateKey); err != nil {
		return ProjectPlan{}, fmt.Errorf("project-revise: %v", err)
	}
	if !in.LockExists {
		return ProjectPlan{}, fmt.Errorf("project-revise: skill %q is human-owned (not-in-lock)", id)
	}
	lock, err := ParseProjectLock(in.LockData)
	if err != nil {
		return ProjectPlan{}, fmt.Errorf("project-revise: %v", err)
	}

	entryIndex := -1
	var entry ProjectLockEntry
	for i, candidate := range lock.Skills {
		if candidate.ID == id {
			entryIndex = i
			entry = candidate
			break
		}
	}
	if entryIndex < 0 {
		return ProjectPlan{}, fmt.Errorf("project-revise: skill %q is human-owned (not-in-lock)", id)
	}

	ownership := EvaluateOwnership(root, entry, in.ReadFile, in.ReadDir, in.ResolvePath)
	if !ownership.AgentOwned {
		return ProjectPlan{}, fmt.Errorf("project-revise: skill %q is human-owned (%s)", id, ownership.Reason)
	}

	// This is deliberately the same preparation call used by registration:
	// provenance stamp, hard lint on the stamped bytes, then hash.
	stamped, sum, err := prepareProjectSkill(in.DraftData, in.CandidateKey)
	if err != nil {
		return ProjectPlan{}, fmt.Errorf("project-revise: %v", err)
	}

	registerInput := RegisterInput{ProjectRoot: root, ResolvePath: in.ResolvePath}
	seen := make(map[string]string, len(entry.Targets)+1)
	writes := make([]ProjectWrite, 0, len(entry.Targets))
	for _, target := range entry.Targets {
		abs, resolved, err := resolveWritePath(registerInput, root, target)
		if err != nil {
			return ProjectPlan{}, fmt.Errorf("project-revise: destination %q: %v", target, err)
		}
		if other, ok := seen[resolved]; ok {
			return ProjectPlan{}, fmt.Errorf("project-revise: destinations %q and %q resolve to the same file, so one would silently overwrite the other", other, target)
		}
		seen[resolved] = target
		backup, err := in.ReadFile(abs)
		if err != nil {
			return ProjectPlan{}, fmt.Errorf("project-revise: reading existing target %q: %v", target, err)
		}
		writes = append(writes, ProjectWrite{
			Rel:    filepath.ToSlash(target),
			Abs:    abs,
			Data:   stamped,
			Mode:   ProjectFileMode,
			Backup: cloneProjectBytes(backup),
		})
	}

	lockAbs, resolvedLock, err := resolveWritePath(registerInput, root, ProjectLockRelPath)
	if err != nil {
		return ProjectPlan{}, fmt.Errorf("project-revise: destination %q: %v", ProjectLockRelPath, err)
	}
	if other, ok := seen[resolvedLock]; ok {
		return ProjectPlan{}, fmt.Errorf("project-revise: destinations %q and %q resolve to the same file, so one would silently overwrite the other", other, ProjectLockRelPath)
	}

	entry.Revision++
	entry.SHA256 = sum
	entry.Targets = append([]string(nil), entry.Targets...)
	lock.Skills[entryIndex] = entry
	lockData, err := SerializeProjectLock(lock)
	if err != nil {
		return ProjectPlan{}, fmt.Errorf("project-revise: %v", err)
	}

	return ProjectPlan{
		ID:       id,
		SHA256:   sum,
		Revision: entry.Revision,
		Writes:   writes,
		Lock: ProjectWrite{
			Rel:    ProjectLockRelPath,
			Abs:    lockAbs,
			Data:   lockData,
			Mode:   ProjectFileMode,
			Backup: cloneProjectBytes(in.LockData),
		},
	}, nil
}
