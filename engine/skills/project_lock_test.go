package skills

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
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
	// Restored reversed-input half (review-d89971d41a526146): asserting the
	// sorted order alone only proves that sorting happened, which
	// TestSerializeProjectLock_SortsByID already proves. Order-independence
	// needs a second serialization of the reversed input compared byte for
	// byte — here at the adjacent-sorting-ids input shape.
	reversed := ProjectLock{Version: 1, Skills: []ProjectLockEntry{l.Skills[1], l.Skills[0]}}
	second, err := SerializeProjectLock(reversed)
	if err != nil {
		t.Fatalf("SerializeProjectLock (reversed input): %v", err)
	}
	if string(data) != string(second) {
		t.Errorf("SerializeProjectLock is not order-independent for adjacent ids:\nfirst:\n%s\nsecond:\n%s", data, second)
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
	data, err := SerializeProjectLock(l)
	if err == nil {
		t.Fatal("SerializeProjectLock: expected refusal on duplicate skill ids, got nil error")
	}
	// review-d89971d41a526146: a refusal must produce no bytes at all, so a
	// future partial-write regression cannot hand a caller half a lock.
	if data != nil {
		t.Errorf("expected nil bytes on refusal, got:\n%s", data)
	}
	if !strings.Contains(err.Error(), "duplicate skill id") {
		t.Errorf("expected the refusal to name the defect, got: %v", err)
	}
	if !strings.Contains(err.Error(), `"dup"`) {
		t.Errorf("expected the refusal to name the duplicated id %q, got: %v", "dup", err)
	}
}

func TestParseProjectLock_DuplicateIDRefused(t *testing.T) {
	data := []byte(`{"version": 1, "skills": [
		{"id": "dup", "provenance": "procedural", "candidate": "c1", "sha256": "` + strings.Repeat("a", 64) + `", "revision": 1, "targets": []},
		{"id": "dup", "provenance": "procedural", "candidate": "c2", "sha256": "` + strings.Repeat("b", 64) + `", "revision": 2, "targets": []}
	]}`)
	_, err := ParseProjectLock(data)
	if err == nil {
		t.Fatal("ParseProjectLock: expected refusal on duplicate skill ids, got nil error")
	}
	// Pinned symmetrically with the serialize half (review-d89971d41a526146):
	// closing only one side would leave the asymmetry the finding was about
	// half open.
	if !strings.Contains(err.Error(), "duplicate skill id") {
		t.Errorf("expected the refusal to name the defect, got: %v", err)
	}
	if !strings.Contains(err.Error(), `"dup"`) {
		t.Errorf("expected the refusal to name the duplicated id %q, got: %v", "dup", err)
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

func TestSerializeProjectLock_MatchesSharedSkillstaleFixture(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	fixturePath := filepath.Join(repoRoot, "longterm-mem", "internal", "skillstale", "testdata", "procedural-skills.lock.json")
	want, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read shared lock fixture %s: %v", fixturePath, err)
	}
	lock, err := ParseProjectLock(want)
	if err != nil {
		t.Fatalf("ParseProjectLock(shared fixture): %v", err)
	}
	got, err := SerializeProjectLock(lock)
	if err != nil {
		t.Fatalf("SerializeProjectLock(shared fixture): %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("SerializeProjectLock does not reproduce the shared fixture byte-for-byte:\ngot:\n%s\nwant:\n%s", got, want)
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
	_, err := StampProvenance([]byte(provenanceFixtureNoExisting), `probe"skill`)
	if err == nil {
		t.Fatal("StampProvenance: expected refusal on candidateKey containing an interior double quote, got nil error")
	}
	// review-d89971d41a526146: the quote and backslash branches used to share
	// one message, so neither test could prove it reached its own branch.
	if !strings.Contains(err.Error(), "double quote") {
		t.Errorf("expected the refusal to name the double quote, got: %v", err)
	}
	if strings.Contains(err.Error(), "backslash") {
		t.Errorf("expected a double-quote-specific refusal, got the backslash branch: %v", err)
	}
}

func TestStampProvenance_RefusesCandidateKeyWithInteriorBackslash(t *testing.T) {
	// R3-candidate-escape-unproved (review-24fc80ac3513305c), decision (a):
	// an interior backslash is refused outright rather than relying on the
	// escaper, which stays as defense in depth only.
	_, err := StampProvenance([]byte(provenanceFixtureNoExisting), `probe\skill`)
	if err == nil {
		t.Fatal("StampProvenance: expected refusal on candidateKey containing an interior backslash, got nil error")
	}
	// review-d89971d41a526146: see the double-quote sibling above — the two
	// branches now name their own character, so each test proves which one it
	// reached.
	if !strings.Contains(err.Error(), "backslash") {
		t.Errorf("expected the refusal to name the backslash, got: %v", err)
	}
	if strings.Contains(err.Error(), "double quote") {
		t.Errorf("expected a backslash-specific refusal, got the double-quote branch: %v", err)
	}
}

// --- escapeYAMLDoubleQuoted (review-d89971d41a526146) ---

// escapeYAMLDoubleQuoted's two branches became unreachable through
// StampProvenance once validateCandidateKey started refusing both characters
// outright. The escaper is kept as defense in depth for the next caller
// (3b-i.5's PlanProjectRegister writes candidate values), per the decision
// already recorded in tasks.md 3a-i.6 and in its doc comment — so it is
// tested directly here instead of being left unexercised.
func TestEscapeYAMLDoubleQuoted_EscapesQuoteAndBackslash(t *testing.T) {
	cases := []struct{ in, want string }{
		{`a"b`, `a\"b`},
		{`a\b`, `a\\b`},
		// Backslashes must be escaped before quotes: reversing the two
		// ReplaceAll calls would yield `a\\\\"b` here instead.
		{`a\"b`, `a\\\"b`},
		{`plain`, `plain`},
	}
	for _, c := range cases {
		if got := escapeYAMLDoubleQuoted(c.in); got != c.want {
			t.Errorf("escapeYAMLDoubleQuoted(%q) = %q, want %q", c.in, got, c.want)
		}
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

// --- HashSkill (task 3a-ii.1) ---

func TestHashSkill_KnownVectors(t *testing.T) {
	// Published SHA-256 vectors, lowercase hex, over the exact input bytes.
	cases := []struct{ in, want string }{
		{"", "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
		{"abc", "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"},
	}
	for _, c := range cases {
		if got := HashSkill([]byte(c.in)); got != c.want {
			t.Errorf("HashSkill(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestHashSkill_DoesNotNormalizeBytes(t *testing.T) {
	// design.md, "Ownership by hash" — no line-ending conversion, no whitespace trim, no
	// frontmatter canonicalization. Each variant must hash differently, so a
	// human edit of any single byte is detectable.
	variants := []string{"a: 1\n", "a: 1\r\n", "a: 1\n\n", "a: 1 \n"}
	seen := make(map[string]string, len(variants))
	for _, v := range variants {
		h := HashSkill([]byte(v))
		if prev, ok := seen[h]; ok {
			t.Errorf("HashSkill collapsed %q and %q to the same hash %s", prev, v, h)
		}
		seen[h] = v
		if len(h) != 64 || strings.ToLower(h) != h {
			t.Errorf("HashSkill(%q) = %q, want 64 lowercase hex characters", v, h)
		}
	}
}

// --- ValidateCandidateKey (task 3a-ii.1) ---

func TestValidateCandidateKey_AcceptsRepeatedSuccessShape(t *testing.T) {
	if err := ValidateCandidateKey("procedural/candidates/repeated-success/probe-skill"); err != nil {
		t.Errorf("ValidateCandidateKey: unexpected refusal: %v", err)
	}
}

func TestValidateCandidateKey_AcceptsFailureRecoveryShape(t *testing.T) {
	if err := ValidateCandidateKey("procedural/candidates/failure-recovery/build-break/rerun-codegen"); err != nil {
		t.Errorf("ValidateCandidateKey: unexpected refusal: %v", err)
	}
}

func TestValidateCandidateKey_RefusesEmptySegment(t *testing.T) {
	err := ValidateCandidateKey("procedural/candidates/repeated-success/")
	if err == nil {
		t.Fatal("ValidateCandidateKey: expected refusal on an empty trailing segment, got nil error")
	}
	if !strings.Contains(err.Error(), "must not be empty") {
		t.Errorf("expected an empty-segment refusal, got: %v", err)
	}
}

func TestValidateCandidateKey_RefusesNonNormalizedSegment(t *testing.T) {
	// "Probe_Skill" normalizes to "probe-skill", so it is not already
	// normalized and NormalizeSlug is not idempotent on it.
	err := ValidateCandidateKey("procedural/candidates/repeated-success/Probe_Skill")
	if err == nil {
		t.Fatal("ValidateCandidateKey: expected refusal on a non-normalized segment, got nil error")
	}
	if !strings.Contains(err.Error(), "not normalized") {
		t.Errorf("expected a non-normalized-segment refusal, got: %v", err)
	}
}

func TestValidateCandidateKey_RefusesUnknownKind(t *testing.T) {
	err := ValidateCandidateKey("procedural/candidates/unknown-kind/probe-skill")
	if err == nil {
		t.Fatal("ValidateCandidateKey: expected refusal on an unknown candidate kind, got nil error")
	}
	if !strings.Contains(err.Error(), "unknown candidate kind") {
		t.Errorf("expected an unknown-kind refusal, got: %v", err)
	}
}

func TestValidateCandidateKey_RefusesWrongSegmentCount(t *testing.T) {
	// repeated-success takes exactly one slug; failure-recovery exactly two.
	wrong := []string{
		"procedural/candidates/repeated-success/probe-skill/extra",
		"procedural/candidates/failure-recovery/build-break",
	}
	for _, key := range wrong {
		err := ValidateCandidateKey(key)
		if err == nil {
			t.Errorf("ValidateCandidateKey(%q): expected refusal on the wrong segment count, got nil error", key)
			continue
		}
		if !strings.Contains(err.Error(), "segments") {
			t.Errorf("ValidateCandidateKey(%q): expected a segment-count refusal, got: %v", key, err)
		}
	}
}

func TestValidateCandidateKey_RefusesWrongPrefix(t *testing.T) {
	if err := ValidateCandidateKey("repeated-success/probe-skill"); err == nil {
		t.Error("ValidateCandidateKey: expected refusal on a key outside procedural/candidates/, got nil error")
	}
}

func TestValidateCandidateKey_RefusesEmptyKey(t *testing.T) {
	if err := ValidateCandidateKey(""); err == nil {
		t.Error("ValidateCandidateKey: expected refusal on an empty key, got nil error")
	}
}

func TestValidateCandidateKey_ShapeValidKeysAreAlsoStampSafe(t *testing.T) {
	// Non-contradiction proof between the two validators: the exported
	// ValidateCandidateKey checks the item-30 key SHAPE, the unexported
	// validateCandidateKey checks YAML stamping SAFETY. The shape-valid set is
	// a strict subset of the stamp-safe set, so the two can never disagree on
	// a key ValidateCandidateKey accepts.
	keys := []string{
		"procedural/candidates/repeated-success/probe-skill",
		"procedural/candidates/repeated-success/a",
		"procedural/candidates/failure-recovery/build-break/rerun-codegen",
		"procedural/candidates/failure-recovery/a1/b2",
	}
	for _, key := range keys {
		if err := ValidateCandidateKey(key); err != nil {
			t.Errorf("ValidateCandidateKey(%q): unexpected refusal: %v", key, err)
			continue
		}
		if err := validateCandidateKey(key); err != nil {
			t.Errorf("shape-valid key %q was refused by the stamp-safety validator: %v", key, err)
		}
	}
}

// --- EvaluateOwnership (task 3a-ii.1) ---

// fakeDirEntry is the minimal fs.DirEntry an injected readDir needs to return:
// EvaluateOwnership only ever reads Name().
type fakeDirEntry struct{ name string }

func (f fakeDirEntry) Name() string               { return f.name }
func (f fakeDirEntry) IsDir() bool                { return false }
func (f fakeDirEntry) Type() fs.FileMode          { return 0 }
func (f fakeDirEntry) Info() (fs.FileInfo, error) { return nil, fs.ErrNotExist }

// fakeFS builds in-memory readFile/readDir funcs over an absolute-path map.
// Every EvaluateOwnership call in this group reads only through these
// injected funcs, and for all but one test the t.TempDir() root is a pure
// string prefix with nothing created under it.
//
// The exception is TestEvaluateOwnership_ReadsOnlyThroughInjectedReaders,
// which deliberately does os.MkdirAll/os.WriteFile under its own t.TempDir()
// so that a real file exists where the injected map has none — that is what
// proves EvaluateOwnership never falls back to os.ReadFile/os.ReadDir. Those
// writes are safe because t.TempDir() is a fresh per-test directory the
// testing package removes afterwards: never $HOME, never .claude/skills,
// .agents/skills, .pi or skills-lock.json.
func fakeFS(files map[string]string) (func(string) ([]byte, error), func(string) ([]fs.DirEntry, error)) {
	readFile := func(p string) ([]byte, error) {
		content, ok := files[p]
		if !ok {
			return nil, fs.ErrNotExist
		}
		return []byte(content), nil
	}
	readDir := func(dir string) ([]fs.DirEntry, error) {
		var names []string
		for p := range files {
			if filepath.Dir(p) == dir {
				names = append(names, filepath.Base(p))
			}
		}
		if len(names) == 0 {
			return nil, fs.ErrNotExist
		}
		sort.Strings(names)
		entries := make([]fs.DirEntry, 0, len(names))
		for _, n := range names {
			entries = append(entries, fakeDirEntry{name: n})
		}
		return entries, nil
	}
	return readFile, readDir
}

const ownershipSkillBody = "---\nname: probe-skill\n---\nbody\n"

func ownershipEntry(sha string) ProjectLockEntry {
	return ProjectLockEntry{
		ID:         "probe-skill",
		Provenance: "procedural",
		Candidate:  "procedural/candidates/repeated-success/probe-skill",
		SHA256:     sha,
		Revision:   1,
		Targets: []string{
			".claude/skills/probe-skill/SKILL.md",
			".agents/skills/probe-skill/SKILL.md",
		},
	}
}

func ownershipFiles(root string, bodies map[string]string) map[string]string {
	out := make(map[string]string, len(bodies))
	for rel, body := range bodies {
		out[filepath.Join(root, filepath.FromSlash(rel))] = body
	}
	return out
}

func TestEvaluateOwnership_AgentOwnedWhenEveryTargetMatches(t *testing.T) {
	root := t.TempDir()
	readFile, readDir := fakeFS(ownershipFiles(root, map[string]string{
		".claude/skills/probe-skill/SKILL.md": ownershipSkillBody,
		".agents/skills/probe-skill/SKILL.md": ownershipSkillBody,
	}))
	got := EvaluateOwnership(root, ownershipEntry(HashSkill([]byte(ownershipSkillBody))), readFile, readDir, identityResolver)
	if !got.AgentOwned {
		t.Errorf("expected agent-owned, got %+v", got)
	}
	if got.Reason != "" {
		t.Errorf("expected an empty reason for agent-owned, got %q", got.Reason)
	}
}

func TestEvaluateOwnership_HashMismatch(t *testing.T) {
	root := t.TempDir()
	readFile, readDir := fakeFS(ownershipFiles(root, map[string]string{
		".claude/skills/probe-skill/SKILL.md": ownershipSkillBody + "human edit\n",
		".agents/skills/probe-skill/SKILL.md": ownershipSkillBody,
	}))
	got := EvaluateOwnership(root, ownershipEntry(HashSkill([]byte(ownershipSkillBody))), readFile, readDir, identityResolver)
	if got.AgentOwned {
		t.Errorf("expected human-owned on a hash mismatch, got %+v", got)
	}
	if got.Reason != "hash-mismatch .claude/skills/probe-skill/SKILL.md" {
		t.Errorf("Reason = %q, want %q", got.Reason, "hash-mismatch .claude/skills/probe-skill/SKILL.md")
	}
}

func TestEvaluateOwnership_Missing(t *testing.T) {
	root := t.TempDir()
	readFile, readDir := fakeFS(ownershipFiles(root, map[string]string{
		".agents/skills/probe-skill/SKILL.md": ownershipSkillBody,
	}))
	got := EvaluateOwnership(root, ownershipEntry(HashSkill([]byte(ownershipSkillBody))), readFile, readDir, identityResolver)
	if got.AgentOwned {
		t.Errorf("expected human-owned on a missing target, got %+v", got)
	}
	if got.Reason != "missing .claude/skills/probe-skill/SKILL.md" {
		t.Errorf("Reason = %q, want %q", got.Reason, "missing .claude/skills/probe-skill/SKILL.md")
	}
}

func TestEvaluateOwnership_ExtraEntry(t *testing.T) {
	root := t.TempDir()
	readFile, readDir := fakeFS(ownershipFiles(root, map[string]string{
		".claude/skills/probe-skill/SKILL.md":  ownershipSkillBody,
		".claude/skills/probe-skill/README.md": "human note\n",
		".agents/skills/probe-skill/SKILL.md":  ownershipSkillBody,
	}))
	got := EvaluateOwnership(root, ownershipEntry(HashSkill([]byte(ownershipSkillBody))), readFile, readDir, identityResolver)
	if got.AgentOwned {
		t.Errorf("expected human-owned on an extra directory entry, got %+v", got)
	}
	if got.Reason != "extra-entry .claude/skills/probe-skill/README.md" {
		t.Errorf("Reason = %q, want %q", got.Reason, "extra-entry .claude/skills/probe-skill/README.md")
	}
}

func TestEvaluateOwnership_NotInLock(t *testing.T) {
	root := t.TempDir()
	readFile, readDir := fakeFS(ownershipFiles(root, map[string]string{
		".claude/skills/probe-skill/SKILL.md": ownershipSkillBody,
	}))
	// The zero entry is what a caller passes when the lock lookup missed.
	got := EvaluateOwnership(root, ProjectLockEntry{}, readFile, readDir, identityResolver)
	if got.AgentOwned {
		t.Errorf("expected human-owned for a skill absent from the lock, got %+v", got)
	}
	if got.Reason != "not-in-lock" {
		t.Errorf("Reason = %q, want %q", got.Reason, "not-in-lock")
	}
}

func TestEvaluateOwnership_ReportsFirstFailingReasonOnly(t *testing.T) {
	// The first target is missing and the second mismatches; only the first
	// failing reason is reported (design.md, "Ownership by hash"), never a list.
	root := t.TempDir()
	readFile, readDir := fakeFS(ownershipFiles(root, map[string]string{
		".agents/skills/probe-skill/SKILL.md": ownershipSkillBody + "drift\n",
	}))
	got := EvaluateOwnership(root, ownershipEntry(HashSkill([]byte(ownershipSkillBody))), readFile, readDir, identityResolver)
	if got.Reason != "missing .claude/skills/probe-skill/SKILL.md" {
		t.Errorf("Reason = %q, want only the first failing reason %q", got.Reason, "missing .claude/skills/probe-skill/SKILL.md")
	}
	if strings.Contains(got.Reason, "hash-mismatch") {
		t.Errorf("expected a single reason, got a combined one: %q", got.Reason)
	}
}

func TestEvaluateOwnership_ReadsOnlyThroughInjectedReaders(t *testing.T) {
	// A real file exists on disk under the root, but the injected readFile
	// does not know it. EvaluateOwnership must report it missing — proving it
	// never falls back to os.ReadFile/os.ReadDir (design.md:507).
	root := t.TempDir()
	real := filepath.Join(root, ".claude", "skills", "probe-skill")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(real, "SKILL.md"), []byte(ownershipSkillBody), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	readFile, readDir := fakeFS(map[string]string{})
	got := EvaluateOwnership(root, ownershipEntry(HashSkill([]byte(ownershipSkillBody))), readFile, readDir, identityResolver)
	if got.AgentOwned || got.Reason != "missing .claude/skills/probe-skill/SKILL.md" {
		t.Errorf("EvaluateOwnership consulted something other than its injected readers: %+v", got)
	}
}

func TestEvaluateOwnership_UnreadableTargetDirectoryIsHumanOwned(t *testing.T) {
	// The file reads and hashes, but its directory cannot be listed: the
	// exactly-one-entry condition is unprovable, so the safe direction is
	// human-owned.
	root := t.TempDir()
	files := ownershipFiles(root, map[string]string{
		".claude/skills/probe-skill/SKILL.md": ownershipSkillBody,
		".agents/skills/probe-skill/SKILL.md": ownershipSkillBody,
	})
	readFile, _ := fakeFS(files)
	readDir := func(string) ([]fs.DirEntry, error) { return nil, fs.ErrPermission }
	got := EvaluateOwnership(root, ownershipEntry(HashSkill([]byte(ownershipSkillBody))), readFile, readDir, identityResolver)
	if got.AgentOwned {
		t.Errorf("expected human-owned when a target directory cannot be listed, got %+v", got)
	}
	if got.Reason != "missing .claude/skills/probe-skill" {
		t.Errorf("Reason = %q, want %q", got.Reason, "missing .claude/skills/probe-skill")
	}
}

func TestEvaluateOwnership_EntryInLockWithNoTargetsIsMalformed(t *testing.T) {
	// An entry that IS in the lock but records no targets is malformed, not
	// unregistered: reporting "not-in-lock" would conflate a broken entry
	// with a skill the agent never wrote.
	root := t.TempDir()
	readFile, readDir := fakeFS(ownershipFiles(root, map[string]string{
		".claude/skills/probe-skill/SKILL.md": ownershipSkillBody,
	}))
	e := ownershipEntry(HashSkill([]byte(ownershipSkillBody)))
	e.Targets = nil
	got := EvaluateOwnership(root, e, readFile, readDir, identityResolver)
	if got.AgentOwned {
		t.Errorf("expected human-owned for an entry with no targets, got %+v", got)
	}
	if got.Reason != "no-targets probe-skill" {
		t.Errorf("Reason = %q, want %q", got.Reason, "no-targets probe-skill")
	}
}

// recordingFS wraps fakeFS and records every path handed to readFile/readDir,
// so a test can prove no read was attempted outside the project root.
func recordingFS(files map[string]string) (func(string) ([]byte, error), func(string) ([]fs.DirEntry, error), *[]string) {
	inner, innerDir := fakeFS(files)
	var seen []string
	readFile := func(p string) ([]byte, error) {
		seen = append(seen, p)
		return inner(p)
	}
	readDir := func(p string) ([]fs.DirEntry, error) {
		seen = append(seen, p)
		return innerDir(p)
	}
	return readFile, readDir, &seen
}

func TestEvaluateOwnership_RejectsTargetsOutsideRoot(t *testing.T) {
	root := t.TempDir()
	cases := []struct {
		name   string
		target string
	}{
		{"parent traversal", "../../etc/passwd"},
		{"absolute path", "/etc/passwd"},
		{"empty target", ""},
		{"escapes after cleaning", ".claude/skills/../../../../etc/passwd"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			readFile, readDir, seen := recordingFS(ownershipFiles(root, map[string]string{
				".claude/skills/probe-skill/SKILL.md": ownershipSkillBody,
			}))
			e := ownershipEntry(HashSkill([]byte(ownershipSkillBody)))
			e.Targets = []string{tc.target}
			got := EvaluateOwnership(root, e, readFile, readDir, identityResolver)
			if got.AgentOwned {
				t.Errorf("target %q was reported agent-owned: %+v", tc.target, got)
			}
			if got.Reason != "invalid-target "+tc.target {
				t.Errorf("Reason = %q, want %q", got.Reason, "invalid-target "+tc.target)
			}
			for _, p := range *seen {
				if !strings.HasPrefix(filepath.Clean(p)+string(filepath.Separator), filepath.Clean(root)+string(filepath.Separator)) {
					t.Errorf("read attempted outside the project root: %q", p)
				}
			}
			if len(*seen) != 0 {
				t.Errorf("a rejected target must be refused before any read, got reads: %v", *seen)
			}
		})
	}
}

// identityResolver is the injected symlink resolver for every test whose
// paths contain no symlinks at all: it returns the path unchanged, which is
// exactly what a real resolver returns for a symlink-free path.
func identityResolver(p string) (string, error) { return p, nil }

// mappingResolver builds a resolver that rewrites any path whose prefix is a
// key of links to the mapped destination, mirroring what a real
// symlink-resolving caller returns for a symlinked component. Paths matching
// no key come back unchanged.
func mappingResolver(links map[string]string) func(string) (string, error) {
	return func(p string) (string, error) {
		for from, to := range links {
			if p == from {
				return to, nil
			}
			if strings.HasPrefix(p, from+string(filepath.Separator)) {
				return to + p[len(from):], nil
			}
		}
		return p, nil
	}
}

// --- SEC-2: a symlinked component must not let a read escape the root ---

func TestEvaluateOwnership_SymlinkedComponentEscapingRootIsRefused(t *testing.T) {
	// root/link is a symlink to a sibling directory outside root. The lexical
	// containment check alone accepts "link/SKILL.md" (it names no ".."), so
	// without symlink resolution the bytes under <base>/secret would be hashed
	// and reported agent-owned.
	base := t.TempDir()
	root := filepath.Join(base, "proj")
	secret := filepath.Join(base, "secret")

	resolve := mappingResolver(map[string]string{filepath.Join(root, "link"): secret})
	// The injected readers follow the link exactly as os.ReadFile/os.ReadDir
	// would, so this reproduces the finding: without symlink resolution the
	// bytes under <base>/secret hash clean and the entry reads agent-owned.
	inner, innerDir, seen := recordingFS(map[string]string{
		filepath.Join(secret, "SKILL.md"): ownershipSkillBody,
	})
	readFile := func(p string) ([]byte, error) { q, _ := resolve(p); return inner(q) }
	readDir := func(p string) ([]fs.DirEntry, error) { q, _ := resolve(p); return innerDir(q) }

	e := ownershipEntry(HashSkill([]byte(ownershipSkillBody)))
	e.Targets = []string{"link/SKILL.md"}

	got := EvaluateOwnership(root, e, readFile, readDir, resolve)
	if got.AgentOwned {
		t.Errorf("a symlink out of the root was reported agent-owned: %+v", got)
	}
	if got.Reason != "escapes-root link/SKILL.md" {
		t.Errorf("Reason = %q, want %q", got.Reason, "escapes-root link/SKILL.md")
	}
	if len(*seen) != 0 {
		t.Errorf("a target resolving outside the root must be refused before any read, got reads: %v", *seen)
	}
}

func TestEvaluateOwnership_SymlinkedRootStaysAgentOwned(t *testing.T) {
	// The root itself is reached through a symlink (the /tmp -> /private/tmp
	// shape). Containment is checked between RESOLVED paths, so the entry is
	// still agent-owned.
	base := t.TempDir()
	root := filepath.Join(base, "link-to-proj")
	realRoot := filepath.Join(base, "real-proj")

	readFile, readDir := fakeFS(ownershipFiles(realRoot, map[string]string{
		".claude/skills/probe-skill/SKILL.md": ownershipSkillBody,
		".agents/skills/probe-skill/SKILL.md": ownershipSkillBody,
	}))
	// Reads still go to the lexical path under root; the fake FS is keyed on
	// the real root, so map both through the resolver for the read side too.
	resolve := mappingResolver(map[string]string{root: realRoot})
	readFileVia := func(p string) ([]byte, error) {
		q, _ := resolve(p)
		return readFile(q)
	}
	readDirVia := func(p string) ([]fs.DirEntry, error) {
		q, _ := resolve(p)
		return readDir(q)
	}

	got := EvaluateOwnership(root, ownershipEntry(HashSkill([]byte(ownershipSkillBody))), readFileVia, readDirVia, resolve)
	if !got.AgentOwned {
		t.Errorf("a symlinked root must not flip the verdict, got %+v", got)
	}
}

func TestEvaluateOwnership_ResolverFailureIsHumanOwned(t *testing.T) {
	root := t.TempDir()
	readFile, readDir, seen := recordingFS(ownershipFiles(root, map[string]string{
		".claude/skills/probe-skill/SKILL.md": ownershipSkillBody,
	}))
	resolve := func(p string) (string, error) {
		if p == filepath.Clean(root) {
			return p, nil
		}
		return "", fs.ErrInvalid
	}
	got := EvaluateOwnership(root, ownershipEntry(HashSkill([]byte(ownershipSkillBody))), readFile, readDir, resolve)
	if got.AgentOwned {
		t.Errorf("an unresolvable target must be human-owned, got %+v", got)
	}
	if got.Reason != "unresolved-target .claude/skills/probe-skill/SKILL.md" {
		t.Errorf("Reason = %q, want %q", got.Reason, "unresolved-target .claude/skills/probe-skill/SKILL.md")
	}
	if len(*seen) != 0 {
		t.Errorf("an unresolvable target must be refused before any read, got reads: %v", *seen)
	}
}

func TestEvaluateOwnership_RootResolverFailureIsHumanOwned(t *testing.T) {
	root := t.TempDir()
	readFile, readDir, seen := recordingFS(ownershipFiles(root, map[string]string{
		".claude/skills/probe-skill/SKILL.md": ownershipSkillBody,
	}))
	resolve := func(string) (string, error) { return "", fs.ErrPermission }
	got := EvaluateOwnership(root, ownershipEntry(HashSkill([]byte(ownershipSkillBody))), readFile, readDir, resolve)
	if got.AgentOwned {
		t.Errorf("an unresolvable root must be human-owned, got %+v", got)
	}
	if got.Reason != "unresolved-root "+root {
		t.Errorf("Reason = %q, want %q", got.Reason, "unresolved-root "+root)
	}
	if len(*seen) != 0 {
		t.Errorf("an unresolvable root must be refused before any read, got reads: %v", *seen)
	}
}

func TestEvaluateOwnership_NilResolverIsHumanOwned(t *testing.T) {
	// A caller that passes no resolver cannot prove containment, so ownership
	// fails closed instead of silently falling back to the lexical check.
	root := t.TempDir()
	readFile, readDir, seen := recordingFS(ownershipFiles(root, map[string]string{
		".claude/skills/probe-skill/SKILL.md": ownershipSkillBody,
		".agents/skills/probe-skill/SKILL.md": ownershipSkillBody,
	}))
	got := EvaluateOwnership(root, ownershipEntry(HashSkill([]byte(ownershipSkillBody))), readFile, readDir, nil)
	if got.AgentOwned {
		t.Errorf("a missing resolver must be human-owned, got %+v", got)
	}
	if got.Reason != "no-resolver probe-skill" {
		t.Errorf("Reason = %q, want %q", got.Reason, "no-resolver probe-skill")
	}
	if len(*seen) != 0 {
		t.Errorf("a missing resolver must be refused before any read, got reads: %v", *seen)
	}
}

// --- SEC-3: a target that names the root itself is not a target ---

func TestEvaluateOwnership_RejectsTargetNamingTheRoot(t *testing.T) {
	root := t.TempDir()
	for _, target := range []string{".", "./", "././"} {
		t.Run(target, func(t *testing.T) {
			readFile, readDir, seen := recordingFS(ownershipFiles(root, map[string]string{
				".claude/skills/probe-skill/SKILL.md": ownershipSkillBody,
			}))
			e := ownershipEntry(HashSkill([]byte(ownershipSkillBody)))
			e.Targets = []string{target}
			got := EvaluateOwnership(root, e, readFile, readDir, identityResolver)
			if got.AgentOwned {
				t.Errorf("target %q was reported agent-owned: %+v", target, got)
			}
			if got.Reason != "invalid-target "+target {
				t.Errorf("Reason = %q, want %q", got.Reason, "invalid-target "+target)
			}
			if len(*seen) != 0 {
				t.Errorf("a target naming the root must be refused before any read, got reads: %v", *seen)
			}
		})
	}
}

// --- ROOT-1: a non-absolute root is a distinct, loud failure ---

func TestEvaluateOwnership_NonAbsoluteRootIsRefused(t *testing.T) {
	for _, root := range []string{"", "rel", "rel/sub"} {
		t.Run("root="+root, func(t *testing.T) {
			readFile, readDir, seen := recordingFS(map[string]string{})
			got := EvaluateOwnership(root, ownershipEntry(HashSkill([]byte(ownershipSkillBody))), readFile, readDir, identityResolver)
			if got.AgentOwned {
				t.Errorf("a non-absolute root must never be agent-owned, got %+v", got)
			}
			if got.Reason != "invalid-root "+root {
				t.Errorf("Reason = %q, want %q", got.Reason, "invalid-root "+root)
			}
			if len(*seen) != 0 {
				t.Errorf("a non-absolute root must be refused before any read, got reads: %v", *seen)
			}
		})
	}
}

// --- COV-1: one test per resolveTarget sub-guard ---

// TestResolveTarget_DotDotComponentGuard is the witness for the explicit
// ".."-component scan: "a/../b/SKILL.md" CLEANS BACK INSIDE root, so the
// containment check accepts it and only the scan refuses it. Deleting the
// scan alone turns this test red.
func TestResolveTarget_DotDotComponentGuard(t *testing.T) {
	root := t.TempDir()
	for _, target := range []string{"a/../b/SKILL.md", ".claude/skills/x/../x/SKILL.md"} {
		if abs, ok := resolveTarget(root, target); ok {
			t.Errorf("resolveTarget(%q, %q) = %q, true; want refused (a %q component is never recorded by the writer)", root, target, abs, "..")
		}
	}
}

// TestResolveTarget_ContainmentGuard is the witness for the containment
// (HasPrefix) check: none of these targets carries a ".." component and none
// is absolute, so the scan and the absolute check both pass them; only the
// containment check refuses them. Deleting that check alone turns this test
// red.
func TestResolveTarget_ContainmentGuard(t *testing.T) {
	cases := []struct{ root, target string }{
		{"", "SKILL.md"}, // cleans to "." — the joined path has no cleanRoot+separator prefix
		{string(filepath.Separator), "x/SKILL.md"}, // root "/" — cleanRoot+sep is "//", which nothing has as a prefix
	}
	for _, tc := range cases {
		if abs, ok := resolveTarget(tc.root, tc.target); ok {
			t.Errorf("resolveTarget(%q, %q) = %q, true; want refused by the containment check", tc.root, tc.target, abs)
		}
	}
}

// TestResolveTarget_DotTargetGuard is the witness for the strictly-below half
// of the containment check (SEC-3): ".", "./" and "././" all clean to ".",
// which resolves to root itself. Deleting withinRoot's `p != cleanRoot`
// clause alone turns this test (and its EvaluateOwnership counterpart) red.
func TestResolveTarget_DotTargetGuard(t *testing.T) {
	root := t.TempDir()
	for _, target := range []string{".", "./", "././"} {
		if abs, ok := resolveTarget(root, target); ok {
			t.Errorf("resolveTarget(%q, %q) = %q, true; want refused (a target must name a path strictly below root)", root, target, abs)
		}
	}
}
