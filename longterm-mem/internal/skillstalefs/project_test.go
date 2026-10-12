package skillstalefs

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/repoevidence"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/skillstale"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/staleness"
)

// projectIn is a project over a fresh directory, named by its resolved path (a temporary directory can be a link,
// and the paths the adapter prints are the ones the test compares).
func projectIn(t *testing.T) (root string, project Project) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolve the temporary directory: %v", err)
	}
	project, err = OpenProject(root, repoevidence.At(root))
	if err != nil {
		t.Fatalf("OpenProject: %v", err)
	}
	return root, project
}

func writeProjectFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", rel, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

func TestOpenProject_RefusesARelativeRoot(t *testing.T) {
	for _, root := range []string{"", "relative/root", "."} {
		_, err := OpenProject(root, repoevidence.At(root))
		if err == nil || !strings.Contains(err.Error(), "project root must be absolute") {
			t.Errorf("OpenProject(%q) = %v, want the refusal of a root that is not absolute", root, err)
		}
	}
}

// A project with no repository evidence cannot say what became of a path, and is refused when it is opened
// rather than when it is asked.
func TestOpenProject_RefusesNoRepositoryEvidence(t *testing.T) {
	_, err := OpenProject(t.TempDir(), nil)
	if err == nil || !strings.Contains(err.Error(), "repository evidence is required") {
		t.Errorf("OpenProject(root, nil) = %v, want the refusal of a project with no repository evidence", err)
	}
}

// evidenceOf answers what staleness asks from a table, and records it.
type evidenceOf struct {
	files   []string
	history map[string]staleness.PathFact
	asked   [][]string
}

func (e *evidenceOf) Files() ([]string, error) { return e.files, nil }

func (e *evidenceOf) History(paths []string) (map[string]staleness.PathFact, error) {
	e.asked = append(e.asked, append([]string(nil), paths...))
	facts := map[string]staleness.PathFact{}
	for _, p := range paths {
		facts[p] = e.history[p]
	}
	return facts, nil
}

// What the project says of a path is what the evidence it was opened with says, in the detector's words: the
// adapter reads no tree and no history of its own, so a table stands in for both.
func TestPathFacts_AreWhatTheEvidenceItWasGivenSays(t *testing.T) {
	root := t.TempDir()
	evidence := &evidenceOf{
		files: []string{"module/present.go"},
		history: map[string]staleness.PathFact{
			"removed.go": {State: staleness.PathDeleted, Commit: "c0ffee1"},
			"moved.go":   {State: staleness.PathRenamed, NewPath: "moved-new.go", Commit: "c0ffee2"},
			"never.go":   {State: staleness.PathUnknown},
		},
	}
	project, err := OpenProject(root, evidence)
	if err != nil {
		t.Fatal(err)
	}

	facts, err := project.PathFacts([]string{"present.go", "removed.go", "moved.go", "never.go"})

	if err != nil {
		t.Fatalf("PathFacts: %v", err)
	}
	want := map[string]skillstale.PathFact{
		"present.go": {State: skillstale.PathPresent},
		"removed.go": {State: skillstale.PathDeleted, Commit: "c0ffee1"},
		"moved.go":   {State: skillstale.PathRenamed, NewPath: "moved-new.go", Commit: "c0ffee2"},
		"never.go":   {State: skillstale.PathUnknown},
	}
	if !reflect.DeepEqual(facts, want) {
		t.Errorf("PathFacts = %+v, want %+v", facts, want)
	}
	if asked := [][]string{{"moved.go", "never.go", "removed.go"}}; !reflect.DeepEqual(evidence.asked, asked) {
		t.Errorf("the history was asked %v, want %v: once, for the paths the tree does not hold", evidence.asked, asked)
	}
}

func TestReadLock_ReturnsTheLockExactlyAsItIsWritten(t *testing.T) {
	root, project := projectIn(t)
	const lock = "{\r\n \"version\": 1}  \n\n"
	writeProjectFile(t, root, ".labdrian/procedural-skills.lock.json", lock)

	got, err := project.ReadLock()

	if err != nil || string(got) != lock {
		t.Errorf("ReadLock = %q, %v; want the bytes of the file untouched", got, err)
	}
}

// A project with no lock says so with the file system's own error, which names the path it looked at: the
// command prints it as it is.
func TestReadLock_ReportsAMissingLockWithTheFileSystemsError(t *testing.T) {
	root, project := projectIn(t)

	_, err := project.ReadLock()

	var pathErr *fs.PathError
	if !errors.Is(err, fs.ErrNotExist) || !errors.As(err, &pathErr) || pathErr.Path != filepath.Join(root, ".labdrian", "procedural-skills.lock.json") {
		t.Errorf("ReadLock = %v, want a not-exist path error naming the lock under the root", err)
	}
}

func TestReadSkill_ReadsTheFileTheTargetNames(t *testing.T) {
	root, project := projectIn(t)
	writeProjectFile(t, root, ".claude/skills/a/SKILL.md", "skill text\n")

	got, err := project.ReadSkill(".claude/skills/a/SKILL.md")

	if err != nil || string(got) != "skill text\n" {
		t.Errorf("ReadSkill = %q, %v; want the file", got, err)
	}
}

func TestReadSkill_RefusesATargetThatLeavesTheProjectOrNamesNoFile(t *testing.T) {
	root, project := projectIn(t)
	writeProjectFile(t, root, "inside/SKILL.md", "x")
	for _, tc := range []struct {
		name, target, want string
	}{
		{"empty", "", `invalid target ""`},
		{"absolute", "/etc/hosts", `invalid target "/etc/hosts"`},
		{"climbing", "../outside/SKILL.md", `invalid target "../outside/SKILL.md"`},
		{"climbing from inside", "inside/../../outside/SKILL.md", `invalid target "inside/../../outside/SKILL.md"`},
		{"the root itself", ".", `target "." escapes project root`},
		{"missing", "inside/absent/SKILL.md", `resolve target "` + filepath.Join(root, "inside", "absent", "SKILL.md") + `"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := project.ReadSkill(tc.target)
			if err == nil || !strings.HasPrefix(err.Error(), tc.want) || got != nil {
				t.Errorf("ReadSkill(%q) = %q, %v; want an error beginning %q and nothing read", tc.target, got, err, tc.want)
			}
		})
	}
}

// A skill file that is not there is told with the words of the refusal above, and the error still is the file
// system's own underneath, so that a caller can ask it what it was.
func TestReadSkill_WrapsTheFileSystemsErrorForAFileThatIsNotThere(t *testing.T) {
	_, project := projectIn(t)

	_, err := project.ReadSkill(".claude/skills/absent/SKILL.md")

	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ReadSkill of a missing file = %v, want an error that is fs.ErrNotExist underneath", err)
	}
}

// A link inside the project is followed; a link that leaves it is refused, and what it points at is not read.
func TestReadSkill_FollowsALinkInsideTheProjectAndRefusesOneOutsideIt(t *testing.T) {
	root, project := projectIn(t)
	outside, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	writeProjectFile(t, root, "shared/SKILL.md", "inside\n")
	writeProjectFile(t, outside, "SKILL.md", "outside\n")
	for link, to := range map[string]string{
		".claude/skills/in/SKILL.md":  filepath.Join(root, "shared", "SKILL.md"),
		".claude/skills/out/SKILL.md": filepath.Join(outside, "SKILL.md"),
	} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, link)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(to, filepath.Join(root, link)); err != nil {
			t.Skipf("cannot make a link here: %v", err)
		}
	}

	if got, err := project.ReadSkill(".claude/skills/in/SKILL.md"); err != nil || string(got) != "inside\n" {
		t.Errorf("ReadSkill through a link inside = %q, %v; want the file it points at", got, err)
	}
	got, err := project.ReadSkill(".claude/skills/out/SKILL.md")
	if err == nil || !strings.Contains(err.Error(), "resolves outside project root") || got != nil {
		t.Errorf("ReadSkill through a link outside = %q, %v; want the refusal, and nothing read", got, err)
	}
}

// gitIn runs git in dir with an identity and a date of the test's own, and no configuration of whoever runs it.
func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = dir
	command.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=fixture", "GIT_AUTHOR_EMAIL=fixture@example.com", "GIT_AUTHOR_DATE=2026-01-01T00:00:00Z",
		"GIT_COMMITTER_NAME=fixture", "GIT_COMMITTER_EMAIL=fixture@example.com", "GIT_COMMITTER_DATE=2026-01-01T00:00:00Z",
	)
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// What the repository says of a path becomes the detector's own fact: present, deleted by a commit, renamed by
// one (with where it went), or never held (unknown).
func TestPathFacts_TellsWhatBecameOfEachPath(t *testing.T) {
	root, project := projectIn(t)
	gitIn(t, root, "init", "-q", "-b", "main")
	writeProjectFile(t, root, "removed.go", "package fixture\n")
	writeProjectFile(t, root, "moved.go", "package fixture\n// distinctive moved body\n")
	writeProjectFile(t, root, "present.go", "package fixture\n")
	gitIn(t, root, "add", "-A")
	gitIn(t, root, "commit", "-q", "-m", "initial")
	gitIn(t, root, "rm", "-q", "removed.go")
	gitIn(t, root, "commit", "-q", "-m", "remove")
	gitIn(t, root, "mv", "moved.go", "moved-new.go")
	gitIn(t, root, "commit", "-q", "-m", "rename")

	facts, err := project.PathFacts([]string{"present.go", "removed.go", "moved.go", "never.go"})
	if err != nil {
		t.Fatalf("PathFacts: %v", err)
	}

	if len(facts) != 4 {
		t.Fatalf("facts = %+v, want one for each of the four paths asked about", facts)
	}
	if facts["present.go"].State != skillstale.PathPresent {
		t.Errorf("present.go = %+v, want present", facts["present.go"])
	}
	if got := facts["removed.go"]; got.State != skillstale.PathDeleted || len(got.Commit) != 40 {
		t.Errorf("removed.go = %+v, want deleted by a commit (its full id)", got)
	}
	if got := facts["moved.go"]; got.State != skillstale.PathRenamed || got.NewPath != "moved-new.go" || len(got.Commit) != 40 {
		t.Errorf("moved.go = %+v, want renamed to moved-new.go by a commit", got)
	}
	if facts["never.go"].State != skillstale.PathUnknown {
		t.Errorf("never.go = %+v, want unknown: the repository never held it", facts["never.go"])
	}
}

// A project that is not a repository cannot tell what became of a path it lost; the error is the history's, not
// a guess, and no fact is returned.
func TestPathFacts_ReportsAProjectThatIsNoRepository(t *testing.T) {
	_, project := projectIn(t)

	facts, err := project.PathFacts([]string{"lost.go"})

	if err == nil || !strings.Contains(err.Error(), "not a readable git repository") || facts != nil {
		t.Errorf("PathFacts = %v, %v; want the history's error and no facts", facts, err)
	}
}

// A path the tree holds needs no history, so a project that is not a repository can still be asked about the
// paths it has.
func TestPathFacts_NeedsNoRepositoryForPathsTheTreeHolds(t *testing.T) {
	root, project := projectIn(t)
	writeProjectFile(t, root, "present.go", "package fixture\n")

	facts, err := project.PathFacts([]string{"present.go"})

	if err != nil || facts["present.go"].State != skillstale.PathPresent {
		t.Errorf("PathFacts = %v, %v; want present.go present", facts, err)
	}
}

// No path asked about, nothing to answer and nothing to ask the repository: a project that is no repository
// answers it as one that is.
func TestPathFacts_AnEmptyListAsksAndAnswersNothing(t *testing.T) {
	root, project := projectIn(t)
	for _, paths := range [][]string{nil, {}} {
		facts, err := project.PathFacts(paths)
		if err != nil || len(facts) != 0 {
			t.Errorf("PathFacts(%#v) in a directory that is no repository = %v, %v; want no facts and no error", paths, facts, err)
		}
	}
	gitIn(t, root, "init", "-q", "-b", "main")
	if facts, err := project.PathFacts(nil); err != nil || len(facts) != 0 {
		t.Errorf("PathFacts(nil) in a repository = %v, %v; want no facts and no error", facts, err)
	}
}

// Every path asked about is answered under the text it was asked by, whatever its shape, and a name with a space
// or a non-ASCII letter (the characters git quotes in its own output) is told as any other: present in the tree,
// or removed by a commit with that commit's id. (The paths the detector asks about come from a strict pattern
// that admits no space and no quote; what the history says of such shapes is its own to say, and the adapter
// must only not lose them on the way.)
func TestPathFacts_AnswersEveryPathUnderTheTextItWasAskedBy(t *testing.T) {
	root, project := projectIn(t)
	gitIn(t, root, "init", "-q", "-b", "main")
	spaced, accented, gone := "has space.go", "se"+string(rune(0xF1))+"al.go", "gone away.go"
	for _, name := range []string{spaced, accented, gone} {
		writeProjectFile(t, root, name, "package fixture\n")
	}
	gitIn(t, root, "add", "-A")
	gitIn(t, root, "commit", "-q", "-m", "initial")
	gitIn(t, root, "rm", "-q", gone)
	gitIn(t, root, "commit", "-q", "-m", "remove a spaced name")
	asked := []string{spaced, accented, gone, `it's "quoted".go`, ""}

	facts, err := project.PathFacts(asked)

	if err != nil {
		t.Fatalf("PathFacts: %v", err)
	}
	for _, question := range asked {
		if _, answered := facts[question]; !answered {
			t.Errorf("no fact for %q in %+v", question, facts)
		}
	}
	if len(facts) != len(asked) {
		t.Errorf("facts = %+v, want one for each of the %d paths asked about", facts, len(asked))
	}
	for _, question := range []string{spaced, accented} {
		if got := facts[question]; got.State != skillstale.PathPresent {
			t.Errorf("%q = %+v, want present", question, got)
		}
	}
	if got := facts[gone]; got.State != skillstale.PathDeleted || len(got.Commit) != 40 {
		t.Errorf("%q = %+v, want deleted by a commit (its full id)", gone, got)
	}
}

// A path outside the project, absolute or climbing out of it, is not a question the history can answer: it
// refuses it, as an error of its own, and the adapter returns that error and no facts, not a guess for the path
// and not a fact for the others.
func TestPathFacts_RefusesAPathOutsideTheProjectAndAnswersNothing(t *testing.T) {
	root, project := projectIn(t)
	gitIn(t, root, "init", "-q", "-b", "main")
	writeProjectFile(t, root, "present.go", "package fixture\n")
	gitIn(t, root, "add", "-A")
	gitIn(t, root, "commit", "-q", "-m", "initial")
	for _, outside := range []string{filepath.Join(filepath.Dir(root), "beside-the-project.go"), "../beside-the-project.go"} {
		facts, err := project.PathFacts([]string{"present.go", outside})

		if err == nil || facts != nil {
			t.Errorf("PathFacts(present.go, %q) = %v, %v; want the history's refusal and no facts", outside, facts, err)
		}
	}
}

// The detector has no word for a state of the history it has not heard of: it is the state it reports nothing
// for.
func TestStateOf_SaysUnknownForWhatItHasNoWordFor(t *testing.T) {
	for state, want := range map[staleness.PathState]skillstale.PathState{
		staleness.PathPresent: skillstale.PathPresent,
		staleness.PathDeleted: skillstale.PathDeleted,
		staleness.PathRenamed: skillstale.PathRenamed,
		staleness.PathUnknown: skillstale.PathUnknown,
		"something-new":       skillstale.PathUnknown,
		"":                    skillstale.PathUnknown,
	} {
		if got := stateOf(state); got != want {
			t.Errorf("stateOf(%q) = %q, want %q", state, got, want)
		}
	}
}
