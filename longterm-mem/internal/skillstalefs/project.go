// Package skillstalefs is the adapter that answers the questions skillstale asks of the world, on a file system
// and a git repository: the project lock and the skill files it points at (skillstale.SkillSource), what the
// repository says of the paths a skill names (skillstale.PathFactSource), and the commands a PATH finds
// (skillstale.CommandResolver). It only reads. Only the composition root names it.
package skillstalefs

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/skillstale"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/staleness"
)

// Project is a project on disk: the directory its lock and its skill files live under, and the evidence of
// its repository.
type Project struct {
	root     string
	evidence staleness.RepoEvidence
}

var (
	_ skillstale.SkillSource    = Project{}
	_ skillstale.PathFactSource = Project{}
)

// OpenProject names the project at root, which must be an absolute path: nothing here resolves a relative one
// against the working directory. What its repository says of a path is asked of evidence, which the caller
// chooses (the composition root hands it the adapter over the project's own tree and git).
func OpenProject(root string, evidence staleness.RepoEvidence) (Project, error) {
	if !filepath.IsAbs(root) {
		return Project{}, fmt.Errorf("skillstale: project root must be absolute: %q", root)
	}
	if evidence == nil {
		return Project{}, fmt.Errorf("skillstale: repository evidence is required")
	}
	return Project{root: root, evidence: evidence}, nil
}

// ReadLock reads the project lock. The error is the file system's own, naming the path it tried.
func (p Project) ReadLock() ([]byte, error) {
	return os.ReadFile(filepath.Join(p.root, skillstale.ProjectLockRelPath))
}

// ReadSkill reads the skill file at target, a path relative to the project. It refuses a target that is empty,
// absolute or climbs out with "..", one that resolves (through links) to a file outside the project, and one that
// is not there.
func (p Project) ReadSkill(target string) ([]byte, error) {
	abs, err := safeTarget(p.root, target)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(abs)
}

// PathFacts asks the repository of the project what became of each path: in the working tree, deleted by a
// commit, renamed by one, or never there.
func (p Project) PathFacts(paths []string) (map[string]skillstale.PathFact, error) {
	facts, err := staleness.ClassifyPaths(p.evidence, paths)
	if err != nil {
		return nil, err
	}
	out := make(map[string]skillstale.PathFact, len(facts))
	for asked, fact := range facts {
		out[asked] = skillstale.PathFact{State: stateOf(fact.State), NewPath: fact.NewPath, Commit: fact.Commit}
	}
	return out, nil
}

// stateOf is the detector's word for what the repository history says. A state it has no word for is the one the
// detector reports nothing for.
func stateOf(state staleness.PathState) skillstale.PathState {
	switch state {
	case staleness.PathPresent:
		return skillstale.PathPresent
	case staleness.PathDeleted:
		return skillstale.PathDeleted
	case staleness.PathRenamed:
		return skillstale.PathRenamed
	default:
		return skillstale.PathUnknown
	}
}

// safeTarget is the absolute path of the skill file a lock records at recorded, or the reason that file is not
// to be read. It checks twice, because a name can be inside the project and its file still not: first the text
// alone (not empty, not absolute, no ".." segment, and what it joins to stays strictly below the root), which
// needs no file and refuses a target before the disk is asked about it; then the file system's answer, with every
// link followed on the root and on the target, which must still be strictly below the resolved root, and which
// fails with the file system's own error when there is no file. The path returned is the lexical one, not the
// resolved one.
func safeTarget(root, recorded string) (string, error) {
	if recorded == "" || path.IsAbs(recorded) || filepath.IsAbs(filepath.FromSlash(recorded)) {
		return "", fmt.Errorf("invalid target %q", recorded)
	}
	for _, segment := range strings.Split(filepath.ToSlash(recorded), "/") {
		if segment == ".." {
			return "", fmt.Errorf("invalid target %q", recorded)
		}
	}
	cleanRoot := filepath.Clean(root)
	candidate := filepath.Clean(filepath.Join(cleanRoot, filepath.FromSlash(recorded)))
	inside, err := strictlyWithin(cleanRoot, candidate)
	if err != nil {
		return "", err
	}
	if !inside {
		return "", fmt.Errorf("target %q escapes project root", recorded)
	}
	resolvedRoot, err := filepath.EvalSymlinks(cleanRoot)
	if err != nil {
		return "", fmt.Errorf("resolve project root: %w", err)
	}
	resolvedTarget, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve target %q: %w", candidate, err)
	}
	inside, err = strictlyWithin(filepath.Clean(resolvedRoot), filepath.Clean(resolvedTarget))
	if err != nil {
		return "", err
	}
	if !inside {
		return "", fmt.Errorf("target %q resolves outside project root", recorded)
	}
	return candidate, nil
}

// strictlyWithin says whether candidate is below root and is not root itself.
func strictlyWithin(root, candidate string) (bool, error) {
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return false, fmt.Errorf("compare target containment: %w", err)
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel), nil
}
