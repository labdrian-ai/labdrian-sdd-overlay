package skills

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/pathguard"
)

// The steps the planners of the project verbs share. Each takes the verb it speaks for, because a
// refusal is worded for the command that was run; apart from that word the sentence is the same
// for every verb. The order the steps run in is each planner's own and is part of what it
// promises: the first refusal a person is told is the first check that fails.

const (
	projectReviseVerb = "project-revise"
	projectRetireVerb = "project-retire"
)

// requireProjectRoot is the first check of every planner (the R-002 precedent from
// RenderValidateCore): the project root is absolute, exists, and is a directory, with no cwd
// fallback. It returns the cleaned root.
func requireProjectRoot(verb, projectRoot string, stat func(string) (fs.FileInfo, error)) (string, error) {
	if !filepath.IsAbs(projectRoot) {
		return "", fmt.Errorf("%s: --project-root %q must be absolute", verb, projectRoot)
	}
	root := filepath.Clean(projectRoot)
	info, err := stat(root)
	if err != nil {
		return "", fmt.Errorf("%s: --project-root %q must exist: %v", verb, projectRoot, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s: --project-root %q must be a directory", verb, projectRoot)
	}
	return root, nil
}

// requireDraftOutsideRoot refuses a draft that lies inside the project root, so it can never be
// committed or used as a target — lexically and after symlink resolution, so a draft reached
// through a link into the project is refused too.
//
// There is deliberately no separate empty-draft branch here: filepath.Clean turns "" into ".",
// and neither "" nor "." is absolute, so the refusal below already owns both. The branch that
// used to sit here became unreachable the moment that absolute-path refusal was added, and an
// unreachable guard nothing can witness is worse than none (review round 2, COV-5).
//
// A RELATIVE draft path defeats both halves of the guard that follows: filepath.Clean does not
// absolutize, so the lexical comparison against an absolute root is always false and the resolver
// hands back an equally relative path that compares false too — a draft sitting INSIDE the project
// root was accepted (review round 3, F2/PLAN-1/SPEC-1). The planner is pure and must never
// resolve against the process working directory, so a non-absolute draft is refused outright,
// exactly as --project-root is.
func requireDraftOutsideRoot(verb, draftPath, root string, resolve pathguard.Resolver) error {
	draft := filepath.Clean(draftPath)
	if !filepath.IsAbs(draft) {
		return fmt.Errorf("%s: the <draft-file> argument %q must be an absolute path", verb, draftPath)
	}
	if pathguard.WithinRoot(root, draft) {
		return fmt.Errorf("%s: draft %q must lie outside the project root %q", verb, draftPath, root)
	}
	inside, err := pathguard.ResolvedWithinRootUsing(resolve, root, draft)
	if err != nil {
		return fmt.Errorf("%s: resolving draft %q: %v", verb, draftPath, err)
	}
	if inside {
		return fmt.Errorf("%s: draft %q resolves inside the project root %q and must lie outside it", verb, draftPath, root)
	}
	return nil
}

// requireNoCarriageReturn refuses a draft that holds a "\r".
func requireNoCarriageReturn(verb, draftPath string, draft []byte) error {
	if strings.ContainsRune(string(draft), '\r') {
		return fmt.Errorf("%s: draft %q must not contain carriage returns", verb, draftPath)
	}
	return nil
}

// readDraftFrontmatter is the frontmatter of the draft, which must hold only allowlisted
// top-level keys (validate step 4).
func readDraftFrontmatter(verb string, draft []byte) (string, error) {
	frontmatter, _, err := SplitSkillFile(draft)
	if err != nil {
		return "", fmt.Errorf("%s: draft frontmatter: %v", verb, err)
	}
	if err := checkFrontmatterAllowlist(verb, frontmatter); err != nil {
		return "", err
	}
	return frontmatter, nil
}

// requireRecordedSkill finds the skill id in the project lock. A project with no lock, a skill the
// lock does not record, and a lock that does not parse are each a refusal; the first two say the
// skill is human-owned, which is all the program knows about a skill it never registered.
func requireRecordedSkill(verb string, lockExists bool, lockData []byte, id string) (ProjectLock, int, ProjectLockEntry, error) {
	notInLock := fmt.Errorf("%s: skill %q is human-owned (not-in-lock)", verb, id)
	if !lockExists {
		return ProjectLock{}, 0, ProjectLockEntry{}, notInLock
	}
	lock, err := ParseProjectLock(lockData)
	if err != nil {
		return ProjectLock{}, 0, ProjectLockEntry{}, fmt.Errorf("%s: %v", verb, err)
	}
	for i, entry := range lock.Skills {
		if entry.ID == id {
			return lock, i, entry, nil
		}
	}
	return ProjectLock{}, 0, ProjectLockEntry{}, notInLock
}

// ownershipProbes are the readers EvaluateOwnership proves a skill's ownership through.
type ownershipProbes struct {
	readFile    func(string) ([]byte, error)
	readDir     func(string) ([]fs.DirEntry, error)
	resolvePath pathguard.Resolver
}

// requireAgentOwned refuses a skill the agent no longer owns, with the reason EvaluateOwnership
// gives, which the CLI and status surfaces keep.
func requireAgentOwned(verb, root, id string, entry ProjectLockEntry, probes ownershipProbes) error {
	ownership := EvaluateOwnership(root, entry, probes.readFile, probes.readDir, probes.resolvePath)
	if !ownership.AgentOwned {
		return fmt.Errorf("%s: skill %q is human-owned (%s)", verb, id, ownership.Reason)
	}
	return nil
}
