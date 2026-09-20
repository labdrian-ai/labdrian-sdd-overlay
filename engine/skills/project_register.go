package skills

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
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
		} else if !os.IsNotExist(err) {
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
//
// It returns the absolute destination AND its RESOLVED, cleaned form, so the
// caller can detect two distinct destinations that name one physical file
// without resolving anything a second time (ALIAS-1).
func resolveWritePath(in RegisterInput, root, rel string) (string, string, error) {
	if underSkillsDir(rel) {
		return "", "", errDestUnderSkillsDir
	}
	abs, ok := resolveTarget(root, rel)
	if !ok {
		return "", "", fmt.Errorf("escapes the project root")
	}
	inside, err := resolvedWithinRootUsing(in.ResolvePath, root, abs)
	if err != nil {
		return "", "", fmt.Errorf("could not be resolved: %v", err)
	}
	if !inside {
		return "", "", fmt.Errorf("escapes the project root through a symlink")
	}
	// Decision (f), applied to the RESOLVED destination. Proving the
	// destination is inside the root never proves it is OUTSIDE <root>/skills,
	// and underSkillsDir above only ever saw the repo-relative string: with
	// `<root>/.claude/skills` a symlink to `<root>/skills`, the registration
	// was accepted and its bytes would have landed physically in the project's
	// own source tree (review round 2, SEC-1). Resolving both sides through the
	// same injected resolver is the only check that sees it.
	// EFF-1 (review round 2, carried to slice 3b-ii): this block used to call
	// resolvedWithinRootUsing — which resolves BOTH its arguments — and then
	// resolve the same two paths again for the equality comparison, four
	// resolutions for two paths. Resolving each side ONCE and deriving both
	// halves from the results is the same decision, unchanged: strictly-below
	// containment (review round 2, SEC-1) OR explicit equality with skills/
	// itself (review round 3, PLAN-3), because the strictly-below helper admits
	// the destination that IS skills/.
	resolvedSkills, err := in.ResolvePath(filepath.Join(root, projectSourceSkillsDir))
	if err != nil {
		return "", "", fmt.Errorf("could not be resolved: %v", err)
	}
	resolvedDest, err := in.ResolvePath(abs)
	if err != nil {
		return "", "", fmt.Errorf("could not be resolved: %v", err)
	}
	if resolvedSkills == "" || resolvedDest == "" {
		// resolvedWithinRootUsing refuses an empty resolution explicitly
		// (review round 2, COV-4) because withinRoot("", p) is true for every
		// absolute path; folding its work in here inherits that obligation.
		return "", "", fmt.Errorf("could not be resolved: the resolver returned an empty path")
	}
	cleanSkills, cleanDest := filepath.Clean(resolvedSkills), filepath.Clean(resolvedDest)
	if withinRoot(cleanSkills, cleanDest) || cleanDest == cleanSkills {
		return "", "", errDestResolvesUnderSkillsDir
	}
	return abs, cleanDest, nil
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

// ── Execution (3b-ii) ───────────────────────────────────────────────────────

// projectDirMode is the mode of every directory a project registration
// creates. design.md legislates the FILE mode (ProjectFileMode, 0644) but
// never a directory mode, and the only MkdirAll in this package (install.go)
// derives its mode from a source directory this capability does not have. 0755
// is chosen because a skill directory must be traversable by the agent
// runtimes that read it, and because it is what the umask-free default of a
// freshly cloned checkout looks like.
const projectDirMode fs.FileMode = 0o755

// projectFS is the filesystem seam ExecuteProjectPlan works through, so a test
// can inject a failure at any single call (design.md: "over an injected
// `projectFS` interface so tests can inject failures"). design.md names the
// interface but never enumerates its methods, so this method set is derived
// from the stage/commit/rollback prose it does specify: MkdirAll for the
// target directories, a same-directory temp write for staging, Rename for the
// commit, Remove for leftover temps and created directories, Stat to learn
// which directories this run created, ReadDir to prove a created directory is
// empty before removing it, and ResolvePath for the check-then-act containment
// proof PlanProjectRegister's point-in-time proof explicitly defers here.
type projectFS interface {
	Stat(name string) (fs.FileInfo, error)
	ReadDir(name string) ([]fs.DirEntry, error)
	MkdirAll(dir string, perm fs.FileMode) error
	// WriteTemp writes data to a fresh temp file in dir at mode perm and
	// returns its path; the caller owns the rename. It is the writeFileAtomic
	// pattern of lifecycle.go (create temp in the destination's own directory,
	// write, sync, close) plus the os.Chmod step cmd/main.go's atomicWriteFile
	// carries — lifecycle.go's helper has no perm argument, so its temps keep
	// os.CreateTemp's 0600 through the rename, which ProjectFileMode forbids.
	WriteTemp(dir string, data []byte, perm fs.FileMode) (string, error)
	Rename(oldPath, newPath string) error
	Remove(name string) error
	ResolvePath(name string) (string, error)
}

// osProjectFS is the production binding of projectFS: the real filesystem,
// with resolvePathKeepingMissing as the resolver so the executor's
// check-then-act proof resolves paths exactly as the planner's did.
type osProjectFS struct{}

func (osProjectFS) Stat(name string) (fs.FileInfo, error) { return os.Stat(name) }

func (osProjectFS) ReadDir(name string) ([]fs.DirEntry, error) { return os.ReadDir(name) }

func (osProjectFS) MkdirAll(dir string, perm fs.FileMode) error { return os.MkdirAll(dir, perm) }

func (osProjectFS) WriteTemp(dir string, data []byte, perm fs.FileMode) (string, error) {
	tmp, err := os.CreateTemp(dir, ".tmp-skills-*")
	if err != nil {
		return "", fmt.Errorf("writeProjectTemp: create temp: %w", err)
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return "", fmt.Errorf("writeProjectTemp: write: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(name)
		return "", fmt.Errorf("writeProjectTemp: sync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return "", fmt.Errorf("writeProjectTemp: close: %w", err)
	}
	// os.CreateTemp creates at 0600; without this the planned ProjectFileMode
	// would never reach the committed file.
	if err := os.Chmod(name, perm); err != nil {
		os.Remove(name)
		return "", fmt.Errorf("writeProjectTemp: chmod: %w", err)
	}
	return name, nil
}

func (osProjectFS) Rename(oldPath, newPath string) error { return os.Rename(oldPath, newPath) }

func (osProjectFS) Remove(name string) error { return os.Remove(name) }

func (osProjectFS) ResolvePath(name string) (string, error) { return resolvePathKeepingMissing(name) }

// projectPlanRoot recovers the project root from the plan. design.md fixes
// ExecuteProjectPlan's signature and it carries no root, but every write pairs
// a repo-relative Rel with the absolute Abs it was resolved to, so the root is
// exactly Abs with that suffix removed. Deriving it from the LOCK write is
// deliberate: the lock is present in every plan PlanProjectRegister produces,
// so a zero or hand-built plan is refused here instead of being half-executed.
func projectPlanRoot(p ProjectPlan) (string, error) {
	if p.Lock.Rel == "" || p.Lock.Abs == "" {
		return "", fmt.Errorf("project-register: the plan carries no lock write, so it was never produced by PlanProjectRegister")
	}
	suffix := string(filepath.Separator) + filepath.FromSlash(p.Lock.Rel)
	abs := filepath.Clean(p.Lock.Abs)
	root := strings.TrimSuffix(abs, suffix)
	if root == abs || root == "" {
		return "", fmt.Errorf("project-register: lock write %q does not end in its repo-relative path %q", p.Lock.Abs, p.Lock.Rel)
	}
	return root, nil
}

// projectCommitOrder is the order the executor stages and commits in: the
// Writes in projectTargets order, then the lock LAST, because the lock is the
// commit marker (design.md, "Commit phase"; ADR-9's registry-last).
func projectCommitOrder(p ProjectPlan) []ProjectWrite {
	return append(append(make([]ProjectWrite, 0, len(p.Writes)+1), p.Writes...), p.Lock)
}

// ExecuteProjectPlan performs one registration: it stages every planned file
// as a same-directory temp, then renames the SKILL.md temps in projectTargets
// order and the lock LAST. Any failure rolls the tree back to its pre-run
// state before returning.
//
// It performs no validation of the registration itself — PlanProjectRegister
// owns all eleven validate-before-write checks — but it DOES re-establish the
// containment proof, because the planner's proof is point-in-time and a path
// component can become a symlink between the plan and this write.
//
// Cross-directory atomicity is not available on POSIX (design.md,
// alternatives). The git commit is the real atomic unit; this function's job
// is to leave either the full planned set or the pre-run state behind.
func ExecuteProjectPlan(p ProjectPlan, fsys projectFS, stdout, stderr io.Writer) error {
	root, err := projectPlanRoot(p)
	if err != nil {
		return err
	}
	order := projectCommitOrder(p)
	if err := checkProjectDestinations(fsys, root, order); err != nil {
		return err
	}

	s := &projectStager{
		fsys:      fsys,
		root:      root,
		order:     order,
		temps:     make([]string, len(order)),
		attempted: make([]bool, len(order)),
		preMode:   make([]fs.FileMode, len(order)),
	}

	// Stage: create each target directory, recording which ones this run
	// created, and write every SKILL.md and the new lock to a same-directory
	// temp at mode 0644.
	for i, w := range order {
		dir := filepath.Dir(w.Abs)
		if err := s.mkdirAll(dir); err != nil {
			return s.rollback(stderr, fmt.Errorf("project-register: creating %q: %w", path.Dir(w.Rel), err))
		}
		tmp, err := fsys.WriteTemp(dir, w.Data, w.Mode)
		if err != nil {
			return s.rollback(stderr, fmt.Errorf("project-register: staging %q: %w", w.Rel, err))
		}
		s.temps[i] = tmp
	}

	// Commit: rename the SKILL.md temps in projectTargets order, the lock last.
	for i, w := range order {
		// The destination's REAL mode, read immediately before it is renamed
		// over, is the only thing that can put it back the way it was. The
		// planned Mode cannot: it is always ProjectFileMode, so a lock the
		// project keeps at 0600 came back at 0644 after a failed run and the
		// headline guarantee — byte-identical, including file modes — was false
		// (review round 4, D1).
		if w.Backup != nil {
			info, err := fsys.Stat(w.Abs)
			if err != nil {
				return s.rollback(stderr, fmt.Errorf("project-register: inspecting %q before committing over it: %w", w.Rel, err))
			}
			s.preMode[i] = info.Mode().Perm()
		}
		// Recorded BEFORE the call, not after it: a rename that reports an error
		// may still have landed, and rollback must sweep the destinations this
		// run reached for. It must equally leave alone the ones it never reached
		// — restoring identical bytes over an untouched file still replaces it,
		// with a new inode and the planned mode (review round 4, D1).
		s.attempted[i] = true
		if err := fsys.Rename(s.temps[i], w.Abs); err != nil {
			return s.rollback(stderr, fmt.Errorf("project-register: committing %q: %w", w.Rel, err))
		}
		s.temps[i] = ""
	}

	// Reported only once the whole set is committed: a `wrote:` line for a file
	// that a later failure rolls back would be a lie, and the agent uses these
	// lines as a git pathspec.
	for _, w := range order {
		fmt.Fprintf(stdout, "wrote: %s\n", w.Rel)
	}
	return nil
}

// ErrRollbackIncomplete marks the one outcome that leaves the operator work to
// do: the run failed AND the rollback could not fully undo it. The CLI maps a
// non-nil return from ExecuteProjectPlan to exit 1; this sentinel is what lets
// it (and a test) tell an honest "nothing happened" from "look at these paths".
var ErrRollbackIncomplete = fmt.Errorf("project-register: rollback incomplete")

// projectStager records what one execution has done so far, which is exactly
// what rollback needs: the directories this run created and the temps it
// staged. The planned writes themselves are already in order.
type projectStager struct {
	fsys      projectFS
	root      string
	order     []ProjectWrite
	temps     []string // "" once renamed away or never staged
	created   []string // directories this run created
	attempted []bool   // a rename over order[i].Abs was reached for
	// preMode[i] is the destination's ACTUAL mode, read immediately before this
	// run renamed over it. It is meaningful only where order[i].Backup != nil,
	// because only those destinations are restored rather than removed.
	preMode []fs.FileMode
}

// mkdirAll creates every ancestor of dir strictly below the root that does not
// exist yet, ONE LEVEL AT A TIME, recording each level as soon as it is made.
//
// A single MkdirAll for the whole path cannot be made safe: os.MkdirAll creates
// the ancestors it can and only then fails on a deeper component, and it never
// reports which ones it created. Recording the whole list only after it returns
// nil therefore leaked every directory a partial failure left behind — rollback
// never learned they existed, so a failed run could leave a `.claude/skills/`
// in a project that had never had one (review round 4, D2). Creating one level
// at a time makes "it was created" and "it was recorded" the same event: each
// call has its parent already in place, so it creates exactly that level or
// nothing at all.
func (s *projectStager) mkdirAll(dir string) error {
	var missing []string
	for cur := filepath.Clean(dir); withinRoot(s.root, cur); cur = filepath.Dir(cur) {
		if _, err := s.fsys.Stat(cur); err == nil {
			break
		} else if !os.IsNotExist(err) {
			return err
		}
		missing = append(missing, cur) // deepest first
	}
	// Shallowest first, so every call's parent already exists.
	for i := len(missing) - 1; i >= 0; i-- {
		if err := s.fsys.MkdirAll(missing[i], projectDirMode); err != nil {
			return err
		}
		s.created = append(s.created, missing[i])
	}
	return nil
}

// rollback returns the tree to its pre-run state and reports cause, or
// ErrRollbackIncomplete naming every path it could not restore.
//
// It walks the PLANNED writes rather than only the renames it saw succeed,
// because a rename that reports an error may still have landed (an NFS or
// wrapper reality). Restoring or removing a destination that was never touched
// is a no-op, so the wider sweep costs nothing and closes that window.
func (s *projectStager) rollback(stderr io.Writer, cause error) error {
	var bad []string

	// 1. Every destination this run reached the rename for, newest first:
	// restore the bytes it captured at plan time AT THE MODE IT REALLY HAD, or
	// remove it when it is a genuinely new file.
	//
	// It sweeps every write whose rename was ATTEMPTED rather than only those
	// that succeeded, because a rename that reports an error may still have
	// landed (an NFS or wrapper reality). It stops at the ones that were never
	// attempted, because those were never touched: rewriting them restored
	// nothing and changed two things it had no business changing, the inode and
	// the mode (review round 4, D1).
	for i := len(s.order) - 1; i >= 0; i-- {
		if !s.attempted[i] {
			continue
		}
		w := s.order[i]
		if w.Backup == nil {
			if err := s.remove(w.Abs); err != nil {
				bad = append(bad, w.Rel)
			}
			continue
		}
		mode := s.preMode[i]
		if mode == 0 {
			// Unreachable while the commit loop records preMode before every
			// attempted rename of a backed-up write; kept so a future caller
			// that sets attempted without it degrades to the planned mode
			// instead of creating a file nobody can read.
			mode = w.Mode
		}
		tmp, err := s.fsys.WriteTemp(filepath.Dir(w.Abs), w.Backup, mode)
		if err != nil {
			bad = append(bad, w.Rel)
			continue
		}
		if err := s.fsys.Rename(tmp, w.Abs); err != nil {
			_ = s.remove(tmp)
			bad = append(bad, w.Rel)
		}
	}

	// 2. Leftover temps.
	for _, tmp := range s.temps {
		if tmp == "" {
			continue
		}
		if err := s.remove(tmp); err != nil {
			bad = append(bad, s.rel(tmp))
		}
	}

	// 3. Directories this run created, deepest first and only if empty.
	for _, dir := range s.createdDeepestFirst() {
		entries, err := s.fsys.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			bad = append(bad, s.rel(dir))
			continue
		}
		if len(entries) > 0 {
			continue
		}
		if err := s.remove(dir); err != nil {
			bad = append(bad, s.rel(dir))
		}
	}

	if len(bad) > 0 {
		for _, rel := range bad {
			fmt.Fprintf(stderr, "error: rollback incomplete: %s\n", rel)
		}
		return fmt.Errorf("%w: %s (after %v)", ErrRollbackIncomplete, strings.Join(bad, ", "), cause)
	}
	return cause
}

// remove deletes p, treating "it is already gone" as success: rollback sweeps
// destinations it may never have created.
func (s *projectStager) remove(p string) error {
	if err := s.fsys.Remove(p); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// createdDeepestFirst orders the created directories so a child is always
// removed before its parent. Sorting by descending path length is enough: a
// child is strictly longer than its parent.
func (s *projectStager) createdDeepestFirst() []string {
	dirs := append([]string(nil), s.created...)
	sort.SliceStable(dirs, func(i, j int) bool { return len(dirs[i]) > len(dirs[j]) })
	return dirs
}

// rel turns an absolute path back into the repo-relative, slash-separated form
// every reported path uses, so a rollback pointer is usable as a git pathspec.
func (s *projectStager) rel(p string) string {
	return filepath.ToSlash(strings.TrimPrefix(p, s.root+string(filepath.Separator)))
}

// checkProjectDestinations re-proves, at execution time, what
// PlanProjectRegister proved when the plan was built: every destination is the
// root joined with its own repo-relative path, lies strictly below the root
// lexically AND after symlink resolution, no destination lies or LANDS under
// the project's own skills/ tree (decision (f)), no two destinations resolve to
// the SAME physical path, and each destination still exists exactly as the plan
// found it.
//
// Every one of those is point-in-time in the plan, which is the whole reason
// this function exists; re-proving only some of them was the gap D4 and D3
// named (review round 4).
func checkProjectDestinations(fsys projectFS, root string, order []ProjectWrite) error {
	resolvedRoot, err := fsys.ResolvePath(root)
	if err != nil {
		return fmt.Errorf("project-register: resolving the project root %q: %w", root, err)
	}
	if resolvedRoot == "" {
		return fmt.Errorf("project-register: the resolver returned an empty path for the project root %q", root)
	}
	resolvedRoot = filepath.Clean(resolvedRoot)

	// Decision (f), resolved once for the whole set (review round 4, D4). The
	// planner re-applies it to the RESOLVED destination precisely because the
	// repo-relative string cannot see a `.claude/skills` symlinked at the
	// project's own source tree; the executor's re-proof dropped that half, so a
	// symlink created after planning was written straight through.
	resolvedSkills, err := fsys.ResolvePath(filepath.Join(root, projectSourceSkillsDir))
	if err != nil {
		return fmt.Errorf("project-register: resolving the project's own %s/ directory: %w", projectSourceSkillsDir, err)
	}
	if resolvedSkills == "" {
		return fmt.Errorf("project-register: the resolver returned an empty path for the project's own %s/ directory", projectSourceSkillsDir)
	}
	resolvedSkills = filepath.Clean(resolvedSkills)

	seen := make(map[string]string, len(order))
	for _, w := range order {
		if w.Rel == "" || w.Abs == "" {
			return fmt.Errorf("project-register: the plan carries a write with no path")
		}
		abs := filepath.Clean(w.Abs)
		if want := filepath.Join(root, filepath.FromSlash(w.Rel)); abs != want {
			return fmt.Errorf("project-register: write %q resolves to %q, want %q", w.Rel, abs, want)
		}
		if !withinRoot(root, abs) {
			return fmt.Errorf("project-register: destination %q escapes the project root", w.Rel)
		}
		resolved, err := fsys.ResolvePath(abs)
		if err != nil {
			return fmt.Errorf("project-register: destination %q could not be resolved: %w", w.Rel, err)
		}
		if resolved == "" {
			return fmt.Errorf("project-register: the resolver returned an empty path for destination %q", w.Rel)
		}
		resolved = filepath.Clean(resolved)
		if !withinRoot(resolvedRoot, resolved) {
			return fmt.Errorf("project-register: destination %q escapes the project root through a symlink", w.Rel)
		}
		if underSkillsDir(w.Rel) {
			return fmt.Errorf("project-register: destination %q %v", w.Rel, errDestUnderSkillsDir)
		}
		if withinRoot(resolvedSkills, resolved) || resolved == resolvedSkills {
			return fmt.Errorf("project-register: destination %q %v", w.Rel, errDestResolvesUnderSkillsDir)
		}
		if other, ok := seen[resolved]; ok {
			return fmt.Errorf("project-register: destinations %q and %q resolve to the same file, so one would silently overwrite the other", other, w.Rel)
		}
		seen[resolved] = w.Rel

		// The plan recorded, per write, whether the destination existed: Backup
		// holds its bytes when it did and is nil when it did not. That fact
		// decides rollback's restore-vs-remove, and it is as point-in-time as
		// the containment proof, so it is re-established here too.
		//
		// A destination the planner found ABSENT that is now present is the
		// planner's "foreign skill" arriving late: writing over it would destroy
		// a file this run did not create, and rolling back would DELETE it
		// (review round 4, D3). It is refused rather than backed up, because
		// answering a race more permissively than the look-first path would mean
		// the tool overwrites on a race exactly what it refuses to overwrite
		// when it checks in time. Refusing costs nothing: nothing has been
		// written yet, so there is nothing to undo.
		//
		// A destination the planner found PRESENT that is now absent is the
		// mirror image: its backup bytes would be laid down as a brand-new file
		// nobody asked this run to create.
		_, statErr := fsys.Stat(abs)
		switch {
		case statErr == nil && w.Backup == nil:
			return fmt.Errorf("project-register: destination %q already exists although the plan found it absent; it was created after the plan was built and this run will not overwrite it", w.Rel)
		case statErr != nil && os.IsNotExist(statErr) && w.Backup != nil:
			return fmt.Errorf("project-register: destination %q no longer exists although the plan captured its contents; it was removed after the plan was built", w.Rel)
		case statErr != nil && !os.IsNotExist(statErr):
			return fmt.Errorf("project-register: inspecting destination %q: %w", w.Rel, statErr)
		}
	}
	return nil
}
