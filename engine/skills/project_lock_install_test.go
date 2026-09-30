package skills

// The install records in the project lock. `skills install` has to know which files
// it wrote, and what they held, to tell what it may replace from what someone else
// put there. The record is an optional "installs" array in the same lock file, beside
// the procedural "skills" array, so that one file and one version say what this
// program owns in a project. These tests pin that the addition changes nothing for a
// lock without it, and that a lock with it is read and written as strictly as the
// rest.

import (
	"strings"
	"testing"
)

const (
	digestA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	digestB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func installLockJSON(installs string) []byte {
	return []byte(`{"version":1,"skills":[` + procedural("tidy-worktree") + `],"installs":[` + installs + `]}`)
}

func procedural(id string) string {
	return `{"id":"` + id + `","provenance":"procedural","candidate":"procedural/candidates/repeated-success/` + id +
		`","sha256":"` + digestA + `","revision":1,"targets":[".claude/skills/` + id + `/SKILL.md",".agents/skills/` + id + `/SKILL.md"]}`
}

func oneInstall(id, files string) string {
	return `{"id":"` + id + `","files":[` + files + `]}`
}

func installFile(path, sum string) string {
	return `{"path":"` + path + `","sha256":"` + sum + `"}`
}

func TestParseProjectLock_ReadsInstallRecords(t *testing.T) {
	lock, err := ParseProjectLock(installLockJSON(oneInstall("pdf-skill",
		installFile("SKILL.md", digestA)+","+installFile("references/guide.md", digestB))))
	if err != nil {
		t.Fatal(err)
	}
	if len(lock.Skills) != 1 || lock.Skills[0].ID != "tidy-worktree" {
		t.Errorf("the procedural entries changed meaning: %+v", lock.Skills)
	}
	if len(lock.Installs) != 1 || lock.Installs[0].ID != "pdf-skill" || len(lock.Installs[0].Files) != 2 {
		t.Fatalf("installs = %+v, want one record of pdf-skill with two files", lock.Installs)
	}
	if f := lock.Installs[0].Files[1]; f.Path != "references/guide.md" || f.SHA256 != digestB {
		t.Errorf("second file = %+v", f)
	}
}

// A lock that never had an install record serializes exactly as before: the field is
// absent, not an empty array, so no existing lock file changes by one byte.
func TestSerializeProjectLock_OmitsTheInstallsWhenThereAreNone(t *testing.T) {
	data, err := SerializeProjectLock(ProjectLock{Version: 1, Skills: []ProjectLockEntry{}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "installs") {
		t.Errorf("serialized lock mentions installs: %s", data)
	}
	want := "{\n  \"version\": 1,\n  \"skills\": []\n}\n"
	if string(data) != want {
		t.Errorf("serialized = %q, want %q", data, want)
	}
}

func TestSerializeProjectLock_SortsInstallsAndTheirFiles(t *testing.T) {
	data, err := SerializeProjectLock(ProjectLock{Version: 1, Skills: []ProjectLockEntry{}, Installs: []ProjectInstallEntry{
		{ID: "zeta", Files: []ProjectInstallFile{{Path: "b.md", SHA256: digestA}, {Path: "a.md", SHA256: digestB}}},
		{ID: "alpha", Files: []ProjectInstallFile{{Path: "SKILL.md", SHA256: digestA}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if strings.Index(s, `"alpha"`) > strings.Index(s, `"zeta"`) {
		t.Errorf("installs are not sorted by id:\n%s", s)
	}
	if strings.Index(s, `"a.md"`) > strings.Index(s, `"b.md"`) {
		t.Errorf("files are not sorted by path:\n%s", s)
	}
	if !strings.HasSuffix(s, "\n") || strings.HasSuffix(s, "\n\n") {
		t.Errorf("want exactly one trailing newline: %q", s[len(s)-3:])
	}
}

func TestParseSerializeProjectLock_RoundTripsInstallRecords(t *testing.T) {
	first, err := ParseProjectLock(installLockJSON(oneInstall("pdf-skill", installFile("SKILL.md", digestA))))
	if err != nil {
		t.Fatal(err)
	}
	data, err := SerializeProjectLock(first)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ParseProjectLock(data)
	if err != nil {
		t.Fatalf("the serialized lock does not parse: %v\n%s", err, data)
	}
	again, err := SerializeProjectLock(second)
	if err != nil || string(again) != string(data) {
		t.Errorf("serialization is not stable (err %v):\n%s\n%s", err, data, again)
	}
}

func TestParseProjectLock_RefusesAnInvalidInstallRecord(t *testing.T) {
	good := installFile("SKILL.md", digestA)
	for name, tc := range map[string]struct {
		installs string
		want     string
	}{
		"an empty id":                  {oneInstall("", good), "id"},
		"an id that is not a slug":     {oneInstall("Bad_ID", good), "id"},
		"the same id twice":            {oneInstall("pdf-skill", good) + "," + oneInstall("pdf-skill", good), "duplicate"},
		"an id also in the skills":     {oneInstall("tidy-worktree", good), "tidy-worktree"},
		"no files":                     {oneInstall("pdf-skill", ""), "no files"},
		"an empty path":                {oneInstall("pdf-skill", installFile("", digestA)), "path"},
		"an absolute path":             {oneInstall("pdf-skill", installFile("/etc/passwd", digestA)), "path"},
		"a path that climbs":           {oneInstall("pdf-skill", installFile("../SKILL.md", digestA)), "path"},
		"a path with a dot segment":    {oneInstall("pdf-skill", installFile("a/./b.md", digestA)), "path"},
		"a path with a backslash":      {oneInstall("pdf-skill", installFile(`a\\b.md`, digestA)), "path"}, // JSON for a\b.md
		"a path ending in a slash":     {oneInstall("pdf-skill", installFile("a/", digestA)), "path"},
		"the approval record":          {oneInstall("pdf-skill", installFile(ApprovalRecordName, digestA)), "path"},
		"the same path twice":          {oneInstall("pdf-skill", good+","+good), "duplicate"},
		"a digest of the wrong size":   {oneInstall("pdf-skill", installFile("SKILL.md", "abc")), "sha256"},
		"an uppercase digest":          {oneInstall("pdf-skill", installFile("SKILL.md", strings.ToUpper(digestA))), "sha256"},
		"a digest that is not hex":     {oneInstall("pdf-skill", installFile("SKILL.md", strings.Repeat("z", 64))), "sha256"},
		"an unknown field in a file":   {oneInstall("pdf-skill", `{"path":"SKILL.md","sha256":"`+digestA+`","mode":"0644"}`), "mode"},
		"an unknown field in a record": {`{"id":"pdf-skill","files":[` + good + `],"revision":2}`, "revision"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ParseProjectLock(installLockJSON(tc.installs))
			if err == nil {
				t.Fatal("the lock was accepted")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not mention %q", err, tc.want)
			}
		})
	}
}

// Writing is as strict as reading: a caller cannot emit a lock that the parser then
// refuses to read back.
func TestSerializeProjectLock_RefusesWhatParseRefuses(t *testing.T) {
	ok := ProjectInstallFile{Path: "SKILL.md", SHA256: digestA}
	for name, l := range map[string]ProjectLock{
		"a duplicate id":      {Installs: []ProjectInstallEntry{{ID: "a", Files: []ProjectInstallFile{ok}}, {ID: "a", Files: []ProjectInstallFile{ok}}}},
		"an id in the skills": {Skills: []ProjectLockEntry{{ID: "a"}}, Installs: []ProjectInstallEntry{{ID: "a", Files: []ProjectInstallFile{ok}}}},
		"no files":            {Installs: []ProjectInstallEntry{{ID: "a"}}},
		"a climbing path":     {Installs: []ProjectInstallEntry{{ID: "a", Files: []ProjectInstallFile{{Path: "../x", SHA256: digestA}}}}},
		"a short digest":      {Installs: []ProjectInstallEntry{{ID: "a", Files: []ProjectInstallFile{{Path: "x", SHA256: "abc"}}}}},
	} {
		if _, err := SerializeProjectLock(l); err == nil {
			t.Errorf("%s: serialized", name)
		}
	}
}

// The procedural verbs rewrite the lock when they register, revise, or retire a
// skill. They must carry the install records through untouched.
func TestTheProceduralVerbsKeepTheInstallRecords(t *testing.T) {
	lock, err := ParseProjectLock(installLockJSON(oneInstall("pdf-skill", installFile("SKILL.md", digestA))))
	if err != nil {
		t.Fatal(err)
	}
	lock.Skills = nil // what project-retire does to the last procedural entry
	data, err := SerializeProjectLock(lock)
	if err != nil {
		t.Fatal(err)
	}
	back, err := ParseProjectLock(data)
	if err != nil || len(back.Installs) != 1 || back.Installs[0].ID != "pdf-skill" {
		t.Errorf("installs after a rewrite = %+v (err %v)", back.Installs, err)
	}
}
