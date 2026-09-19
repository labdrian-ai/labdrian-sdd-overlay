package skills

import (
	"reflect"
	"strings"
	"testing"
)

// --- ParseProjectLock / SerializeProjectLock (task 3a-i.1) ---

func sampleLockUnsorted() ProjectLock {
	return ProjectLock{
		Version: 1,
		Skills: []ProjectLockEntry{
			{
				ID:         "zeta-skill",
				Provenance: "procedural",
				Candidate:  "procedural/candidates/repeated-success/zeta-skill",
				SHA256:     strings.Repeat("a", 64),
				Revision:   1,
				Targets: []string{
					".claude/skills/zeta-skill/SKILL.md",
					".agents/skills/zeta-skill/SKILL.md",
				},
			},
			{
				ID:         "alpha-skill",
				Provenance: "procedural",
				Candidate:  "procedural/candidates/repeated-success/alpha-skill",
				SHA256:     strings.Repeat("b", 64),
				Revision:   1,
				Targets: []string{
					".claude/skills/alpha-skill/SKILL.md",
					".agents/skills/alpha-skill/SKILL.md",
				},
			},
		},
	}
}

func TestSerializeProjectLock_SortsByID(t *testing.T) {
	l := sampleLockUnsorted()
	data, err := SerializeProjectLock(l)
	if err != nil {
		t.Fatalf("SerializeProjectLock: %v", err)
	}
	idxAlpha := strings.Index(string(data), `"alpha-skill"`)
	idxZeta := strings.Index(string(data), `"zeta-skill"`)
	if idxAlpha < 0 || idxZeta < 0 {
		t.Fatalf("serialized output missing an id: %s", data)
	}
	if idxAlpha > idxZeta {
		t.Errorf("expected alpha-skill before zeta-skill in serialized (sorted) output, got:\n%s", data)
	}
}

func TestSerializeProjectLock_DeterministicIndentAndTrailingNewline(t *testing.T) {
	l := sampleLockUnsorted()
	first, err := SerializeProjectLock(l)
	if err != nil {
		t.Fatalf("SerializeProjectLock: %v", err)
	}
	// Reorder input skills; output must be byte-identical (order-independent).
	l2 := sampleLockUnsorted()
	l2.Skills[0], l2.Skills[1] = l2.Skills[1], l2.Skills[0]
	second, err := SerializeProjectLock(l2)
	if err != nil {
		t.Fatalf("SerializeProjectLock (reordered input): %v", err)
	}
	if string(first) != string(second) {
		t.Errorf("SerializeProjectLock is not order-independent/deterministic:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
	if !strings.HasSuffix(string(first), "\n") {
		t.Errorf("expected trailing newline, got: %q", first)
	}
	if strings.HasSuffix(string(first), "\n\n") {
		t.Errorf("expected exactly one trailing newline, got: %q", first)
	}
	for _, line := range strings.Split(strings.TrimRight(string(first), "\n"), "\n") {
		if line == "" {
			continue
		}
		trimmed := strings.TrimLeft(line, " ")
		indentLen := len(line) - len(trimmed)
		if indentLen%2 != 0 {
			t.Errorf("expected 2-space indentation, got indent of %d on line %q", indentLen, line)
		}
	}
}

func TestParseSerializeProjectLock_RoundTrip(t *testing.T) {
	l := sampleLockUnsorted()
	data, err := SerializeProjectLock(l)
	if err != nil {
		t.Fatalf("SerializeProjectLock: %v", err)
	}
	got, err := ParseProjectLock(data)
	if err != nil {
		t.Fatalf("ParseProjectLock: %v", err)
	}
	if got.Version != 1 {
		t.Errorf("Version = %d, want 1", got.Version)
	}
	if len(got.Skills) != 2 {
		t.Fatalf("len(Skills) = %d, want 2", len(got.Skills))
	}
	// SerializeProjectLock sorts by id, so the parsed order is deterministic:
	// alpha-skill, then zeta-skill.
	want := []ProjectLockEntry{l.Skills[1], l.Skills[0]}
	if !reflect.DeepEqual(got.Skills, want) {
		t.Errorf("round-trip mismatch:\ngot:  %+v\nwant: %+v", got.Skills, want)
	}
}

func TestParseProjectLock_UnknownFieldRefused(t *testing.T) {
	data := []byte(`{
  "version": 1,
  "skills": [],
  "unexpected_field": "should not be accepted"
}
`)
	if _, err := ParseProjectLock(data); err == nil {
		t.Error("ParseProjectLock: expected refusal on unknown top-level field, got nil error")
	}
}

func TestParseProjectLock_UnknownFieldInEntryRefused(t *testing.T) {
	data := []byte(`{
  "version": 1,
  "skills": [
    {
      "id": "foo",
      "provenance": "procedural",
      "candidate": "procedural/candidates/repeated-success/foo",
      "sha256": "` + strings.Repeat("a", 64) + `",
      "revision": 1,
      "targets": [".claude/skills/foo/SKILL.md"],
      "extra": "nope"
    }
  ]
}
`)
	if _, err := ParseProjectLock(data); err == nil {
		t.Error("ParseProjectLock: expected refusal on unknown field inside a skills entry, got nil error")
	}
}

func TestParseProjectLock_VersionMismatchRefused(t *testing.T) {
	data := []byte(`{"version": 2, "skills": []}`)
	if _, err := ParseProjectLock(data); err == nil {
		t.Error("ParseProjectLock: expected refusal for version != 1, got nil error")
	}
}

func TestParseProjectLock_VersionMissingRefused(t *testing.T) {
	// A zero-value (absent) version field is also not version 1.
	data := []byte(`{"skills": []}`)
	if _, err := ParseProjectLock(data); err == nil {
		t.Error("ParseProjectLock: expected refusal for missing/zero version, got nil error")
	}
}

func TestParseProjectLock_MalformedJSONRefused(t *testing.T) {
	if _, err := ParseProjectLock([]byte(`{not json`)); err == nil {
		t.Error("ParseProjectLock: expected refusal on malformed JSON, got nil error")
	}
}

// --- StampProvenance (task 3a-i.1) ---

const provenanceFixtureNoExisting = `---
name: probe-skill
description: "Trigger: probe event. Reply with the probe result."
license: MIT
metadata:
  version: "1.0"
---
## Activation Contract

Trigger: probe event.

## Hard Rules

Always do X.
`

func TestStampProvenance_InsertsIntoExistingMetadataBlock(t *testing.T) {
	out, err := StampProvenance([]byte(provenanceFixtureNoExisting), "procedural/candidates/repeated-success/probe-skill")
	if err != nil {
		t.Fatalf("StampProvenance: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, `  author: "`+ProceduralAuthor+`"`) {
		t.Errorf("expected stamped author line, got:\n%s", s)
	}
	if !strings.Contains(s, "  provenance: procedural") {
		t.Errorf("expected stamped provenance line, got:\n%s", s)
	}
	if !strings.Contains(s, "  candidate: procedural/candidates/repeated-success/probe-skill") {
		t.Errorf("expected stamped candidate line, got:\n%s", s)
	}
	// Preserves other metadata lines byte-for-byte.
	if !strings.Contains(s, `  version: "1.0"`) {
		t.Errorf("expected preserved version metadata line, got:\n%s", s)
	}
}

const provenanceFixtureWithExisting = `---
name: probe-skill
description: "Trigger: probe event. Reply with the probe result."
license: MIT
metadata:
  version: "2.0"
  author: "someone-else"
  provenance: manual
  candidate: procedural/candidates/repeated-success/old-key
---
## Activation Contract

Trigger: probe event.

## Hard Rules

Always do X.
`

func TestStampProvenance_ReplacesExistingProvenanceLines(t *testing.T) {
	out, err := StampProvenance([]byte(provenanceFixtureWithExisting), "procedural/candidates/repeated-success/probe-skill")
	if err != nil {
		t.Fatalf("StampProvenance: %v", err)
	}
	s := string(out)

	if strings.Contains(s, "someone-else") {
		t.Errorf("expected old author line replaced, got:\n%s", s)
	}
	if strings.Contains(s, "manual") {
		t.Errorf("expected old provenance value replaced, got:\n%s", s)
	}
	if strings.Contains(s, "old-key") {
		t.Errorf("expected old candidate value replaced, got:\n%s", s)
	}
	if strings.Count(s, "author:") != 1 {
		t.Errorf("expected exactly one author: line, got:\n%s", s)
	}
	if strings.Count(s, "provenance:") != 1 {
		t.Errorf("expected exactly one provenance: line, got:\n%s", s)
	}
	if strings.Count(s, "candidate:") != 1 {
		t.Errorf("expected exactly one candidate: line, got:\n%s", s)
	}
	if !strings.Contains(s, `  author: "`+ProceduralAuthor+`"`) {
		t.Errorf("expected new author line, got:\n%s", s)
	}
	if !strings.Contains(s, "  provenance: procedural") {
		t.Errorf("expected new provenance line, got:\n%s", s)
	}
	if !strings.Contains(s, "  candidate: procedural/candidates/repeated-success/probe-skill") {
		t.Errorf("expected new candidate line, got:\n%s", s)
	}
	// Preserves other metadata lines (version) byte-for-byte in original order.
	if !strings.Contains(s, `  version: "2.0"`) {
		t.Errorf("expected preserved version metadata line, got:\n%s", s)
	}
}

func TestStampProvenance_PreservesOtherMetadataOrder(t *testing.T) {
	fixture := `---
name: probe-skill
description: "Trigger: probe event. Reply with the probe result."
license: MIT
metadata:
  version: "1.0"
  extra-note: keep-me
---
## Activation Contract

Trigger: probe event.
`
	out, err := StampProvenance([]byte(fixture), "procedural/candidates/repeated-success/probe-skill")
	if err != nil {
		t.Fatalf("StampProvenance: %v", err)
	}
	s := string(out)
	idxVersion := strings.Index(s, "version:")
	idxExtra := strings.Index(s, "extra-note:")
	idxAuthor := strings.Index(s, "author:")
	if idxVersion < 0 || idxExtra < 0 || idxAuthor < 0 {
		t.Fatalf("expected version, extra-note and author lines present, got:\n%s", s)
	}
	if !(idxVersion < idxExtra && idxExtra < idxAuthor) {
		t.Errorf("expected order version < extra-note < (author, provenance, candidate), got:\n%s", s)
	}
}

func TestStampProvenance_RefusesMissingMetadataBlock(t *testing.T) {
	fixture := `---
name: probe-skill
description: "Trigger: probe event. Reply with the probe result."
license: MIT
---
## Activation Contract

Trigger: probe event.
`
	if _, err := StampProvenance([]byte(fixture), "procedural/candidates/repeated-success/probe-skill"); err == nil {
		t.Error("StampProvenance: expected refusal on a missing metadata: block, got nil error")
	}
}

func TestStampProvenance_RefusesMissingFrontmatterFence(t *testing.T) {
	fixture := "no frontmatter here at all\n"
	if _, err := StampProvenance([]byte(fixture), "procedural/candidates/repeated-success/probe-skill"); err == nil {
		t.Error("StampProvenance: expected refusal on a missing frontmatter fence, got nil error")
	}
}

func TestStampProvenance_Idempotent(t *testing.T) {
	once, err := StampProvenance([]byte(provenanceFixtureNoExisting), "procedural/candidates/repeated-success/probe-skill")
	if err != nil {
		t.Fatalf("StampProvenance (first): %v", err)
	}
	twice, err := StampProvenance(once, "procedural/candidates/repeated-success/probe-skill")
	if err != nil {
		t.Fatalf("StampProvenance (second): %v", err)
	}
	if string(once) != string(twice) {
		t.Errorf("StampProvenance is not idempotent:\nfirst:\n%s\nsecond:\n%s", once, twice)
	}
}

func TestStampProvenance_OutputPassesLintSkillFile(t *testing.T) {
	out, err := StampProvenance([]byte(provenanceFixtureNoExisting), "procedural/candidates/repeated-success/probe-skill")
	if err != nil {
		t.Fatalf("StampProvenance: %v", err)
	}
	hard, _ := LintSkillFile(out)
	if len(hard) != 0 {
		t.Errorf("expected zero hard lint errors on stamped output, got: %v\noutput:\n%s", hard, out)
	}
}
