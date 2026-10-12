package skillstale_test

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/skillstale"
)

// The detector is a function of its inputs, so these tests hand it fakes of its ports and a time of their own:
// no file, no repository, no PATH and no clock is involved, and none of the cases depends on the day it runs or
// on the machine. The adapters that answer the ports on a real project are tested in skillstalefs.

const testProject = "retirement-detector-fixture"

// testNow is the instant the cases judge by.
var testNow = time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

// fakeSource is the skill source of a project that is a table: the lock it returns, the skill files by target,
// and what it was asked.
type fakeSource struct {
	lock       []byte
	lockErr    error
	skills     map[string][]byte
	skillErr   map[string]error
	lockReads  int
	skillsRead []string
}

func (f *fakeSource) ReadLock() ([]byte, error) {
	f.lockReads++
	return f.lock, f.lockErr
}

func (f *fakeSource) ReadSkill(target string) ([]byte, error) {
	f.skillsRead = append(f.skillsRead, target)
	if err := f.skillErr[target]; err != nil {
		return nil, err
	}
	data, ok := f.skills[target]
	if !ok {
		return nil, fmt.Errorf("no skill file at %s", target)
	}
	return data, nil
}

// fakeObservations is the lister port skillstale owns (ObservationLister), implemented without a database.
type fakeObservations struct {
	asked        []string
	observations []memory.Observation
	err          error
}

func (f *fakeObservations) ListObservations(project string) ([]memory.Observation, error) {
	f.asked = append(f.asked, project)
	return f.observations, f.err
}

// fakeFacts answers what the repository says of a path from a table, and records what it was asked.
type fakeFacts struct {
	facts map[string]skillstale.PathFact
	err   error
	asked [][]string
}

func (f *fakeFacts) PathFacts(paths []string) (map[string]skillstale.PathFact, error) {
	f.asked = append(f.asked, append([]string(nil), paths...))
	if f.err != nil {
		return nil, f.err
	}
	return f.facts, nil
}

// fakeCommands resolves the commands of a set and records the ones it was asked about.
type fakeCommands struct {
	found map[string]bool
	asked []string
}

func (f *fakeCommands) ResolvesCommand(command string) bool {
	f.asked = append(f.asked, command)
	return f.found[command]
}

// world is the project of a case and the ports over it.
type world struct {
	source       *fakeSource
	observations *fakeObservations
	facts        *fakeFacts
	commands     *fakeCommands
	ids          []string
	superseded   map[string]string
}

func newWorld() *world {
	return &world{
		source:       &fakeSource{skills: map[string][]byte{}, skillErr: map[string]error{}},
		observations: &fakeObservations{},
		facts:        &fakeFacts{facts: map[string]skillstale.PathFact{}},
		commands:     &fakeCommands{found: map[string]bool{}},
	}
}

func candidateOf(id string) string { return "procedural/candidates/repeated-success/" + id }
func targetOf(id string) string    { return ".claude/skills/" + id + "/SKILL.md" }

// skill adds a skill of the lock whose file has the text given.
func (w *world) skill(id, text string) *world {
	w.ids = append(w.ids, id)
	w.source.skills[targetOf(id)] = []byte(text)
	return w
}

// observed records the candidate of the skill, last observed at the given time (empty: the record says nothing
// of it).
func (w *world) observed(id, lastObserved string) *world {
	content := "**Status**: registered\n"
	if lastObserved != "" {
		content += "**LastObserved**: " + lastObserved + "\n"
	}
	w.observations.observations = append(w.observations.observations, memory.Observation{
		ID: int64(len(w.observations.observations) + 1), Title: id, Type: "pattern", TopicKey: candidateOf(id),
		Content: content, UpdatedAt: "2025-12-31 00:00:00",
	})
	return w
}

// lockBytes renders the lock of the skills added so far, unless the case set the bytes itself.
func (w *world) lockBytes() []byte {
	if w.source.lock != nil {
		return w.source.lock
	}
	entries := make([]string, len(w.ids))
	for i, id := range w.ids {
		entries[i] = fmt.Sprintf(`{"id": %q, "provenance": "procedural", "candidate": %q, "sha256": %q, "revision": 1, "targets": [%q, ".agents/skills/%s/SKILL.md"]}`,
			id, candidateOf(id), strings.Repeat("a", 64), targetOf(id), id)
	}
	return []byte(`{"version": 1, "skills": [` + strings.Join(entries, ", ") + `]}`)
}

func (w *world) config() skillstale.Config {
	w.source.lock = w.lockBytes()
	return skillstale.Config{
		Project:      testProject,
		Source:       w.source,
		Observations: w.observations,
		Facts:        w.facts,
		Commands:     w.commands,
		Now:          testNow,
		SupersededBy: w.superseded,
	}
}

func (w *world) detect(t *testing.T) []skillstale.Finding {
	t.Helper()
	findings, err := skillstale.Detect(w.config())
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	return findings
}

const (
	fence      = "```"
	recentTime = "2026-07-30T00:00:00Z"
)

// text puts a body under the front matter every skill file has.
func text(body string) string {
	return "---\nname: s\ndescription: \"Trigger: a case.\"\n---\n" + body
}

func kinds(finding skillstale.Finding) []skillstale.SignalKind {
	out := make([]skillstale.SignalKind, len(finding.Signals))
	for i, signal := range finding.Signals {
		out[i] = signal.Kind
	}
	return out
}

func findingOf(t *testing.T, findings []skillstale.Finding, id string) skillstale.Finding {
	t.Helper()
	for _, finding := range findings {
		if finding.SkillID == id {
			return finding
		}
	}
	t.Fatalf("no finding for %q; got %+v", id, findings)
	return skillstale.Finding{}
}

// A skill that has gone quiet, names a path that was removed and one that moved, runs a command nobody can
// find and has been replaced is reported with every one of those, in the order the report lists them. A moved
// path is a move and never also a removal.
func TestDetect_ReportsEachKindOfSignalInTheOrderOfTheReport(t *testing.T) {
	w := newWorld().
		skill("old-skill", text("Inspect `removed.go` and `moved.go`, and keep `present.go` and `never.go`.\n"+fence+"sh\nmissing-tool run\n"+fence+"\n")).
		observed("old-skill", "2000-01-01T00:00:00Z")
	w.facts.facts = map[string]skillstale.PathFact{
		"removed.go": {State: skillstale.PathDeleted, Commit: "c0ffee"},
		"moved.go":   {State: skillstale.PathRenamed, NewPath: "moved-new.go", Commit: "beef"},
		"present.go": {State: skillstale.PathPresent},
		"never.go":   {State: skillstale.PathUnknown, Commit: "ignored"},
	}
	w.superseded = map[string]string{"old-skill": "global/newer-skill"}

	findings := w.detect(t)

	want := []skillstale.Finding{{
		SkillID: "old-skill", Candidate: candidateOf("old-skill"), Status: "registered",
		Signals: []skillstale.Signal{
			{Kind: skillstale.SignalQuiet, Since: "2000-01-01T00:00:00Z"},
			{Kind: skillstale.SignalRemoved, Path: "removed.go", Commit: "c0ffee"},
			{Kind: skillstale.SignalMoved, Path: "moved.go", NewPath: "moved-new.go", Commit: "beef"},
			{Kind: skillstale.SignalUnresolvedCommand, Command: "missing-tool"},
			{Kind: skillstale.SignalSuperseded, By: "global/newer-skill"},
		},
	}}
	if !reflect.DeepEqual(findings, want) {
		t.Errorf("findings =\n%+v\nwant\n%+v", findings, want)
	}
}

// A skill with nothing to report is not in the report, whatever else is: present and unknown paths say
// nothing, a command that is found says nothing, and a record observed lately says nothing.
func TestDetect_LeavesOutASkillWithNothingToReport(t *testing.T) {
	w := newWorld().
		skill("clean-skill", text("Use `present.go` and `never.go`.\n"+fence+"sh\nfound-tool run\n"+fence+"\n")).
		skill("flagged-skill", text(fence+"sh\nmissing-tool\n"+fence+"\n")).
		observed("clean-skill", recentTime).
		observed("flagged-skill", recentTime)
	w.facts.facts = map[string]skillstale.PathFact{
		"present.go": {State: skillstale.PathPresent},
		"never.go":   {State: skillstale.PathUnknown},
	}
	w.commands.found["found-tool"] = true

	findings := w.detect(t)

	if len(findings) != 1 || findings[0].SkillID != "flagged-skill" {
		t.Errorf("findings = %+v, want the flagged skill alone, in the order of the lock", findings)
	}
}

func TestDetect_ListsFindingsInTheOrderOfTheLock(t *testing.T) {
	w := newWorld()
	for _, id := range []string{"zeta", "alpha", "mid"} {
		w.skill(id, text(fence+"sh\nmissing-tool\n"+fence+"\n"))
	}

	var got []string
	for _, finding := range w.detect(t) {
		got = append(got, finding.SkillID)
	}

	if want := []string{"zeta", "alpha", "mid"}; !reflect.DeepEqual(got, want) {
		t.Errorf("order = %v, want the lock's %v", got, want)
	}
}

// Whether a record has gone quiet is a question of the instant the detector is given and of nothing else: a day
// short of QuietAfter is not quiet, a second past it is, and a time that cannot be read says nothing.
func TestDetect_JudgesQuietnessByTheTimeItIsGiven(t *testing.T) {
	at := func(offset time.Duration, layout string) string { return testNow.Add(offset).Format(layout) }
	for _, tc := range []struct {
		name         string
		lastObserved string
		quiet        bool
	}{
		{"exactly QuietAfter before", at(-skillstale.QuietAfter, time.RFC3339), false},
		{"a second past QuietAfter", at(-skillstale.QuietAfter-time.Second, time.RFC3339), true},
		{"a day short of QuietAfter", at(-skillstale.QuietAfter+24*time.Hour, time.RFC3339), false},
		{"a year before", at(-365*24*time.Hour, time.RFC3339), true},
		{"in the future", at(24*time.Hour, time.RFC3339), false},
		{"a date alone", "2026-01-01", false},
		{"the layout with no zone", at(-skillstale.QuietAfter-time.Hour, "2006-01-02T15:04:05"), true},
		{"a zone other than UTC", testNow.Add(-skillstale.QuietAfter - time.Hour).In(time.FixedZone("x", 5*3600)).Format(time.RFC3339), true},
		{"not a time", "sometime last spring", false},
		{"nothing", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newWorld().skill("s", text("Nothing to run.\n")).observed("s", tc.lastObserved)

			findings := w.detect(t)

			if got := len(findings) == 1 && reflect.DeepEqual(kinds(findings[0]), []skillstale.SignalKind{skillstale.SignalQuiet}); got != tc.quiet {
				t.Errorf("quiet = %v for LastObserved %q at %s, want %v (findings %+v)", got, tc.lastObserved, testNow.Format(time.RFC3339), tc.quiet, findings)
			}
			if tc.quiet && len(findings) == 1 && findings[0].Signals[0].Since != tc.lastObserved {
				t.Errorf("Since = %q, want the record's own words %q", findings[0].Signals[0].Since, tc.lastObserved)
			}
		})
	}
}

// The same record is quiet at one instant and not at another: the detector reads the time from Config and from
// nowhere else.
func TestDetect_TheSameRecordIsQuietOrNotAccordingToTheTimeGiven(t *testing.T) {
	for _, tc := range []struct {
		now   time.Time
		quiet bool
	}{
		{time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), false},
		{time.Date(2027, 8, 1, 0, 0, 0, 0, time.UTC), true},
		{time.Date(1999, 8, 1, 0, 0, 0, 0, time.UTC), false},
	} {
		w := newWorld().skill("s", text("Nothing to run.\n")).observed("s", "2026-07-01T00:00:00Z")
		cfg := w.config()
		cfg.Now = tc.now

		findings, err := skillstale.Detect(cfg)

		if err != nil || (len(findings) == 1) != tc.quiet {
			t.Errorf("at %s: findings %+v, error %v; want quiet = %v", tc.now.Format(time.DateOnly), findings, err, tc.quiet)
		}
	}
}

// A skill's instructions are not a record of the deletion of what they name: a removed path is reported even
// when the candidate was observed after the removal.
func TestDetect_ReportsARemovedPathWhateverTheCandidateSays(t *testing.T) {
	w := newWorld().skill("removed-path-skill", text("Inspect `removed.go` first.\n")).observed("removed-path-skill", "2099-01-01T00:00:00Z")
	w.facts.facts["removed.go"] = skillstale.PathFact{State: skillstale.PathDeleted, Commit: "c0ffee"}

	findings := w.detect(t)

	finding := findingOf(t, findings, "removed-path-skill")
	if !reflect.DeepEqual(kinds(finding), []skillstale.SignalKind{skillstale.SignalRemoved}) || finding.Signals[0].Path != "removed.go" {
		t.Errorf("signals = %+v, want REMOVED removed.go alone", finding.Signals)
	}
}

// A candidate has many observations over time; the one updated last speaks for it, and among those updated at
// the same moment the one with the highest id. A record with no topic key is no candidate's.
func TestDetect_ReadsTheNewestRecordOfACandidate(t *testing.T) {
	observation := func(id int64, updated, lastObserved, topic string) memory.Observation {
		return memory.Observation{ID: id, TopicKey: topic, UpdatedAt: updated,
			Content: "**Status**: status-" + fmt.Sprint(id) + "\n**LastObserved**: " + lastObserved + "\n"}
	}
	w := newWorld().skill("s", text("Nothing to run.\n"))
	w.observations.observations = []memory.Observation{
		observation(1, "2026-01-01 00:00:00", "2000-01-01T00:00:00Z", candidateOf("s")),
		observation(3, "2026-02-01 00:00:00", "2000-02-02T00:00:00Z", candidateOf("s")),
		observation(2, "2026-02-01 00:00:00", "2000-03-03T00:00:00Z", candidateOf("s")),
		observation(9, "2030-01-01 00:00:00", "2000-04-04T00:00:00Z", ""),
	}

	findings := w.detect(t)

	finding := findingOf(t, findings, "s")
	if finding.Status != "status-3" || finding.Signals[0].Since != "2000-02-02T00:00:00Z" {
		t.Errorf("finding = %+v, want the record of observation 3: the latest update, and the higher id of the two made together", finding)
	}
}

func TestDetect_ReadsTheCandidatesOfTheProjectItWasGiven(t *testing.T) {
	w := newWorld().skill("s", text("Nothing to run.\n"))

	w.detect(t)

	if !reflect.DeepEqual(w.observations.asked, []string{testProject}) {
		t.Errorf("the lister was asked for %v, want exactly the configured project %q once", w.observations.asked, testProject)
	}
}

// What is asked of the repository is what each skill names in its body: the paths it spells in code, in the order
// it spells them, and none of the front matter's. It is asked for every skill, one skill at a time, and for a
// skill that names no path as well.
func TestDetect_AsksForThePathsEachSkillNamesBelowItsFrontMatter(t *testing.T) {
	w := newWorld().
		skill("first", "---\nname: first\nnotes: see `front/matter.go`\n---\nSee `b/second.go` then `a/first.go`.\n").
		skill("second", text("No path here.\n")).
		skill("third", text("Only `c/third.go`.\n"))

	w.detect(t)

	want := [][]string{{"b/second.go", "a/first.go"}, nil, {"c/third.go"}}
	if !reflect.DeepEqual(w.facts.asked, want) {
		t.Errorf("the repository was asked about %q, want %q", w.facts.asked, want)
	}
}

func TestDetect_ReadsOnlyTheFirstTargetOfAnEntry(t *testing.T) {
	w := newWorld().skill("s", text("Nothing to run.\n"))

	w.detect(t)

	if want := []string{targetOf("s")}; !reflect.DeepEqual(w.source.skillsRead, want) {
		t.Errorf("skill files read = %v, want only the first target %v (all targets hold the same bytes)", w.source.skillsRead, want)
	}
	if w.source.lockReads != 1 {
		t.Errorf("the lock was read %d times, want once", w.source.lockReads)
	}
}

// Each command a skill runs is asked about once however often it is spelled, in the order it first appears; a
// comment, a prompt sign and the punctuation around a word are not part of the command, and a line with no
// command in it is not one.
func TestDetect_ResolvesEachCommandOnceAndOnlyThoseInFencedBlocks(t *testing.T) {
	w := newWorld().skill("s", text(
		"missing-prose is prose, and `inline-code` is not a block.\n"+
			fence+"sh\n# a comment\n$ first-tool run\nsecond-tool;\nfirst-tool again\n`\n\n"+fence+"\n"+
			"text between\n"+
			"````\nthird-tool\n"+fence+"\nstill-third-tool\n````\n"))

	findings := w.detect(t)

	if want := []string{"first-tool", "second-tool", "third-tool", "still-third-tool"}; !reflect.DeepEqual(w.commands.asked, want) {
		t.Errorf("commands resolved = %v, want %v", w.commands.asked, want)
	}
	var commands []string
	for _, signal := range findings[0].Signals {
		commands = append(commands, signal.Command)
	}
	if want := []string{"first-tool", "second-tool", "third-tool", "still-third-tool"}; !reflect.DeepEqual(commands, want) {
		t.Errorf("unresolved commands = %v, want %v", commands, want)
	}
}

// How a file begins decides what is its body: the front matter is not read, whether the file starts with a byte
// order mark or has carriage returns, and a file with no front matter, or whose front matter never closes, is
// all body.
func TestDetect_ReadsTheBodyBelowTheFrontMatterWhateverTheShapeOfTheFile(t *testing.T) {
	for _, tc := range []struct {
		name, content string
		paths         []string
	}{
		{"front matter", "---\nnotes: `front.go`\n---\nSee `body.go`.\n", []string{"body.go"}},
		{"a byte order mark", "\ufeff---\nnotes: `front.go`\n---\nSee `body.go`.\n", []string{"body.go"}},
		{"carriage returns", "---\r\nnotes: `front.go`\r\n---\r\nSee `body.go`.\r\n", []string{"body.go"}},
		{"no front matter", "See `body.go`.\n", []string{"body.go"}},
		{"a front matter that never closes", "---\nnotes: `front.go`\nSee `body.go`.\n", []string{"front.go", "body.go"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newWorld().skill("s", tc.content)

			w.detect(t)

			if want := [][]string{tc.paths}; !reflect.DeepEqual(w.facts.asked, want) {
				t.Errorf("the repository was asked about %q, want %q", w.facts.asked, want)
			}
		})
	}
}

// The runtime that looked the skill up globally can say what replaces one. The skill's own id is looked up
// first and its candidate after, and an empty answer says nothing.
func TestDetect_ReportsWhatSupersedesASkillByItsIDBeforeItsCandidate(t *testing.T) {
	for _, tc := range []struct {
		name       string
		superseded map[string]string
		by         string
	}{
		{"by the id", map[string]string{"s": "by-id"}, "by-id"},
		{"by the candidate", map[string]string{candidateOf("s"): "by-candidate"}, "by-candidate"},
		{"the id wins", map[string]string{"s": "by-id", candidateOf("s"): "by-candidate"}, "by-id"},
		{"an empty answer", map[string]string{"s": ""}, ""},
		{"a skill that is not this one", map[string]string{"other": "x"}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newWorld().skill("s", text("Nothing to run.\n"))
			w.superseded = tc.superseded

			findings := w.detect(t)

			if tc.by == "" {
				if len(findings) != 0 {
					t.Errorf("findings = %+v, want none", findings)
				}
				return
			}
			if len(findings) != 1 || !reflect.DeepEqual(findings[0].Signals, []skillstale.Signal{{Kind: skillstale.SignalSuperseded, By: tc.by}}) {
				t.Errorf("findings = %+v, want SUPERSEDED by %q", findings, tc.by)
			}
		})
	}
}

// The first thing that cannot be read ends the run, in the words of the step that failed and wrapping the
// port's own error, and nothing is asked of the ports after it.
func TestDetect_StopsAtTheFirstFailureAndSaysWhichStepFailed(t *testing.T) {
	broken := errors.New("the port is broken")
	for _, tc := range []struct {
		name    string
		breakIt func(w *world)
		want    string
		after   func(t *testing.T, w *world)
	}{
		{"the lock cannot be read", func(w *world) { w.source.lockErr = broken },
			"skillstale: read .labdrian/procedural-skills.lock.json: the port is broken",
			func(t *testing.T, w *world) {
				if len(w.observations.asked) != 0 || len(w.source.skillsRead) != 0 {
					t.Errorf("the detector went on after the lock: observations %v, skills %v", w.observations.asked, w.source.skillsRead)
				}
			}},
		{"the lock is not a lock", func(w *world) { w.source.lock = []byte(`{"version": 2, "skills": []}`) },
			"parse project lock: unsupported version 2, want 1",
			func(t *testing.T, w *world) {
				if len(w.observations.asked) != 0 {
					t.Errorf("the observations were listed after the lock was refused: %v", w.observations.asked)
				}
			}},
		{"the observations cannot be listed", func(w *world) { w.observations.err = broken },
			"skillstale: list candidate observations: the port is broken",
			func(t *testing.T, w *world) {
				if len(w.source.skillsRead) != 0 {
					t.Errorf("skill files were read after the observations failed: %v", w.source.skillsRead)
				}
			}},
		{"a skill file cannot be read", func(w *world) { w.source.skillErr[targetOf("first")] = broken },
			"skillstale: first: the port is broken",
			func(t *testing.T, w *world) {
				if len(w.source.skillsRead) != 1 {
					t.Errorf("skill files read = %v, want the first alone: the run ends there", w.source.skillsRead)
				}
			}},
		{"the repository cannot answer", func(w *world) { w.facts.err = broken },
			"skillstale: classify paths for first: the port is broken",
			func(t *testing.T, w *world) {
				if len(w.source.skillsRead) != 1 || len(w.facts.asked) != 1 {
					t.Errorf("skill files read = %v, repository asked %d times; want the run to end at the first skill", w.source.skillsRead, len(w.facts.asked))
				}
			}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newWorld().skill("first", text("Nothing.\n")).skill("second", text("Nothing.\n"))
			tc.breakIt(w)

			findings, err := skillstale.Detect(w.config())

			if err == nil || err.Error() != tc.want || findings != nil {
				t.Fatalf("Detect = %v, %v; want the error %q and no findings", findings, err, tc.want)
			}
			if strings.Contains(tc.want, "port is broken") && !errors.Is(err, broken) {
				t.Errorf("the error %v does not wrap the port's own", err)
			}
			tc.after(t, w)
		})
	}
}

func TestDetect_RefusesALockEntryWithNoTargets(t *testing.T) {
	w := newWorld()
	w.source.lock = []byte(`{"version": 1, "skills": [{"id": "bare", "provenance": "procedural", "candidate": "c", "sha256": "` + strings.Repeat("a", 64) + `", "revision": 1, "targets": []}]}`)

	_, err := skillstale.Detect(w.config())

	if err == nil || err.Error() != "skillstale: bare: lock entry has no targets" {
		t.Errorf("Detect = %v, want the entry named and the missing targets said", err)
	}
	if len(w.source.skillsRead) != 0 {
		t.Errorf("skill files read = %v, want none", w.source.skillsRead)
	}
}

// Every input is required, and the detector says which is missing in its own words: a port that is not there
// (an absent interface, or a nil pointer inside one, which is how a store that failed to open arrives) must not
// panic in the first call that would use it, and a time that was never set must not read as the year one.
func TestDetect_RequiresEveryInput(t *testing.T) {
	var nilSource *fakeSource
	var nilObservations *fakeObservations
	var nilFacts *fakeFacts
	var nilCommands *fakeCommands
	for _, tc := range []struct {
		name    string
		breakIt func(cfg *skillstale.Config)
		want    string
	}{
		{"no source", func(cfg *skillstale.Config) { cfg.Source = nil }, "skill source is required"},
		{"a nil source", func(cfg *skillstale.Config) { cfg.Source = nilSource }, "skill source is required"},
		{"no observations", func(cfg *skillstale.Config) { cfg.Observations = nil }, "observation lister is required"},
		{"a nil observations pointer", func(cfg *skillstale.Config) { cfg.Observations = nilObservations }, "observation lister is required"},
		{"no facts", func(cfg *skillstale.Config) { cfg.Facts = nil }, "path fact source is required"},
		{"a nil facts pointer", func(cfg *skillstale.Config) { cfg.Facts = nilFacts }, "path fact source is required"},
		{"no commands", func(cfg *skillstale.Config) { cfg.Commands = nil }, "command resolver is required"},
		{"a nil commands pointer", func(cfg *skillstale.Config) { cfg.Commands = nilCommands }, "command resolver is required"},
		{"no time", func(cfg *skillstale.Config) { cfg.Now = time.Time{} }, "the time to judge quietness by is required"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newWorld().skill("s", text("Nothing.\n"))
			cfg := w.config()
			tc.breakIt(&cfg)

			findings, err := skillstale.Detect(cfg)

			if err == nil || !strings.Contains(err.Error(), tc.want) || findings != nil {
				t.Errorf("Detect = %v, %v; want an error that says %q", findings, err, tc.want)
			}
			if w.source.lockReads != 0 {
				t.Errorf("the lock was read before the inputs were checked")
			}
		})
	}
}
