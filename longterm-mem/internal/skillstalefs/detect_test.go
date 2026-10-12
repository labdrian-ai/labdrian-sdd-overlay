package skillstalefs_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/skillstale"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/skillstalefs"
)

// These cases put the detector over the adapters on a real project: files, a git repository and a PATH of the
// test's own. The detector's rules are tested without them (skillstale); what is tested here is that the two fit,
// and that a report changes nothing.

// sharedLock is the lock the engine writes, as the detector and the engine pin it (contracts/project-lock).
const sharedLock = "../../../contracts/project-lock/procedural-skills.lock.json"

const fixtureProject = "retirement-detector-fixture"

// fakeObservations lists the candidate records the case wants the detector to see.
type fakeObservations []memory.Observation

func (f fakeObservations) ListObservations(string) ([]memory.Observation, error) { return f, nil }

type fixtureRepo struct {
	root string
	bin  string
	now  time.Time
}

func writeAt(t *testing.T, root, rel, content string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", rel, err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = dir
	command.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=fixture", "GIT_AUTHOR_EMAIL=fixture@example.com",
		"GIT_COMMITTER_NAME=fixture", "GIT_COMMITTER_EMAIL=fixture@example.com",
	)
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// makeFixture builds a project with the five skills of the shared lock fixture, a repository in which one path
// they name was removed and another renamed, and a directory of commands that holds present-command.
func makeFixture(t *testing.T) fixtureRepo {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	lock, err := os.ReadFile(sharedLock)
	if err != nil {
		t.Fatalf("read the shared lock fixture: %v", err)
	}
	runGit(t, root, "init", "-q", "-b", "main")
	writeAt(t, root, "removed.go", "package fixture\n", 0o644)
	writeAt(t, root, "moved.go", "package fixture\n// distinctive moved body\n", 0o644)
	writeAt(t, root, "present.go", "package fixture\n", 0o644)
	writeAt(t, root, ".labdrian/procedural-skills.lock.json", string(lock), 0o644)
	const tick, fence = "`", "```"
	for id, content := range map[string]string{
		"clean-skill":              "---\nname: clean-skill\n---\nUse " + tick + "present.go" + tick + " when needed.\n" + fence + "sh\npresent-command present.go\n" + fence + "\n",
		"moved-path-skill":         "---\nname: moved-path-skill\n---\nThe old path is " + tick + "moved.go" + tick + ", but its content was renamed.\n",
		"quiet-skill":              "---\nname: quiet-skill\n---\nUse " + tick + "present.go" + tick + ".\n",
		"removed-path-skill":       "---\nname: removed-path-skill\n---\nInspect " + tick + "removed.go" + tick + " before changing this procedure.\n",
		"unresolved-command-skill": "---\nname: unresolved-command-skill\n---\n" + fence + "sh\ndefinitely-missing-command present.go\n" + fence + "\n",
	} {
		writeAt(t, root, ".claude/skills/"+id+"/SKILL.md", content, 0o644)
		writeAt(t, root, ".agents/skills/"+id+"/SKILL.md", content, 0o644)
	}
	runGit(t, root, "add", "-A")
	runGit(t, root, "commit", "-q", "-m", "initial fixture")
	runGit(t, root, "rm", "-q", "removed.go")
	runGit(t, root, "commit", "-q", "-m", "remove referenced path")
	runGit(t, root, "mv", "moved.go", "moved-new.go")
	runGit(t, root, "commit", "-q", "-m", "rename referenced path")

	bin := t.TempDir()
	writeAt(t, bin, "present-command", "#!/bin/sh\nexit 0\n", 0o755)
	return fixtureRepo{root: root, bin: bin, now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
}

func candidates(lastObserved map[string]string) fakeObservations {
	var out fakeObservations
	i := int64(0)
	for _, id := range []string{"removed-path-skill", "moved-path-skill", "clean-skill", "unresolved-command-skill", "quiet-skill"} {
		i++
		out = append(out, memory.Observation{
			ID: i, Title: id, Type: "pattern", TopicKey: "procedural/candidates/repeated-success/" + id,
			Content: "**Status**: registered\n**LastObserved**: " + lastObserved[id] + "\n", UpdatedAt: "2025-12-31 00:00:00",
		})
	}
	return out
}

func detect(t *testing.T, fixture fixtureRepo, observations fakeObservations) []skillstale.Finding {
	t.Helper()
	project, err := skillstalefs.OpenProject(fixture.root)
	if err != nil {
		t.Fatalf("OpenProject: %v", err)
	}
	findings, err := skillstale.Detect(skillstale.Config{
		Project:      fixtureProject,
		Source:       project,
		Observations: observations,
		Facts:        project,
		Commands:     skillstalefs.PathCommands{PathEnv: fixture.bin},
		Now:          fixture.now,
	})
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	return findings
}

func snapshotTree(t *testing.T, root string) map[string][]byte {
	t.Helper()
	got := map[string][]byte{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		got[filepath.ToSlash(rel)] = data
		return err
	})
	if err != nil {
		t.Fatalf("snapshot %s: %v", root, err)
	}
	return got
}

func TestDetectOverAProjectOnDisk_ReportsStaleSignalsAndDoesNotMutate(t *testing.T) {
	fixture := makeFixture(t)
	before := snapshotTree(t, fixture.root)

	findings := detect(t, fixture, candidates(map[string]string{
		"removed-path-skill": "2025-12-31T00:00:00Z", "moved-path-skill": "2025-12-31T00:00:00Z", "clean-skill": "2025-12-31T00:00:00Z",
		"unresolved-command-skill": "2025-12-31T00:00:00Z", "quiet-skill": "2000-01-01T00:00:00Z",
	}))

	if after := snapshotTree(t, fixture.root); !reflect.DeepEqual(after, before) {
		t.Fatalf("detector mutated the project:\nbefore=%v\nafter=%v", before, after)
	}
	byID := map[string]skillstale.Finding{}
	for _, finding := range findings {
		byID[finding.SkillID] = finding
	}
	if _, flagged := byID["clean-skill"]; flagged {
		t.Errorf("clean skill was flagged: %+v", byID["clean-skill"])
	}
	signalsOf := func(id string) []skillstale.Signal { return byID[id].Signals }

	if got := signalsOf("removed-path-skill"); len(got) != 1 || got[0].Kind != skillstale.SignalRemoved || got[0].Path != "removed.go" || len(got[0].Commit) != 40 {
		t.Errorf("removed-path-skill = %+v, want REMOVED removed.go with the commit that removed it", got)
	}
	if got := signalsOf("moved-path-skill"); len(got) != 1 || got[0].Kind != skillstale.SignalMoved || got[0].Path != "moved.go" || got[0].NewPath != "moved-new.go" {
		t.Errorf("moved-path-skill = %+v, want MOVED moved.go -> moved-new.go and nothing else (never also a removal)", got)
	}
	if got := signalsOf("unresolved-command-skill"); !reflect.DeepEqual(got, []skillstale.Signal{{Kind: skillstale.SignalUnresolvedCommand, Command: "definitely-missing-command"}}) {
		t.Errorf("unresolved-command-skill = %+v, want definitely-missing-command unresolved", got)
	}
	if got := signalsOf("quiet-skill"); !reflect.DeepEqual(got, []skillstale.Signal{{Kind: skillstale.SignalQuiet, Since: "2000-01-01T00:00:00Z"}}) {
		t.Errorf("quiet-skill = %+v, want QUIET since the record's own LastObserved", got)
	}
}

// A removed path is reported whatever the candidate says: the skill's instructions are not a record of the
// deletion, so a candidate observed after it does not excuse them.
func TestDetectOverAProjectOnDisk_ReportsARemovedPathWhateverTheCandidateSays(t *testing.T) {
	fixture := makeFixture(t)

	findings := detect(t, fixture, candidates(map[string]string{"removed-path-skill": "2099-01-01T00:00:00Z"}))

	for _, finding := range findings {
		if finding.SkillID == "removed-path-skill" {
			if len(finding.Signals) != 1 || finding.Signals[0].Kind != skillstale.SignalRemoved || finding.Signals[0].Path != "removed.go" {
				t.Errorf("removed-path-skill = %+v, want REMOVED removed.go even when the candidate was observed after", finding.Signals)
			}
			return
		}
	}
	t.Errorf("no finding for removed-path-skill in %+v", findings)
}
