package app_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
)

// The tests of RecordSweeper and RecordPolisher (own-rdd T6b4, S6 c) run over the fakes of the other use cases. The
// commit of a sweeper is sweepSHA, a child of the head with the subject of a sweep.
const sweepSHA = "4444444444444444444444444444444444444444"

type roleRig struct {
	t       *testing.T
	svc     app.Service
	git     *fakeGit
	results *fakeResults
}

func newRoleRig(t *testing.T) *roleRig {
	t.Helper()
	g, r := newFakeGit(), newFakeResults()
	g.parents[sweepSHA] = headSHA
	g.messages = map[string]string{}
	g.messages[sweepSHA] = "chore(sweep): remove the dead helper\n\nbody\n"
	return &roleRig{t: t, svc: app.New(g, r), git: g, results: r}
}

// fired stores a sweep of the head that fired on two lines of the working tree, as number n, and returns its name.
func (r *roleRig) fired(n int, lines ...string) string {
	r.t.Helper()
	c := candidateOf(r.t, r.git)
	file, err := review.EncodeSweep(c, review.SweepFindings{T3: lines})
	if err != nil {
		r.t.Fatalf("setup: %v", err)
	}
	name := review.SweepName(c, n)
	r.results.put("own-rdd", name, file)
	return name
}

func (r *roleRig) sweeper(report string) (app.RoleRecorded, error) {
	return r.svc.RecordSweeper("/work/wt/feature", "own-rdd", "HEAD~1", "HEAD", []byte(report))
}

func (r *roleRig) polisher(findings string) (app.RoleRecorded, error) {
	return r.svc.RecordPolisher("/work/wt/feature", "own-rdd", "HEAD~1", "HEAD", []byte(findings))
}

func answer(candidate, disposition string) string {
	return fmt.Sprintf(`{"candidate":%q,"disposition":%q,"evidence":"read it"}`, candidate, disposition)
}

func report(commit string, answers ...string) string {
	return fmt.Sprintf(`{"commit":%s,"candidates":[%s]}`, commit, strings.Join(answers, ","))
}

func refusedBy(t *testing.T, err, sentinel error) {
	t.Helper()
	var refused *app.ReplyRefusedError
	if !errors.As(err, &refused) || !errors.Is(err, sentinel) {
		t.Errorf("error = %v, want a *ReplyRefusedError that wraps %v", err, sentinel)
	}
}

func TestRecordSweeperStoresTheReportWithTheBindingAndTheNameOfTheSweepItAnswers(t *testing.T) {
	r := newRoleRig(t)
	sweep := r.fired(1, "?? a.txt", "?? b.txt")

	got, err := r.sweeper(report(`"`+sweepSHA+`"`, answer("T3: ?? a.txt", "removed"), answer("T3: ?? b.txt", "live")))

	if err != nil {
		t.Fatalf("RecordSweeper error = %v", err)
	}
	c := candidateOf(t, r.git)
	name := review.SweeperName(c, 2)
	file := r.results.files["own-rdd"][name]
	if file == nil {
		t.Fatalf("no file stored as %s, stored %v", name, r.results.calls)
	}
	if !reflect.DeepEqual(got.Candidate, c) || got.Number != 2 || got.Path != review.ResultPath("own-rdd", name) || got.SHA256 != sha256Of(file) {
		t.Errorf("RoleRecorded = %+v, want candidate %v, number 2 (the sweep is 1), path %s, sha %s", got, c, review.ResultPath("own-rdd", name), sha256Of(file))
	}
	stored, err := review.DecodeSweeper(file)
	if err != nil || stored.Sweep != sweep || review.CheckRecordBinding(stored.Binding, c) != nil ||
		stored.Report.Commit == nil || *stored.Report.Commit != sweepSHA || len(stored.Report.Candidates) != 2 {
		t.Errorf("stored sweeper = %+v, error = %v; want it to read back with the binding of the candidate, the sweep %s and the report", stored, err, sweep)
	}
}

func TestRecordSweeperRefusesWhenNoFiredSweepBindsTheHead(t *testing.T) {
	clean := func(r *roleRig) {
		c := candidateOf(r.t, r.git)
		file, err := review.EncodeSweep(c, review.SweepFindings{})
		if err != nil {
			r.t.Fatalf("setup: %v", err)
		}
		r.results.put("own-rdd", review.SweepName(c, 1), file)
	}
	for _, tc := range []struct {
		name  string
		setup func(*roleRig)
	}{
		{"no sweep at all", func(*roleRig) {}},
		{"only a clean sweep", clean},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newRoleRig(t)
			tc.setup(r)
			before := r.results.created()

			_, err := r.sweeper(report("null"))

			refusedBy(t, err, review.ErrInvalidSweeperReport)
			if r.results.created() != before {
				t.Errorf("a file was stored for a head with no fired sweep: %v", r.results.calls)
			}
		})
	}
}

func TestRecordSweeperAnswersForTheLatestFiredSweepOfTheHead(t *testing.T) {
	r := newRoleRig(t)
	r.fired(1, "?? old.txt")
	r.fired(2, "?? new.txt")

	if _, err := r.sweeper(report("null", answer("T3: ?? old.txt", "live"))); err == nil {
		t.Error("a report that answers for the first sweep was stored, want it refused: the second sweep is the one that counts")
	}
	if _, err := r.sweeper(report("null", answer("T3: ?? new.txt", "live"))); err != nil {
		t.Errorf("a report that answers for the latest fired sweep: error = %v", err)
	}
}

func TestRecordSweeperRefusesAReportThatDoesNotAnswerForEveryCandidateOnce(t *testing.T) {
	for _, tc := range []struct {
		name   string
		report string
	}{
		{"a candidate missing", report("null", answer("T3: ?? a.txt", "live"))},
		{"a candidate answered twice", report("null", answer("T3: ?? a.txt", "live"), answer("T3: ?? a.txt", "live"), answer("T3: ?? b.txt", "live"))},
		{"a candidate that is not one of the sweep", report("null", answer("T3: ?? a.txt", "live"), answer("T3: ?? b.txt", "live"), answer("T3: ?? c.txt", "live"))},
		{"a removal with no commit", report("null", answer("T3: ?? a.txt", "removed"), answer("T3: ?? b.txt", "live"))},
		{"a correction with no commit", report("null", answer("T3: ?? a.txt", "corrected"), answer("T3: ?? b.txt", "live"))},
		{"a disposition outside the enum", report("null", answer("T3: ?? a.txt", "kept"), answer("T3: ?? b.txt", "live"))},
		{"an evidence with no text", report("null", `{"candidate":"T3: ?? a.txt","disposition":"live","evidence":" "}`, answer("T3: ?? b.txt", "live"))},
		{"not a report", `["T3: ?? a.txt"]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newRoleRig(t)
			r.fired(1, "?? a.txt", "?? b.txt")

			_, err := r.sweeper(tc.report)

			refusedBy(t, err, review.ErrInvalidSweeperReport)
			if r.results.created() != 0 {
				t.Errorf("a refused report stored a file: %v", r.results.calls)
			}
		})
	}
}

func TestRecordSweeperHoldsTheCommitToAChildOfTheHeadWithTheSubjectOfASweep(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(*roleRig)
	}{
		{"a commit whose first parent is not the head", func(r *roleRig) { r.git.parents[sweepSHA] = baseSHA }},
		{"a commit whose subject is not a sweep", func(r *roleRig) { r.git.messages[sweepSHA] = "fix: something else\n" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newRoleRig(t)
			r.fired(1, "?? a.txt")
			tc.setup(r)

			_, err := r.sweeper(report(`"`+sweepSHA+`"`, answer("T3: ?? a.txt", "removed")))

			refusedBy(t, err, review.ErrInvalidSweeperReport)
			if r.results.created() != 0 {
				t.Errorf("a refused report stored a file: %v", r.results.calls)
			}
		})
	}
}

func TestRecordSweeperRefusesACommitGitDoesNotHaveOrThatIsNotACommitBeforeAskingAnythingElseOfIt(t *testing.T) {
	r := newRoleRig(t)
	r.fired(1, "?? a.txt")
	r.git.notCommit = map[string]bool{sweepSHA: true}

	_, err := r.sweeper(report(`"`+sweepSHA+`"`, answer("T3: ?? a.txt", "removed")))

	refusedBy(t, err, review.ErrInvalidSweeperReport)
	if err == nil || !strings.Contains(err.Error(), sweepSHA) {
		t.Errorf("error = %v, want it to name the commit", err)
	}
	if r.results.created() != 0 || r.git.count("Parent "+sweepSHA) != 0 || r.git.count("Message "+sweepSHA) != 0 {
		t.Errorf("a refused report stored %d files, calls: %v, want none stored and no question about the parent or the message", r.results.created(), r.git.calls)
	}
}

func TestRecordSweeperIsAPlainErrorWhenGitCannotSayWhetherTheCommitIsOne(t *testing.T) {
	r := newRoleRig(t)
	r.fired(1, "?? a.txt")
	boom := errors.New("git is gone")
	r.git.fail["IsCommit"] = boom

	_, err := r.sweeper(report(`"`+sweepSHA+`"`, answer("T3: ?? a.txt", "removed")))

	var refused *app.ReplyRefusedError
	if !errors.Is(err, boom) || errors.As(err, &refused) || r.results.created() != 0 {
		t.Errorf("error = %v, created = %d, want the error of git, not a refusal of the report, and nothing stored", err, r.results.created())
	}
}

func TestRecordSweeperReturnsWhatGitCouldNotSayAboutTheCommit(t *testing.T) {
	r := newRoleRig(t)
	r.fired(1, "?? a.txt")
	boom := errors.New("git is gone")
	r.git.fail["Message"] = boom

	_, err := r.sweeper(report(`"`+sweepSHA+`"`, answer("T3: ?? a.txt", "removed")))

	if !errors.Is(err, boom) || r.results.created() != 0 {
		t.Errorf("error = %v, created = %d, want the error of git and nothing stored", err, r.results.created())
	}
}

func TestRecordSweeperAndRecordPolisherNeedTheBaseToBeTheFirstParentOfTheHead(t *testing.T) {
	r := newRoleRig(t)
	r.fired(1, "?? a.txt")
	r.git.parents[headSHA] = otherSHA

	_, sweeperErr := r.sweeper(report("null", answer("T3: ?? a.txt", "live")))
	_, polisherErr := r.polisher(`{"findings":[]}`)

	for name, err := range map[string]error{"sweeper": sweeperErr, "polisher": polisherErr} {
		if !errors.Is(err, review.ErrNotOneCommit) {
			t.Errorf("%s error = %v, want it to wrap ErrNotOneCommit", name, err)
		}
	}
	if r.results.created() != 0 {
		t.Errorf("a file was stored for a range of several commits: %v", r.results.calls)
	}
}

func TestRecordSweeperRefusesAStoredSweepItCannotReadBack(t *testing.T) {
	r := newRoleRig(t)
	c := candidateOf(t, r.git)
	r.results.put("own-rdd", review.SweepName(c, 1), []byte(`{"kind":"sweep"}`))

	_, err := r.sweeper(report("null"))

	if !errors.Is(err, review.ErrInvalidStored) || r.results.created() != 0 {
		t.Errorf("error = %v, created = %d, want ErrInvalidStored and nothing stored: a record that does not read back is not skipped", err, r.results.created())
	}
}

func TestRecordPolisherStoresTheFindingsVerbatimWithTheBinding(t *testing.T) {
	r := newRoleRig(t)
	r.results.put("own-rdd", review.ResultName(candidateOf(t, r.git), 1), []byte("{}"))
	findings := `{"findings":[{"severity":"x","evidence":["read it"]}]}`

	got, err := r.polisher(findings)

	if err != nil {
		t.Fatalf("RecordPolisher error = %v", err)
	}
	c := candidateOf(t, r.git)
	name := review.PolisherName(c, 2)
	file := r.results.files["own-rdd"][name]
	if file == nil {
		t.Fatalf("no file stored as %s, stored %v", name, r.results.calls)
	}
	if !reflect.DeepEqual(got.Candidate, c) || got.Number != 2 || got.Path != review.ResultPath("own-rdd", name) || got.SHA256 != sha256Of(file) {
		t.Errorf("RoleRecorded = %+v, want candidate %v, number 2, path %s, sha %s", got, c, review.ResultPath("own-rdd", name), sha256Of(file))
	}
	stored, err := review.DecodePolisher(file)
	var compact bytes.Buffer
	if err != nil || review.CheckRecordBinding(stored.Binding, c) != nil || json.Compact(&compact, stored.Findings) != nil || compact.String() != findings {
		t.Errorf("stored polisher = %s, error = %v; want it to read back with the binding of the candidate and the findings %s", stored.Findings, err, findings)
	}
}

func TestRecordPolisherRefusesFindingsThatAreNotTheObjectOfTheJudge(t *testing.T) {
	for _, tc := range []struct {
		name     string
		findings string
		want     error
	}{
		{"not an object", `[]`, review.ErrInvalidPolisherFindings},
		{"no findings member", `{}`, review.ErrInvalidPolisherFindings},
		{"a finding with no evidence", `{"findings":[{"severity":"x"}]}`, review.ErrInvalidPolisherFindings},
		{"input over the bound", `{"findings":[],"pad":"` + strings.Repeat("x", review.MaxReplyBytes) + `"}`, review.ErrReplyTooLarge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newRoleRig(t)

			_, err := r.polisher(tc.findings)

			refusedBy(t, err, tc.want)
			if r.results.created() != 0 || r.git.called("Locate") != 0 {
				t.Errorf("a refused input stored a file or asked git: %v %v", r.results.calls, r.git.calls)
			}
		})
	}
}

func TestRecordPolisherRefusesFindingsWhoseStoredFileWouldBeOverTheStoredBound(t *testing.T) {
	r := newRoleRig(t)
	// Deep nesting indents to far more bytes than it takes as input.
	deep := strings.Repeat(`[`, 5000) + strings.Repeat(`]`, 5000)
	findings := `{"findings":[{"evidence":["e"],"deep":` + deep + `}]}`
	if len(findings) > review.MaxReplyBytes {
		t.Fatalf("setup: the input is %d bytes, over the input bound", len(findings))
	}

	_, err := r.polisher(findings)

	refusedBy(t, err, review.ErrReplyTooLarge)
	if r.results.created() != 0 {
		t.Errorf("a file was stored: %v", r.results.calls)
	}
}

func TestRecordRoleRefusesAnUnsafeFeatureBeforeAskingAnyone(t *testing.T) {
	r := newRoleRig(t)

	_, sweeperErr := r.svc.RecordSweeper("/work/wt/feature", "../x", "HEAD~1", "HEAD", []byte(report("null")))
	_, polisherErr := r.svc.RecordPolisher("/work/wt/feature", "../x", "HEAD~1", "HEAD", []byte(`{"findings":[]}`))

	for name, err := range map[string]error{"sweeper": sweeperErr, "polisher": polisherErr} {
		if !errors.Is(err, review.ErrInvalidFeature) {
			t.Errorf("%s error = %v, want ErrInvalidFeature", name, err)
		}
	}
	if len(r.git.calls) != 0 || len(r.results.calls) != 0 {
		t.Errorf("git calls %v and store calls %v, want none", r.git.calls, r.results.calls)
	}
}

func TestRecordRoleReportsARecordWhoseDirectoryWasNotFlushedAsStored(t *testing.T) {
	r := newRoleRig(t)
	r.fired(1, "?? a.txt")
	r.results.unflushed = fmt.Errorf("%w: disk", app.ErrNotFlushed)

	got, err := r.polisher(`{"findings":[]}`)

	if err != nil || !errors.Is(got.NotFlushed, app.ErrNotFlushed) || got.Path == "" {
		t.Errorf("RoleRecorded = %+v, error = %v, want the record reported as stored with NotFlushed set", got, err)
	}
}
