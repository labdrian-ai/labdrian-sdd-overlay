package skills

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// abcDigest is the published SHA-256 test vector for the three bytes "abc".
const abcDigest = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"

// goodRecordJSON returns a well-formed record body of the current version for id and digest, built
// by hand so the parser tests do not depend on the serializer under test.
func goodRecordJSON(id, digest string) string { return recordJSON(2, id, digest) }

// legacyRecordJSON returns a well-formed record body of version 1, the one that covered SKILL.md
// alone.
func legacyRecordJSON(id, digest string) string { return recordJSON(1, id, digest) }

func recordJSON(version int, id, digest string) string {
	return fmt.Sprintf(`{
  "version": %d,
  "skill": %q,
  "sha256": %q,
  "approved_at": "2026-09-30T12:00:00Z",
  "approver": "reviewer"
}
`, version, id, digest)
}

// abcSkill is a skill whose directory holds the three bytes "abc" as its SKILL.md, and abcSkillDigest
// is what an approval of it binds: the SHA-256 of the one line sha256sum writes for the file,
//
//	ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad  SKILL.md
//
// made apart from this code, as the digest of its list.
func abcSkill() []SkillEntry { return skillMDOnly("abc") }

const abcSkillDigest = "d5f246a0e520fe46b7e50e2b8502e9a487e421577de9b85991e95a52b0b0fc37"

// abdSkillDigest is the same for a SKILL.md of the three bytes "abd".
const abdSkillDigest = "6c7ab96c9533273a3ae0a23110dd7c5330f29590844140b6fadee7955acc4893"

func TestApprovalRecordName_IsDotPrefixed(t *testing.T) {
	// The on-disk gate (ScanSkillFiles) skips dot-prefixed names, so the
	// record needs no overlay.manifest row and is never deployed. This pins
	// the property the record location depends on.
	if !strings.HasPrefix(ApprovalRecordName, ".") {
		t.Fatalf("ApprovalRecordName = %q, want a dot-prefixed name", ApprovalRecordName)
	}
}

func TestApprovalRecordPath_JoinsRootIDAndName(t *testing.T) {
	got := ApprovalRecordPath(filepath.Join("repo", "skills"), "my-skill")
	want := filepath.Join("repo", "skills", "my-skill", ApprovalRecordName)
	if got != want {
		t.Fatalf("ApprovalRecordPath = %q, want %q", got, want)
	}
}

func TestSerializeApprovalRecord_IsDeterministicAndRoundTrips(t *testing.T) {
	rec := ApprovalRecord{
		Version:    2,
		Skill:      "my-skill",
		SHA256:     abcDigest,
		ApprovedAt: "2026-09-30T12:00:00Z",
		Approver:   "reviewer",
	}
	first, err := SerializeApprovalRecord(rec)
	if err != nil {
		t.Fatalf("SerializeApprovalRecord: %v", err)
	}
	second, _ := SerializeApprovalRecord(rec)
	if string(first) != string(second) {
		t.Fatal("serialization must be deterministic")
	}
	if want := goodRecordJSON("my-skill", abcDigest); string(first) != want {
		t.Fatalf("serialized record:\n%s\nwant:\n%s", first, want)
	}
	if !strings.HasSuffix(string(first), "}\n") || strings.HasSuffix(string(first), "\n\n") {
		t.Fatalf("record must end with exactly one newline: %q", first)
	}
	back, err := ParseApprovalRecord(first)
	if err != nil {
		t.Fatalf("ParseApprovalRecord(round trip): %v", err)
	}
	if back != rec {
		t.Fatalf("round trip = %+v, want %+v", back, rec)
	}
}

func TestSerializeApprovalRecord_NormalizesZeroVersionAndRefusesInvalid(t *testing.T) {
	rec := ApprovalRecord{Skill: "my-skill", SHA256: abcDigest, ApprovedAt: "2026-09-30T12:00:00Z", Approver: "reviewer"}
	data, err := SerializeApprovalRecord(rec)
	if err != nil {
		t.Fatalf("a zero Version is the from-scratch path and must normalize to 2: %v", err)
	}
	if !strings.Contains(string(data), `"version": 2`) {
		t.Fatalf("serialized record must carry version 2: %s", data)
	}

	// A writer must never emit a record its own parser then refuses.
	bad := rec
	bad.SHA256 = "XYZ"
	if _, err := SerializeApprovalRecord(bad); err == nil {
		t.Fatal("SerializeApprovalRecord must refuse a malformed digest")
	}
	// The version before this one is read to be told stale, and is never written again.
	for _, version := range []int{1, 3} {
		bad = rec
		bad.Version = version
		if _, err := SerializeApprovalRecord(bad); err == nil {
			t.Fatalf("SerializeApprovalRecord must refuse version %d: this engine writes version 2 only", version)
		}
	}
}

func TestParseApprovalRecord_AcceptsAWellFormedRecord(t *testing.T) {
	rec, err := ParseApprovalRecord([]byte(goodRecordJSON("my-skill", abcDigest)))
	if err != nil {
		t.Fatalf("ParseApprovalRecord: %v", err)
	}
	want := ApprovalRecord{Version: 2, Skill: "my-skill", SHA256: abcDigest, ApprovedAt: "2026-09-30T12:00:00Z", Approver: "reviewer"}
	if rec != want {
		t.Fatalf("record = %+v, want %+v", rec, want)
	}
}

func TestParseApprovalRecord_RefusesEveryInvalidShape(t *testing.T) {
	obj := func(fields string) string { return "{" + fields + "}" }
	const (
		ver    = `"version":2,`
		skill  = `"skill":"my-skill",`
		digest = `"sha256":"` + abcDigest + `",`
		at     = `"approved_at":"2026-09-30T12:00:00Z",`
		who    = `"approver":"reviewer"`
	)
	tests := []struct {
		name string
		in   string
		want string // substring the error must contain
	}{
		{"empty input", ``, "parse approval record"},
		{"not json", `not json`, "parse approval record"},
		{"array not object", `[]`, "parse approval record"},
		{"truncated", `{"version":2,`, "parse approval record"},
		{"unknown field", obj(ver + skill + digest + at + who + `,"extra":true`), "unknown field"},
		{"missing version", obj(skill + digest + at + who), "unsupported version"},
		{"version zero", obj(`"version":0,` + skill + digest + at + who), "unsupported version"},
		// A record of version 1 covered SKILL.md alone; ParseApprovalRecord does not take it for an
		// approval, and ClassifyApproval is what tells it from a record that cannot be read.
		{"version one", obj(`"version":1,` + skill + digest + at + who), "unsupported version"},
		{"version three", obj(`"version":3,` + skill + digest + at + who), "unsupported version"},
		{"trailing data", obj(ver+skill+digest+at+who) + `{}`, "trailing data"},
		{"missing skill", obj(ver + digest + at + who), `field "skill"`},
		{"invalid skill slug", obj(ver + `"skill":"Not A Slug",` + digest + at + who), `field "skill"`},
		{"missing digest", obj(ver + skill + at + who), `field "sha256"`},
		{"short digest", obj(ver + skill + `"sha256":"abc123",` + at + who), `field "sha256"`},
		{"uppercase digest", obj(ver + skill + `"sha256":"` + strings.ToUpper(abcDigest) + `",` + at + who), `field "sha256"`},
		{"non-hex digest", obj(ver + skill + `"sha256":"` + strings.Repeat("g", 64) + `",` + at + who), `field "sha256"`},
		{"missing time", obj(ver + skill + digest + who), `field "approved_at"`},
		{"non-utc time", obj(ver + skill + digest + `"approved_at":"2026-09-30T12:00:00+02:00",` + who), `field "approved_at"`},
		{"impossible month", obj(ver + skill + digest + `"approved_at":"2026-13-01T00:00:00Z",` + who), `field "approved_at"`},
		{"impossible day", obj(ver + skill + digest + `"approved_at":"2026-02-30T00:00:00Z",` + who), `field "approved_at"`},
		{"impossible hour", obj(ver + skill + digest + `"approved_at":"2026-09-30T24:00:00Z",` + who), `field "approved_at"`},
		{"missing approver", obj(ver + skill + digest + strings.TrimSuffix(at, ",")), `field "approver"`},
		{"empty approver", obj(ver + skill + digest + at + `"approver":""`), `field "approver"`},
		{"blank approver", obj(ver + skill + digest + at + `"approver":"   "`), `field "approver"`},
		{"control character in approver", obj(ver + skill + digest + at + `"approver":"a\nb"`), `field "approver"`},
		{"oversized approver", obj(ver + skill + digest + at + `"approver":"` + strings.Repeat("x", ApprovalApproverMaxRunes+1) + `"`), `field "approver"`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseApprovalRecord([]byte(tc.in))
			if err == nil {
				t.Fatalf("ParseApprovalRecord(%q) = nil error, want one containing %q", tc.in, tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not contain %q", err, tc.want)
			}
		})
	}
}

func TestParseApprovalRecord_AcceptsALeapDay(t *testing.T) {
	in := strings.Replace(goodRecordJSON("my-skill", abcDigest), "2026-09-30", "2028-02-29", 1)
	if _, err := ParseApprovalRecord([]byte(in)); err != nil {
		t.Fatalf("2028-02-29 is a real date: %v", err)
	}
	in = strings.Replace(goodRecordJSON("my-skill", abcDigest), "2026-09-30", "2026-02-29", 1)
	if _, err := ParseApprovalRecord([]byte(in)); err == nil {
		t.Fatal("2026-02-29 does not exist and must be refused")
	}
}

func TestClassifyApproval_AllFiveStates(t *testing.T) {
	skill := abcSkill()
	other := skillMDOnly("abd")

	t.Run("absent", func(t *testing.T) {
		st := ClassifyApproval("my-skill", skill, nil, false)
		if st.State != ApprovalAbsent {
			t.Fatalf("state = %q, want %q", st.State, ApprovalAbsent)
		}
		if st.Digest != abcSkillDigest {
			t.Fatalf("an absent status must carry the digest a new record is written with: %q, want %q", st.Digest, abcSkillDigest)
		}
	})
	t.Run("valid", func(t *testing.T) {
		st := ClassifyApproval("my-skill", skill, []byte(goodRecordJSON("my-skill", abcSkillDigest)), true)
		if st.State != ApprovalValid {
			t.Fatalf("state = %q (%s), want %q", st.State, st.Detail, ApprovalValid)
		}
		if st.Record.Approver != "reviewer" {
			t.Fatalf("a valid status must carry the record: %+v", st.Record)
		}
		if st.Digest != abcSkillDigest {
			t.Fatalf("digest = %q, want %q", st.Digest, abcSkillDigest)
		}
	})
	t.Run("stale when SKILL.md changed after approval", func(t *testing.T) {
		st := ClassifyApproval("my-skill", other, []byte(goodRecordJSON("my-skill", abcSkillDigest)), true)
		if st.State != ApprovalStale {
			t.Fatalf("state = %q, want %q", st.State, ApprovalStale)
		}
		if !strings.Contains(st.Detail, abcSkillDigest) || !strings.Contains(st.Detail, abdSkillDigest) {
			t.Fatalf("stale detail must name both digests: %q", st.Detail)
		}
		if st.Digest != abdSkillDigest {
			t.Fatalf("a stale status must carry the digest of the files as they are now: %q, want %q", st.Digest, abdSkillDigest)
		}
	})
	t.Run("malformed", func(t *testing.T) {
		st := ClassifyApproval("my-skill", skill, []byte("{"), true)
		if st.State != ApprovalMalformed {
			t.Fatalf("state = %q, want %q", st.State, ApprovalMalformed)
		}
		if st.Detail == "" {
			t.Fatal("a malformed status must explain why")
		}
	})
	t.Run("a record naming another skill is malformed", func(t *testing.T) {
		// Copying a valid record into a different skill directory must not
		// approve that skill, even when the bytes happen to match.
		st := ClassifyApproval("my-skill", skill, []byte(goodRecordJSON("another-skill", abcSkillDigest)), true)
		if st.State != ApprovalMalformed {
			t.Fatalf("state = %q, want %q", st.State, ApprovalMalformed)
		}
		if !strings.Contains(st.Detail, "another-skill") {
			t.Fatalf("detail must name the foreign skill id: %q", st.Detail)
		}
	})
	t.Run("an empty record file is malformed, not absent", func(t *testing.T) {
		st := ClassifyApproval("my-skill", skill, []byte{}, true)
		if st.State != ApprovalMalformed {
			t.Fatalf("state = %q, want %q", st.State, ApprovalMalformed)
		}
	})
	t.Run("unverifiable when the directory holds what no digest covers", func(t *testing.T) {
		withLink := append(abcSkill(), SkillEntry{Rel: "references/link.md", Type: fs.ModeSymlink})
		// A record that would otherwise be valid, one that is not there, and one that cannot be read
		// all give way to the entry that cannot be covered.
		for name, record := range map[string][]byte{
			"a record that matches the rest": []byte(goodRecordJSON("my-skill", abcSkillDigest)),
			"no record":                      nil,
			"a record that is not JSON":      []byte("{"),
		} {
			st := ClassifyApproval("my-skill", withLink, record, record != nil)
			if st.State != ApprovalUnverifiable {
				t.Errorf("%s: state = %q (%s), want %q", name, st.State, st.Detail, ApprovalUnverifiable)
				continue
			}
			if !strings.Contains(st.Detail, "references/link.md is a symbolic link") {
				t.Errorf("%s: detail must name the entry it cannot cover: %q", name, st.Detail)
			}
			if st.Digest != "" {
				t.Errorf("%s: an unverifiable status carries no digest, since nothing may be approved with it: %q", name, st.Digest)
			}
		}
	})
}

func TestClassifyApproval_AFileBesideSKILLmdIsPartOfWhatWasApproved(t *testing.T) {
	record := []byte(goodRecordJSON("my-skill", abcSkillDigest))
	if st := ClassifyApproval("my-skill", abcSkill(), record, true); st.State != ApprovalValid {
		t.Fatalf("before the change: state = %q (%s), want %q", st.State, st.Detail, ApprovalValid)
	}
	for name, entries := range map[string][]SkillEntry{
		"a script added":                      append(abcSkill(), SkillEntry{Rel: "scripts/run.sh", Data: []byte("echo hi\n")}),
		"a reference added in a subdirectory": append(abcSkill(), SkillEntry{Rel: "references/deep/notes.md", Data: []byte("notes\n")}),
		"an empty file added":                 append(abcSkill(), SkillEntry{Rel: "empty"}),
	} {
		st := ClassifyApproval("my-skill", entries, record, true)
		if st.State != ApprovalStale {
			t.Errorf("%s: state = %q, want %q: an approval that left it out would still hold", name, st.State, ApprovalStale)
		}
	}
	// And the other way: a file the approval covered is gone.
	covered := append(abcSkill(), SkillEntry{Rel: "scripts/run.sh", Data: []byte("echo hi\n")})
	coveredDigest := mustDigest(t, covered)
	if st := ClassifyApproval("my-skill", covered, []byte(goodRecordJSON("my-skill", coveredDigest)), true); st.State != ApprovalValid {
		t.Fatalf("a record of the skill with its script: state = %q (%s), want %q", st.State, st.Detail, ApprovalValid)
	}
	if st := ClassifyApproval("my-skill", abcSkill(), []byte(goodRecordJSON("my-skill", coveredDigest)), true); st.State != ApprovalStale {
		t.Errorf("a file removed after approval: state = %q, want %q", st.State, ApprovalStale)
	}
}

func TestClassifyApproval_ARecordOfVersionOneIsStaleWhateverItsDigest(t *testing.T) {
	// A record of version 1 bound SKILL.md alone. The files beside it were never approved by it, so
	// it approves nothing now, whether or not its digest still matches SKILL.md (abcDigest), and
	// even when it happens to hold the digest a record of this version would have.
	for name, digest := range map[string]string{
		"its digest still matches SKILL.md":          abcDigest,
		"its digest is the one a version 2 would be": abcSkillDigest,
		"its digest matches nothing":                 strings.Repeat("0", 64),
	} {
		st := ClassifyApproval("my-skill", abcSkill(), []byte(legacyRecordJSON("my-skill", digest)), true)
		if st.State != ApprovalStale {
			t.Errorf("%s: state = %q (%s), want %q", name, st.State, st.Detail, ApprovalStale)
			continue
		}
		for _, want := range []string{"version 1", "only SKILL.md", "every file of the skill"} {
			if !strings.Contains(st.Detail, want) {
				t.Errorf("%s: detail %q does not say %q", name, st.Detail, want)
			}
		}
		if st.Record.Version != 1 || st.Record.Skill != "my-skill" {
			t.Errorf("%s: the status must carry the record it read: %+v", name, st.Record)
		}
		if st.Digest != abcSkillDigest {
			t.Errorf("%s: digest = %q, want the digest of the files now, %q", name, st.Digest, abcSkillDigest)
		}
	}
}

func TestClassifyApproval_ARecordOfAnyOtherVersionIsMalformed(t *testing.T) {
	for _, version := range []int{0, 3, 99, -1} {
		st := ClassifyApproval("my-skill", abcSkill(), []byte(recordJSON(version, "my-skill", abcSkillDigest)), true)
		if st.State != ApprovalMalformed {
			t.Errorf("version %d: state = %q (%s), want %q", version, st.State, st.Detail, ApprovalMalformed)
			continue
		}
		if !strings.Contains(st.Detail, "unsupported version") {
			t.Errorf("version %d: detail = %q, want it to say the version is unsupported", version, st.Detail)
		}
	}
}

func TestClassifyApproval_ALegacyRecordNamingAnotherSkillIsMalformedNotStale(t *testing.T) {
	// A copied record names a skill that is not this one. Telling it to be re-approved would send the
	// reader to approve a skill on the strength of another's record; the name is checked first.
	st := ClassifyApproval("my-skill", abcSkill(), []byte(legacyRecordJSON("another-skill", abcDigest)), true)
	if st.State != ApprovalMalformed {
		t.Fatalf("state = %q (%s), want %q", st.State, st.Detail, ApprovalMalformed)
	}
	if !strings.Contains(st.Detail, "another-skill") {
		t.Fatalf("detail must name the foreign skill id: %q", st.Detail)
	}
}

func TestClassifyApproval_ALegacyRecordIsHeldToTheFieldsOfAnyRecord(t *testing.T) {
	// Telling a record of version 1 stale does not excuse the rest of it: a digest that is not hex is
	// a record that cannot be read, not one that can be re-approved.
	st := ClassifyApproval("my-skill", abcSkill(), []byte(legacyRecordJSON("my-skill", "XYZ")), true)
	if st.State != ApprovalMalformed {
		t.Fatalf("state = %q (%s), want %q", st.State, st.Detail, ApprovalMalformed)
	}
}

func TestReadApprovalStatus_ReadsTheRecordNextToTheSkill(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "my-skill")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	skill := abcSkill()

	st, err := ReadApprovalStatus(root, "my-skill", skill, fileApprovals(os.ReadFile))
	if err != nil || st.State != ApprovalAbsent {
		t.Fatalf("no record file: state=%q err=%v, want absent and no error", st.State, err)
	}

	if err := os.WriteFile(filepath.Join(dir, ApprovalRecordName), []byte(goodRecordJSON("my-skill", abcSkillDigest)), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err = ReadApprovalStatus(root, "my-skill", skill, fileApprovals(os.ReadFile))
	if err != nil || st.State != ApprovalValid {
		t.Fatalf("matching record: state=%q err=%v, want valid", st.State, err)
	}
	if st.Digest != abcSkillDigest {
		t.Fatalf("digest = %q, want %q", st.Digest, abcSkillDigest)
	}

	// The record is judged against the entries the caller read, not against what the directory holds
	// by the time the record is read: a record of the files that were read is valid for them only.
	st, err = ReadApprovalStatus(root, "my-skill", skillMDOnly("abd"), fileApprovals(os.ReadFile))
	if err != nil || st.State != ApprovalStale {
		t.Fatalf("a record of other files: state=%q err=%v, want stale", st.State, err)
	}
}

func TestReadApprovalStatus_AnEntryNoDigestCoversIsUnverifiableEvenWithoutARecord(t *testing.T) {
	root := t.TempDir()
	withPipe := append(abcSkill(), SkillEntry{Rel: "pipe", Type: fs.ModeNamedPipe})
	st, err := ReadApprovalStatus(root, "my-skill", withPipe, fileApprovals(os.ReadFile))
	if err != nil {
		t.Fatalf("ReadApprovalStatus: %v: an unverifiable skill is a status, not a failure to read", err)
	}
	if st.State != ApprovalUnverifiable || !strings.Contains(st.Detail, "pipe is a named pipe") {
		t.Fatalf("state=%q detail=%q, want unverifiable and the entry named", st.State, st.Detail)
	}
}

func TestReadApprovalStatus_AReadFailureIsAnErrorNotAbsent(t *testing.T) {
	// Only "does not exist" means absent. Any other failure (permissions, a
	// directory in the record's place) must not be read as "no record": the
	// caller then treats the approval as unverifiable and refuses.
	boom := errors.New("input/output error")
	_, err := ReadApprovalStatus("root", "my-skill", abcSkill(), fileApprovals(func(string) ([]byte, error) { return nil, boom }))
	if err == nil {
		t.Fatal("ReadApprovalStatus must surface a non-not-exist read failure")
	}
	if !strings.Contains(err.Error(), "my-skill") {
		t.Fatalf("error must name the skill: %v", err)
	}

	missing := func(string) ([]byte, error) { return nil, &os.PathError{Op: "open", Path: "x", Err: os.ErrNotExist} }
	st, err := ReadApprovalStatus("root", "my-skill", abcSkill(), fileApprovals(missing))
	if err != nil || st.State != ApprovalAbsent {
		t.Fatalf("a not-exist error means absent: state=%q err=%v", st.State, err)
	}
}
