package skills

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// ProceduralAuthor is the fixed metadata.author value stamped into every
// project-tier procedural skill (decision (g),
// openspec/changes/procedural-memory-lifecycle/proposal.md:140). It never
// varies per skill, per project, or per agent.
const ProceduralAuthor = "labdrian-overlay procedural"

// ProjectLockRelPath is the overlay-owned lock file path, relative to a
// project root (decision (b),
// openspec/changes/procedural-memory-lifecycle/design.md:167). It is never
// skills-lock.json.
const ProjectLockRelPath = ".labdrian/procedural-skills.lock.json"

// ProjectLock is the top-level shape of the project-tier lock file.
type ProjectLock struct {
	Version int                `json:"version"`
	Skills  []ProjectLockEntry `json:"skills"`
}

// ProjectLockEntry records one project-tier procedural skill: its identity,
// provenance, content hash, revision and the repo-relative target paths it
// was written to.
type ProjectLockEntry struct {
	ID         string   `json:"id"`
	Provenance string   `json:"provenance"`
	Candidate  string   `json:"candidate"`
	SHA256     string   `json:"sha256"`
	Revision   int      `json:"revision"`
	Targets    []string `json:"targets"`
}

// ParseProjectLock parses lock file bytes strictly: unknown fields at any
// level are refused, any version other than 1 is refused, the bytes must
// hold exactly one JSON value with nothing trailing it, and no two entries
// may share the same id. A missing file is the caller's concern (an empty
// lock), not this function's — it only ever receives bytes that exist.
func ParseProjectLock(data []byte) (ProjectLock, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	var l ProjectLock
	if err := dec.Decode(&l); err != nil {
		return ProjectLock{}, fmt.Errorf("parse project lock: %w", err)
	}
	if err := dec.Decode(new(json.RawMessage)); err != io.EOF {
		return ProjectLock{}, fmt.Errorf("parse project lock: trailing data after the lock value")
	}
	if l.Version != 1 {
		return ProjectLock{}, fmt.Errorf("parse project lock: unsupported version %d, want 1", l.Version)
	}
	seen := make(map[string]bool, len(l.Skills))
	for _, e := range l.Skills {
		if seen[e.ID] {
			return ProjectLock{}, fmt.Errorf("parse project lock: duplicate skill id %q", e.ID)
		}
		seen[e.ID] = true
	}
	return l, nil
}

// SerializeProjectLock renders l as deterministic, stable JSON: entries
// sorted by id with ties broken by preserving their original relative order
// (sort.SliceStable), 2-space indentation, and exactly one trailing newline.
// The input slice is never mutated; a sorted copy is serialized.
//
// A zero-value Version (the empty-lock path: a caller building a
// ProjectLock from scratch without setting Version) is normalized to 1. Any
// other version that is not 1 is refused, so a write can never silently
// produce a lock file that ParseProjectLock's version pin then rejects.
//
// No two entries may share the same id, mirroring ParseProjectLock's
// duplicate-id refusal (review-24fc80ac3513305c,
// R3-duplicate-id-write-asymmetry): a writer must never be able to emit a
// lock that ParseProjectLock then refuses to read back.
func SerializeProjectLock(l ProjectLock) ([]byte, error) {
	version := l.Version
	if version == 0 {
		version = 1
	}
	if version != 1 {
		return nil, fmt.Errorf("serialize project lock: unsupported version %d, want 1", version)
	}

	seen := make(map[string]bool, len(l.Skills))
	for _, e := range l.Skills {
		if seen[e.ID] {
			return nil, fmt.Errorf("serialize project lock: duplicate skill id %q", e.ID)
		}
		seen[e.ID] = true
	}

	sorted := make([]ProjectLockEntry, len(l.Skills))
	copy(sorted, l.Skills)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })

	out := ProjectLock{Version: version, Skills: sorted}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("serialize project lock: %w", err)
	}
	return append(data, '\n'), nil
}

// provenanceMetadataKeys are the metadata sub-keys StampProvenance owns: any
// existing occurrence is removed before the fresh, deterministic values are
// appended (making the stamp idempotent).
var provenanceMetadataKeys = map[string]bool{
	"author":     true,
	"provenance": true,
	"candidate":  true,
}

// yamlPlainScalarIndicators are the leading characters that make a value not
// a plain (unquoted) YAML scalar. StampProvenance still double-quotes and
// escapes candidateKey before writing it, but refuses a key starting with
// one of these as defense in depth, independent of the fuller shape check
// ValidateCandidateKey performs elsewhere.
const yamlPlainScalarIndicators = "-?:,[]{}#&*!|>'\"%@`"

// validateCandidateKey rejects a candidateKey that is unsafe to stamp into
// YAML frontmatter: empty, containing a control character (including
// newline, carriage return or tab), not a plain scalar (a leading YAML
// indicator character, or leading/trailing whitespace), or containing a
// sequence (": " or " #") that would end a plain scalar mid-value.
func validateCandidateKey(candidateKey string) error {
	if candidateKey == "" {
		return fmt.Errorf("stamp provenance: candidateKey must not be empty")
	}
	for _, r := range candidateKey {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("stamp provenance: candidateKey must not contain control characters, found %q", r)
		}
	}
	if strings.ContainsRune(yamlPlainScalarIndicators, rune(candidateKey[0])) {
		return fmt.Errorf("stamp provenance: candidateKey must be a plain scalar, got leading indicator character %q", candidateKey[0])
	}
	if strings.HasPrefix(candidateKey, " ") || strings.HasSuffix(candidateKey, " ") {
		return fmt.Errorf("stamp provenance: candidateKey must not have leading or trailing spaces")
	}
	if strings.Contains(candidateKey, ": ") || strings.Contains(candidateKey, " #") {
		return fmt.Errorf("stamp provenance: candidateKey must not contain %q or %q", ": ", " #")
	}
	// A leading '"' or '\' is already refused above as a plain-scalar
	// indicator; this catches an interior occurrence (review-24fc80ac3513305c,
	// R3-candidate-escape-unproved, decision (a)). escapeYAMLDoubleQuoted still
	// escapes both below as defense in depth, but no candidateKey reaching it
	// may contain either.
	//
	// The two characters get one branch and one message each
	// (review-d89971d41a526146): a shared message left the two refusals
	// indistinguishable, so neither test could prove which branch it reached.
	if strings.Contains(candidateKey, `"`) {
		return fmt.Errorf("stamp provenance: candidateKey must not contain a double quote %q", `"`)
	}
	if strings.Contains(candidateKey, `\`) {
		return fmt.Errorf("stamp provenance: candidateKey must not contain a backslash %q", `\`)
	}
	return nil
}

// escapeYAMLDoubleQuoted escapes s for embedding inside a YAML
// double-quoted scalar: backslashes and double quotes are backslash-escaped.
func escapeYAMLDoubleQuoted(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

// metadataLineKind classifies a line inside (or immediately after) a
// top-level metadata: block. The block-end scan and the keep/drop filter in
// StampProvenance both call classifyMetadataLine so the definition of what
// belongs to the block lives in exactly one place.
type metadataLineKind int

const (
	metadataLineOther metadataLineKind = iota
	metadataLineBlank
	metadataLineIndented
)

// classifyMetadataLine reports whether line is blank, indented (with a
// leading space or tab, so it can only be a child of a mapping block), or
// neither (a new top-level line that ends the block).
func classifyMetadataLine(line string) metadataLineKind {
	if strings.TrimSpace(line) == "" {
		return metadataLineBlank
	}
	if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
		return metadataLineIndented
	}
	return metadataLineOther
}

// StampProvenance rewrites the metadata: block of a draft SKILL.md's
// frontmatter, deterministically: any existing author, provenance or
// candidate line is removed, every other metadata line is preserved
// byte-for-byte in its original relative order, and the three provenance
// lines are appended in this fixed order: author (ProceduralAuthor),
// provenance ("procedural"), candidate (candidateKey, double-quoted and
// escaped like author). The stamped lines reuse the existing metadata
// block's indentation (detected from its first non-blank line), falling
// back to two spaces when the block is empty.
//
// It is pure (no filesystem or network I/O) and idempotent: stamping
// already-stamped bytes with the same candidateKey reproduces the same
// bytes. It refuses when the frontmatter fence or the metadata: block
// cannot be found, when the metadata: line carries an inline value (a flow
// mapping, null, or any other non-empty value — a block opener must be
// bare), or when candidateKey fails validateCandidateKey — it never
// guesses at malformed frontmatter or unsafe input.
func StampProvenance(draft []byte, candidateKey string) ([]byte, error) {
	if err := validateCandidateKey(candidateKey); err != nil {
		return nil, err
	}

	lines := strings.Split(string(draft), "\n")

	if !isFenceLine(lines[0]) {
		return nil, fmt.Errorf("stamp provenance: missing opening frontmatter fence")
	}
	closeIdx := -1
	for i := 1; i < len(lines); i++ {
		if isFenceLine(lines[i]) {
			closeIdx = i
			break
		}
	}
	if closeIdx == -1 {
		return nil, fmt.Errorf("stamp provenance: missing closing frontmatter fence")
	}

	metaIdx := -1
	for i := 1; i < closeIdx; i++ {
		if strings.HasPrefix(lines[i], " ") || strings.HasPrefix(lines[i], "\t") {
			continue // indented — cannot be the top-level metadata: line
		}
		key, val, ok := splitKeyValue(lines[i])
		if ok && key == "metadata" {
			if val != "" {
				return nil, fmt.Errorf("stamp provenance: metadata: line must not carry an inline value, got %q", val)
			}
			metaIdx = i
			break
		}
	}
	if metaIdx == -1 {
		return nil, fmt.Errorf("stamp provenance: no metadata: block found in frontmatter")
	}

	blockEnd := metaIdx + 1
	for blockEnd < closeIdx {
		if kind := classifyMetadataLine(lines[blockEnd]); kind == metadataLineBlank || kind == metadataLineIndented {
			blockEnd++
			continue
		}
		break
	}

	indent := "  "
	for _, line := range lines[metaIdx+1 : blockEnd] {
		if classifyMetadataLine(line) != metadataLineIndented {
			continue
		}
		trimmed := strings.TrimLeft(line, " \t")
		indent = line[:len(line)-len(trimmed)]
		break
	}

	var kept []string
	for _, line := range lines[metaIdx+1 : blockEnd] {
		if classifyMetadataLine(line) == metadataLineBlank {
			kept = append(kept, line)
			continue
		}
		key, _, ok := splitKeyValue(strings.TrimSpace(line))
		if ok && provenanceMetadataKeys[key] {
			continue
		}
		kept = append(kept, line)
	}

	stampLines := []string{
		indent + fmt.Sprintf(`author: "%s"`, ProceduralAuthor),
		indent + "provenance: procedural",
		indent + fmt.Sprintf(`candidate: "%s"`, escapeYAMLDoubleQuoted(candidateKey)),
	}

	out := make([]string, 0, len(lines)+3)
	out = append(out, lines[:metaIdx+1]...)
	out = append(out, kept...)
	out = append(out, stampLines...)
	out = append(out, lines[blockEnd:]...)

	return []byte(strings.Join(out, "\n")), nil
}

// HashSkill returns the lowercase hex SHA-256 of data, which for a registered
// skill is the exact stamped bytes written to every target SKILL.md
// (design.md:203). There is deliberately no normalization first: no
// line-ending conversion, no whitespace trim and no frontmatter
// canonicalization. Any byte change means someone other than the agent
// touched the file, and normalizing would hide a real edit; the
// false-positive direction (a core.autocrlf checkout reading as human-owned)
// is the safe one, because the agent then stops.
func HashSkill(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// candidateKeyPrefix is the fixed item-30 prefix every candidate topic key
// carries (design.md:163).
const candidateKeyPrefix = "procedural/candidates/"

// candidateKindSlugs maps each accepted candidate kind to the exact number of
// slug segments that must follow it: repeated-success/<s> and
// failure-recovery/<s>/<s> (design.md:163).
var candidateKindSlugs = map[string]int{
	"repeated-success": 1,
	"failure-recovery": 2,
}

// ValidateCandidateKey reports whether key is one of the two item-30
// candidate topic-key shapes — procedural/candidates/repeated-success/<s> and
// procedural/candidates/failure-recovery/<s>/<s> — where every <s> is
// non-empty and already normalized (NormalizeSlug(<s>) == <s>).
//
// This is a SHAPE check and is distinct from the unexported
// validateCandidateKey, which checks whether a key is SAFE to stamp into YAML
// frontmatter and therefore carries a "stamp provenance: " prefix on its
// errors. The two cannot disagree: a shape-valid key is the literal prefix
// plus NormalizeSlug output joined by '/', so it is drawn from [a-z0-9/-],
// which the stamp-safety validator accepts — the shape-valid set is a strict
// subset of the stamp-safe set. TestValidateCandidateKey_ShapeValidKeysAreAlsoStampSafe
// pins that relation.
func ValidateCandidateKey(key string) error {
	if key == "" {
		return fmt.Errorf("validate candidate key: must not be empty")
	}
	if !strings.HasPrefix(key, candidateKeyPrefix) {
		return fmt.Errorf("validate candidate key: %q must start with %q", key, candidateKeyPrefix)
	}
	rest := strings.Split(strings.TrimPrefix(key, candidateKeyPrefix), "/")
	kind := rest[0]
	wantSlugs, ok := candidateKindSlugs[kind]
	if !ok {
		return fmt.Errorf("validate candidate key: unknown candidate kind %q, want %q or %q", kind, "repeated-success", "failure-recovery")
	}
	slugs := rest[1:]
	if len(slugs) != wantSlugs {
		return fmt.Errorf("validate candidate key: %q must have exactly %d slug segments after %q, got %d", key, wantSlugs, kind, len(slugs))
	}
	for i, s := range slugs {
		if s == "" {
			return fmt.Errorf("validate candidate key: slug segment %d of %q must not be empty", i+1, key)
		}
		if got := NormalizeSlug(s); got != s {
			return fmt.Errorf("validate candidate key: slug segment %q of %q is not normalized, want %q", s, key, got)
		}
	}
	return nil
}

// Ownership is the verdict of the ownership-by-hash check for one skill.
// Reason is "" when AgentOwned is true, and otherwise the FIRST failing
// reason — never a list — in one of these forms: "hash-mismatch <path>",
// "missing <path>", "extra-entry <path>" or "not-in-lock" (design.md:210),
// plus the malformed-lock and unprovable-containment reasons this code adds
// to that vocabulary (all six carried into design.md's amended reason list):
//   - "invalid-target <target>" — a recorded target that does not lexically
//     resolve strictly inside the project root (empty, absolute, carrying a
//     ".." component, or naming the root itself).
//   - "no-targets <id>" — an entry present in the lock recording no targets.
//   - "invalid-root <root>" — root is not absolute, so containment cannot be
//     decided at all.
//   - "no-resolver <id>" — no symlink resolver was injected, so resolved
//     containment cannot be proved.
//   - "unresolved-root <root>" / "unresolved-target <target>" — the injected
//     resolver failed on root, or on that target.
//
// The last four mean "ownership cannot be proved", which folds into
// human-owned: the safe direction, because the agent then stops.
// Each <path> is the repo-relative, slash-separated path as recorded in the
// lock, so a status line stays independent of where the project is checked
// out. <target> is likewise the raw recorded string, quoted verbatim so the
// offending lock line is identifiable.
type Ownership struct {
	AgentOwned bool
	Reason     string
}

// projectSkillFileName is the only file a project-tier procedural skill
// directory may contain: project-tier skills are single-file by construction,
// so assets/, references/ and scripts/ are never written (design.md:203).
const projectSkillFileName = "SKILL.md"

// EvaluateOwnership decides whether the skill recorded by e is still
// agent-owned under root. It is agent-owned only when all three conditions
// hold (design.md:205-208): the lock entry exists, every recorded target file
// exists and hashes to e.SHA256, and every target skill directory contains
// exactly one entry, SKILL.md. Anything else is human-owned, reported with
// the first failing reason.
//
// The lock's sha256 is the single source of truth: this never reads or
// compares against the candidate record's Registered/Promoted hash line,
// which is an informational mirror for humans, not a second source
// (design.md:210).
//
// All filesystem access goes through the injected readFile and readDir, so
// callers (and tests) fully control what is read; EvaluateOwnership itself
// never touches os, git or Engram. A caller whose lock lookup missed passes
// the zero ProjectLockEntry, which reports "not-in-lock".
//
// Every target is resolved and containment-checked BEFORE any read, so a
// lock entry carrying "../../etc/passwd", an absolute path, or a path whose
// directory components are symlinks pointing out of the project can never
// cause a read outside root, nor report AgentOwned for a file the project
// does not own.
//
// Containment is proved in two steps, because the first is purely lexical:
//
//  1. resolveTarget applies the lexical guards (no empty target, no absolute
//     target, no ".." component, no target naming root itself, cleaned form
//     under root).
//  2. resolvePath, the injected symlink resolver, then resolves both root and
//     the target, and containment is re-checked between the RESOLVED paths.
//
// Step 2 is what closes the symlink hole: "link/SKILL.md", where root/link is
// a symlink to a sibling of root, passes every lexical guard (it names no
// ".."), so only the resolved comparison refuses it
// (review-slice-3a-ii-round-2, SEC-2). The resolver is injected rather than
// called through path/filepath so EvaluateOwnership keeps its defining
// property: it performs no filesystem access of its own, and a caller (or a
// test) fully controls every path it sees.
//
// resolvePath's contract: return the argument with every symlink in its
// EXISTING ancestry resolved, keeping components that do not exist literal
// (filepath.EvalSymlinks alone does not satisfy this — it fails on a missing
// final component — so the caller wraps it; see tasks.md 3b-i.5b), and return
// an error only when resolution genuinely failed. That contract is what keeps
// an ordinary absent target reporting "missing <path>" rather than a
// resolution failure.
//
// A nil resolvePath fails closed with "no-resolver <id>": ownership never
// silently degrades to the lexical check alone, because that check is exactly
// what a symlinked component defeats.
func EvaluateOwnership(root string, e ProjectLockEntry, readFile func(string) ([]byte, error), readDir func(string) ([]fs.DirEntry, error), resolvePath func(string) (string, error)) Ownership {
	if e.ID == "" {
		return Ownership{Reason: "not-in-lock"}
	}
	// A non-absolute root makes every containment test meaningless (a relative
	// or empty root silently turns every project skill human-owned), so it is
	// one loud failure naming the bad root, never a per-target refusal.
	if !filepath.IsAbs(root) {
		return Ownership{Reason: "invalid-root " + root}
	}
	if len(e.Targets) == 0 {
		return Ownership{Reason: "no-targets " + e.ID}
	}
	if resolvePath == nil {
		return Ownership{Reason: "no-resolver " + e.ID}
	}

	cleanRoot := filepath.Clean(root)
	resolvedRoot, err := resolvePath(cleanRoot)
	if err != nil {
		// Without a resolved root there is nothing to compare targets against
		// (a root reached through a symlink would otherwise read as an escape).
		return Ownership{Reason: "unresolved-root " + root}
	}

	for _, target := range e.Targets {
		abs, ok := resolveTarget(root, target)
		if !ok {
			return Ownership{Reason: "invalid-target " + target}
		}
		resolved, err := resolvePath(abs)
		if err != nil {
			return Ownership{Reason: "unresolved-target " + target}
		}
		if !withinRoot(filepath.Clean(resolvedRoot), filepath.Clean(resolved)) {
			return Ownership{Reason: "escapes-root " + target}
		}
		data, err := readFile(abs)
		if err != nil {
			return Ownership{Reason: "missing " + target}
		}
		if HashSkill(data) != e.SHA256 {
			return Ownership{Reason: "hash-mismatch " + target}
		}

		dirRel := path.Dir(target)
		entries, err := readDir(filepath.Dir(abs))
		if err != nil {
			// The exactly-one-entry condition cannot be proved, so the safe
			// direction is human-owned.
			return Ownership{Reason: "missing " + dirRel}
		}
		for _, entry := range entries {
			if entry.Name() != projectSkillFileName {
				return Ownership{Reason: "extra-entry " + path.Join(dirRel, entry.Name())}
			}
		}
	}

	return Ownership{AgentOwned: true}
}

// resolveTarget turns one lock-recorded, slash-separated target into an
// absolute path under root, reporting false when the target may not be read
// at all. It mirrors the R-055 containment guard in PlanInstall
// (install.go:43-49): clean the joined path, then require the cleaned form to
// still sit under the cleaned root plus a separator. A local helper rather
// than a shared one because PlanInstall's guard is inlined there and returns
// an error, while ownership must fold the refusal into an Ownership reason;
// the containment test itself is byte-for-byte the same shape.
//
// Refused: an empty target, an absolute target, a target carrying a ".."
// component, any target whose cleaned form escapes root, and a target that
// names root itself rather than a path strictly below it (".", "./" and
// "././" all clean to ".", which resolves to root and would hand a directory
// to readFile — review-slice-3a-ii-round-2, SEC-3). That last case needs no
// branch of its own: withinRoot is strictly-below, so it refuses root itself. The ".." check is explicit and precedes the containment
// test because filepath.Join cleans "../.." away, so a target could resolve
// back inside root while still meaning something the lock never recorded.
//
// This guard is LEXICAL ONLY: it cannot see a symlink, so a target whose
// directory components leave root through one still passes here. Resolved
// containment is EvaluateOwnership's second step
// (review-slice-3a-ii-round-2, SEC-2); a caller reusing resolveTarget for a
// write path owes itself the same second step.
func resolveTarget(root, target string) (string, bool) {
	if target == "" {
		return "", false
	}
	if path.IsAbs(target) || filepath.IsAbs(filepath.FromSlash(target)) {
		return "", false
	}
	for _, seg := range strings.Split(target, "/") {
		if seg == ".." {
			return "", false
		}
	}
	cleanRoot := filepath.Clean(root)
	abs := filepath.Clean(filepath.Join(cleanRoot, filepath.FromSlash(target)))
	if !withinRoot(cleanRoot, abs) {
		return "", false
	}
	return abs, true
}

// withinRoot reports whether the cleaned absolute path p sits STRICTLY below
// the cleaned root: p equal to root is not within it. Both the lexical guard
// in resolveTarget and the resolved-path check in EvaluateOwnership use this
// one definition, so the two steps can never disagree about what containment
// means.
func withinRoot(cleanRoot, p string) bool {
	return strings.HasPrefix(p+string(filepath.Separator), cleanRoot+string(filepath.Separator)) && p != cleanRoot
}
