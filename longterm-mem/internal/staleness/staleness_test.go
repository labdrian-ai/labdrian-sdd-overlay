package staleness_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/staleness"
)

// Measured against a real database, a naive path scrape pulls in issue
// references, diff stats and URLs and calls them files. Every one of those
// would later be reported as a missing file, which is a finding invented
// out of punctuation.
func TestPaths_TakesFilesAndNothingElse(t *testing.T) {
	content := "**What**: a thing\n" +
		"**Where**: internal/promote/writer.go, .github/workflows/ci.yml (#139/#138), +111/-1, " +
		"https://example.com/a/b.go, README.md\n" +
		"**Learned**: nothing\n"

	got := staleness.Paths(content)
	want := map[string]bool{
		"internal/promote/writer.go": true,
		".github/workflows/ci.yml":   true,
		"README.md":                  true,
	}
	if len(got) != len(want) {
		t.Fatalf("extracted %v, want exactly %d file paths", got, len(want))
	}
	for _, p := range got {
		if !want[p] {
			t.Errorf("%q is not a file path this repository could hold", p)
		}
	}
}

// A skill's instructions name files in code spans and fenced blocks, and only there: prose, URLs and the same
// path twice are not references. (The comment that sat here before described the finding Detect exists for,
// which is the one on TestDetect_APathRemovedAfterTheMemoryWasWritten below.)
func TestReferencedPaths_ExtractsOnlyCodeSpansAndFencedBlocks(t *testing.T) {
	text := "prose internal/not-code.go and https://example.test/nope.go\n" +
		"inline `internal/inline.go`, `internal/inline.go`\n" +
		"```sh\ncat internal/fenced.sh\nkeep internal/fenced.json\n```\n"

	got := staleness.ReferencedPaths(text)
	want := []string{"internal/inline.go", "internal/fenced.sh", "internal/fenced.json"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ReferencedPaths() = %#v, want %#v", got, want)
	}
}

// fakeEvidence is a repository in a table: the files of its tree, and what its history says of the paths it is
// asked about (a path it has no word for is unknown, as a repository that never held it would say). It records
// every question, so that a test can say what was asked and what was not.
type fakeEvidence struct {
	files      []string
	filesErr   error
	history    map[string]staleness.PathFact
	historyErr error

	filesAsked int
	asked      [][]string
}

func (f *fakeEvidence) Files() ([]string, error) {
	f.filesAsked++
	if f.filesErr != nil {
		return nil, f.filesErr
	}
	return f.files, nil
}

func (f *fakeEvidence) History(paths []string) (map[string]staleness.PathFact, error) {
	f.asked = append(f.asked, append([]string(nil), paths...))
	if f.historyErr != nil {
		return nil, f.historyErr
	}
	facts := make(map[string]staleness.PathFact, len(paths))
	for _, p := range paths {
		fact, known := f.history[p]
		if !known {
			fact = staleness.PathFact{State: staleness.PathUnknown}
		}
		fact.Path = p
		facts[p] = fact
	}
	return facts, nil
}

// theRepository is the repository most of the cases below ask about: kept.go and a module's writer are in the
// tree, gone.go was removed by a commit of 2027, and pkg/moved.go was renamed by one.
func theRepository() *fakeEvidence {
	return &fakeEvidence{
		files: []string{"kept.go", "module/promote/writer.go", "module/moved-root.go"},
		history: map[string]staleness.PathFact{
			"gone.go":      {State: staleness.PathDeleted, Commit: "c0ffee1", At: time.Date(2027, 6, 1, 0, 0, 0, 0, time.UTC)},
			"pkg/moved.go": {State: staleness.PathRenamed, NewPath: "pkg/renamed.go", Commit: "c0ffee2", At: time.Date(2027, 6, 2, 0, 0, 0, 0, time.UTC)},
			// The tree was listed before this one came back as a file; a fact of "present" from the
			// history is not a finding.
			"elsewhere.go": {State: staleness.PathPresent},
		},
	}
}

func TestClassifyPaths_UsesTreeAndHistory(t *testing.T) {
	evidence := theRepository()

	got, err := staleness.ClassifyPaths(evidence, []string{"kept.go", "gone.go", "pkg/moved.go", "never-was.go"})

	if err != nil {
		t.Fatalf("ClassifyPaths: %v", err)
	}
	want := map[string]staleness.PathFact{
		"kept.go":      {Path: "kept.go", State: staleness.PathPresent},
		"gone.go":      {Path: "gone.go", State: staleness.PathDeleted, Commit: "c0ffee1", At: time.Date(2027, 6, 1, 0, 0, 0, 0, time.UTC)},
		"pkg/moved.go": {Path: "pkg/moved.go", State: staleness.PathRenamed, NewPath: "pkg/renamed.go", Commit: "c0ffee2", At: time.Date(2027, 6, 2, 0, 0, 0, 0, time.UTC)},
		"never-was.go": {Path: "never-was.go", State: staleness.PathUnknown},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ClassifyPaths = %+v, want %+v", got, want)
	}
}

// Only a path the tree cannot account for is worth a question of the history, and it is asked once, for all of
// them, each distinct path once and in order: a path the tree holds never reaches the history.
func TestClassifyPaths_AsksTheHistoryOnceForTheDistinctPathsTheTreeDoesNotHold(t *testing.T) {
	evidence := theRepository()

	_, err := staleness.ClassifyPaths(evidence, []string{"kept.go", "zeta.go", "gone.go", "kept.go", "gone.go", "alpha.go"})

	if err != nil {
		t.Fatalf("ClassifyPaths: %v", err)
	}
	if want := [][]string{{"alpha.go", "gone.go", "zeta.go"}}; !reflect.DeepEqual(evidence.asked, want) {
		t.Errorf("the history was asked %v, want %v", evidence.asked, want)
	}
	if evidence.filesAsked != 1 {
		t.Errorf("the tree was listed %d times, want once", evidence.filesAsked)
	}
}

// The history is not asked at all when the tree accounts for everything, nor when there is nothing to ask about.
func TestClassifyPaths_NeedsNoHistoryWhenTheTreeHoldsEverything(t *testing.T) {
	for _, paths := range [][]string{{"kept.go", "kept.go"}, {"kept.go", "promote/writer.go"}, nil, {}} {
		evidence := theRepository()

		got, err := staleness.ClassifyPaths(evidence, paths)

		if err != nil || len(evidence.asked) != 0 {
			t.Errorf("ClassifyPaths(%v) asked the history %v and failed with %v; want no question and no error", paths, evidence.asked, err)
		}
		for _, p := range paths {
			if got[p].State != staleness.PathPresent || got[p].Path != p {
				t.Errorf("ClassifyPaths(%v)[%q] = %+v, want present", paths, p, got[p])
			}
		}
	}
}

// A path recorded under a different root of the tree is resolved against the tree by its trailing segments (and
// without a leading "./"), before the history is consulted: it is alive, not a suspected deletion.
func TestClassifyPaths_ResolvesAPathByTheTrailingSegmentsOfTheTree(t *testing.T) {
	evidence := theRepository()

	got, err := staleness.ClassifyPaths(evidence, []string{"promote/writer.go", "writer.go", "./kept.go", "module/promote/writer.go", "module/promote", "promote"})

	if err != nil {
		t.Fatalf("ClassifyPaths: %v", err)
	}
	for _, held := range []string{"promote/writer.go", "writer.go", "./kept.go", "module/promote/writer.go"} {
		if got[held].State != staleness.PathPresent {
			t.Errorf("%q = %+v, want present: the tree holds a file that ends in it", held, got[held])
		}
	}
	// A directory is not a file of the tree: its name alone resolves nothing.
	if want := [][]string{{"module/promote", "promote"}}; !reflect.DeepEqual(evidence.asked, want) {
		t.Errorf("the history was asked %v, want %v: a directory's name is no file", evidence.asked, want)
	}
}

// What a fact from the history says is not second-guessed: a path the tree did not account for that the history
// knows to be there is there.
func TestClassifyPaths_TakesTheHistorysWordForAPathTheTreeDoesNotName(t *testing.T) {
	got, err := staleness.ClassifyPaths(theRepository(), []string{"elsewhere.go"})

	if err != nil || got["elsewhere.go"].State != staleness.PathPresent {
		t.Errorf("ClassifyPaths = %+v, %v; want elsewhere.go present, as the history says", got, err)
	}
}

// The texts the strict pattern of the detectors never produces are still questions: an empty text, an absolute
// path and a detour that stays inside the project are not resolved against the tree, whatever the tree holds,
// and go to the history as they are written; a leading "./" is the one thing taken off. (What the history
// answers for them is its own, pinned in internal/repohistory.)
func TestClassifyPaths_PassesWhatTheTreeCannotResolveToTheHistoryAsWritten(t *testing.T) {
	evidence := &fakeEvidence{files: []string{"name.go", "a/name.go"}}

	_, err := staleness.ClassifyPaths(evidence, []string{"", "/name.go", "a/../name.go", "./name.go", ".//name.go"})

	if err != nil {
		t.Fatalf("ClassifyPaths: %v", err)
	}
	want := [][]string{{"", ".//name.go", "/name.go", "a/../name.go"}}
	if !reflect.DeepEqual(evidence.asked, want) {
		t.Errorf("the history was asked %v, want %v", evidence.asked, want)
	}
}

// A path that climbs out of the project (once its detours are taken out) names a place the repository has
// nothing to say about: it is unknown, and it is not a question for the history, which would refuse it and so
// take the whole report down with it. (The pattern of the detectors admits a climb in the middle of a path.)
// This is so whatever the tree holds: a climb is no path of the tree, even when its last segments name a file.
func TestClassifyPaths_APathThatClimbsOutOfTheProjectIsUnknownAndNeverAsked(t *testing.T) {
	climbing := []string{"a/../../outside.go", "../name.go", "..", "a/b/../../../name.go", "./../name.go", "a/../../a/name.go", "../"}
	history := map[string]staleness.PathFact{}
	for _, p := range climbing {
		history[p] = staleness.PathFact{State: staleness.PathDeleted, Commit: "c0ffee3", At: time.Date(2027, 6, 1, 0, 0, 0, 0, time.UTC)}
	}
	evidence := &fakeEvidence{files: []string{"name.go", "a/name.go"}, history: history}

	got, err := staleness.ClassifyPaths(evidence, climbing)

	if err != nil {
		t.Fatalf("ClassifyPaths: %v", err)
	}
	for _, p := range climbing {
		if want := (staleness.PathFact{Path: p, State: staleness.PathUnknown}); got[p] != want {
			t.Errorf("ClassifyPaths[%q] = %+v, want %+v", p, got[p], want)
		}
	}
	if len(got) != len(climbing) {
		t.Errorf("ClassifyPaths gave %d facts for %d paths: %+v", len(got), len(climbing), got)
	}
	if len(evidence.asked) != 0 {
		t.Errorf("the history was asked %v; a path that climbs out of the project is not a question for it", evidence.asked)
	}
}

// What stays inside the project is still a question, however it detours: only a climb out of the root is
// unknown, and a name that merely starts with dots is a name.
func TestClassifyPaths_AsksAboutAPathThatDetoursButStaysInsideTheProject(t *testing.T) {
	evidence := &fakeEvidence{files: []string{"name.go"}}
	inside := []string{"a/../gone.go", "a/b/../../gone2.go", "a/b/../c.go", "..hidden/x.go", "a/..b/x.go", "a/b../x.go", "...go", "./a/../gone3.go"}

	_, err := staleness.ClassifyPaths(evidence, inside)

	if err != nil {
		t.Fatalf("ClassifyPaths: %v", err)
	}
	want := [][]string{{"...go", "..hidden/x.go", "./a/../gone3.go", "a/../gone.go", "a/..b/x.go", "a/b../x.go", "a/b/../../gone2.go", "a/b/../c.go"}}
	if !reflect.DeepEqual(evidence.asked, want) {
		t.Errorf("the history was asked %v, want %v", evidence.asked, want)
	}
}

// One path that leaves the project does not take the others down: those that can be judged are, with the one
// question of the history for them alone.
func TestClassifyPaths_AClimbingPathLeavesTheOthersToTheirOwnEvidence(t *testing.T) {
	evidence := theRepository()

	got, err := staleness.ClassifyPaths(evidence, []string{"kept.go", "a/../../outside.go", "gone.go"})

	if err != nil {
		t.Fatalf("ClassifyPaths: %v", err)
	}
	if got["kept.go"].State != staleness.PathPresent || got["gone.go"].State != staleness.PathDeleted || got["a/../../outside.go"].State != staleness.PathUnknown {
		t.Errorf("ClassifyPaths = %+v", got)
	}
	if want := [][]string{{"gone.go"}}; !reflect.DeepEqual(evidence.asked, want) {
		t.Errorf("the history was asked %v, want %v", evidence.asked, want)
	}
}

// The tree that cannot be listed is the answer, before the history is asked about anything.
func TestClassifyPaths_FailsWithTheTreesErrorBeforeAskingTheHistory(t *testing.T) {
	boom := errors.New("the tree is gone")
	evidence := &fakeEvidence{filesErr: boom}

	got, err := staleness.ClassifyPaths(evidence, []string{"gone.go"})

	if !errors.Is(err, boom) || got != nil || len(evidence.asked) != 0 {
		t.Errorf("ClassifyPaths = %v, %v (history asked %v); want the tree's error, no facts, and no question", got, err, evidence.asked)
	}
}

// Nothing to classify still lists the tree: a repository that cannot be read is told, with or without paths.
func TestClassifyPaths_ListsTheTreeEvenWithNothingToClassify(t *testing.T) {
	boom := errors.New("the tree is gone")

	got, err := staleness.ClassifyPaths(&fakeEvidence{filesErr: boom}, nil)

	if !errors.Is(err, boom) || got != nil {
		t.Errorf("ClassifyPaths(nil) = %v, %v; want the tree's error and no facts", got, err)
	}
}

// A history that cannot answer is the answer for all the paths at once, and no fact is returned: not those the
// tree accounts for either.
func TestClassifyPaths_FailsWithTheHistorysErrorAndNoFacts(t *testing.T) {
	boom := errors.New("the history is unreadable")
	evidence := &fakeEvidence{files: []string{"kept.go"}, historyErr: boom}

	got, err := staleness.ClassifyPaths(evidence, []string{"kept.go", "lost.go"})

	if !errors.Is(err, boom) || got != nil {
		t.Errorf("ClassifyPaths = %v, %v; want the history's error and no facts", got, err)
	}
}

func observation(id int64, title, where, updatedAt string) memory.Observation {
	return memory.Observation{ID: id, Title: title, Content: "**Where**: " + where + "\n", UpdatedAt: updatedAt}
}

func detect(t *testing.T, evidence *fakeEvidence, obs ...memory.Observation) []staleness.Finding {
	t.Helper()
	got, err := staleness.Detect(evidence, obs)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	return got
}

// The finding the whole feature exists for: a memory describing something that was removed from the project
// AFTER the memory was written. Left unreported, an agent reads it and reintroduces what was extirpated.
func TestDetect_APathRemovedAfterTheMemoryWasWritten(t *testing.T) {
	got := detect(t, theRepository(), observation(1, "how gone.go works", "gone.go", "2026-01-01 00:00:00"))

	if len(got) != 1 || got[0].ObservationID != 1 || got[0].Title != "how gone.go works" {
		t.Fatalf("a memory describing a removed file must be reported: got %+v", got)
	}
	if len(got[0].Removed) != 1 || got[0].Removed[0].Commit != "c0ffee1" || got[0].Removed[0].Path != "gone.go" {
		t.Fatalf("the finding must carry the commit that removed it: %+v", got[0])
	}
}

// The inverse, and it matters just as much: a memory written AFTER the removal is very likely the record OF the
// removal. Reporting it would invite deleting the one memory that says the thing is gone -- the exact knowledge
// that stops it being reintroduced. A memory written at the very instant is not after it either.
func TestDetect_APathRemovedBeforeOrAtTheInstantTheMemoryWasWrittenIsNotStale(t *testing.T) {
	for _, written := range []string{"2030-01-01 00:00:00", "2027-06-01 00:00:00"} {
		got := detect(t, theRepository(), observation(2, "we removed gone.go on purpose", "gone.go", written))
		if len(got) != 0 {
			t.Errorf("a memory written %s records the removal; reporting it invites deleting the very thing that prevents the revival: %+v", written, got)
		}
	}
}

// The false positive measured on the real database: a path recorded relative to a different root. It looked
// absent and is alive under a module of the tree; without resolving it against the tree this is a false report
// of a removal, which is the finding that gets a live memory deleted -- and the history is never asked.
func TestDetect_APathRecordedUnderAnotherRootIsAlive(t *testing.T) {
	evidence := theRepository()
	evidence.history["moved-root.go"] = staleness.PathFact{State: staleness.PathDeleted, Commit: "c0ffee3", At: time.Date(2027, 6, 3, 0, 0, 0, 0, time.UTC)}

	got := detect(t, evidence, observation(3, "about a file recorded without its module", "moved-root.go", "2026-01-01 00:00:00"))

	if len(got) != 0 || len(evidence.asked) != 0 {
		t.Fatalf("a file that exists under a different root is alive, not removed, and the history is not asked: %+v (asked %v)", got, evidence.asked)
	}
}

// A rename leaves the old path absent and the content alive. The memory is out of date about WHERE, never wrong
// about WHAT.
func TestDetect_ARenameIsReportedAsMovedNotRemoved(t *testing.T) {
	got := detect(t, theRepository(), observation(4, "about a moved file", "pkg/moved.go", "2026-01-01 00:00:00"))

	if len(got) != 1 {
		t.Fatalf("a moved file is worth saying out loud: %+v", got)
	}
	if len(got[0].Removed) != 0 {
		t.Errorf("a rename must never be reported as a removal: %+v", got[0])
	}
	if len(got[0].Moved) != 1 || got[0].Moved[0].NewPath != "pkg/renamed.go" || got[0].Moved[0].Commit != "c0ffee2" {
		t.Errorf("the move must say where it went: %+v", got[0].Moved)
	}
}

// A move is reported whenever the memory was written: it is not a claim that anything was removed after.
func TestDetect_AMoveIsReportedWhateverTheTimeOfTheMemory(t *testing.T) {
	for _, written := range []string{"2026-01-01 00:00:00", "2030-01-01 00:00:00", "yesterday"} {
		got := detect(t, theRepository(), observation(4, "about a moved file", "pkg/moved.go", written))
		if len(got) != 1 || len(got[0].Moved) != 1 {
			t.Errorf("a memory written %q: %+v, want the move reported", written, got)
		}
	}
}

// The other false positive from the real database: observations naming another repository's files. This
// repository has nothing to say about them, and saying something anyway is inventing evidence.
func TestDetect_APathThisRepositoryNeverHadIsIgnored(t *testing.T) {
	got := detect(t, theRepository(), observation(5, "about gentle-ai's code", "internal/cli/review_facade.go", "2026-01-01 00:00:00"))

	if len(got) != 0 {
		t.Fatalf("another project's file is not evidence about this one: %+v", got)
	}
}

// A path the tree did not name that the history knows to be there is not a finding.
func TestDetect_APathTheHistoryKnowsToBeThereIsNotAFinding(t *testing.T) {
	if got := detect(t, theRepository(), observation(6, "about elsewhere.go", "elsewhere.go", "2026-01-01 00:00:00")); len(got) != 0 {
		t.Fatalf("a path that is there was reported: %+v", got)
	}
}

// Undatable memory is not reported as stale: assuming a zero time would make every deletion look later than every
// memory, turning an unparsed timestamp into a finding. The layouts Engram's timestamps come in are read, the
// update first and the creation when there is none.
func TestDetect_ReadsTheLayoutsOfAMemorysTimeAndNothingElse(t *testing.T) {
	for _, tc := range []struct {
		name               string
		createdAt, updated string
		reported           bool
	}{
		{"SQLite text", "2026-01-01 00:00:00", "2026-01-01 00:00:00", true},
		{"RFC 3339", "2030-01-01 00:00:00", "2026-01-01T00:00:00Z", true},
		{"a T and no zone", "2030-01-01 00:00:00", "2026-01-01T00:00:00", true},
		{"no update, created before", "2026-01-01 00:00:00", "", true},
		{"no update, created after", "2030-01-01 00:00:00", "  ", false},
		{"updated after, created before", "2026-01-01 00:00:00", "2030-01-01 00:00:00", false},
		{"updated in no layout, created before", "2026-01-01 00:00:00", "yesterday", true},
		{"nothing readable", "last week", "yesterday", false},
		{"nothing", "", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := observation(7, "t", "gone.go", tc.updated)
			o.CreatedAt = tc.createdAt

			got := detect(t, theRepository(), o)

			if reported := len(got) == 1 && len(got[0].Removed) == 1; reported != tc.reported {
				t.Errorf("created %q, updated %q: findings %+v, want reported = %v", tc.createdAt, tc.updated, got, tc.reported)
			}
		})
	}
}

// Every memory's own paths decide its own findings, in the order the memories came, and within a memory in the
// order the paths are named; one question of the history serves them all, and a path two memories name is asked
// once.
func TestDetect_ReportsEachMemoryByItsOwnPathsInOrderWithOneQuestionOfTheHistory(t *testing.T) {
	evidence := theRepository()

	got := detect(t, evidence,
		observation(30, "first: a path that is there", "kept.go", "2026-01-01 00:00:00"),
		observation(10, "second: a move and a removal", "pkg/moved.go, kept.go, gone.go, never.go", "2026-01-01 00:00:00"),
		observation(20, "third: written after the removal", "gone.go", "2030-01-01 00:00:00"),
		observation(40, "fourth: the removal again", "gone.go", "2026-01-01 00:00:00"),
	)

	if len(got) != 2 || got[0].ObservationID != 10 || got[1].ObservationID != 40 {
		t.Fatalf("findings = %+v, want those of the memories 10 and 40, in the order they came", got)
	}
	if len(got[0].Moved) != 1 || len(got[0].Removed) != 1 || got[0].Removed[0].Path != "gone.go" || got[0].Moved[0].Path != "pkg/moved.go" {
		t.Errorf("memory 10 = %+v, want a move and a removal", got[0])
	}
	if want := [][]string{{"gone.go", "never.go", "pkg/moved.go"}}; !reflect.DeepEqual(evidence.asked, want) {
		t.Errorf("the history was asked %v, want %v: once, for the distinct paths the tree does not hold", evidence.asked, want)
	}
}

// Two memories that carry the same id (a database does not allow it) are still two memories, each judged by its
// own paths.
func TestDetect_JudgesAMemoryByItsOwnPathsWhateverItsId(t *testing.T) {
	got := detect(t, theRepository(),
		observation(1, "names a removed path", "gone.go", "2026-01-01 00:00:00"),
		observation(1, "names a path that is there", "kept.go", "2026-01-01 00:00:00"),
	)

	if len(got) != 1 || got[0].Title != "names a removed path" {
		t.Fatalf("findings = %+v, want the one of the memory that names the removed path", got)
	}
}

// A memory that names a path that climbs out of the project (the pattern admits it) neither fails the report nor
// hides what the rest of it says: that path is unknown and no finding, and the other paths of the memory and
// the other memories are reported as they would be without it.
func TestDetect_APathThatClimbsOutOfTheProjectIsSkippedAndTheRestOfTheReportStands(t *testing.T) {
	evidence := theRepository()

	got := detect(t, evidence,
		observation(1, "names a path above the project", "a/../../outside.go", "2026-01-01 00:00:00"),
		observation(2, "names it beside a removed one", "kept.go, a/../../outside.go, gone.go", "2026-01-01 00:00:00"),
		observation(3, "names a path that is moved", "pkg/moved.go", "2026-01-01 00:00:00"),
	)

	if len(got) != 2 || got[0].ObservationID != 2 || got[1].ObservationID != 3 {
		t.Fatalf("findings = %+v, want those of memories 2 and 3", got)
	}
	if len(got[0].Removed) != 1 || got[0].Removed[0].Path != "gone.go" || len(got[0].Moved) != 0 {
		t.Errorf("memory 2 = %+v, want gone.go alone as removed", got[0])
	}
	if want := [][]string{{"gone.go", "pkg/moved.go"}}; !reflect.DeepEqual(evidence.asked, want) {
		t.Errorf("the history was asked %v, want %v", evidence.asked, want)
	}
}

// Nothing the tree does not hold, nothing to ask: no question of the history and no findings.
func TestDetect_NeedsNoHistoryWhenTheTreeHoldsEveryPath(t *testing.T) {
	evidence := theRepository()

	got := detect(t, evidence,
		observation(1, "a path that is there", "kept.go, promote/writer.go", "2026-01-01 00:00:00"),
		memory.Observation{ID: 2, Title: "names no file", Content: "**What**: nothing\n"},
	)

	if got != nil || len(evidence.asked) != 0 {
		t.Errorf("Detect = %+v, history asked %v; want no findings (nil) and no question", got, evidence.asked)
	}
}

// With no memory at all the tree is still listed, so a repository that cannot be read is told.
func TestDetect_ListsTheTreeEvenWithNoMemory(t *testing.T) {
	boom := errors.New("the tree is gone")

	got, err := staleness.Detect(&fakeEvidence{filesErr: boom}, nil)

	if !errors.Is(err, boom) || got != nil {
		t.Errorf("Detect = %v, %v; want the tree's error", got, err)
	}
}

// The errors of the evidence are the answer, unchanged.
func TestDetect_FailsWithTheEvidencesErrors(t *testing.T) {
	treeBoom, historyBoom := errors.New("the tree is gone"), errors.New("the history is unreadable")
	o := observation(1, "t", "gone.go", "2026-01-01 00:00:00")

	if got, err := staleness.Detect(&fakeEvidence{filesErr: treeBoom}, []memory.Observation{o}); !errors.Is(err, treeBoom) || got != nil {
		t.Errorf("Detect over a tree that cannot be listed = %v, %v; want its error", got, err)
	}
	if got, err := staleness.Detect(&fakeEvidence{historyErr: historyBoom}, []memory.Observation{o}); !errors.Is(err, historyBoom) || got != nil {
		t.Errorf("Detect over a history that cannot answer = %v, %v; want its error", got, err)
	}
}
