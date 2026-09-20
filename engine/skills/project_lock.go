package skills

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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
	if strings.ContainsAny(candidateKey, `"\`) {
		return fmt.Errorf("stamp provenance: candidateKey must not contain %q or %q", `"`, `\`)
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
