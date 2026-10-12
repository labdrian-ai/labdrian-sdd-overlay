package skillstale_test

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/skillstale"
)

// sharedLockFixtures is where the lock files this detector reads and the engine writes are pinned. The engine
// (a module that does not import this one, and that this one does not import) reads the same files in its own
// tests and checks that what it writes is those bytes, so a change to the format in one module that the other
// does not follow fails a test in each (engine/skills, TestSerializeProjectLock_ReproducesTheSharedLockFixtures).
const sharedLockFixtures = "../../../contracts/project-lock"

func readSharedLock(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(sharedLockFixtures, name))
	if err != nil {
		t.Fatalf("read the shared lock fixture: %v", err)
	}
	return data
}

func TestParseProjectLock_ReadsSharedFixture(t *testing.T) {
	lock, err := skillstale.ParseProjectLock(readSharedLock(t, "procedural-skills.lock.json"))
	if err != nil {
		t.Fatalf("ParseProjectLock: %v", err)
	}
	if lock.Version != 1 {
		t.Fatalf("lock version = %d, want 1", lock.Version)
	}
	if len(lock.Skills) != 5 {
		t.Fatalf("len(lock.Skills) = %d, want 5", len(lock.Skills))
	}
	ids := make([]string, 0, len(lock.Skills))
	for _, entry := range lock.Skills {
		ids = append(ids, entry.ID)
		if len(entry.Targets) != 2 {
			t.Errorf("%s has %d targets, want 2", entry.ID, len(entry.Targets))
		}
	}
	if !sort.StringsAreSorted(ids) {
		t.Errorf("fixture ids are not sorted: %v", ids)
	}
}

// The lock `skills install` and project-register leave together, in the bytes the engine writes: the
// procedural skills with their targets, and the install records beside them, which the detector reads past.
func TestParseProjectLock_ReadsTheSharedFixtureWithInstallRecords(t *testing.T) {
	lock, err := skillstale.ParseProjectLock(readSharedLock(t, "procedural-skills-with-installs.lock.json"))
	if err != nil {
		t.Fatalf("ParseProjectLock: %v", err)
	}

	want := []skillstale.ProjectLockEntry{
		{ID: "alpha-skill", Provenance: "procedural", Candidate: "procedural/candidates/repeated-success/alpha-skill",
			SHA256: strings.Repeat("1", 64), Revision: 2,
			Targets: []string{".claude/skills/alpha-skill/SKILL.md", ".agents/skills/alpha-skill/SKILL.md"}},
		{ID: "zeta-skill", Provenance: "procedural", Candidate: "procedural/candidates/repeated-success/zeta-skill",
			SHA256: strings.Repeat("2", 64), Revision: 1,
			Targets: []string{".claude/skills/zeta-skill/SKILL.md", ".agents/skills/zeta-skill/SKILL.md"}},
	}
	if lock.Version != 1 || !reflect.DeepEqual(lock.Skills, want) {
		t.Errorf("lock = %+v, want version 1 and skills %+v", lock, want)
	}
	if !strings.Contains(string(lock.Installs), `"pdf"`) || !strings.Contains(string(lock.Installs), `"xlsx"`) {
		t.Errorf("the install records were not kept as they were written: %s", lock.Installs)
	}
}

// `skills install` records the registry skills it installed in the same lock file,
// in an "installs" array beside the procedural "skills". The detector looks only at
// the procedural skills, but it must still read a lock that has the records: a
// project that ran `skills install` would otherwise lose its staleness report.
func TestParseProjectLock_ToleratesTheInstallRecordsOfSkillsInstall(t *testing.T) {
	data := []byte(`{
  "version": 1,
  "skills": [
    {"id": "tidy-worktree", "provenance": "procedural", "candidate": "procedural/candidates/repeated-success/tidy-worktree",
     "sha256": "` + strings.Repeat("a", 64) + `", "revision": 1,
     "targets": [".claude/skills/tidy-worktree/SKILL.md", ".agents/skills/tidy-worktree/SKILL.md"]}
  ],
  "installs": [
    {"id": "pdf", "files": [{"path": "SKILL.md", "sha256": "` + strings.Repeat("b", 64) + `"}]}
  ]
}`)

	lock, err := skillstale.ParseProjectLock(data)
	if err != nil {
		t.Fatalf("ParseProjectLock refused a lock with install records: %v", err)
	}
	if len(lock.Skills) != 1 || lock.Skills[0].ID != "tidy-worktree" {
		t.Errorf("procedural skills = %+v, want tidy-worktree only", lock.Skills)
	}
}

// Tolerating the one field install writes does not make the parser lenient: any
// other unknown field is still refused.
func TestParseProjectLock_StillRefusesOtherUnknownFields(t *testing.T) {
	for name, data := range map[string]string{
		"at the top":   `{"version": 1, "skills": [], "installs": [], "extra": true}`,
		"in an entry":  `{"version": 1, "skills": [{"id": "a", "provenance": "p", "candidate": "c", "sha256": "s", "revision": 1, "targets": [], "note": "x"}]}`,
		"a misspelled": `{"version": 1, "skills": [], "install": []}`,
	} {
		if _, err := skillstale.ParseProjectLock([]byte(data)); err == nil {
			t.Errorf("%s: the lock was accepted", name)
		}
	}
}

func TestParseProjectLock_RefusesWhatTheEngineRefuses(t *testing.T) {
	for name, data := range map[string]string{
		"another version": `{"version": 2, "skills": []}`,
		"no version":      `{"skills": []}`,
		"trailing value":  `{"version": 1, "skills": []} {}`,
		"duplicate id":    `{"version": 1, "skills": [{"id": "a"}, {"id": "a"}]}`,
		"not json":        `{not json`,
		"empty":           ``,
	} {
		if _, err := skillstale.ParseProjectLock([]byte(data)); err == nil {
			t.Errorf("%s: the lock was accepted", name)
		}
	}
}
