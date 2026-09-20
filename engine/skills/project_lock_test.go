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
	// Exact indentation, key order and field names are pinned precisely by
	// TestSerializeProjectLock_GoldenBytes below (a modulo-2 length check
	// here would also pass for 4-space or tab indentation).
}

func TestSerializeProjectLock_GoldenBytes(t *testing.T) {
	l := ProjectLock{
		Version: 1,
		Skills: []ProjectLockEntry{
			{
				ID:         "alpha-skill",
				Provenance: "procedural",
				Candidate:  "procedural/candidates/repeated-success/alpha-skill",
				SHA256:     strings.Repeat("b", 64),
				Revision:   1,
				Targets:    []string{".claude/skills/alpha-skill/SKILL.md"},
			},
		},
	}
	want := `{
  "version": 1,
  "skills": [
    {
      "id": "alpha-skill",
      "provenance": "procedural",
      "candidate": "procedural/candidates/repeated-success/alpha-skill",
      "sha256": "` + strings.Repeat("b", 64) + `",
      "revision": 1,
      "targets": [
        ".claude/skills/alpha-skill/SKILL.md"
      ]
    }
  ]
}
`
	got, err := SerializeProjectLock(l)
	if err != nil {
		t.Fatalf("SerializeProjectLock: %v", err)
	}
	if string(got) != want {
		t.Errorf("SerializeProjectLock golden mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestSerializeProjectLock_StableForNonDuplicateEntries(t *testing.T) {
	// Regression guard for the stable-sort behavior once duplicate ids are
	// refused outright (review-24fc80ac3513305c, R3-duplicate-id-write-asymmetry):
	// two distinct ids that happen to sort adjacently still serialize in a
	// deterministic, order-independent way.
	l := ProjectLock{Version: 1, Skills: []ProjectLockEntry{
		{ID: "dup-a", Provenance: "procedural", Candidate: "first", SHA256: strings.Repeat("a", 64), Revision: 1, Targets: []string{"t1"}},
		{ID: "dup-b", Provenance: "procedural", Candidate: "second", SHA256: strings.Repeat("b", 64), Revision: 2, Targets: []string{"t2"}},
	}}
	data, err := SerializeProjectLock(l)
	if err != nil {
		t.Fatalf("SerializeProjectLock: %v", err)
	}
	idxFirst := strings.Index(string(data), `"first"`)
	idxSecond := strings.Index(string(data), `"second"`)
	if idxFirst < 0 || idxSecond < 0 {
		t.Fatalf("serialized output missing an expected candidate: %s", data)
	}
	if idxFirst > idxSecond {
		t.Errorf("expected dup-a before dup-b in serialized (sorted) output, got:\n%s", data)
	}
}

func TestSerializeProjectLock_RefusesDuplicateIDs(t *testing.T) {
	// R3-duplicate-id-write-asymmetry (review-24fc80ac3513305c): ParseProjectLock
	// refuses duplicate skill ids, so SerializeProjectLock must refuse them too —
	// a writer must never be able to emit a lock nobody can read back.
	l := ProjectLock{Version: 1, Skills: []ProjectLockEntry{
		{ID: "dup", Provenance: "procedural", Candidate: "first", SHA256: strings.Repeat("a", 64), Revision: 1, Targets: []string{"t1"}},
		{ID: "dup", Provenance: "procedural", Candidate: "second", SHA256: strings.Repeat("b", 64), Revision: 2, Targets: []string{"t2"}},
	}}
	if _, err := SerializeProjectLock(l); err == nil {
		t.Error("SerializeProjectLock: expected refusal on duplicate skill ids, got nil error")
	}
}

func TestParseProjectLock_DuplicateIDRefused(t *testing.T) {
	data := []byte(`{"version": 1, "skills": [
		{"id": "dup", "provenance": "procedural", "candidate": "c1", "sha256": "` + strings.Repeat("a", 64) + `", "revision": 1, "targets": []},
		{"id": "dup", "provenance": "procedural", "candidate": "c2", "sha256": "` + strings.Repeat("b", 64) + `", "revision": 2, "targets": []}
	]}`)
	if _, err := ParseProjectLock(data); err == nil {
		t.Error("ParseProjectLock: expected refusal on duplicate skill ids, got nil error")
	}
}

func TestParseProjectLock_TrailingObjectRefused(t *testing.T) {
	data := []byte(`{"version": 1, "skills": []}{"version": 1, "skills": []}`)
	if _, err := ParseProjectLock(data); err == nil {
		t.Error("ParseProjectLock: expected refusal on a second JSON value trailing the lock, got nil error")
	}
}

func TestParseProjectLock_TrailingBytesRefused(t *testing.T) {
	data := []byte(`{"version": 1, "skills": []}` + "\ngarbage")
	if _, err := ParseProjectLock(data); err == nil {
		t.Error("ParseProjectLock: expected refusal on trailing non-JSON bytes after the lock, got nil error")
	}
}

func TestSerializeProjectLock_NormalizesZeroVersion(t *testing.T) {
	data, err := SerializeProjectLock(ProjectLock{})
	if err != nil {
		t.Fatalf("SerializeProjectLock: %v", err)
	}
	if !strings.Contains(string(data), `"version": 1`) {
		t.Errorf("expected zero-value Version normalized to 1, got:\n%s", data)
	}
}

func TestSerializeProjectLock_RefusesUnsupportedVersion(t *testing.T) {
	if _, err := SerializeProjectLock(ProjectLock{Version: 2}); err == nil {
		t.Error("SerializeProjectLock: expected refusal for version 2, got nil error")
	}
}

func TestProceduralAuthor_ExactValue(t *testing.T) {
	if ProceduralAuthor != "labdrian-overlay procedural" {
		t.Errorf("ProceduralAuthor = %q, want %q", ProceduralAuthor, "labdrian-overlay procedural")
	}
}

func TestProjectLockRelPath_ExactValue(t *testing.T) {
	if ProjectLockRelPath != ".labdrian/procedural-skills.lock.json" {
		t.Errorf("ProjectLockRelPath = %q, want %q", ProjectLockRelPath, ".labdrian/procedural-skills.lock.json")
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
	if !strings.Contains(s, `  candidate: "procedural/candidates/repeated-success/probe-skill"`) {
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

// extractMetadataBlock returns the lines of the top-level metadata: block in
// s (the lines strictly between the metadata: line and the next top-level
// line or closing fence), so assertions can anchor to the block itself
// instead of scanning the whole document (R2-unanchored-test-assertions).
func extractMetadataBlock(t *testing.T, s string) string {
	t.Helper()
	lines := strings.Split(s, "\n")
	metaIdx := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "metadata:" {
			metaIdx = i
			break
		}
	}
	if metaIdx == -1 {
		t.Fatalf("no top-level metadata: block found in:\n%s", s)
	}
	end := metaIdx + 1
	for end < len(lines) {
		line := lines[end]
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			end++
			continue
		}
		break
	}
	return strings.Join(lines[metaIdx+1:end], "\n")
}

func TestStampProvenance_ReplacesExistingProvenanceLines(t *testing.T) {
	out, err := StampProvenance([]byte(provenanceFixtureWithExisting), "procedural/candidates/repeated-success/probe-skill")
	if err != nil {
		t.Fatalf("StampProvenance: %v", err)
	}
	s := string(out)
	block := extractMetadataBlock(t, s)

	if strings.Contains(block, "someone-else") {
		t.Errorf("expected old author value replaced within the metadata block, got:\n%s", block)
	}
	if strings.Contains(block, "manual") {
		t.Errorf("expected old provenance value replaced within the metadata block, got:\n%s", block)
	}
	if strings.Contains(block, "old-key") {
		t.Errorf("expected old candidate value replaced within the metadata block, got:\n%s", block)
	}
	if strings.Count(block, "author:") != 1 {
		t.Errorf("expected exactly one author: line within the metadata block, got:\n%s", block)
	}
	if strings.Count(block, "provenance:") != 1 {
		t.Errorf("expected exactly one provenance: line within the metadata block, got:\n%s", block)
	}
	if strings.Count(block, "candidate:") != 1 {
		t.Errorf("expected exactly one candidate: line within the metadata block, got:\n%s", block)
	}
	if !strings.Contains(s, `  author: "`+ProceduralAuthor+`"`) {
		t.Errorf("expected new author line, got:\n%s", s)
	}
	if !strings.Contains(s, "  provenance: procedural") {
		t.Errorf("expected new provenance line, got:\n%s", s)
	}
	if !strings.Contains(s, `  candidate: "procedural/candidates/repeated-success/probe-skill"`) {
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

// --- StampProvenance candidateKey validation (review-70263a4dee1c98b7) ---

func TestStampProvenance_RefusesEmptyCandidateKey(t *testing.T) {
	if _, err := StampProvenance([]byte(provenanceFixtureNoExisting), ""); err == nil {
		t.Error("StampProvenance: expected refusal on empty candidateKey, got nil error")
	}
}

func TestStampProvenance_RefusesCandidateKeyWithNewline(t *testing.T) {
	if _, err := StampProvenance([]byte(provenanceFixtureNoExisting), "procedural/candidates/repeated-success/probe\nskill"); err == nil {
		t.Error("StampProvenance: expected refusal on candidateKey containing a newline, got nil error")
	}
}

func TestStampProvenance_RefusesCandidateKeyWithControlChar(t *testing.T) {
	if _, err := StampProvenance([]byte(provenanceFixtureNoExisting), "probe\x01skill"); err == nil {
		t.Error("StampProvenance: expected refusal on candidateKey containing a control character, got nil error")
	}
}

func TestStampProvenance_RefusesCandidateKeyWithColonSpace(t *testing.T) {
	if _, err := StampProvenance([]byte(provenanceFixtureNoExisting), ": probe-skill"); err == nil {
		t.Error("StampProvenance: expected refusal on candidateKey starting with a colon indicator, got nil error")
	}
}

func TestStampProvenance_RefusesCandidateKeyWithLeadingQuote(t *testing.T) {
	if _, err := StampProvenance([]byte(provenanceFixtureNoExisting), `"probe-skill`); err == nil {
		t.Error("StampProvenance: expected refusal on candidateKey starting with a quote, got nil error")
	}
}

// R3-validation-branches-unexercised (review-24fc80ac3513305c): each of
// validateCandidateKey's five refusal branches gets its own test that
// reaches exactly that branch (not an earlier one) and asserts the specific
// error it returns, so the branches stay distinguishable. This is a
// test-only addition proving already-correct behavior — no production code
// changes for this finding, so RED is not expected.

func TestStampProvenance_RefusesCandidateKeyWithLeadingSpace(t *testing.T) {
	_, err := StampProvenance([]byte(provenanceFixtureNoExisting), " probe-skill")
	if err == nil {
		t.Fatal("StampProvenance: expected refusal on candidateKey with a leading space, got nil error")
	}
	if !strings.Contains(err.Error(), "leading or trailing spaces") {
		t.Errorf("expected leading/trailing-spaces error, got: %v", err)
	}
}

func TestStampProvenance_RefusesCandidateKeyWithTrailingSpace(t *testing.T) {
	_, err := StampProvenance([]byte(provenanceFixtureNoExisting), "probe-skill ")
	if err == nil {
		t.Fatal("StampProvenance: expected refusal on candidateKey with a trailing space, got nil error")
	}
	if !strings.Contains(err.Error(), "leading or trailing spaces") {
		t.Errorf("expected leading/trailing-spaces error, got: %v", err)
	}
}

func TestStampProvenance_RefusesCandidateKeyWithInteriorColonSpace(t *testing.T) {
	// Distinct from TestStampProvenance_RefusesCandidateKeyWithColonSpace,
	// which starts with ":" and hits the leading-indicator branch instead.
	_, err := StampProvenance([]byte(provenanceFixtureNoExisting), "procedural/candidates: repeated-success")
	if err == nil {
		t.Fatal("StampProvenance: expected refusal on candidateKey with an interior \": \", got nil error")
	}
	if !strings.Contains(err.Error(), `": "`) {
		t.Errorf("expected the interior-sequence error naming %q, got: %v", ": ", err)
	}
}

func TestStampProvenance_RefusesCandidateKeyWithInteriorHash(t *testing.T) {
	_, err := StampProvenance([]byte(provenanceFixtureNoExisting), "procedural/candidates #repeated-success")
	if err == nil {
		t.Fatal("StampProvenance: expected refusal on candidateKey with an interior \" #\", got nil error")
	}
	if !strings.Contains(err.Error(), `" #"`) {
		t.Errorf("expected the interior-sequence error naming %q, got: %v", " #", err)
	}
}

func TestStampProvenance_RefusesCandidateKeyWithInteriorDoubleQuote(t *testing.T) {
	// R3-candidate-escape-unproved (review-24fc80ac3513305c), decision (a):
	// an interior double quote is refused outright rather than relying on
	// the escaper, which stays as defense in depth only.
	if _, err := StampProvenance([]byte(provenanceFixtureNoExisting), `probe"skill`); err == nil {
		t.Error("StampProvenance: expected refusal on candidateKey containing an interior double quote, got nil error")
	}
}

func TestStampProvenance_RefusesCandidateKeyWithInteriorBackslash(t *testing.T) {
	// R3-candidate-escape-unproved (review-24fc80ac3513305c), decision (a):
	// an interior backslash is refused outright rather than relying on the
	// escaper, which stays as defense in depth only.
	if _, err := StampProvenance([]byte(provenanceFixtureNoExisting), `probe\skill`); err == nil {
		t.Error("StampProvenance: expected refusal on candidateKey containing an interior backslash, got nil error")
	}
}

func TestStampProvenance_QuotesAndEscapesCandidateValue(t *testing.T) {
	out, err := StampProvenance([]byte(provenanceFixtureNoExisting), "procedural/candidates/repeated-success/probe-skill")
	if err != nil {
		t.Fatalf("StampProvenance: %v", err)
	}
	if !strings.Contains(string(out), `candidate: "procedural/candidates/repeated-success/probe-skill"`) {
		t.Errorf("expected double-quoted candidate value, got:\n%s", out)
	}
}

// --- StampProvenance inline metadata refusal (review-70263a4dee1c98b7) ---

func TestStampProvenance_RefusesInlineMetadataFlowMapping(t *testing.T) {
	fixture := `---
name: probe-skill
description: "Trigger: probe event. Reply with the probe result."
license: MIT
metadata: {}
---
## Activation Contract

Trigger: probe event.
`
	if _, err := StampProvenance([]byte(fixture), "procedural/candidates/repeated-success/probe-skill"); err == nil {
		t.Error("StampProvenance: expected refusal on inline flow-mapping metadata value, got nil error")
	}
}

func TestStampProvenance_RefusesInlineMetadataNull(t *testing.T) {
	fixture := `---
name: probe-skill
description: "Trigger: probe event. Reply with the probe result."
license: MIT
metadata: null
---
## Activation Contract

Trigger: probe event.
`
	if _, err := StampProvenance([]byte(fixture), "procedural/candidates/repeated-success/probe-skill"); err == nil {
		t.Error("StampProvenance: expected refusal on inline null metadata value, got nil error")
	}
}

// --- StampProvenance stamp indentation (review-70263a4dee1c98b7) ---

func TestStampProvenance_PreservesExistingFourSpaceIndent(t *testing.T) {
	fixture := `---
name: probe-skill
description: "Trigger: probe event. Reply with the probe result."
license: MIT
metadata:
    version: "1.0"
---
## Activation Contract

Trigger: probe event.
`
	out, err := StampProvenance([]byte(fixture), "procedural/candidates/repeated-success/probe-skill")
	if err != nil {
		t.Fatalf("StampProvenance: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "    version: \"1.0\"") {
		t.Fatalf("expected preserved four-space version line, got:\n%s", s)
	}
	if !strings.Contains(s, `    author: "`+ProceduralAuthor+`"`) {
		t.Errorf("expected stamped author line to reuse the block's four-space indent, got:\n%s", s)
	}
	if !strings.Contains(s, "    provenance: procedural") {
		t.Errorf("expected stamped provenance line to reuse the block's four-space indent, got:\n%s", s)
	}
	if !strings.Contains(s, `    candidate: "procedural/candidates/repeated-success/probe-skill"`) {
		t.Errorf("expected stamped candidate line to reuse the block's four-space indent, got:\n%s", s)
	}
}

func TestStampProvenance_FallsBackToTwoSpaceIndentForEmptyBlock(t *testing.T) {
	fixture := `---
name: probe-skill
description: "Trigger: probe event. Reply with the probe result."
license: MIT
metadata:
---
## Activation Contract

Trigger: probe event.
`
	out, err := StampProvenance([]byte(fixture), "procedural/candidates/repeated-success/probe-skill")
	if err != nil {
		t.Fatalf("StampProvenance: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, `  author: "`+ProceduralAuthor+`"`) {
		t.Errorf("expected two-space fallback indent on the author line for an empty metadata block, got:\n%s", s)
	}
	if !strings.Contains(s, "  provenance: procedural") {
		t.Errorf("expected two-space fallback indent on the provenance line for an empty metadata block, got:\n%s", s)
	}
	if !strings.Contains(s, `  candidate: "procedural/candidates/repeated-success/probe-skill"`) {
		t.Errorf("expected two-space fallback indent on the candidate line for an empty metadata block, got:\n%s", s)
	}
}
