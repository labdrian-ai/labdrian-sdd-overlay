package skills

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// ProceduralAuthor is the fixed metadata.author value stamped into every
// project-tier procedural skill (decision (g)). It never varies per skill,
// per project, or per agent.
const ProceduralAuthor = "labdrian-overlay procedural"

// ProjectLockRelPath is the overlay-owned lock file path, relative to a
// project root (decision (b)). It is never skills-lock.json.
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
// level are refused, and any version other than 1 is refused. A missing
// file is the caller's concern (an empty lock), not this function's — it
// only ever receives bytes that exist.
func ParseProjectLock(data []byte) (ProjectLock, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	var l ProjectLock
	if err := dec.Decode(&l); err != nil {
		return ProjectLock{}, fmt.Errorf("parse project lock: %w", err)
	}
	if l.Version != 1 {
		return ProjectLock{}, fmt.Errorf("parse project lock: unsupported version %d, want 1", l.Version)
	}
	return l, nil
}

// SerializeProjectLock renders l as deterministic, stable JSON: entries
// sorted by id, 2-space indentation, and exactly one trailing newline. The
// input slice is never mutated; a sorted copy is serialized.
func SerializeProjectLock(l ProjectLock) ([]byte, error) {
	sorted := make([]ProjectLockEntry, len(l.Skills))
	copy(sorted, l.Skills)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })

	out := ProjectLock{Version: l.Version, Skills: sorted}
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

// StampProvenance rewrites the metadata: block of a draft SKILL.md's
// frontmatter, deterministically: any existing author, provenance or
// candidate line is removed, every other metadata line is preserved
// byte-for-byte in its original relative order, and the three provenance
// lines are appended in this fixed order: author (ProceduralAuthor),
// provenance ("procedural"), candidate (candidateKey verbatim).
//
// It is pure (no filesystem or network I/O) and idempotent: stamping
// already-stamped bytes with the same candidateKey reproduces the same
// bytes. It refuses when the frontmatter fence or the metadata: block
// cannot be found — it never guesses at malformed frontmatter.
func StampProvenance(draft []byte, candidateKey string) ([]byte, error) {
	lines := strings.Split(string(draft), "\n")

	if len(lines) == 0 || !isFenceLine(lines[0]) {
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
		key, _, ok := splitKeyValue(lines[i])
		if ok && key == "metadata" {
			metaIdx = i
			break
		}
	}
	if metaIdx == -1 {
		return nil, fmt.Errorf("stamp provenance: no metadata: block found in frontmatter")
	}

	blockEnd := metaIdx + 1
	for blockEnd < closeIdx {
		line := lines[blockEnd]
		if strings.TrimSpace(line) == "" {
			blockEnd++
			continue
		}
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			blockEnd++
			continue
		}
		break
	}

	var kept []string
	for _, line := range lines[metaIdx+1 : blockEnd] {
		if strings.TrimSpace(line) == "" {
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
		fmt.Sprintf(`  author: "%s"`, ProceduralAuthor),
		"  provenance: procedural",
		"  candidate: " + candidateKey,
	}

	out := make([]string, 0, len(lines)+3)
	out = append(out, lines[:metaIdx+1]...)
	out = append(out, kept...)
	out = append(out, stampLines...)
	out = append(out, lines[blockEnd:]...)

	return []byte(strings.Join(out, "\n")), nil
}
