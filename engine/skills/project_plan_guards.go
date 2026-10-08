package skills

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/pathguard"
)

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

// prepareProjectSkill is the one stamp -> lint -> hash seam shared by initial
// registration and revision. No caller may hash or write draft bytes before
// this helper returns: provenance is stamped first, hard lint runs against the
// exact stamped bytes, and only then is the hash computed.
func prepareProjectSkill(draft []byte, candidateKey string) ([]byte, string, error) {
	stamped, err := StampProvenance(draft, candidateKey)
	if err != nil {
		return nil, "", err
	}
	if hard, _ := LintSkillFile(stamped); len(hard) > 0 {
		return nil, "", fmt.Errorf("stamped draft fails lint: %v", hard[0])
	}
	return stamped, HashSkill(stamped), nil
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

// writeGuard is the one shared write guard (tasks.md 3b-i.5a): it proves, for one project root,
// that a destination may be written. Fresh destinations and lock-recorded targets, in every verb
// that plans a write, go through it. The resolver is injected, so the planners stay pure and a
// test controls every path the guard sees.
type writeGuard struct {
	root    string
	resolve pathguard.Resolver
}

// destination turns one repo-relative, slash-separated destination into
// an absolute path under the root, proving containment in BOTH steps: the lexical
// guard (resolveTarget — no empty, absolute or ".."-carrying target, nothing
// naming root itself) and then the resolved guard (pathguard.ResolvedWithinRootUsing,
// which refuses a destination reached through a symlinked `.claude` or
// `.agents` pointing out of the project). It also applies decision (f): no
// destination under <root>/skills/.
//
// It returns the absolute destination AND its RESOLVED, cleaned form, so the
// caller can detect two distinct destinations that name one physical file
// without resolving anything a second time (ALIAS-1).
func (g writeGuard) destination(rel string) (string, string, error) {
	if underSkillsDir(rel) {
		return "", "", errDestUnderSkillsDir
	}
	abs, ok := resolveTarget(g.root, rel)
	if !ok {
		return "", "", fmt.Errorf("escapes the project root")
	}
	inside, err := pathguard.ResolvedWithinRootUsing(g.resolve, g.root, abs)
	if err != nil {
		return "", "", fmt.Errorf("could not be resolved: %v", err)
	}
	if !inside {
		return "", "", fmt.Errorf("escapes the project root through a symlink")
	}
	return g.refuseTheProjectsOwnSkills(abs)
}

// refuseTheProjectsOwnSkills is decision (f), applied to the RESOLVED destination. Proving the
// destination is inside the root never proves it is OUTSIDE <root>/skills, and underSkillsDir
// above only ever saw the repo-relative string: with `<root>/.claude/skills` a symlink to
// `<root>/skills`, the registration was accepted and its bytes would have landed physically in
// the project's own source tree (review round 2, SEC-1). Resolving both sides through the same
// injected resolver is the only check that sees it.
//
// EFF-1 (review round 2, carried to slice 3b-ii): this used to call
// pathguard.ResolvedWithinRootUsing — which resolves BOTH its arguments — and then resolve the
// same two paths again for the equality comparison, four resolutions for two paths. Resolving
// each side ONCE and deriving both halves from the results is the same decision, unchanged:
// strictly-below containment (review round 2, SEC-1) OR explicit equality with skills/ itself
// (review round 3, PLAN-3), because the strictly-below helper admits the destination that IS
// skills/.
func (g writeGuard) refuseTheProjectsOwnSkills(abs string) (string, string, error) {
	resolvedSkills, err := g.resolve(filepath.Join(g.root, projectSourceSkillsDir))
	if err != nil {
		return "", "", fmt.Errorf("could not be resolved: %v", err)
	}
	resolvedDest, err := g.resolve(abs)
	if err != nil {
		return "", "", fmt.Errorf("could not be resolved: %v", err)
	}
	if resolvedSkills == "" || resolvedDest == "" {
		// pathguard.ResolvedWithinRootUsing refuses an empty resolution explicitly
		// (review round 2, COV-4) because pathguard.WithinRoot("", p) is true for every
		// absolute path; folding its work in here inherits that obligation.
		return "", "", fmt.Errorf("could not be resolved: the resolver returned an empty path")
	}
	cleanSkills, cleanDest := filepath.Clean(resolvedSkills), filepath.Clean(resolvedDest)
	if pathguard.WithinRoot(cleanSkills, cleanDest) || cleanDest == cleanSkills {
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
func checkFrontmatterAllowlist(verb, frontmatter string) error {
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
			return fmt.Errorf("%s: frontmatter key %q is not allowed (allowed: name, description, license, metadata); a procedural skill must not pre-approve tools", verb, key)
		}
	}
	return nil
}

// checkSkillID applies design.md's Identity rules to the skill id taken from the frontmatter name:
// non-empty, matching slugRe, and already normalized (NormalizeSlug is the single definition, so
// this also bounds the id at 48 bytes with no consecutive, leading or trailing hyphens — which
// satisfies Pi's name rule).
func checkSkillID(verb, id string) error {
	if id == "" {
		return fmt.Errorf("%s: the draft frontmatter declares no name, so the skill id is empty", verb)
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
		return fmt.Errorf("%s: id %q does not match the skill identifier pattern", verb, id)
	}
	if got := NormalizeSlug(id); got != id {
		return fmt.Errorf("%s: id %q is not normalized, want %q", verb, id, got)
	}
	return nil
}

// checkRegisterIdentity applies the Identity rules of checkSkillID to a skill being registered,
// and refuses an id that matches an overlay registry skill. A registry match means the identity
// belongs to the global tier and is handled through Disposition: extend:<id> on the human path,
// never shadowed by a project copy.
func checkRegisterIdentity(id string, reg Registry) error {
	if err := checkSkillID(projectRegisterVerb, id); err != nil {
		return err
	}
	if matched, skillPath := MatchCandidate(reg, id); matched {
		return fmt.Errorf("%s: id %q matches the overlay registry skill %q; extend it on the human path instead of shadowing it", projectRegisterVerb, id, skillPath)
	}
	return nil
}
