package skills

import (
	"strings"
	"testing"
)

// The two JSON records the skills domain reads, the approval record of a skill and the project lock,
// are decoded through jsonstrict (Phase 9, H19, decision D4): a key that appears twice at any depth
// and bytes that are not UTF-8 are refused, where the standard decoder takes the last value of the
// key and replaces the bytes with U+FFFD in silence. The record that was accepted that way named
// something other than what a reader of the file would see.

const lockEntryJSON = `{"id": "foo", "provenance": "procedural", "candidate": "procedural/candidates/repeated-success/foo", "sha256": "%s", "revision": 1, "targets": [".claude/skills/foo/SKILL.md"]}`

func lockWith(entry string) string { return `{"version": 1, "skills": [` + entry + `]}` }

func TestParseApprovalRecordRefusesAKeyThatAppearsTwice(t *testing.T) {
	const (
		ver    = `"version":1,`
		skill  = `"skill":"my-skill",`
		digest = `"sha256":"` + abcDigest + `",`
		at     = `"approved_at":"2026-09-30T12:00:00Z",`
		who    = `"approver":"reviewer"`
	)
	for name, in := range map[string]string{
		"the skill, the first one the good one":  "{" + ver + skill + `"skill":"other-skill",` + digest + at + who + "}",
		"the digest, the last one the good one":  "{" + ver + `"sha256":"` + strings.Repeat("0", 64) + `",` + skill + digest + at + who + "}",
		"the version":                            "{" + ver + ver + skill + digest + at + who + "}",
		"the approver, at the end of the record": "{" + ver + skill + digest + at + who + `,"approver":"someone-else"}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ParseApprovalRecord([]byte(in))
			if err == nil || !strings.Contains(err.Error(), "duplicate JSON object key") || !strings.HasPrefix(err.Error(), "parse approval record: ") {
				t.Errorf("ParseApprovalRecord(%q) = %v, want a refusal that names the duplicate key", in, err)
			}
		})
	}
}

func TestParseApprovalRecordRefusesBytesThatAreNotUTF8(t *testing.T) {
	in := strings.Replace(goodRecordJSON("my-skill", abcDigest), `"reviewer"`, "\"review\xffer\"", 1)
	_, err := ParseApprovalRecord([]byte(in))
	if err == nil || !strings.Contains(err.Error(), "not valid UTF-8") || !strings.HasPrefix(err.Error(), "parse approval record: ") {
		t.Errorf("ParseApprovalRecord = %v, want a refusal that says the input is not UTF-8", err)
	}
}

// What the strict decoder always refused keeps its words.
func TestParseApprovalRecordKeepsTheWordsOfWhatItAlwaysRefused(t *testing.T) {
	good := goodRecordJSON("my-skill", abcDigest)
	for want, in := range map[string]string{
		"unknown field":                        strings.Replace(good, `"approver"`, `"extra": 1, "approver"`, 1),
		"trailing data after the record value": good + `{}`,
	} {
		if _, err := ParseApprovalRecord([]byte(in)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("ParseApprovalRecord = %v, want a refusal that says %q", err, want)
		}
	}
}

func TestParseProjectLockRefusesAKeyThatAppearsTwiceAtAnyDepth(t *testing.T) {
	digest := strings.Repeat("a", 64)
	entry := strings.Replace(lockEntryJSON, "%s", digest, 1)
	for name, in := range map[string]string{
		"the version at the top":     `{"version": 1, "version": 1, "skills": []}`,
		"skills at the top":          `{"version": 1, "skills": [], "skills": []}`,
		"the id of an entry":         lockWith(strings.Replace(entry, `"id": "foo",`, `"id": "foo", "id": "bar",`, 1)),
		"the targets of an entry":    lockWith(strings.Replace(entry, `"revision": 1,`, `"revision": 1, "targets": [],`, 1)),
		"a key of the second entry":  lockWith(entry + `, ` + strings.Replace(strings.Replace(entry, `"foo"`, `"two"`, 1), `"revision": 1`, `"revision": 1, "revision": 2`, 1)),
		"installs as well as skills": `{"version": 1, "skills": [], "installs": [{"id": "x", "files": []}], "installs": []}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ParseProjectLock([]byte(in))
			if err == nil || !strings.Contains(err.Error(), "duplicate JSON object key") || !strings.HasPrefix(err.Error(), "parse project lock: ") {
				t.Errorf("ParseProjectLock(%s) = %v, want a refusal that names the duplicate key", in, err)
			}
		})
	}
}

func TestParseProjectLockRefusesBytesThatAreNotUTF8(t *testing.T) {
	in := lockWith(strings.Replace(strings.Replace(lockEntryJSON, "%s", strings.Repeat("a", 64), 1), `"candidate": "procedural`, "\"candidate\": \"proced\xffural", 1))
	_, err := ParseProjectLock([]byte(in))
	if err == nil || !strings.Contains(err.Error(), "not valid UTF-8") || !strings.HasPrefix(err.Error(), "parse project lock: ") {
		t.Errorf("ParseProjectLock = %v, want a refusal that says the input is not UTF-8", err)
	}
}

// What the records the program writes look like still reads: the strict decoder only refuses what
// the writer never produces.
func TestTheRecordsTheProgramWritesStillRead(t *testing.T) {
	record, err := SerializeApprovalRecord(ApprovalRecord{Skill: "my-skill", SHA256: abcDigest, ApprovedAt: "2026-09-30T12:00:00Z", Approver: "revisor ñ 日本"})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := ParseApprovalRecord(record); err != nil || got.Approver != "revisor ñ 日本" {
		t.Errorf("ParseApprovalRecord of what SerializeApprovalRecord wrote = %+v, %v", got, err)
	}
	lock, err := SerializeProjectLock(ProjectLock{Version: 1, Skills: []ProjectLockEntry{{ID: "foo", Provenance: "procedural", Candidate: "procedural/candidates/repeated-success/foo", SHA256: strings.Repeat("a", 64), Revision: 1, Targets: []string{".claude/skills/foo/SKILL.md"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := ParseProjectLock(lock); err != nil || len(got.Skills) != 1 {
		t.Errorf("ParseProjectLock of what SerializeProjectLock wrote = %+v, %v", got, err)
	}
}
