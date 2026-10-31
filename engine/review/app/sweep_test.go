package app_test

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
)

// sweepRig is a Service wired for Sweep over the fakes. The commit it sweeps changes docs/a.md only, which needs
// no tool; a test sets what it wants the commit to change.
type sweepRig struct {
	t       *testing.T
	svc     app.Service
	git     *fakeGit
	results *fakeResults
	tools   *fakeAnalyzers
	copies  *fakeCopies
}

func newSweepRig(t *testing.T) *sweepRig {
	t.Helper()
	g, r, a, c := newFakeGit(), newFakeResults(), newFakeAnalyzers(), newFakeCopies()
	g.paths = []string{"docs/a.md"}
	return &sweepRig{t: t, svc: app.New(g, r).WithSweep(a, c), git: g, results: r, tools: a, copies: c}
}

func (s *sweepRig) sweep() (app.Swept, error) {
	return s.svc.Sweep("/work/wt/feature", "own-rdd", "HEAD~1", "HEAD")
}

// mustSweep sweeps and fails the test when the sweep is refused.
func (s *sweepRig) mustSweep() app.Swept {
	s.t.Helper()
	got, err := s.sweep()
	if err != nil {
		s.t.Fatalf("Sweep error = %v", err)
	}
	return got
}

// stored is the sweep record stored for the head, read back as a reader would.
func (s *sweepRig) stored() review.StoredSweep {
	s.t.Helper()
	for name, data := range s.results.files["own-rdd"] {
		if strings.HasSuffix(name, ".sweep.json") {
			got, err := review.DecodeSweep(data)
			if err != nil {
				s.t.Fatalf("the stored sweep %s does not read back: %v", name, err)
			}
			return got
		}
	}
	s.t.Fatal("no sweep record is stored")
	return review.StoredSweep{}
}

func TestSweepStoresACleanSweepForACommitThatNeedsNoTool(t *testing.T) {
	s := newSweepRig(t)

	got := s.mustSweep()

	c := candidateOf(t, s.git)
	name := review.SweepName(c, 1)
	file := s.results.files["own-rdd"][name]
	if file == nil {
		t.Fatalf("no file stored as %s, stored %v", name, s.results.calls)
	}
	if !reflect.DeepEqual(got.Candidate, c) || got.Number != 1 || got.Path != review.ResultPath("own-rdd", name) || got.SHA256 != sha256Of(file) || got.Outcome != review.OutcomeClean {
		t.Errorf("Swept = %+v, want candidate %v, number 1, path %s, sha %s, clean", got, c, review.ResultPath("own-rdd", name), sha256Of(file))
	}
	if len(got.Candidates) != 0 {
		t.Errorf("Candidates = %q, want none for a clean sweep", got.Candidates)
	}
	stored := s.stored()
	if err := review.CheckRecordBinding(stored.Binding, c); err != nil || stored.Fired() {
		t.Errorf("stored sweep: binding error = %v, fired = %v, want a binding that holds and clean", err, stored.Fired())
	}
	if len(s.tools.calls) != 0 || s.git.called("Archive") != 0 {
		t.Errorf("tools %q and archives %d, want none for a commit with no Go module and no shell file", s.tools.calls, s.git.called("Archive"))
	}
	if want := []string{"Lock /work/main", "Unlock"}; !reflect.DeepEqual(s.copies.calls, want) {
		t.Errorf("copies calls = %q, want %q: the lock is taken and given back", s.copies.calls, want)
	}
}

func TestSweepNumbersItsRecordWithTheOtherRecordsOfTheHead(t *testing.T) {
	s := newSweepRig(t)
	c := candidateOf(t, s.git)
	s.results.put("own-rdd", review.ResultName(c, 1), []byte("{}"))
	s.results.put("own-rdd", review.ExemptionName(c, 2), []byte("{}"))

	got := s.mustSweep()

	if got.Number != 3 || got.Path != review.ResultPath("own-rdd", review.SweepName(c, 3)) {
		t.Errorf("Swept number = %d, path = %s, want 3 and the sweep name of number 3", got.Number, got.Path)
	}
}

func TestSweepFiresOnAPathTheCommitDeletedOrRenamed(t *testing.T) {
	s := newSweepRig(t)
	s.git.deleted = []string{"old/a.go", "old/b.sh"}

	got := s.mustSweep()

	stored := s.stored()
	if !stored.Fired() || !reflect.DeepEqual(stored.Findings.T1.DeletedOrRenamed, s.git.deleted) {
		t.Errorf("stored sweep fired = %v, t1 paths = %q, want fired with %q", stored.Fired(), stored.Findings.T1.DeletedOrRenamed, s.git.deleted)
	}
	want := []string{"T1 path: old/a.go", "T1 path: old/b.sh"}
	if got.Outcome != review.OutcomeFired || !reflect.DeepEqual(got.Candidates, want) {
		t.Errorf("Swept outcome = %q, candidates = %q, want fired and %q", got.Outcome, got.Candidates, want)
	}
	if want := "DeletedOrRenamed " + baseSHA + " " + headSHA; s.git.count(want) != 1 {
		t.Errorf("calls = %q, want %q once, with the full names", s.git.calls, want)
	}
}

func TestSweepListsTheRemovedNamesThatTheHeadStillMentionsOutsideTheDiff(t *testing.T) {
	s := newSweepRig(t)
	s.git.paths = []string{"pkg/a.go", "pkg/b.go"}
	s.git.removedLines = []review.ChangedLines{{Path: "pkg/a.go", Lines: []string{"func gone() {}", "func movedAway() {}", "func declaredAgain() {}", "func nobodyMentions() {}"}}}
	s.git.addedLines = []review.ChangedLines{{Path: "pkg/b.go", Lines: []string{"func declaredAgain() {}"}}}
	s.git.mentions = map[string][]string{
		"gone":      {"pkg/a.go", "pkg/other.go", "docs/x.md"},
		"movedAway": {"pkg/a.go", "pkg/b.go"},
	}

	got := s.mustSweep()

	want := []review.RemovedName{{Name: "gone", Hits: []string{"docs/x.md", "pkg/other.go"}}}
	if names := s.stored().Findings.T1.RemovedNames; !reflect.DeepEqual(names, want) {
		t.Errorf("stored removed names = %v, want %v: only the paths outside the diff count, sorted", names, want)
	}
	if !reflect.DeepEqual(got.Candidates, []string{"T1 name: gone"}) {
		t.Errorf("Candidates = %q, want only the name that is still mentioned", got.Candidates)
	}
	var searches []string
	for _, c := range s.git.calls {
		if strings.HasPrefix(c, "Mentions ") {
			searches = append(searches, c)
		}
	}
	if want := []string{"Mentions " + headSHA + " gone,movedAway,nobodyMentions"}; !reflect.DeepEqual(searches, want) {
		t.Errorf("searches = %q, want one for all the names, none for a name the head declares again", searches)
	}
}

// T4 lists the leftovers a commit adds (own-rdd T12b, S15 b): a changed path ending .orig or .bak that the commit
// does not delete.
func TestSweepFiresOnALeftoverTheCommitAdds(t *testing.T) {
	s := newSweepRig(t)
	s.git.paths = []string{"docs/a.md", "d/y.bak", "x.go.orig", "gone.orig"}
	s.git.deleted = []string{"gone.orig"}

	got := s.mustSweep()

	stored := s.stored()
	if want := []string{"d/y.bak", "x.go.orig"}; !stored.Fired() || !reflect.DeepEqual(stored.Findings.T4.Leftovers, want) {
		t.Errorf("stored sweep fired = %v, t4 leftovers = %q, want fired with %q", stored.Fired(), stored.Findings.T4.Leftovers, want)
	}
	if len(stored.Findings.T4.OrphanedFixtures) != 0 {
		t.Errorf("t4 orphaned fixtures = %q, want none for a commit with no package", stored.Findings.T4.OrphanedFixtures)
	}
	want := []string{"T1 path: gone.orig", "T4 leftover: d/y.bak", "T4 leftover: x.go.orig"}
	if got.Outcome != review.OutcomeFired || !reflect.DeepEqual(got.Candidates, want) {
		t.Errorf("Swept outcome = %q, candidates = %q, want fired and %q", got.Outcome, got.Candidates, want)
	}
}

// T4 lists the fixtures a commit leaves unread (own-rdd T12c, S15 c): orphaned at the head and not at the base.
func TestSweepFiresOnAFixtureTheCommitLeavesUnread(t *testing.T) {
	s := newSweepRig(t)
	s.git.paths = []string{"a/x_test.go"}
	files := []string{"a/x_test.go", "a/testdata/old.json", "a/testdata/kept.json"}
	s.git.treeFiles = map[string][]string{baseSHA: files, headSHA: files}
	s.git.mentionsAt = map[string]map[string][]string{
		baseSHA: {"old.json": {"a/x_test.go"}, "kept.json": {"a/x_test.go"}},
		headSHA: {"kept.json": {"a/x_test.go"}},
	}

	got := s.mustSweep()

	stored := s.stored()
	if want := []string{"a/testdata/old.json"}; !stored.Fired() || !reflect.DeepEqual(stored.Findings.T4.OrphanedFixtures, want) {
		t.Errorf("stored sweep fired = %v, t4 fixtures = %q, want fired with %q", stored.Fired(), stored.Findings.T4.OrphanedFixtures, want)
	}
	if got.Outcome != review.OutcomeFired || !reflect.DeepEqual(got.Candidates, []string{"T4 fixture: a/testdata/old.json"}) {
		t.Errorf("Swept outcome = %q, candidates = %q, want fired with the fixture", got.Outcome, got.Candidates)
	}
	var searches []string
	for _, c := range s.git.calls {
		if strings.HasPrefix(c, "Mentions ") {
			searches = append(searches, c)
		}
	}
	want := []string{"Mentions " + baseSHA + " kept.json,old.json", "Mentions " + headSHA + " kept.json,old.json"}
	if !reflect.DeepEqual(searches, want) {
		t.Errorf("searches = %q, want one per side with the names of the fixtures", searches)
	}
}

func TestSweepDoesNotFireOnAFixtureAlreadyUnreadAtTheBase(t *testing.T) {
	s := newSweepRig(t)
	s.git.paths = []string{"a/x_test.go"}
	files := []string{"a/x_test.go", "a/testdata/old.json"}
	s.git.treeFiles = map[string][]string{baseSHA: files, headSHA: files}

	got := s.mustSweep()

	if got.Outcome != review.OutcomeClean || len(s.stored().Findings.T4.OrphanedFixtures) != 0 {
		t.Errorf("outcome = %q, fixtures = %q, want clean: the fixture was unread at the base too", got.Outcome, s.stored().Findings.T4.OrphanedFixtures)
	}
}

func TestSweepAsksNoFixtureSearchForACommitWithNoPackage(t *testing.T) {
	s := newSweepRig(t)
	s.git.paths = []string{"README.md"}

	s.mustSweep()

	if s.git.called("TreeFiles") != 0 {
		t.Errorf("calls = %q, want no TreeFiles for a commit that changes only README.md", s.git.calls)
	}
}

func TestSweepRefusesAndStoresNothingWhenTheFilesOfATreeCannotBeListed(t *testing.T) {
	s := newSweepRig(t)
	s.git.paths = []string{"a/x_test.go"}
	s.git.fail["TreeFiles"] = errors.New("boom")

	_, err := s.sweep()

	if err == nil || !strings.Contains(err.Error(), "list the files of") || len(s.results.files["own-rdd"]) != 0 {
		t.Errorf("error = %v, stored %v, want a refusal naming the listing and nothing stored", err, s.results.files)
	}
}

func TestSweepFiresOnARemovedFlagTheHeadStillMentionsOutsideTheDiff(t *testing.T) {
	s := newSweepRig(t)
	s.git.paths = []string{"pkg/a.go"}
	s.git.removedLines = []review.ChangedLines{{Path: "pkg/a.go", Lines: []string{"func gone() {}", `f.has("--onto")`}}}
	s.git.mentions = map[string][]string{"gone": {"docs/x.md"}, "--onto": {"README.md", "pkg/a.go"}}

	got := s.mustSweep()

	want := []review.RemovedName{{Name: "--onto", Hits: []string{"README.md"}}, {Name: "gone", Hits: []string{"docs/x.md"}}}
	if names := s.stored().Findings.T1.RemovedNames; !reflect.DeepEqual(names, want) {
		t.Errorf("stored removed names = %v, want %v: the flag first, hits outside the diff", names, want)
	}
	if got.Outcome != review.OutcomeFired || !reflect.DeepEqual(got.Candidates, []string{"T1 name: --onto", "T1 name: gone"}) {
		t.Errorf("Swept outcome = %q, candidates = %q, want fired with the flag and the name", got.Outcome, got.Candidates)
	}
	var searches []string
	for _, c := range s.git.calls {
		if strings.HasPrefix(c, "Mentions ") {
			searches = append(searches, c)
		}
	}
	if want := []string{"Mentions " + headSHA + " gone", "Mentions " + headSHA + " --onto"}; !reflect.DeepEqual(searches, want) {
		t.Errorf("searches = %q, want the names in one call and the flags in a call of their own", searches)
	}
}

func TestSweepIsCleanForAFlagNothingOutsideTheDiffMentions(t *testing.T) {
	s := newSweepRig(t)
	s.git.paths = []string{"pkg/a.go"}
	s.git.removedLines = []review.ChangedLines{{Path: "pkg/a.go", Lines: []string{`f.has("--onto")`}}}
	s.git.mentions = map[string][]string{"--onto": {"pkg/a.go"}}

	got := s.mustSweep()

	if got.Outcome != review.OutcomeClean || len(s.stored().Findings.T1.RemovedNames) != 0 {
		t.Errorf("Swept outcome = %q, removed names = %v, want clean: the only hit is inside the diff", got.Outcome, s.stored().Findings.T1.RemovedNames)
	}
}

func TestSweepSearchesNothingWhenTheCommitRemovesNoName(t *testing.T) {
	s := newSweepRig(t)

	s.mustSweep()

	for _, c := range s.git.calls {
		if strings.HasPrefix(c, "Mentions ") {
			t.Errorf("git was asked %q, want no search for no name", c)
		}
	}
}

func TestSweepRunsDeadcodeAndStaticcheckOnTheCopiesOfEveryModuleTheCommitChanges(t *testing.T) {
	s := newSweepRig(t)
	s.git.paths = []string{"mod/a/x.go", "mod/a/y.go", "mod/b/z.go", "mod/go.sum", "docs/a.md"}
	s.git.modules = []string{"mod"}
	s.tools.out["Deadcode /state/base mod"] = []string{"/state/base/mod/a/x.go:3:1: unreachable func: Same"}
	s.tools.out["Deadcode /state/head mod"] = []string{"/state/head/mod/a/x.go:9:1: unreachable func: Same", "/state/head/mod/a/x.go:12:1: unreachable func: New"}
	s.tools.out["Staticcheck /state/head mod ./a,./b"] = []string{"/state/head/mod/b/z.go:4:2: this is wrong (SA1000)"}

	got := s.mustSweep()

	want := []review.SweepT2{
		{Tool: "deadcode", Module: "mod", New: []string{"mod/a/x.go: unreachable func: New"}},
		{Tool: "staticcheck", Module: "mod", New: []string{"mod/b/z.go: this is wrong (SA1000)"}},
	}
	if t2 := s.stored().Findings.T2; !reflect.DeepEqual(t2, want) {
		t.Errorf("stored t2 = %v, want %v: the new lines only, without the root of the copy or the position", t2, want)
	}
	wantIDs := []string{"T2 deadcode mod: mod/a/x.go: unreachable func: New", "T2 staticcheck mod: mod/b/z.go: this is wrong (SA1000)"}
	if !reflect.DeepEqual(got.Candidates, wantIDs) {
		t.Errorf("Candidates = %q, want %q", got.Candidates, wantIDs)
	}
	wantTools := []string{
		"Deadcode /state/base mod", "Deadcode /state/head mod",
		"Staticcheck /state/base mod ./a,./b", "Staticcheck /state/head mod ./a,./b",
		"Tidy /state/base mod", "Tidy /state/head mod", // mod/go.sum is changed
	}
	if !reflect.DeepEqual(s.tools.calls, wantTools) {
		t.Errorf("tool calls = %q, want %q: the whole module for deadcode, the packages of the changed files for staticcheck", s.tools.calls, wantTools)
	}
	wantCopies := []string{"Lock /work/main", "Reset base", "Reset head", "Unlock"}
	if !reflect.DeepEqual(s.copies.calls, wantCopies) {
		t.Errorf("copies calls = %q, want %q", s.copies.calls, wantCopies)
	}
	for _, call := range []string{"Archive " + baseSHA + " /state/base", "Archive " + headSHA + " /state/head"} {
		if s.git.count(call) != 1 {
			t.Errorf("calls = %q, want %q once", s.git.calls, call)
		}
	}
}

func TestSweepOfOnlyModuleFilesRunsDeadcodeAndNoStaticcheck(t *testing.T) {
	s := newSweepRig(t)
	s.git.paths = []string{"mod/go.mod", "mod/go.sum"}
	s.git.modules = []string{"mod"}

	s.mustSweep()

	want := []string{"Deadcode /state/base mod", "Deadcode /state/head mod", "Tidy /state/base mod", "Tidy /state/head mod"}
	if !reflect.DeepEqual(s.tools.calls, want) {
		t.Errorf("tool calls = %q, want %q: no changed .go file, no package for staticcheck", s.tools.calls, want)
	}
}

func TestSweepGivesAChangedFileToTheInnermostModuleOnly(t *testing.T) {
	s := newSweepRig(t)
	s.git.paths = []string{"tools/x/y.go", "z.go", "loose/w.go"}
	s.git.modules = []string{".", "tools/x"}

	s.mustSweep()

	want := []string{
		"Deadcode /state/base .", "Deadcode /state/head .",
		"Staticcheck /state/base . .,./loose", "Staticcheck /state/head . .,./loose",
		"Deadcode /state/base tools/x", "Deadcode /state/head tools/x",
		"Staticcheck /state/base tools/x .", "Staticcheck /state/head tools/x .",
	}
	if !reflect.DeepEqual(s.tools.calls, want) {
		t.Errorf("tool calls = %q, want %q", s.tools.calls, want)
	}
}

func TestSweepRunsNoGoToolForAGoFileOutsideEveryModule(t *testing.T) {
	s := newSweepRig(t)
	s.git.paths = []string{"scratch/a.go"}
	s.git.modules = []string{"mod"}

	s.mustSweep()

	if len(s.tools.calls) != 0 || s.git.called("Archive") != 0 {
		t.Errorf("tool calls = %q, archives = %d, want none for a file no module holds", s.tools.calls, s.git.called("Archive"))
	}
}

func TestSweepRunsShellcheckOnTheChangedShellFiles(t *testing.T) {
	s := newSweepRig(t)
	s.git.paths = []string{"scripts/b.sh", "scripts/a.sh", "docs/a.md"}
	s.tools.out["Shellcheck /state/head scripts/b.sh,scripts/a.sh"] = []string{"/state/head/scripts/a.sh:3:5: warning: quote this [SC2086]"}

	got := s.mustSweep()

	want := []review.SweepT2{{Tool: "shellcheck", Module: ".", New: []string{"scripts/a.sh: warning: quote this [SC2086]"}}}
	if t2 := s.stored().Findings.T2; !reflect.DeepEqual(t2, want) {
		t.Errorf("stored t2 = %v, want %v", t2, want)
	}
	if !reflect.DeepEqual(got.Candidates, []string{"T2 shellcheck .: scripts/a.sh: warning: quote this [SC2086]"}) {
		t.Errorf("Candidates = %q", got.Candidates)
	}
	if n := len(s.tools.calls); n != 2 {
		t.Errorf("tool calls = %q, want shellcheck on the base and on the head only", s.tools.calls)
	}
}

func TestSweepIsCleanWhenTheHeadAddsNoLineToWhatTheBaseHad(t *testing.T) {
	s := newSweepRig(t)
	s.git.paths = []string{"mod/a.go"}
	s.git.modules = []string{"mod"}
	s.tools.out["Deadcode /state/base mod"] = []string{"/state/base/a.go:3:1: unreachable func: Old"}
	s.tools.out["Deadcode /state/head mod"] = []string{"/state/head/a.go:8:1: unreachable func: Old"}

	got := s.mustSweep()

	if got.Outcome != review.OutcomeClean || len(s.stored().Findings.T2) != 0 {
		t.Errorf("outcome = %q, t2 = %v, want clean with no t2 entry for a finding that was already there", got.Outcome, s.stored().Findings.T2)
	}
}

func TestSweepT3IsTheStatusOfTheWorkingTreeWithoutTheRecordsWaitingForTheirCommit(t *testing.T) {
	s := newSweepRig(t)
	s.git.status = []string{
		" M a.txt",
		"?? odd/review-results/own-rdd/abc123456789-1.json",
		"?? odd/review-results-extra/x.json",
		`?? "odd/review-results/caf\303\251/x.json"`,
		"?? b.txt",
	}

	got := s.mustSweep()

	want := []string{" M a.txt", "?? odd/review-results-extra/x.json", "?? b.txt"}
	if t3 := s.stored().Findings.T3; !reflect.DeepEqual(t3, want) {
		t.Errorf("stored t3 = %q, want %q", t3, want)
	}
	if !reflect.DeepEqual(got.Candidates, []string{"T3:  M a.txt", "T3: ?? odd/review-results-extra/x.json", "T3: ?? b.txt"}) {
		t.Errorf("Candidates = %q", got.Candidates)
	}
}

func TestSweepRefusesWhileAnotherSweepOfTheRepositoryRuns(t *testing.T) {
	s := newSweepRig(t)
	s.git.paths = []string{"x.go"}
	s.git.modules = []string{"."}
	s.copies.fail["Lock"] = app.ErrSweepBusy

	_, err := s.sweep()

	if err == nil || err.Error() != "another review sweep of this repository is running" || !errors.Is(err, app.ErrSweepBusy) {
		t.Errorf("Sweep error = %v, want ErrSweepBusy: another review sweep of this repository is running", err)
	}
	if s.results.created() != 0 || len(s.tools.calls) != 0 || s.git.called("Archive") != 0 || s.git.called("Status") != 0 {
		t.Errorf("created = %d, tool calls = %q, archives = %d, status = %d, want nothing done while the lock is not held", s.results.created(), s.tools.calls, s.git.called("Archive"), s.git.called("Status"))
	}
	if want := []string{"Lock /work/main"}; !reflect.DeepEqual(s.copies.calls, want) {
		t.Errorf("copies calls = %q, want only the refused lock: a sweep that never held it gives nothing back", s.copies.calls)
	}
}

func TestSweepGivesTheLockBackAndStoresNothingWhenAnythingAfterItFails(t *testing.T) {
	boom := errors.New("boom")
	cases := []struct {
		name  string
		setup func(s *sweepRig)
	}{
		{"deleted paths", func(s *sweepRig) { s.git.fail["DeletedOrRenamed"] = boom }},
		{"changed lines", func(s *sweepRig) { s.git.fail["ChangedLines"] = boom }},
		{"mentions", func(s *sweepRig) {
			s.git.removedLines = []review.ChangedLines{{Path: "a.go", Lines: []string{"func gone() {}"}}}
			s.git.fail["Mentions"] = boom
		}},
		{"modules", func(s *sweepRig) { s.git.fail["Modules"] = boom }},
		{"reset of a copy", func(s *sweepRig) {
			s.git.paths = []string{"x.go"}
			s.git.modules = []string{"."}
			s.copies.fail["Reset"] = boom
		}},
		{"archive of a copy", func(s *sweepRig) {
			s.git.paths = []string{"x.go"}
			s.git.modules = []string{"."}
			s.git.fail["Archive"] = boom
		}},
		{"deadcode", func(s *sweepRig) {
			s.git.paths = []string{"x.go"}
			s.git.modules = []string{"."}
			s.tools.fail["Deadcode"] = boom
		}},
		{"staticcheck", func(s *sweepRig) {
			s.git.paths = []string{"x.go"}
			s.git.modules = []string{"."}
			s.tools.fail["Staticcheck"] = boom
		}},
		{"shellcheck", func(s *sweepRig) { s.git.paths = []string{"x.sh"}; s.tools.fail["Shellcheck"] = boom }},
		{"status", func(s *sweepRig) { s.git.fail["Status"] = boom }},
		{"the store", func(s *sweepRig) { s.results.fail["Create"] = boom }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newSweepRig(t)
			c.setup(s)

			_, err := s.sweep()

			if !errors.Is(err, boom) {
				t.Errorf("Sweep error = %v, want one wrapping boom", err)
			}
			if n := len(s.copies.calls); n == 0 || s.copies.calls[n-1] != "Unlock" {
				t.Errorf("copies calls = %q, want the lock given back last", s.copies.calls)
			}
			if c.name != "the store" && s.results.created() != 0 {
				t.Errorf("created = %d, want nothing stored", s.results.created())
			}
		})
	}
}

func TestSweepFiresOnARequirementTheCommitLeavesUnused(t *testing.T) {
	s := newSweepRig(t)
	s.git.paths = []string{"engine/x.go"}
	s.git.modules = []string{"engine"}
	s.git.removedLines = []review.ChangedLines{{Path: "engine/x.go", Lines: []string{"\t\"github.com/x/y\""}}}
	s.tools.out["Tidy /state/head engine"] = []string{"go.mod: require github.com/x/y v1.2.3"}

	got := s.mustSweep()

	want := []review.SweepT2{{Tool: "go-mod-tidy", Module: "engine", New: []string{"go.mod: require github.com/x/y v1.2.3"}}}
	if t2 := s.stored().Findings.T2; !reflect.DeepEqual(t2, want) {
		t.Errorf("stored t2 = %v, want %v", t2, want)
	}
	if got.Outcome != review.OutcomeFired || !reflect.DeepEqual(got.Candidates, []string{"T2 go-mod-tidy engine: go.mod: require github.com/x/y v1.2.3"}) {
		t.Errorf("outcome = %q, candidates = %q, want fired with the requirement", got.Outcome, got.Candidates)
	}
	wantCalls := []string{"Deadcode /state/base engine", "Deadcode /state/head engine", "Staticcheck /state/base engine .", "Staticcheck /state/head engine .", "Tidy /state/base engine", "Tidy /state/head engine"}
	if !reflect.DeepEqual(s.tools.calls, wantCalls) {
		t.Errorf("tool calls = %q, want %q: tidy after the other two, on both copies", s.tools.calls, wantCalls)
	}
}

func TestSweepRunsTidyForAModuleWhoseGoModOrGoSumTheCommitChanges(t *testing.T) {
	s := newSweepRig(t)
	s.git.paths = []string{"mod/go.sum"}
	s.git.modules = []string{"mod"}

	s.mustSweep()

	if s.git.called("ChangedLines") != 1 {
		t.Errorf("calls = %q, want the lines read once, by T1 alone: go.sum already picks the module", s.git.calls)
	}
	if want := []string{"Tidy /state/base mod", "Tidy /state/head mod"}; len(s.tools.calls) != 4 || !reflect.DeepEqual(s.tools.calls[2:], want) {
		t.Errorf("tool calls = %q, want tidy on both copies after deadcode", s.tools.calls)
	}
}

func TestSweepRunsNoTidyForACommitThatTouchesNoDependency(t *testing.T) {
	s := newSweepRig(t)
	s.git.paths = []string{"engine/x.go"}
	s.git.modules = []string{"engine"}
	s.git.removedLines = []review.ChangedLines{{Path: "engine/x.go", Lines: []string{"\t\"fmt\"", "func gone() {}"}}}

	s.mustSweep()

	for _, call := range s.tools.calls {
		if strings.HasPrefix(call, "Tidy") {
			t.Errorf("tool calls = %q, want no tidy: a .go change with no removed import and no go.mod or go.sum", s.tools.calls)
		}
	}
}

func TestSweepStoresNothingWhenTidyDidNotRun(t *testing.T) {
	s := newSweepRig(t)
	s.git.paths = []string{"mod/go.mod"}
	s.git.modules = []string{"mod"}
	s.tools.fail["Tidy"] = fmt.Errorf("%w: go mod tidy: exit status 1", app.ErrToolDidNotRun)

	_, err := s.sweep()

	if !errors.Is(err, app.ErrToolDidNotRun) || !strings.Contains(err.Error(), "go mod tidy") {
		t.Errorf("Sweep error = %v, want ErrToolDidNotRun naming go mod tidy", err)
	}
	if s.results.created() != 0 {
		t.Errorf("created = %d, want nothing stored", s.results.created())
	}
}

func TestSweepThatCannotRunACheckNamesTheToolAndStoresNothing(t *testing.T) {
	s := newSweepRig(t)
	s.git.paths = []string{"x.go"}
	s.git.modules = []string{"."}
	s.tools.fail["Staticcheck"] = fmt.Errorf("%w: staticcheck was not found on the PATH", app.ErrToolDidNotRun)

	_, err := s.sweep()

	if !errors.Is(err, app.ErrToolDidNotRun) || !strings.Contains(err.Error(), "staticcheck") {
		t.Errorf("Sweep error = %v, want ErrToolDidNotRun naming staticcheck", err)
	}
	if s.results.created() != 0 || s.git.called("Status") != 0 {
		t.Errorf("created = %d, status = %d, want a sweep that stops at the check that did not run", s.results.created(), s.git.called("Status"))
	}
}

func TestSweepRefusesBeforeItTakesTheLock(t *testing.T) {
	cases := []struct {
		name    string
		feature string
		setup   func(s *sweepRig)
		want    error
	}{
		{"a feature that is not a segment", "../x", func(*sweepRig) {}, review.ErrInvalidFeature},
		{"a base that is not the first parent of the head", "own-rdd", func(s *sweepRig) { s.git.parents[headSHA] = otherSHA }, review.ErrNotOneCommit},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newSweepRig(t)
			c.setup(s)

			_, err := s.svc.Sweep("/work/wt/feature", c.feature, "HEAD~1", "HEAD")

			if !errors.Is(err, c.want) {
				t.Errorf("Sweep error = %v, want one wrapping %v", err, c.want)
			}
			if len(s.copies.calls) != 0 || s.results.created() != 0 {
				t.Errorf("copies calls = %q, created = %d, want nothing done", s.copies.calls, s.results.created())
			}
		})
	}
}

func TestSweepRefusesToRunWithoutBothPortsOfWithSweep(t *testing.T) {
	cases := []struct {
		name  string
		build func(svc app.Service, a *fakeAnalyzers, c *fakeCopies) app.Service
	}{
		{"no WithSweep", func(svc app.Service, _ *fakeAnalyzers, _ *fakeCopies) app.Service { return svc }},
		{"only the analyzers", func(svc app.Service, a *fakeAnalyzers, _ *fakeCopies) app.Service { return svc.WithSweep(a, nil) }},
		{"only the copies", func(svc app.Service, _ *fakeAnalyzers, c *fakeCopies) app.Service { return svc.WithSweep(nil, c) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g, r := newFakeGit(), newFakeResults()
			svc := c.build(app.New(g, r), newFakeAnalyzers(), newFakeCopies())

			_, err := svc.Sweep("/work/wt/feature", "own-rdd", "HEAD~1", "HEAD")

			if err == nil || len(g.calls) != 0 || r.created() != 0 {
				t.Errorf("Sweep error = %v, git calls = %q, created = %d, want a refusal before anything is asked", err, g.calls, r.created())
			}
		})
	}
}

func TestSweepRunsWithBothPortsOfWithSweep(t *testing.T) {
	s := newSweepRig(t)

	got := s.mustSweep()

	if got.Outcome != review.OutcomeClean || s.results.created() != 1 {
		t.Errorf("outcome = %q, created = %d, want a clean sweep stored once", got.Outcome, s.results.created())
	}
}
