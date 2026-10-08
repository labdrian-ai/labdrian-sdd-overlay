package skills

import (
	"fmt"
	"io/fs"
)

// PlanProjectRegister validates a registration completely and returns the
// full set of writes it implies. It is PURE: every filesystem fact reaches it
// through RegisterInput, and it mutates nothing. Any refusal returns the zero
// ProjectPlan, so a partial plan can never escape.
//
// The checks run in design.md's documented validate-before-write order (items
// 1-11), with the Identity rules folded in where the frontmatter first
// becomes available:
//
//  1. ProjectRoot is absolute, exists, and is a directory (no cwd fallback,
//     the R-002 precedent from RenderValidateCore).
//  2. The draft lies OUTSIDE the project root, so it can never be committed
//     or used as a target.
//  3. The draft contains no "\r".
//  4. The frontmatter holds only allowlisted top-level keys.
//  5. Provenance is stamped, then LintSkillFile runs on the STAMPED bytes
//     with zero hard errors. The order is stamp, lint, hash, write.
//     Identity: the id is the frontmatter name; it must be normalized, match
//     slugRe, and must not match the overlay registry.
//  6. The candidate key validates (shape).
//  7. The lock parses and has version 1.
//  8. No lock entry already has this id.
//  9. No target directory <root>/<dir>/<id> exists; one that does, with no
//     lock entry claiming it, is a "foreign skill" refusal.
//  10. Every destination passes pathguard.WithinRoot AND pathguard.ResolvedWithinRootUsing.
//  11. No destination lies under <root>/skills/.
//
// Step 10 also covers every target string already recorded in the lock
// (tasks.md 3b-i.5a): those strings are consumed here as file-WRITE paths, so
// a poisoned lock must never be able to steer a write outside the project,
// lexically or through a symlink.
//
// The containment proof here is point-in-time. ExecuteProjectPlan (3b-ii) is
// check-then-act and must re-establish it at creation time, because a
// component can become a symlink between this plan and that write.
func PlanProjectRegister(in RegisterInput) (ProjectPlan, error) {
	if err := requireRegisterProbes(in); err != nil {
		return ProjectPlan{}, err
	}
	root, err := requireProjectRoot(projectRegisterVerb, in.ProjectRoot, in.Stat)
	if err != nil {
		return ProjectPlan{}, err
	}
	if err := requireDraftOutsideRoot(projectRegisterVerb, in.DraftPath, root, in.ResolvePath); err != nil {
		return ProjectPlan{}, err
	}
	if err := requireNoCarriageReturn(projectRegisterVerb, in.DraftPath, in.DraftData); err != nil {
		return ProjectPlan{}, err
	}
	frontmatter, err := readDraftFrontmatter(projectRegisterVerb, in.DraftData)
	if err != nil {
		return ProjectPlan{}, err
	}

	// 5. Stamp, then lint the STAMPED bytes, then hash them. Registration and
	// revision both use this exact helper so neither path can drift in the
	// ordering that guards what reaches disk.
	stamped, sum, err := prepareProjectSkill(in.DraftData, in.CandidateKey)
	if err != nil {
		return ProjectPlan{}, fmt.Errorf("%s: %v", projectRegisterVerb, err)
	}

	// Identity: the id is the frontmatter name.
	id := parseFrontmatterFields(frontmatter).Name
	if err := checkRegisterIdentity(id, in.Registry); err != nil {
		return ProjectPlan{}, err
	}

	// 6. Candidate key shape.
	if err := ValidateCandidateKey(in.CandidateKey); err != nil {
		return ProjectPlan{}, fmt.Errorf("%s: %v", projectRegisterVerb, err)
	}

	lock, err := readRegisterLock(in, id)
	if err != nil {
		return ProjectPlan{}, err
	}
	guard := writeGuard{root: root, resolve: in.ResolvePath}
	if err := checkRecordedTargets(guard, lock); err != nil {
		return ProjectPlan{}, err
	}

	writes, aliased, err := planRegisterTargets(guard, in.Stat, id, stamped)
	if err != nil {
		return ProjectPlan{}, err
	}
	lockWrite, err := planRegisterLock(guard, in, lock, aliased, ProjectLockEntry{
		ID:         id,
		Provenance: "procedural",
		Candidate:  in.CandidateKey,
		SHA256:     sum,
		Revision:   1,
		Targets:    relsOf(writes),
	})
	if err != nil {
		return ProjectPlan{}, err
	}

	return ProjectPlan{
		ID:       id,
		SHA256:   sum,
		Revision: 1,
		Writes:   writes,
		Lock:     lockWrite,
	}, nil
}

// requireRegisterProbes refuses a planner that was given nothing to look at the file system with.
// A nil ResolvePath is a fail-closed refusal: ownership and the write path never degrade to the
// lexical check alone.
func requireRegisterProbes(in RegisterInput) error {
	if in.Stat == nil {
		return fmt.Errorf("%s: no stat probe was injected", projectRegisterVerb)
	}
	if in.ResolvePath == nil {
		return fmt.Errorf("%s: no symlink resolver was injected", projectRegisterVerb)
	}
	return nil
}

// readRegisterLock is steps 7 and 8: the lock parses and has version 1 (a project with no lock
// yet has an empty one), and no lock entry already has this id.
func readRegisterLock(in RegisterInput, id string) (ProjectLock, error) {
	lock := ProjectLock{Version: 1}
	if in.LockExists {
		parsed, err := ParseProjectLock(in.LockData)
		if err != nil {
			return ProjectLock{}, fmt.Errorf("%s: %v", projectRegisterVerb, err)
		}
		lock = parsed
	}
	for _, e := range lock.Skills {
		if e.ID == id {
			return ProjectLock{}, fmt.Errorf("%s: id %q is already registered in the lock", projectRegisterVerb, id)
		}
	}
	return lock, nil
}

// checkRecordedTargets is 3b-i.5a: every target string the lock already records is consumed by a
// registration as a file-WRITE path (the lock is rewritten carrying them), so each one passes the
// same shared guard as a fresh destination.
func checkRecordedTargets(guard writeGuard, lock ProjectLock) error {
	for _, e := range lock.Skills {
		for _, target := range e.Targets {
			_, _, err := guard.destination(target)
			switch err {
			case nil:
			case errDestUnderSkillsDir:
				// Such a target is plainly INSIDE the root; calling it
				// "outside the project root" contradicts itself (review
				// round 3, PLAN-3).
				return fmt.Errorf("%s: lock entry %q records target %q under the project's own skills/ directory, which is never a registration destination", projectRegisterVerb, e.ID, target)
			case errDestResolvesUnderSkillsDir:
				return fmt.Errorf("%s: lock entry %q records target %q, which resolves into the project's own skills/ tree and is never a registration destination", projectRegisterVerb, e.ID, target)
			default:
				return fmt.Errorf("%s: lock entry %q records target %q outside the project root: %v", projectRegisterVerb, e.ID, target, err)
			}
		}
	}
	return nil
}

// planRegisterTargets is steps 9 to 11: it builds each destination of the skill, refusing a
// foreign directory and proving containment before anything is planned. It also returns, per
// RESOLVED destination, the first planned path that reached it: the two fixed targets are distinct
// strings, but `.agents` symlinked at `.claude` makes them one file on disk (ALIAS-1, review round
// 2). The lock would then record two targets for a single file, and rollback could no longer
// promise a byte-identical tree, because the second write silently overwrites the first and only
// one of the two removals can succeed. Refusing is the only outcome that keeps both properties
// unconditional.
func planRegisterTargets(guard writeGuard, stat func(string) (fs.FileInfo, error), id string, stamped []byte) ([]ProjectWrite, map[string]string, error) {
	writes := make([]ProjectWrite, 0, len(projectTargets))
	aliased := make(map[string]string, len(projectTargets)+1)
	for _, target := range projectTargets {
		dirRel := target.Dir + "/" + id
		rel := dirRel + "/" + projectSkillFileName

		dirAbs, _, err := guard.destination(dirRel)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: destination %q: %v", projectRegisterVerb, dirRel, err)
		}
		if _, err := stat(dirAbs); err == nil {
			// The id is not in the lock (step 8 proved that), so whatever
			// sits here belongs to someone else.
			return nil, nil, fmt.Errorf("%s: foreign skill: %q already exists and %q is not in the lock", projectRegisterVerb, dirRel, id)
		} else if !isAbsent(err) {
			return nil, nil, fmt.Errorf("%s: inspecting destination %q: %v", projectRegisterVerb, dirRel, err)
		}

		abs, resolved, err := guard.destination(rel)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: destination %q: %v", projectRegisterVerb, rel, err)
		}
		if other, ok := aliased[resolved]; ok {
			return nil, nil, fmt.Errorf("%s: destinations %q and %q resolve to the same file, so one would silently overwrite the other", projectRegisterVerb, other, rel)
		}
		aliased[resolved] = rel

		writes = append(writes, ProjectWrite{Rel: rel, Abs: abs, Data: stamped, Mode: ProjectFileMode})
	}
	return writes, aliased, nil
}

// planRegisterLock is the lock as a destination like any other: it passes the same guard, may not
// alias a skill file, and is rewritten with the new entry. It is the commit marker, so a plan
// always carries it.
func planRegisterLock(guard writeGuard, in RegisterInput, lock ProjectLock, aliased map[string]string, entry ProjectLockEntry) (ProjectWrite, error) {
	lockAbs, resolvedLock, err := guard.destination(ProjectLockRelPath)
	if err != nil {
		return ProjectWrite{}, fmt.Errorf("%s: destination %q: %v", projectRegisterVerb, ProjectLockRelPath, err)
	}
	if other, ok := aliased[resolvedLock]; ok {
		return ProjectWrite{}, fmt.Errorf("%s: destinations %q and %q resolve to the same file, so one would silently overwrite the other", projectRegisterVerb, other, ProjectLockRelPath)
	}

	lock.Version = 1
	lock.Skills = append(lock.Skills, entry)
	lockData, err := SerializeProjectLock(lock)
	if err != nil {
		return ProjectWrite{}, fmt.Errorf("%s: %v", projectRegisterVerb, err)
	}

	lockWrite := ProjectWrite{Rel: ProjectLockRelPath, Abs: lockAbs, Data: lockData, Mode: ProjectFileMode}
	if in.LockExists {
		lockWrite.Backup = in.LockData
	}
	return lockWrite, nil
}

// relsOf lists the repo-relative paths of the writes, in order.
func relsOf(writes []ProjectWrite) []string {
	rels := make([]string, 0, len(writes))
	for _, w := range writes {
		rels = append(rels, w.Rel)
	}
	return rels
}
