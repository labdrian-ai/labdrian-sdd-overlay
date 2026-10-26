package skills

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/sha256hex"
)

// ApprovalRecordName is the file name of a skill's human-approval record. The
// record lives next to the skill it approves, at
// <sourceRoot>/<id>/ApprovalRecordName.
//
// The name is dot-prefixed on purpose. ScanSkillFiles skips dot-prefixed
// entries, so the on-disk gate needs no overlay.manifest row for the record;
// sync-manifest only ever rewrites */SKILL.md rows; and the deploy step is
// driven by manifest rows, so the record is never copied into a runtime as
// skill content. The two whole-tree copiers (ExecuteInstall and pipkg) skip it
// explicitly for the same reason.
const ApprovalRecordName = ".approval.json"

// ApprovalRecordVersion is the only record version this engine writes, and the only one it takes
// for an approval. Version 2 binds every file of the skill (SkillDigest).
const ApprovalRecordVersion = 2

// legacyApprovalRecordVersion is the version before it: a record whose digest was the SHA-256 of
// SKILL.md alone, so that a script or a reference beside it could change with the approval still
// holding. It is still read, to be told apart from a record this engine cannot read, and it never
// approves anything: ClassifyApproval calls it stale.
const legacyApprovalRecordVersion = 1

// ApprovalApproverMaxRunes bounds the approver label. The label is a free-text
// name, so an unbounded value would let a record carry arbitrary payload.
const ApprovalApproverMaxRunes = 100

// ApprovalRecord is the typed human-approval record for one skill: the skill
// id, the digest of the files that were approved (SkillDigest: every file in
// the directory of the skill, not SKILL.md alone, and whether the owner can
// execute each), when, and by whom. It is bound to those files, so any later
// change to any of them invalidates it (ApprovalStale).
//
// The engine cannot prove that a human ran `skills approve`: Approver is a
// label the caller supplied, and any process that can write the file can write
// a record. What the record guarantees, and what is tested, is that it matches
// the exact files of the skill it sits next to.
type ApprovalRecord struct {
	Version    int    `json:"version"`
	Skill      string `json:"skill"`
	SHA256     string `json:"sha256"`
	ApprovedAt string `json:"approved_at"`
	Approver   string `json:"approver"`
}

// ApprovalState classifies a skill's approval record against the files of the skill.
type ApprovalState string

const (
	// ApprovalAbsent means the skill has no record file.
	ApprovalAbsent ApprovalState = "absent"
	// ApprovalValid means a well-formed record of the current version names this
	// skill and its digest matches the files of the skill as they are now.
	ApprovalValid ApprovalState = "valid"
	// ApprovalStale means a well-formed record names this skill but does not
	// approve what the skill is now: its digest no longer matches (a file changed,
	// was added or was removed after it was approved, or had its owner execute
	// bit set or cleared), or it is a record of the
	// version before this one, which covered SKILL.md alone.
	ApprovalStale ApprovalState = "stale"
	// ApprovalMalformed means the record file exists but cannot be trusted: it
	// does not parse strictly, or it names a different skill.
	ApprovalMalformed ApprovalState = "malformed"
	// ApprovalUnverifiable means the directory of the skill holds something an
	// approval cannot cover (SkillDigest has no digest for it), so no record can
	// be valid for it, whatever the record says or whether there is one.
	ApprovalUnverifiable ApprovalState = "unverifiable"
)

// ApprovalStatus is the result of classifying one skill's record. Record is set
// for ApprovalValid and ApprovalStale. Digest is the digest of the files of the
// skill as they are now, set in every state but ApprovalUnverifiable: it is the
// one a new record is written with, from the same read that was classified.
// Detail explains ApprovalStale, ApprovalMalformed and ApprovalUnverifiable in
// one line and is empty otherwise.
type ApprovalStatus struct {
	State  ApprovalState
	Record ApprovalRecord
	Digest string
	Detail string
}

// utcTimestampRe matches the only timestamp shape a record carries:
// RFC 3339, UTC, whole seconds. Ranges are checked by validUTCTimestamp.
var utcTimestampRe = regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})Z$`)

// ApprovalRecordPath returns where the approval record for id lives under
// sourceRoot.
func ApprovalRecordPath(sourceRoot, id string) string {
	return filepath.Join(sourceRoot, id, ApprovalRecordName)
}

// digitsToInt converts a string of ASCII digits to its value. utcTimestampRe
// has already guaranteed the input is digits only.
func digitsToInt(s string) int {
	n := 0
	for _, r := range s {
		n = n*10 + int(r-'0')
	}
	return n
}

// validUTCTimestamp reports whether s is a real RFC 3339 UTC timestamp with
// whole seconds: the shape utcTimestampRe describes, with every field in range
// (including days per month and leap years). It exists because the package
// import allowlist (zero_fetch_test.go) does not admit "time".
func validUTCTimestamp(s string) bool {
	m := utcTimestampRe.FindStringSubmatch(s)
	if m == nil {
		return false
	}
	year, month, day := digitsToInt(m[1]), digitsToInt(m[2]), digitsToInt(m[3])
	hour, minute, second := digitsToInt(m[4]), digitsToInt(m[5]), digitsToInt(m[6])
	if month < 1 || month > 12 || hour > 23 || minute > 59 || second > 59 {
		return false
	}
	days := [...]int{31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}
	limit := days[month-1]
	if month == 2 && (year%4 == 0 && (year%100 != 0 || year%400 == 0)) {
		limit = 29
	}
	return day >= 1 && day <= limit
}

// ApproverLabelError explains why label is not an acceptable approver label,
// or returns "" when it is. A label is a non-blank, bounded, single-line name.
func ApproverLabelError(label string) string {
	if strings.TrimSpace(label) == "" {
		return "must not be blank"
	}
	if n := len([]rune(label)); n > ApprovalApproverMaxRunes {
		return fmt.Sprintf("is %d characters, exceeds the bound of %d", n, ApprovalApproverMaxRunes)
	}
	for _, r := range label {
		if r < 0x20 || r == 0x7f {
			return fmt.Sprintf("must not contain control characters, found %q", r)
		}
	}
	return ""
}

// validateApprovalRecord checks every field of rec, which must be a record of
// the given version, and returns the first problem, or nil. Parse and Serialize
// both call it, so a writer can never emit a record its own reader refuses.
func validateApprovalRecord(rec ApprovalRecord, version int) error {
	if rec.Version != version {
		return fmt.Errorf("unsupported version %d, want %d", rec.Version, version)
	}
	if !slugRe.MatchString(rec.Skill) {
		return fmt.Errorf(`field "skill" %q is not a valid skill id`, rec.Skill)
	}
	if !sha256hex.IsValid(rec.SHA256) {
		return fmt.Errorf(`field "sha256" %q is not a lowercase 64-character hex digest`, rec.SHA256)
	}
	if !validUTCTimestamp(rec.ApprovedAt) {
		return fmt.Errorf(`field "approved_at" %q is not an RFC 3339 UTC timestamp (YYYY-MM-DDTHH:MM:SSZ)`, rec.ApprovedAt)
	}
	if problem := ApproverLabelError(rec.Approver); problem != "" {
		return fmt.Errorf(`field "approver" %s`, problem)
	}
	return nil
}

// ParseApprovalRecord parses record bytes strictly. It refuses:
//   - unknown fields and a key that appears twice;
//   - bytes that are not UTF-8, or that hold more than one JSON object or anything after it;
//   - a version other than 2, a record of version 1 included: it approved SKILL.md alone, and
//     ClassifyApproval is what tells it from a record that cannot be read;
//   - an invalid field: a skill id that is not real, a digest that is not 64 lowercase hex
//     characters, a timestamp that is not a real UTC one, a blank approver.
//
// A missing file is the caller's concern (ApprovalAbsent), not this function's.
func ParseApprovalRecord(data []byte) (ApprovalRecord, error) {
	return parseApprovalRecord(data, false)
}

// parseApprovalRecord is ParseApprovalRecord, reading a record of the version before this one as
// well when legacy says so (its fields are held to the same rules, and it is told by its version).
func parseApprovalRecord(data []byte, legacy bool) (ApprovalRecord, error) {
	var rec ApprovalRecord
	if err := decodeRecord(data, "record", &rec); err != nil {
		return ApprovalRecord{}, refuseRecord("approval record", err)
	}
	version := ApprovalRecordVersion
	if legacy && rec.Version == legacyApprovalRecordVersion {
		version = legacyApprovalRecordVersion
	}
	if err := validateApprovalRecord(rec, version); err != nil {
		return ApprovalRecord{}, fmt.Errorf("parse approval record: %w", err)
	}
	return rec, nil
}

// SerializeApprovalRecord renders rec as deterministic JSON: fixed field order,
// 2-space indentation, and exactly one trailing newline. A zero Version (the
// from-scratch path) is normalized to 2; any record ParseApprovalRecord would
// refuse is refused here too.
func SerializeApprovalRecord(rec ApprovalRecord) ([]byte, error) {
	if rec.Version == 0 {
		rec.Version = ApprovalRecordVersion
	}
	if err := validateApprovalRecord(rec, ApprovalRecordVersion); err != nil {
		return nil, fmt.Errorf("serialize approval record: %w", err)
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(rec); err != nil {
		return nil, fmt.Errorf("serialize approval record: %w", err)
	}
	return buf.Bytes(), nil
}

// ClassifyApproval classifies the record for skill id against the files of the
// skill as they are now. present reports whether a record file exists;
// recordData is its content when it does. It is pure: no filesystem access.
//
// A skill whose files have no digest (SkillDigest) is unverifiable, whatever
// its record says or whether it has one: nothing can approve what an approval
// cannot cover.
//
// A record that names a different skill is malformed, never valid: copying one
// skill's record into another skill's directory must not approve it. A record
// of the version before this one is stale, not malformed, and is told so before
// its digest is looked at: it covered SKILL.md alone, so what it matches says
// nothing about the other files.
func ClassifyApproval(id string, entries []SkillEntry, recordData []byte, present bool) ApprovalStatus {
	digest, err := SkillDigest(entries)
	if err != nil {
		return ApprovalStatus{State: ApprovalUnverifiable, Detail: err.Error()}
	}
	if !present {
		return ApprovalStatus{State: ApprovalAbsent, Digest: digest}
	}
	rec, err := parseApprovalRecord(recordData, true)
	if err != nil {
		return ApprovalStatus{State: ApprovalMalformed, Digest: digest, Detail: err.Error()}
	}
	if rec.Skill != id {
		return ApprovalStatus{
			State:  ApprovalMalformed,
			Digest: digest,
			Detail: fmt.Sprintf("record names skill %q, expected %q", rec.Skill, id),
		}
	}
	if rec.Version == legacyApprovalRecordVersion {
		return ApprovalStatus{
			State:  ApprovalStale,
			Record: rec,
			Digest: digest,
			Detail: "a record of version 1 covers only SKILL.md, and an approval now covers every file of the skill",
		}
	}
	if rec.SHA256 != digest {
		return ApprovalStatus{
			State:  ApprovalStale,
			Record: rec,
			Digest: digest,
			Detail: fmt.Sprintf("the skill changed after approval: record digest %s, current digest %s", rec.SHA256, digest),
		}
	}
	return ApprovalStatus{State: ApprovalValid, Record: rec, Digest: digest}
}

// ReadApprovalStatus reads the record next to skill id under sourceRoot and
// classifies it against entries, the files of the skill as the caller read
// them. Only "does not exist" means ApprovalAbsent; any other read failure is
// returned as an error so the caller can refuse rather than treat an
// unreadable record as no record.
func ReadApprovalStatus(sourceRoot, id string, entries []SkillEntry, records ApprovalRecordStore) (ApprovalStatus, error) {
	path := ApprovalRecordPath(sourceRoot, id)
	data, err := records.ReadRecord(sourceRoot, id)
	if err != nil {
		if isAbsent(err) {
			return ClassifyApproval(id, entries, nil, false), nil
		}
		return ApprovalStatus{}, fmt.Errorf("skill %q: reading approval record %q: %w", id, path, err)
	}
	return ClassifyApproval(id, entries, data, true), nil
}
