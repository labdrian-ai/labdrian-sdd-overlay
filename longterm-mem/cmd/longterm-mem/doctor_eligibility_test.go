package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/ops"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/opstest"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/promote"
)

// The promoted-pages-eligible check of `longterm-mem doctor` reads the promoted pages of the vault against the
// observations of Engram they were promoted from, and warns of each whose observation sync no longer keeps
// current (issue #293). A warning is advisory: it is printed, in the text and in the JSON, and the exit code is
// the one the command would have answered without it.
//
// The worlds here are the healthy world of embedding-ops-02 (a complete index, every other check passing, exit
// 0) with pages promoted from chosen observations of the golden project, so that nothing but the eligibility of
// those pages is in question: observation 12 is not pinned and has no topic_key, 1 is pinned, 2 and 3 carry
// curated topics, 4 is soft-deleted and 5 belongs to another project.

// firstPromotedAddress is the address of the first page promotedPages writes; the next ones follow it.
const firstPromotedAddress = 901

// promotedPages writes, into the vault of the world, the page a promotion makes of each observation named (read
// from the database as it is now), and everything the other checks read of a promoted page: its address map
// entry, its precedence record and its catalog and log entries. The pages are c-000901, c-000902, ... in the
// order given.
func (w *retrievalWorld) promotedPages(t *testing.T, ids ...int64) {
	t.Helper()
	store, err := engram.Open(w.dbPath)
	if err != nil {
		t.Fatalf("open the fixture database: %v", err)
	}
	defer store.Close()
	addressMap := map[string]string{}
	for i, id := range ids {
		obs, found, err := store.ObservationByID(id)
		if err != nil || !found {
			t.Fatalf("observation %d of the fixture database: found=%v err=%v", id, found, err)
		}
		address := fmt.Sprintf("c-%06d", firstPromotedAddress+i)
		page, err := promote.EmitPage(opstest.EngramMemory(obs), address, nil, fixtureInstant)
		if err != nil {
			t.Fatalf("EmitPage for observation %d: %v", id, err)
		}
		full := filepath.Join(w.vaultRoot, page.Path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir for %s: %v", page.Path, err)
		}
		if err := os.WriteFile(full, []byte(page.Frontmatter+page.Body), 0o644); err != nil {
			t.Fatalf("write %s: %v", page.Path, err)
		}
		addressMap[page.Path] = address
		opstest.WritePrecedenceEntry(t, w.vaultRoot, page)
		opstest.RegisterPage(t, w.vaultRoot, address, obs.Title)
	}
	opstest.WriteAddressMap(t, w.vaultRoot, addressMap)
}

// eligibilityWorld is the healthy world with one page promoted from each of the observations named, after edit
// has changed the database (a person pinning an observation, or taking its topic away). The index is built last,
// of the live rows as they are.
func eligibilityWorld(t *testing.T, edit func(t *testing.T, w *retrievalWorld), promoted ...int64) *retrievalWorld {
	t.Helper()
	putPython3OnPath(t)
	w := newRetrievalWorld(t)
	w.promotedPages(t, promoted...)
	if edit != nil {
		edit(t, w)
	}
	w.mirrorToMemorycard(t)
	w.buildIndexOfLiveRows(t, nil, nil)
	return w
}

// mirrorToMemorycard gives the world the memorycard database `doctor` joins the pages to: a memory of every observation
// of the Engram database that is not soft-deleted, under the same id, with its project, topic and pin, and the
// import_map row that names the observation it was imported from, as it is when the edits to the world are done.
func (w *retrievalWorld) mirrorToMemorycard(t *testing.T) {
	t.Helper()
	db, err := sql.Open("sqlite", w.dbPath)
	if err != nil {
		t.Fatalf("open the fixture database: %v", err)
	}
	defer db.Close()
	rows, err := db.Query(`select id, project, coalesce(topic_key, ''), pinned from observations where deleted_at is null order by id`)
	if err != nil {
		t.Fatalf("read the observations: %v", err)
	}
	defer rows.Close()
	var statements []string
	for rows.Next() {
		var id, pinned int64
		var project, topic string
		if err := rows.Scan(&id, &project, &topic, &pinned); err != nil {
			t.Fatal(err)
		}
		statements = append(statements,
			fmt.Sprintf(`insert into memories(id, project, title, body, topic_key, pinned) values (%d, '%s', 'Memory %d', 'Body.', nullif('%s', ''), %d)`, id, project, id, topic, pinned),
			fmt.Sprintf(`insert into import_map values ('engram', '%d', 1, %d)`, id, id))
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	t.Setenv(memorycardDBEnvVar, memorycardDatabase(t, statements...))
}

// pinObservation12 is the edit that keeps the page promoted from observation 12 current.
func pinObservation12(t *testing.T, w *retrievalWorld) {
	t.Helper()
	w.execSQL(t, `UPDATE observations SET pinned = 1 WHERE id = 12`)
}

// doctorOf runs `doctor` in the world, in the text form or as JSON, and returns the exit code and the output.
func doctorOf(t *testing.T, w *retrievalWorld, extra ...string) (code int, stdout, stderr string) {
	t.Helper()
	return w.run(t, append([]string{"doctor", "--project", queryGoldenProject}, extra...))
}

// frozenLine is the line the text form prints for the frozen page c-000901, promoted from observation 12.
const frozenLine = "  [warn] promoted-pages-eligible (c-000901: memory 12 is not pinned and has no topic_key, so sync does not refresh this page; run `longterm-mem promote --source memorycard --id 12` to refresh it now, or pin memory 12 in memorycard so sync keeps it current)\n"

// A frozen page is a warning in the text form, naming the page, the observation, why it is not eligible and the
// two ways out; the command exits as it does for the same vault whose observation is pinned, which is 0.
func TestCmdDoctor_AFrozenPageIsAWarningAndTheExitCodeIsTheOneWithoutIt(t *testing.T) {
	t.Run("the observation is not eligible", func(t *testing.T) {
		w := eligibilityWorld(t, nil, 12)

		code, stdout, stderr := doctorOf(t, w)

		if !strings.Contains(stdout, frozenLine) {
			t.Errorf("stdout lacks the warning:\n%s\nwant the line\n%s", stdout, frozenLine)
		}
		if code != exitOK {
			t.Errorf("doctor exited %d with only a warning to report, want %d", code, exitOK)
		}
		if stderr != "" {
			t.Errorf("stderr = %q, want nothing", stderr)
		}
	})
	t.Run("the same vault with the observation pinned", func(t *testing.T) {
		w := eligibilityWorld(t, pinObservation12, 12)

		code, stdout, _ := doctorOf(t, w)

		if !strings.Contains(stdout, "  [PASS] promoted-pages-eligible\n") {
			t.Errorf("stdout lacks the passing check:\n%s", stdout)
		}
		if strings.Contains(stdout, "[warn]") {
			t.Errorf("stdout carries a warning for a pinned observation:\n%s", stdout)
		}
		if code != exitOK {
			t.Errorf("doctor exited %d, want %d", code, exitOK)
		}
	})
}

// The JSON carries the word warn as the status of the check, with the detail the text form prints, and the exit
// code is the same.
func TestCmdDoctor_JSONCarriesWarnForAFrozenPage(t *testing.T) {
	w := eligibilityWorld(t, nil, 12)

	code, stdout, _ := doctorOf(t, w, "--json")

	var report ops.DoctorReport
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("decode the JSON: %v\n%s", err, stdout)
	}
	var found *ops.Check
	for i := range report.Checks {
		if report.Checks[i].Name == "promoted-pages-eligible" {
			found = &report.Checks[i]
		}
	}
	if found == nil {
		t.Fatalf("the report has no promoted-pages-eligible check:\n%s", stdout)
	}
	if found.Status != "warn" {
		t.Errorf("status = %q, want %q", found.Status, "warn")
	}
	if want := strings.TrimSuffix(strings.TrimPrefix(frozenLine, "  [warn] promoted-pages-eligible ("), ")\n"); found.Detail != want {
		t.Errorf("detail = %q, want %q", found.Detail, want)
	}
	if !strings.Contains(stdout, `"status": "warn"`) {
		t.Errorf("the JSON does not spell the status as warn:\n%s", stdout)
	}
	if len(report.Checks) != 9 || report.Checks[8].Name != "promoted-pages-eligible" {
		t.Errorf("the checks are %+v, want nine, the eligibility check the last", report.Checks)
	}
	if code != exitOK {
		t.Errorf("doctor --json exited %d with only a warning to report, want %d", code, exitOK)
	}
}

// A warning does not stand in for a failure or hide one: with a page the log does not name, the command exits 9
// as it does for that vault without the frozen page, and reports both.
func TestCmdDoctor_AWarningBesideAFailureStillExitsNine(t *testing.T) {
	removeLog := func(t *testing.T, w *retrievalWorld) {
		t.Helper()
		if err := os.Remove(filepath.Join(w.vaultRoot, "wiki", "log.md")); err != nil {
			t.Fatalf("remove the log: %v", err)
		}
	}
	// A world changes the working directory of its test, so each of the two is built in a subtest of its own.
	var withoutTheWarning, withTheWarning int
	var stdout string
	t.Run("the same failure with every page kept current", func(t *testing.T) {
		w := eligibilityWorld(t, func(t *testing.T, w *retrievalWorld) {
			pinObservation12(t, w)
			removeLog(t, w)
		}, 12)
		withoutTheWarning, _, _ = doctorOf(t, w)
	})
	t.Run("the failure beside a frozen page", func(t *testing.T) {
		w := eligibilityWorld(t, removeLog, 12)
		withTheWarning, stdout, _ = doctorOf(t, w)
	})

	if withoutTheWarning != exitDoctorChecksFailed {
		t.Fatalf("the vault without the frozen page exited %d, want %d: the fixture's failure is gone", withoutTheWarning, exitDoctorChecksFailed)
	}
	if withTheWarning != withoutTheWarning {
		t.Errorf("doctor exited %d with a warning beside the failure, want %d, the code without the warning", withTheWarning, withoutTheWarning)
	}
	for _, want := range []string{"  [FAIL] wiki-registration-consistency", frozenLine} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, stdout)
		}
	}
}

// Pages kept current pass, as the pinned page and the pages of curated topics do, and a page the check has no
// observation to judge it by is not its concern: the observation was deleted, or it is a row of another project.
func TestCmdDoctor_PagesKeptCurrentAndPagesWithoutALiveObservationPass(t *testing.T) {
	for _, tc := range []struct {
		name     string
		edit     func(t *testing.T, w *retrievalWorld)
		promoted []int64
	}{
		{name: "a pinned observation and curated topics", promoted: []int64{1, 2, 3}},
		{name: "an observation soft-deleted since", promoted: []int64{4}},
		{
			name:     "an observation of another project, however little it is curated",
			promoted: []int64{5},
			edit: func(t *testing.T, w *retrievalWorld) {
				t.Helper()
				w.execSQL(t, `UPDATE observations SET topic_key = NULL WHERE id = 5`)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := eligibilityWorld(t, tc.edit, tc.promoted...)

			code, stdout, _ := doctorOf(t, w)

			if !strings.Contains(stdout, "  [PASS] promoted-pages-eligible\n") {
				t.Errorf("stdout lacks the passing check:\n%s", stdout)
			}
			if code != exitOK {
				t.Errorf("doctor exited %d, want %d:\n%s", code, exitOK, stdout)
			}
		})
	}
}

// A memorycard database the check cannot read is said as a warning that it cannot judge, not as a failure of its own:
// nothing else reads memorycard, so the command exits as it would without the check.
func TestCmdDoctor_AMemorycardDatabaseThatCannotBeReadIsAWarningOfThisCheck(t *testing.T) {
	w := eligibilityWorld(t, nil, 12)
	t.Setenv(memorycardDBEnvVar, filepath.Join(w.home, "absent", "memory.db"))

	code, stdout, _ := doctorOf(t, w)

	var line string
	for _, l := range strings.Split(stdout, "\n") {
		if strings.Contains(l, "promoted-pages-eligible") {
			line = l
		}
	}
	if !strings.HasPrefix(line, "  [warn] promoted-pages-eligible (eligibility cannot be judged: could not read the live memories of goldenproj: ") {
		t.Errorf("the eligibility line is %q, want a warning that it cannot judge", line)
	}
	if code != exitOK {
		t.Errorf("doctor exited %d with only a warning to report, want %d:\n%s", code, exitOK, stdout)
	}
}

// The check reads no Engram database (S11): with Engram out of reach the frozen page is still named, and the command
// exits 9 only for the freshness check that does read it. (Previously: the check read the observations of Engram, and
// said it could not judge.)
func TestCmdDoctor_TheEligibilityCheckReadsNoEngramDatabase(t *testing.T) {
	w := eligibilityWorld(t, nil, 12)
	t.Setenv(engramDBEnvVar, filepath.Join(w.home, "absent", "engram.db"))

	code, stdout, _ := doctorOf(t, w)

	if !strings.Contains(stdout, frozenLine) {
		t.Errorf("stdout lacks the warning:\n%s\nwant the line\n%s", stdout, frozenLine)
	}
	if !strings.Contains(stdout, "  [FAIL] embedding-index-fresh") || code != exitDoctorChecksFailed {
		t.Errorf("doctor exited %d, want %d for the freshness check that reads the Engram database:\n%s", code, exitDoctorChecksFailed, stdout)
	}
}

// The exit code of doctor is decided by FAIL alone: warn, like PASS, is not a verdict about the vault.
func TestDoctorExitCode_CountsFailedChecksAndNothingElse(t *testing.T) {
	check := func(status string) ops.Check { return ops.Check{Name: "c", Status: status} }
	for _, tc := range []struct {
		name   string
		checks []ops.Check
		want   int
	}{
		{"no check", nil, exitOK},
		{"all passing", []ops.Check{check(ops.CheckPassed), check(ops.CheckPassed)}, exitOK},
		{"a warning", []ops.Check{check(ops.CheckPassed), check(ops.CheckWarned)}, exitOK},
		{"only warnings", []ops.Check{check(ops.CheckWarned), check(ops.CheckWarned)}, exitOK},
		{"a failure", []ops.Check{check(ops.CheckPassed), check(ops.CheckFailed)}, exitDoctorChecksFailed},
		{"a warning and a failure", []ops.Check{check(ops.CheckWarned), check(ops.CheckFailed)}, exitDoctorChecksFailed},
		{"a failure and then a warning", []ops.Check{check(ops.CheckFailed), check(ops.CheckWarned)}, exitDoctorChecksFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := doctorExitCode(ops.DoctorReport{Checks: tc.checks}); got != tc.want {
				t.Errorf("doctorExitCode = %d, want %d", got, tc.want)
			}
		})
	}
}

// The help of doctor says what the JSON's statuses are, now that there are three.
func TestCmdDoctor_HelpNamesTheThreeStatuses(t *testing.T) {
	_, stderr := captureStreams(t, func() { run([]string{"doctor", "-h"}) })

	for _, want := range []string{"-json", "PASS", "FAIL", "warn"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("doctor -h does not mention %q:\n%s", want, stderr)
		}
	}
}

// The Engram read of the freshness check is the live rows of the project: the soft-deleted row and the row of
// another project are not among them. liveObservationIDs is held to what it always answered.
func TestTheEngramReadOfTheDoctorIsTheLiveRowsOfTheProject(t *testing.T) {
	t.Setenv(engramDBEnvVar, queryGoldenDatabase(t))

	ids, err := liveObservationIDs(queryGoldenProject)
	if err != nil {
		t.Fatalf("liveObservationIDs: %v", err)
	}

	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	if want := []int64{1, 2, 3, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}; fmt.Sprint(ids) != fmt.Sprint(want) {
		t.Errorf("liveObservationIDs = %v, want %v", ids, want)
	}
	if _, err := liveObservationIDs(queryGoldenProject + "-absent"); err != nil {
		t.Errorf("liveObservationIDs of a project with no rows: %v", err)
	}
}

// The eligibility check reads memorycard (S11): the live memories of the project, each with the fields promotion's
// rule reads and the Engram observation it was imported from, which is how a page of an earlier release is joined.
// A memory of another project, and a memory that was archived, are not among them. (Previously: the live rows of
// the Engram database.)
func TestTheMemorycardReadOfTheDoctorIsTheLiveMemoriesOfTheProject(t *testing.T) {
	path := memorycardDatabase(t,
		`insert into memories(id, project, title, body, topic_key, pinned, status) values
			(1, 'golden-project', 'Card Only', 'Body.', 'bugfix/register-order', 1, 'active'),
			(2, 'golden-project', 'Imported', 'Body.', NULL, 0, 'active'),
			(3, 'golden-project', 'Replaced', 'Body.', 'sdd/x/tasks', 0, 'superseded'),
			(4, 'golden-project', 'Archived', 'Body.', NULL, 0, 'archived'),
			(5, 'another-project', 'Elsewhere', 'Body.', NULL, 0, 'active')`,
		`insert into import_map values ('engram', '41', 1, 2)`)
	t.Setenv(memorycardDBEnvVar, path)

	rows, err := liveMemoryEligibility(promoteGoldenProject)
	if err != nil {
		t.Fatalf("liveMemoryEligibility: %v", err)
	}

	want := []ops.MemoryEligibility{
		{ID: 1, Pinned: true, TopicKey: "bugfix/register-order"},
		{ID: 2, EngramID: 41},
		{ID: 3, TopicKey: "sdd/x/tasks"},
	}
	if fmt.Sprint(rows) != fmt.Sprint(want) {
		t.Errorf("liveMemoryEligibility = %+v, want %+v", rows, want)
	}
}

// A database that is not there is the error of the read, in the words of the adapter.
func TestTheMemorycardReadOfTheDoctorFailsWithTheDatabaseThatIsNotThere(t *testing.T) {
	t.Setenv(memorycardDBEnvVar, filepath.Join(t.TempDir(), "absent", "memory.db"))

	if _, err := liveMemoryEligibility(promoteGoldenProject); err == nil {
		t.Error("liveMemoryEligibility answered no error for a database that is not there")
	}
}

// The advice of promoted-pages-eligible names a command that works (S11, the verifier's B1): for a page the
// memorycard memory 1 was promoted to, `doctor` says to run `promote --source memorycard --id 1`, and running it is
// accepted (exit 0, the page is left as it is), where `promote --id 1` alone is refused for want of a source.
func TestCmdDoctor_TheAdviceOfAFrozenMemorycardPageIsACommandThatWorks(t *testing.T) {
	memorycardWorld(t)
	if code, stdout, stderr := runPromoteGolden(t, []string{"promote", "--project", promoteGoldenProject, "--source", "memorycard", "--id", "1"}); code != exitOK {
		t.Fatalf("promote: exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	_, stdout, _ := runPromoteGolden(t, []string{"doctor", "--project", promoteGoldenProject, "--json"})

	var report ops.DoctorReport
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("decode the JSON: %v\n%s", err, stdout)
	}
	var detail string
	for _, check := range report.Checks {
		if check.Name == "promoted-pages-eligible" {
			detail = check.Detail
		}
	}
	const advised = "longterm-mem promote --source memorycard --id 1"
	if !strings.Contains(detail, "c-000901: memory 1 is not pinned and has no topic_key") || !strings.Contains(detail, "run `"+advised+"` to refresh it now") {
		t.Fatalf("promoted-pages-eligible detail = %q, want the memory and a promote that names its source", detail)
	}

	code, stdout, stderr := runPromoteGolden(t, strings.Fields(strings.TrimPrefix(advised, "longterm-mem ")+" --project "+promoteGoldenProject))
	if code != exitOK || !strings.Contains(stdout, "already promoted as c-000901") {
		t.Errorf("the advised command: exit %d, stdout %q, stderr %q, want exit 0 and the page left as it is", code, stdout, stderr)
	}
	if code, _, stderr := runPromoteGolden(t, []string{"promote", "--project", promoteGoldenProject, "--id", "1"}); code != exitUsage || !strings.Contains(stderr, "--source is required") {
		t.Errorf("the old advice: exit %d, stderr %q, want usage for want of --source", code, stderr)
	}
}
