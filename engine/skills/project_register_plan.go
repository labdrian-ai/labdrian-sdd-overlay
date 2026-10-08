package skills

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/pathguard"
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
	if in.Stat == nil {
		return ProjectPlan{}, fmt.Errorf("project-register: no stat probe was injected")
	}
	if in.ResolvePath == nil {
		// Fail closed: ownership and the write path never degrade to the
		// lexical check alone.
		return ProjectPlan{}, fmt.Errorf("project-register: no symlink resolver was injected")
	}

	// 1. Project root.
	if !filepath.IsAbs(in.ProjectRoot) {
		return ProjectPlan{}, fmt.Errorf("project-register: --project-root %q must be absolute", in.ProjectRoot)
	}
	root := filepath.Clean(in.ProjectRoot)
	info, err := in.Stat(root)
	if err != nil {
		return ProjectPlan{}, fmt.Errorf("project-register: --project-root %q must exist: %v", in.ProjectRoot, err)
	}
	if !info.IsDir() {
		return ProjectPlan{}, fmt.Errorf("project-register: --project-root %q must be a directory", in.ProjectRoot)
	}

	// 2. The draft lives outside the project root — lexically and after
	// symlink resolution, so a draft reached through a link into the project
	// is refused too.
	draft := filepath.Clean(in.DraftPath)
	// There is deliberately no separate empty-draft branch here: filepath.Clean
	// turns "" into ".", and neither "" nor "." is absolute, so the refusal
	// below already owns both. The branch that used to sit here became
	// unreachable the moment that absolute-path refusal was added, and an
	// unreachable guard nothing can witness is worse than none (review round 2,
	// COV-5).
	//
	// A RELATIVE draft path defeats both halves of the guard below:
	// filepath.Clean does not absolutize, so the lexical comparison against an
	// absolute root is always false and the resolver hands back an equally
	// relative path that compares false too — a draft sitting INSIDE the
	// project root was accepted (review round 3, F2/PLAN-1/SPEC-1). The
	// planner is pure and must never resolve against the process working
	// directory, so a non-absolute draft is refused outright, exactly as
	// --project-root is (step 1 above).
	if !filepath.IsAbs(draft) {
		return ProjectPlan{}, fmt.Errorf("project-register: the <draft-file> argument %q must be an absolute path", in.DraftPath)
	}
	if pathguard.WithinRoot(root, draft) {
		return ProjectPlan{}, fmt.Errorf("project-register: draft %q must lie outside the project root %q", in.DraftPath, root)
	}
	if inside, err := pathguard.ResolvedWithinRootUsing(in.ResolvePath, root, draft); err != nil {
		return ProjectPlan{}, fmt.Errorf("project-register: resolving draft %q: %v", in.DraftPath, err)
	} else if inside {
		return ProjectPlan{}, fmt.Errorf("project-register: draft %q resolves inside the project root %q and must lie outside it", in.DraftPath, root)
	}

	// 3. No carriage returns.
	if strings.ContainsRune(string(in.DraftData), '\r') {
		return ProjectPlan{}, fmt.Errorf("project-register: draft %q must not contain carriage returns", in.DraftPath)
	}

	// 4. Frontmatter allowlist.
	frontmatter, _, err := SplitSkillFile(in.DraftData)
	if err != nil {
		return ProjectPlan{}, fmt.Errorf("project-register: draft frontmatter: %v", err)
	}
	if err := checkFrontmatterAllowlist(frontmatter); err != nil {
		return ProjectPlan{}, err
	}

	// 5. Stamp, then lint the STAMPED bytes, then hash them. Registration and
	// revision both use this exact helper so neither path can drift in the
	// ordering that guards what reaches disk.
	stamped, sum, err := prepareProjectSkill(in.DraftData, in.CandidateKey)
	if err != nil {
		return ProjectPlan{}, fmt.Errorf("project-register: %v", err)
	}

	// Identity: the id is the frontmatter name.
	id := parseFrontmatterFields(frontmatter).Name
	if err := checkRegisterIdentity(id, in.Registry); err != nil {
		return ProjectPlan{}, err
	}

	// 6. Candidate key shape.
	if err := ValidateCandidateKey(in.CandidateKey); err != nil {
		return ProjectPlan{}, fmt.Errorf("project-register: %v", err)
	}

	// 7. The lock parses and has version 1.
	lock := ProjectLock{Version: 1}
	if in.LockExists {
		parsed, err := ParseProjectLock(in.LockData)
		if err != nil {
			return ProjectPlan{}, fmt.Errorf("project-register: %v", err)
		}
		lock = parsed
	}

	// 8. The id is not already registered.
	for _, e := range lock.Skills {
		if e.ID == id {
			return ProjectPlan{}, fmt.Errorf("project-register: id %q is already registered in the lock", id)
		}
	}

	// 3b-i.5a: every target string the lock already records is consumed here
	// as a file-WRITE path (the lock is rewritten carrying them), so each one
	// passes the same shared guard as a fresh destination.
	for _, e := range lock.Skills {
		for _, target := range e.Targets {
			if _, _, err := resolveWritePath(in, root, target); err != nil {
				if err == errDestUnderSkillsDir {
					// Such a target is plainly INSIDE the root; calling it
					// "outside the project root" contradicts itself (review
					// round 3, PLAN-3).
					return ProjectPlan{}, fmt.Errorf("project-register: lock entry %q records target %q under the project's own skills/ directory, which is never a registration destination", e.ID, target)
				}
				if err == errDestResolvesUnderSkillsDir {
					return ProjectPlan{}, fmt.Errorf("project-register: lock entry %q records target %q, which resolves into the project's own skills/ tree and is never a registration destination", e.ID, target)
				}
				return ProjectPlan{}, fmt.Errorf("project-register: lock entry %q records target %q outside the project root: %v", e.ID, target, err)
			}
		}
	}

	// 9/10/11: build each destination, refusing a foreign directory and
	// proving containment before anything is planned.
	rels := make([]string, 0, len(projectTargets))
	writes := make([]ProjectWrite, 0, len(projectTargets))
	// aliased records, per RESOLVED destination, the first planned path that
	// reached it. The two fixed targets are distinct strings, but `.agents`
	// symlinked at `.claude` makes them one file on disk (ALIAS-1, review round
	// 2): the lock would then record two targets for a single file, and
	// rollback could no longer promise a byte-identical tree, because the
	// second write silently overwrites the first and only one of the two
	// removals can succeed. Refusing is the only outcome that keeps both
	// properties unconditional.
	aliased := make(map[string]string, len(projectTargets)+1)
	for _, target := range projectTargets {
		dirRel := target.Dir + "/" + id
		rel := dirRel + "/" + projectSkillFileName

		dirAbs, _, err := resolveWritePath(in, root, dirRel)
		if err != nil {
			return ProjectPlan{}, fmt.Errorf("project-register: destination %q: %v", dirRel, err)
		}
		if _, err := in.Stat(dirAbs); err == nil {
			// The id is not in the lock (step 8 proved that), so whatever
			// sits here belongs to someone else.
			return ProjectPlan{}, fmt.Errorf("project-register: foreign skill: %q already exists and %q is not in the lock", dirRel, id)
		} else if !isAbsent(err) {
			return ProjectPlan{}, fmt.Errorf("project-register: inspecting destination %q: %v", dirRel, err)
		}

		abs, resolved, err := resolveWritePath(in, root, rel)
		if err != nil {
			return ProjectPlan{}, fmt.Errorf("project-register: destination %q: %v", rel, err)
		}
		if other, ok := aliased[resolved]; ok {
			return ProjectPlan{}, fmt.Errorf("project-register: destinations %q and %q resolve to the same file, so one would silently overwrite the other", other, rel)
		}
		aliased[resolved] = rel

		rels = append(rels, rel)
		writes = append(writes, ProjectWrite{Rel: rel, Abs: abs, Data: stamped, Mode: ProjectFileMode})
	}

	// The lock is a destination like any other.
	lockAbs, resolvedLock, err := resolveWritePath(in, root, ProjectLockRelPath)
	if err != nil {
		return ProjectPlan{}, fmt.Errorf("project-register: destination %q: %v", ProjectLockRelPath, err)
	}
	if other, ok := aliased[resolvedLock]; ok {
		return ProjectPlan{}, fmt.Errorf("project-register: destinations %q and %q resolve to the same file, so one would silently overwrite the other", other, ProjectLockRelPath)
	}

	lock.Version = 1
	lock.Skills = append(lock.Skills, ProjectLockEntry{
		ID:         id,
		Provenance: "procedural",
		Candidate:  in.CandidateKey,
		SHA256:     sum,
		Revision:   1,
		Targets:    rels,
	})
	lockData, err := SerializeProjectLock(lock)
	if err != nil {
		return ProjectPlan{}, fmt.Errorf("project-register: %v", err)
	}

	lockWrite := ProjectWrite{Rel: ProjectLockRelPath, Abs: lockAbs, Data: lockData, Mode: ProjectFileMode}
	if in.LockExists {
		lockWrite.Backup = in.LockData
	}

	return ProjectPlan{
		ID:       id,
		SHA256:   sum,
		Revision: 1,
		Writes:   writes,
		Lock:     lockWrite,
	}, nil
}
