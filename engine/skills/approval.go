package skills

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"
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

// ApprovalRecordVersion is the only record version this engine reads or writes.
const ApprovalRecordVersion = 1

// ApprovalApproverMaxRunes bounds the approver label. The label is a free-text
// name, so an unbounded value would let a record carry arbitrary payload.
const ApprovalApproverMaxRunes = 100

// ApprovalRecord is the typed human-approval record for one skill: the skill
// id, the SHA-256 of the exact SKILL.md bytes that were approved, when, and by
// whom. It is bound to the file bytes, so any later change to SKILL.md
// invalidates it (ApprovalStale).
//
// The engine cannot prove that a human ran `skills approve`: Approver is a
// label the caller supplied, and any process that can write the file can write
// a record. What the record guarantees, and what is tested, is that it matches
// the exact bytes of the skill it sits next to.
type ApprovalRecord struct {
	Version    int    `json:"version"`
	Skill      string `json:"skill"`
	SHA256     string `json:"sha256"`
	ApprovedAt string `json:"approved_at"`
	Approver   string `json:"approver"`
}

// ApprovalState classifies a skill's approval record against its SKILL.md.
type ApprovalState string

const (
	// ApprovalAbsent means the skill has no record file.
	ApprovalAbsent ApprovalState = "absent"
	// ApprovalValid means a well-formed record names this skill and its digest
	// matches the current SKILL.md bytes.
	ApprovalValid ApprovalState = "valid"
	// ApprovalStale means a well-formed record names this skill but its digest
	// no longer matches the current SKILL.md bytes: the file changed after it
	// was approved.
	ApprovalStale ApprovalState = "stale"
	// ApprovalMalformed means the record file exists but cannot be trusted: it
	// does not parse strictly, or it names a different skill.
	ApprovalMalformed ApprovalState = "malformed"
)

// ApprovalStatus is the result of classifying one skill's record. Record is set
// for ApprovalValid and ApprovalStale. Detail explains ApprovalStale and
// ApprovalMalformed in one line and is empty otherwise.
type ApprovalStatus struct {
	State  ApprovalState
	Record ApprovalRecord
	Detail string
}

var sha256HexRe = regexp.MustCompile(`^[0-9a-f]{64}$`)

// utcTimestampRe matches the only timestamp shape a record carries:
// RFC 3339, UTC, whole seconds. Ranges are checked by validUTCTimestamp.
var utcTimestampRe = regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})Z$`)

// SkillDigest returns the lowercase hex SHA-256 of the exact SKILL.md bytes.
// It hashes the bytes as read, with no normalization of line endings, a BOM or
// trailing whitespace: approval binds the file, not an interpretation of it.
func SkillDigest(skillMD []byte) string {
	sum := sha256.Sum256(skillMD)
	return hex.EncodeToString(sum[:])
}

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

// validateApprovalRecord checks every field of rec and returns the first
// problem, or nil. Parse and Serialize both call it, so a writer can never emit
// a record its own reader refuses.
func validateApprovalRecord(rec ApprovalRecord) error {
	if rec.Version != ApprovalRecordVersion {
		return fmt.Errorf("unsupported version %d, want %d", rec.Version, ApprovalRecordVersion)
	}
	if !slugRe.MatchString(rec.Skill) {
		return fmt.Errorf(`field "skill" %q is not a valid skill id`, rec.Skill)
	}
	if !sha256HexRe.MatchString(rec.SHA256) {
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

// ParseApprovalRecord parses record bytes strictly: unknown fields are
// refused, the bytes must hold exactly one JSON object with nothing trailing
// it, the version must be 1, and every field must be valid (a real skill id, a
// lowercase 64-character hex digest, a real UTC timestamp, a non-blank
// approver). A missing file is the caller's concern (ApprovalAbsent), not this
// function's.
func ParseApprovalRecord(data []byte) (ApprovalRecord, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	var rec ApprovalRecord
	if err := dec.Decode(&rec); err != nil {
		return ApprovalRecord{}, fmt.Errorf("parse approval record: %w", err)
	}
	if err := dec.Decode(new(json.RawMessage)); err != io.EOF {
		return ApprovalRecord{}, fmt.Errorf("parse approval record: trailing data after the record value")
	}
	if err := validateApprovalRecord(rec); err != nil {
		return ApprovalRecord{}, fmt.Errorf("parse approval record: %w", err)
	}
	return rec, nil
}

// SerializeApprovalRecord renders rec as deterministic JSON: fixed field order,
// 2-space indentation, and exactly one trailing newline. A zero Version (the
// from-scratch path) is normalized to 1; any record ParseApprovalRecord would
// refuse is refused here too.
func SerializeApprovalRecord(rec ApprovalRecord) ([]byte, error) {
	if rec.Version == 0 {
		rec.Version = ApprovalRecordVersion
	}
	if err := validateApprovalRecord(rec); err != nil {
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

// ClassifyApproval classifies the record for skill id against the current
// SKILL.md bytes. present reports whether a record file exists; recordData is
// its content when it does. It is pure: no filesystem access.
//
// A record that names a different skill is malformed, never valid: copying one
// skill's record into another skill's directory must not approve it.
func ClassifyApproval(id string, skillMD, recordData []byte, present bool) ApprovalStatus {
	if !present {
		return ApprovalStatus{State: ApprovalAbsent}
	}
	rec, err := ParseApprovalRecord(recordData)
	if err != nil {
		return ApprovalStatus{State: ApprovalMalformed, Detail: err.Error()}
	}
	if rec.Skill != id {
		return ApprovalStatus{
			State:  ApprovalMalformed,
			Detail: fmt.Sprintf("record names skill %q, expected %q", rec.Skill, id),
		}
	}
	if digest := SkillDigest(skillMD); rec.SHA256 != digest {
		return ApprovalStatus{
			State:  ApprovalStale,
			Record: rec,
			Detail: fmt.Sprintf("SKILL.md changed after approval: record digest %s, file digest %s", rec.SHA256, digest),
		}
	}
	return ApprovalStatus{State: ApprovalValid, Record: rec}
}

// ReadApprovalStatus reads the record next to skill id under sourceRoot and
// classifies it against skillMD. Only "does not exist" means ApprovalAbsent;
// any other read failure is returned as an error so the caller can refuse
// rather than treat an unreadable record as no record.
func ReadApprovalStatus(sourceRoot, id string, skillMD []byte, readFile readFileFn) (ApprovalStatus, error) {
	path := ApprovalRecordPath(sourceRoot, id)
	data, err := readFile(path)
	if err != nil {
		if isAbsent(err) {
			return ApprovalStatus{State: ApprovalAbsent}, nil
		}
		return ApprovalStatus{}, fmt.Errorf("skill %q: reading approval record %q: %w", id, path, err)
	}
	return ClassifyApproval(id, skillMD, data, true), nil
}
