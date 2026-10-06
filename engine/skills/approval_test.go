package skills

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// abcDigest is the published SHA-256 test vector for the three bytes "abc".
const abcDigest = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"

// goodRecordJSON returns a well-formed record body for id and digest, built
// by hand so the parser tests do not depend on the serializer under test.
func goodRecordJSON(id, digest string) string {
	return fmt.Sprintf(`{
  "version": 1,
  "skill": %q,
  "sha256": %q,
  "approved_at": "2026-09-30T12:00:00Z",
  "approver": "reviewer"
}
`, id, digest)
}

func TestSkillDigest_IsSHA256OfTheExactBytes(t *testing.T) {
	if got := SkillDigest([]byte("abc")); got != abcDigest {
		t.Fatalf("SkillDigest(abc) = %s, want %s", got, abcDigest)
	}
	// The digest binds the exact bytes: a line-ending change is a change.
	if SkillDigest([]byte("a\nb\n")) == SkillDigest([]byte("a\r\nb\r\n")) {
		t.Fatal("LF and CRLF renderings of the same text must not share a digest")
	}
	// An empty file still has a defined digest (never the empty string).
	if got := SkillDigest(nil); len(got) != 64 {
		t.Fatalf("SkillDigest(nil) = %q, want a 64-character digest", got)
	}
}

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
		Version:    1,
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
		t.Fatalf("a zero Version is the from-scratch path and must normalize to 1: %v", err)
	}
	if !strings.Contains(string(data), `"version": 1`) {
		t.Fatalf("serialized record must carry version 1: %s", data)
	}

	// A writer must never emit a record its own parser then refuses.
	bad := rec
	bad.SHA256 = "XYZ"
	if _, err := SerializeApprovalRecord(bad); err == nil {
		t.Fatal("SerializeApprovalRecord must refuse a malformed digest")
	}
	bad = rec
	bad.Version = 2
	if _, err := SerializeApprovalRecord(bad); err == nil {
		t.Fatal("SerializeApprovalRecord must refuse an unsupported version")
	}
}

func TestParseApprovalRecord_AcceptsAWellFormedRecord(t *testing.T) {
	rec, err := ParseApprovalRecord([]byte(goodRecordJSON("my-skill", abcDigest)))
	if err != nil {
		t.Fatalf("ParseApprovalRecord: %v", err)
	}
	want := ApprovalRecord{Version: 1, Skill: "my-skill", SHA256: abcDigest, ApprovedAt: "2026-09-30T12:00:00Z", Approver: "reviewer"}
	if rec != want {
		t.Fatalf("record = %+v, want %+v", rec, want)
	}
}

func TestParseApprovalRecord_RefusesEveryInvalidShape(t *testing.T) {
	obj := func(fields string) string { return "{" + fields + "}" }
	const (
		ver    = `"version":1,`
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
		{"truncated", `{"version":1,`, "parse approval record"},
		{"unknown field", obj(ver + skill + digest + at + who + `,"extra":true`), "unknown field"},
		{"missing version", obj(skill + digest + at + who), "unsupported version"},
		{"version zero", obj(`"version":0,` + skill + digest + at + who), "unsupported version"},
		{"version two", obj(`"version":2,` + skill + digest + at + who), "unsupported version"},
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

func TestClassifyApproval_AllFourStates(t *testing.T) {
	skill := []byte("abc")
	other := []byte("abd")

	t.Run("absent", func(t *testing.T) {
		st := ClassifyApproval("my-skill", skill, nil, false)
		if st.State != ApprovalAbsent {
			t.Fatalf("state = %q, want %q", st.State, ApprovalAbsent)
		}
	})
	t.Run("valid", func(t *testing.T) {
		st := ClassifyApproval("my-skill", skill, []byte(goodRecordJSON("my-skill", abcDigest)), true)
		if st.State != ApprovalValid {
			t.Fatalf("state = %q (%s), want %q", st.State, st.Detail, ApprovalValid)
		}
		if st.Record.Approver != "reviewer" {
			t.Fatalf("a valid status must carry the record: %+v", st.Record)
		}
	})
	t.Run("stale when the file changed after approval", func(t *testing.T) {
		st := ClassifyApproval("my-skill", other, []byte(goodRecordJSON("my-skill", abcDigest)), true)
		if st.State != ApprovalStale {
			t.Fatalf("state = %q, want %q", st.State, ApprovalStale)
		}
		if !strings.Contains(st.Detail, abcDigest) || !strings.Contains(st.Detail, SkillDigest(other)) {
			t.Fatalf("stale detail must name both digests: %q", st.Detail)
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
		st := ClassifyApproval("my-skill", skill, []byte(goodRecordJSON("another-skill", abcDigest)), true)
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
}

func TestReadApprovalStatus_ReadsTheRecordNextToTheSkill(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "my-skill")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	skill := []byte("abc")

	st, err := ReadApprovalStatus(root, "my-skill", skill, fileApprovals(os.ReadFile))
	if err != nil || st.State != ApprovalAbsent {
		t.Fatalf("no record file: state=%q err=%v, want absent and no error", st.State, err)
	}

	if err := os.WriteFile(filepath.Join(dir, ApprovalRecordName), []byte(goodRecordJSON("my-skill", abcDigest)), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err = ReadApprovalStatus(root, "my-skill", skill, fileApprovals(os.ReadFile))
	if err != nil || st.State != ApprovalValid {
		t.Fatalf("matching record: state=%q err=%v, want valid", st.State, err)
	}
}

func TestReadApprovalStatus_AReadFailureIsAnErrorNotAbsent(t *testing.T) {
	// Only "does not exist" means absent. Any other failure (permissions, a
	// directory in the record's place) must not be read as "no record": the
	// caller then treats the approval as unverifiable and refuses.
	boom := errors.New("input/output error")
	_, err := ReadApprovalStatus("root", "my-skill", []byte("abc"), fileApprovals(func(string) ([]byte, error) { return nil, boom }))
	if err == nil {
		t.Fatal("ReadApprovalStatus must surface a non-not-exist read failure")
	}
	if !strings.Contains(err.Error(), "my-skill") {
		t.Fatalf("error must name the skill: %v", err)
	}

	missing := func(string) ([]byte, error) { return nil, &os.PathError{Op: "open", Path: "x", Err: os.ErrNotExist} }
	st, err := ReadApprovalStatus("root", "my-skill", []byte("abc"), fileApprovals(missing))
	if err != nil || st.State != ApprovalAbsent {
		t.Fatalf("a not-exist error means absent: state=%q err=%v", st.State, err)
	}
}
