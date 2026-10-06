package main

import (
	"flag"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// The golden files under testdata/records-strict-golden record what the verbs say of the two JSON
// records the skills domain reads when one of them has a key that appears twice or is not UTF-8
// (Phase 9, H19, decision D4): 'validate' of a skill whose approval record is one, and 'project-
// status', 'install' and 'adopt' over a project whose lock is one. The words are what a person is
// told, so they are held to the byte. Rewrite them deliberately with
//
//	go test ./cmd -run TestRecordsStrictGolden -update-records-strict-golden
//
// and read the diff before committing it.
var updateRecordsStrictGolden = flag.Bool("update-records-strict-golden", false, "rewrite the golden files of the strict records")

// recordWithADuplicateKey is a valid approval record of skill alpha with its approver said twice.
func recordWithADuplicateKey(content string) string {
	valid, err := skills.SerializeApprovalRecord(skills.ApprovalRecord{
		Skill: "alpha", SHA256: skills.SkillDigest([]byte(content)), ApprovedAt: "2026-09-30T12:00:00Z", Approver: "fixture-reviewer",
	})
	if err != nil {
		panic(err)
	}
	return strings.Replace(string(valid), `"approver": "fixture-reviewer"`, `"approver": "fixture-reviewer",`+"\n  "+`"approver": "someone-else"`, 1)
}

func recordsStrictCases() []registryGoldenCase {
	const lockOfOneInstall = `{"version": 1, "skills": [], "installs": [{"id": "by-directory", "files": [{"path": "SKILL.md", "sha256": "` +
		"0000000000000000000000000000000000000000000000000000000000000000" + `"}]}]%s}`
	return []registryGoldenCase{
		{"validate-says-an-approval-record-names-a-key-twice", func(w *registryWorld) {
			w.overlay()
			w.put("skills/alpha/"+skills.ApprovalRecordName, recordWithADuplicateKey(skillFile("alpha")))
			w.label("a global skill whose approval record has the approver twice")
			w.run("skills", "validate", "--source-root", w.path("skills"))
		}},
		{"validate-says-an-approval-record-is-not-utf-8", func(w *registryWorld) {
			w.overlay()
			valid := w.read("skills/beta/" + skills.ApprovalRecordName)
			w.put("skills/beta/"+skills.ApprovalRecordName, strings.Replace(valid, `"fixture-reviewer"`, "\"fixture-\xffreviewer\"", 1))
			w.label("a global skill whose approval record holds a byte that is not UTF-8")
			w.run("skills", "validate", "--source-root", w.path("skills"))
		}},
		{"project-status-refuses-a-lock-that-names-a-key-twice", func(w *registryWorld) {
			w.put("overlay/"+worldRegistry, registryOf("unrelated"))
			w.mkdir("project")
			w.put("project/.labdrian/procedural-skills.lock.json", `{"version": 1, "version": 1, "skills": []}`)
			w.run("skills", "project-status", "--project-root", w.path("project"), "--registry", w.path("overlay/"+worldRegistry))
			w.show("project/.labdrian/procedural-skills.lock.json")
		}},
		{"install-and-adopt-refuse-a-lock-that-names-a-key-twice", func(w *registryWorld) {
			w.identityWorld()
			w.mkdir("demo")
			w.put("demo/.labdrian/procedural-skills.lock.json", strings.Replace(lockOfOneInstall, "%s", `, "installs": []`, 1))
			for _, verb := range []string{"install", "adopt"} {
				w.label("%s over a lock that has installs twice", verb)
				w.runIn("demo", w.identityArgs(verb)...)
			}
			w.tree("demo")
		}},
		{"install-refuses-a-lock-that-is-not-utf-8", func(w *registryWorld) {
			w.identityWorld()
			w.mkdir("demo")
			w.put("demo/.labdrian/procedural-skills.lock.json", strings.Replace(strings.Replace(lockOfOneInstall, `"by-directory"`, "\"by-\xffdirectory\"", 1), "%s", "", 1))
			w.label("a lock with a byte that is not UTF-8 inside a string")
			w.runIn("demo", w.identityArgs("install")...)
			w.tree("demo")
		}},
	}
}

// TestRecordsStrictGolden runs every case and compares its transcript with its golden file.
func TestRecordsStrictGolden(t *testing.T) {
	for _, tc := range recordsStrictCases() {
		t.Run(tc.name, func(t *testing.T) {
			w := newRegistryWorld(t)
			tc.run(w)
			checkGoldenIn(t, "records-strict-golden", tc.name, w.text(), updateRecordsStrictGolden, "-update-records-strict-golden")
		})
	}
}
