package skills

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// ProjectTarget is one runtime-visible directory a project-tier procedural
// skill is written to. Name is the runtime label used in output; Dir is the
// repo-relative, slash-separated directory that holds <id>/SKILL.md.
type ProjectTarget struct{ Name, Dir string }

// projectTargets is the FIXED, ordered target table (design.md, "Fixed
// table"; contract section 10). It is never `.pi/skills`, and no code
// branches on the target name or on the Codex smoke-test verdict — the Codex
// status cell in the contract is prose only.
// TestProjectTargetsMatchContractTable pins these rows against the contract
// document.
var projectTargets = []ProjectTarget{
	{"claude", ".claude/skills"},
	{"agents", ".agents/skills"},
}

// ProjectFileMode is the mode of every file a project registration writes:
// 0644, never executable (design.md, "Execution"; threat matrix).
const ProjectFileMode fs.FileMode = 0o644

// PiTrustNote is the disclosure every SUCCESSFUL project-register run prints
// (design.md, "Pi trust note"; contract section 10). Writing `.agents/skills`
// in a project Pi has not yet trusted makes Pi prompt once for project trust
// on its next start, and a non-interactive Pi run ignores the skill until
// then. It is printed by the CLI on success only — never on a refusal and
// never under --dry-run.
const PiTrustNote = "note: Pi loads .agents/skills only after the project is trusted; Pi may prompt once for project trust."

// allowedFrontmatterKeys is the complete set of top-level frontmatter keys a
// project-tier procedural skill may declare (design.md, validate step 4).
// `allowed-tools`, `disable-model-invocation`, `model`, `hooks` and anything
// else refuses, because a procedural skill must not pre-approve tools.
var allowedFrontmatterKeys = map[string]bool{
	"name":        true,
	"description": true,
	"license":     true,
	"metadata":    true,
}

// ProjectWrite is one planned file write. Rel is the repo-relative,
// slash-separated path (ready to use as a git pathspec); Abs is the resolved
// absolute path; Data is the exact bytes to write. Backup holds the
// destination's current bytes when it already existed, and is nil for a
// genuinely new file — the distinction rollback needs to decide between
// restoring and removing (3b-ii).
//
// Mode is the mode the executor creates the file with. It is always
// ProjectFileMode: carrying it on the write is what BINDS the constant the
// design mandates to the bytes that will actually be created, instead of
// leaving it declared but unused (review round 3, PLAN-2).
type ProjectWrite struct {
	Rel    string
	Abs    string
	Data   []byte
	Mode   fs.FileMode
	Backup []byte
}

// ProjectPlan is the complete, validated description of one registration,
// produced before any filesystem mutation. Lock is kept apart from Writes
// because it is the commit marker: ExecuteProjectPlan renames the Writes in
// projectTargets order and the lock LAST (3b-ii).
type ProjectPlan struct {
	ID       string
	SHA256   string
	Revision int
	Writes   []ProjectWrite
	Deletes  []string
	Lock     ProjectWrite
}

// RegisterInput carries the pre-read state PlanProjectRegister needs, so the
// planner itself performs no filesystem access: the draft bytes, the lock
// bytes, the overlay registry, and two injected probes.
//
// design.md names PlanProjectRegister(in RegisterInput) but never enumerates
// RegisterInput's fields, so these are derived from the validate-before-write
// steps and the CLI surface rather than quoted. Stat and ResolvePath are
// injected for the same reason EvaluateOwnership injects its readers: the
// planner stays pure and a test controls every path it sees.
//
// LockExists distinguishes "no lock file in this project yet" (an empty lock,
// and a lock write with no backup) from "a lock file exists and holds these
// bytes". LockData is ignored when LockExists is false.
//
// A nil ResolvePath is a fail-closed refusal, never a silent degradation to
// the lexical guard alone — that guard is exactly what a symlinked component
// defeats (tasks.md 3b-i.5b).
type RegisterInput struct {
	ProjectRoot  string
	DraftPath    string
	DraftData    []byte
	CandidateKey string
	Registry     Registry
	LockData     []byte
	LockExists   bool

	Stat        func(string) (fs.FileInfo, error)
	ResolvePath func(string) (string, error)
}

// projectSourceSkillsDir is the project's own skill source tree, relative to
// the project root: the one directory decision (f) forbids as a registration
// destination. It is named once because BOTH halves of that guard —
// underSkillsDir's lexical pass and resolveWritePath's resolved pass — must
// mean the same directory.
const projectSourceSkillsDir = "skills"

// underSkillsDir reports whether a repo-relative, slash-separated destination
// lies under the project's own `skills/` directory (design decision (f)): the
// overlay's source tree is never a registration destination. A merely
// skills-prefixed sibling such as `skills-other/` is not under it.
//
// This is the LEXICAL half of decision (f) and it is not sufficient on its
// own: it inspects the repo-relative string only, so a `.claude/skills`
// symlinked at the project's own `skills/` tree reads as an ordinary
// destination here (review round 2, SEC-1). resolveWritePath re-applies the
// decision to the RESOLVED destination for that reason; this stays as the
// cheap first pass.
func underSkillsDir(rel string) bool {
	clean := path.Clean(rel)
	return clean == projectSourceSkillsDir || strings.HasPrefix(clean, projectSourceSkillsDir+"/")
}

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
//  10. Every destination passes withinRoot AND resolvedWithinRoot.
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
	if withinRoot(root, draft) {
		return ProjectPlan{}, fmt.Errorf("project-register: draft %q must lie outside the project root %q", in.DraftPath, root)
	}
	if inside, err := resolvedWithinRootUsing(in.ResolvePath, root, draft); err != nil {
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

	// 5. Stamp, then lint the STAMPED bytes, then hash them.
	stamped, err := StampProvenance(in.DraftData, in.CandidateKey)
	if err != nil {
		return ProjectPlan{}, fmt.Errorf("project-register: %v", err)
	}
	if hard, _ := LintSkillFile(stamped); len(hard) > 0 {
		return ProjectPlan{}, fmt.Errorf("project-register: stamped draft fails lint: %v", hard[0])
	}
	sum := HashSkill(stamped)

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
			if _, err := resolveWritePath(in, root, target); err != nil {
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
	for _, target := range projectTargets {
		dirRel := target.Dir + "/" + id
		rel := dirRel + "/" + projectSkillFileName

		dirAbs, err := resolveWritePath(in, root, dirRel)
		if err != nil {
			return ProjectPlan{}, fmt.Errorf("project-register: destination %q: %v", dirRel, err)
		}
		if _, err := in.Stat(dirAbs); err == nil {
			// The id is not in the lock (step 8 proved that), so whatever
			// sits here belongs to someone else.
			return ProjectPlan{}, fmt.Errorf("project-register: foreign skill: %q already exists and %q is not in the lock", dirRel, id)
		} else if !os.IsNotExist(err) {
			return ProjectPlan{}, fmt.Errorf("project-register: inspecting destination %q: %v", dirRel, err)
		}

		abs, err := resolveWritePath(in, root, rel)
		if err != nil {
			return ProjectPlan{}, fmt.Errorf("project-register: destination %q: %v", rel, err)
		}

		rels = append(rels, rel)
		writes = append(writes, ProjectWrite{Rel: rel, Abs: abs, Data: stamped, Mode: ProjectFileMode})
	}

	// The lock is a destination like any other.
	lockAbs, err := resolveWritePath(in, root, ProjectLockRelPath)
	if err != nil {
		return ProjectPlan{}, fmt.Errorf("project-register: destination %q: %v", ProjectLockRelPath, err)
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

// resolveWritePath turns one repo-relative, slash-separated destination into
// an absolute path under root, proving containment in BOTH steps: the lexical
// guard (resolveTarget — no empty, absolute or ".."-carrying target, nothing
// naming root itself) and then the resolved guard (resolvedWithinRootUsing,
// which refuses a destination reached through a symlinked `.claude` or
// `.agents` pointing out of the project). It also applies decision (f): no
// destination under <root>/skills/.
//
// This is the one shared write guard tasks.md 3b-i.5a asks for: fresh
// destinations and lock-recorded targets both go through it.
func resolveWritePath(in RegisterInput, root, rel string) (string, error) {
	if underSkillsDir(rel) {
		return "", errDestUnderSkillsDir
	}
	abs, ok := resolveTarget(root, rel)
	if !ok {
		return "", fmt.Errorf("escapes the project root")
	}
	inside, err := resolvedWithinRootUsing(in.ResolvePath, root, abs)
	if err != nil {
		return "", fmt.Errorf("could not be resolved: %v", err)
	}
	if !inside {
		return "", fmt.Errorf("escapes the project root through a symlink")
	}
	// Decision (f), applied to the RESOLVED destination. Proving the
	// destination is inside the root never proves it is OUTSIDE <root>/skills,
	// and underSkillsDir above only ever saw the repo-relative string: with
	// `<root>/.claude/skills` a symlink to `<root>/skills`, the registration
	// was accepted and its bytes would have landed physically in the project's
	// own source tree (review round 2, SEC-1). Resolving both sides through the
	// same injected resolver is the only check that sees it.
	underSkills, err := resolvedWithinRootUsing(in.ResolvePath, filepath.Join(root, projectSourceSkillsDir), abs)
	if err != nil {
		return "", fmt.Errorf("could not be resolved: %v", err)
	}
	if underSkills {
		return "", errDestResolvesUnderSkillsDir
	}
	// resolvedWithinRootUsing is strictly-below, so the destination resolving
	// to skills/ ITSELF would slip past it; decision (f) forbids that path too.
	resolvedSkills, err := in.ResolvePath(filepath.Join(root, projectSourceSkillsDir))
	if err != nil {
		return "", fmt.Errorf("could not be resolved: %v", err)
	}
	resolvedDest, err := in.ResolvePath(abs)
	if err != nil {
		return "", fmt.Errorf("could not be resolved: %v", err)
	}
	if filepath.Clean(resolvedDest) == filepath.Clean(resolvedSkills) {
		return "", errDestResolvesUnderSkillsDir
	}
	return abs, nil
}

// errDestUnderSkillsDir is the decision-(f) refusal resolveWritePath returns,
// as a single comparable value so a caller can tell it apart from a
// containment failure and word its own message honestly (review round 3,
// PLAN-3): a destination under <root>/skills/ is inside the project root, not
// outside it.
var errDestUnderSkillsDir = fmt.Errorf("lies under skills/, which is never a registration destination")

// errDestResolvesUnderSkillsDir is the RESOLVED half of the same decision-(f)
// refusal (review round 2, SEC-1). It is a distinct value from
// errDestUnderSkillsDir because the two say different things to a human: the
// lexical one names a destination that spells skills/, this one names a
// destination that spells something else and lands there anyway.
var errDestResolvesUnderSkillsDir = fmt.Errorf("resolves into the project's own skills/ tree, which is never a registration destination")

// checkFrontmatterAllowlist refuses any top-level frontmatter key outside
// allowedFrontmatterKeys (validate step 4). Indented lines are children of a
// block (metadata's author/version, a folded description) and are not
// top-level keys.
func checkFrontmatterAllowlist(frontmatter string) error {
	for _, line := range strings.Split(frontmatter, "\n") {
		// Defence in depth, shadowed by the splitKeyValue check below: a blank
		// line carries no "key:" and is dropped there anyway, so deleting this
		// changes no verdict and no test can witness it (review round 2,
		// COV-5). It is kept because it states the intent — blank lines are not
		// keys — at the top of the loop rather than as a side effect of parsing.
		if strings.TrimSpace(line) == "" {
			continue
		}
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}
		key, _, ok := splitKeyValue(line)
		if !ok {
			continue
		}
		if !allowedFrontmatterKeys[key] {
			return fmt.Errorf("project-register: frontmatter key %q is not allowed (allowed: name, description, license, metadata); a procedural skill must not pre-approve tools", key)
		}
	}
	return nil
}

// checkRegisterIdentity applies design.md's Identity rules to the skill id
// taken from the frontmatter name: non-empty, already normalized
// (NormalizeSlug is the single definition, so this also bounds the id at 48
// bytes with no consecutive, leading or trailing hyphens — which satisfies
// Pi's name rule), matching slugRe, and not matching an overlay registry
// skill. A registry match means the identity belongs to the global tier and
// is handled through Disposition: extend:<id> on the human path, never
// shadowed by a project copy.
func checkRegisterIdentity(id string, reg Registry) error {
	if id == "" {
		return fmt.Errorf("project-register: the draft frontmatter declares no name, so the skill id is empty")
	}
	// The pattern check runs BEFORE the normalization check. Behind the
	// normalization check it was unreachable — NormalizeSlug's output always
	// matches slugRe — so the two guards were indistinguishable and one of
	// them had no case at all (review round 3, TQ-5). Ahead of it, a
	// malformed id (an uppercase letter, a dot, a slash, a leading hyphen) is
	// named for what it is, and the normalization refusal is left for ids
	// that match the pattern yet still differ from their normal form
	// ("a--b", "a-").
	if !slugRe.MatchString(id) {
		return fmt.Errorf("project-register: id %q does not match the skill identifier pattern", id)
	}
	if got := NormalizeSlug(id); got != id {
		return fmt.Errorf("project-register: id %q is not normalized, want %q", id, got)
	}
	if matched, skillPath := MatchCandidate(reg, id); matched {
		return fmt.Errorf("project-register: id %q matches the overlay registry skill %q; extend it on the human path instead of shadowing it", id, skillPath)
	}
	return nil
}
