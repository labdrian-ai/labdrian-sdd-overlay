package main

import (
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
		page, err := promote.EmitPage(obs, address, nil, fixtureInstant)
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
	w.buildIndexOfLiveRows(t, nil, nil)
	return w
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
const frozenLine = "  [warn] promoted-pages-eligible (c-000901: observation 12 is not pinned and has no topic_key, so sync does not refresh this page; run `longterm-mem promote --id 12` to refresh it now, or pin observation 12 in Engram so sync keeps it current)\n"

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

// An Engram database the check cannot read is said as a warning that it cannot judge, not as a failure of its
// own: the command exits 9 here, but for the check that reads the same database and fails, embedding-index-fresh.
func TestCmdDoctor_AnEngramDatabaseThatCannotBeReadIsAWarningOfThisCheck(t *testing.T) {
	w := eligibilityWorld(t, nil, 12)
	t.Setenv(engramDBEnvVar, filepath.Join(w.home, "absent", "engram.db"))

	code, stdout, _ := doctorOf(t, w)

	var line string
	for _, l := range strings.Split(stdout, "\n") {
		if strings.Contains(l, "promoted-pages-eligible") {
			line = l
		}
	}
	if !strings.HasPrefix(line, "  [warn] promoted-pages-eligible (eligibility cannot be judged: could not read the live observations of goldenproj: ") {
		t.Errorf("the eligibility line is %q, want a warning that it cannot judge", line)
	}
	if !strings.Contains(stdout, "  [FAIL] embedding-index-fresh") || code != exitDoctorChecksFailed {
		t.Errorf("doctor exited %d, want %d for the freshness check that reads the same database:\n%s", code, exitDoctorChecksFailed, stdout)
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

// The Engram reads the check is wired to are the live rows of the project, as embedding-index-fresh's are: the
// soft-deleted row and the row of another project are not among them, and the eligibility fields travel with the
// id. liveObservationIDs is held to what it always answered, now that both read through one listing.
func TestEngramReadsOfTheDoctorAreTheLiveRowsOfTheProject(t *testing.T) {
	t.Setenv(engramDBEnvVar, queryGoldenDatabase(t))

	ids, err := liveObservationIDs(queryGoldenProject)
	if err != nil {
		t.Fatalf("liveObservationIDs: %v", err)
	}
	rows, err := liveObservationEligibility(queryGoldenProject)
	if err != nil {
		t.Fatalf("liveObservationEligibility: %v", err)
	}

	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	if want := []int64{1, 2, 3, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}; fmt.Sprint(ids) != fmt.Sprint(want) {
		t.Errorf("liveObservationIDs = %v, want %v", ids, want)
	}
	byID := map[int64]ops.ObservationEligibility{}
	for _, row := range rows {
		byID[row.ID] = row
	}
	if len(rows) != len(ids) || len(byID) != len(ids) {
		t.Errorf("liveObservationEligibility returned %d rows (%d distinct ids), want the %d live rows once each", len(rows), len(byID), len(ids))
	}
	for id, want := range map[int64]ops.ObservationEligibility{
		1:  {ID: 1, Pinned: true, TopicKey: "architecture/auth-model"},
		2:  {ID: 2, TopicKey: "bugfix/register-order"},
		12: {ID: 12},
	} {
		if got := byID[id]; got != want {
			t.Errorf("liveObservationEligibility for observation %d = %+v, want %+v", id, got, want)
		}
	}
	for _, gone := range []int64{4, 5} {
		if _, there := byID[gone]; there {
			t.Errorf("liveObservationEligibility returned observation %d, which is soft-deleted or of another project", gone)
		}
	}
}

// A database that is not there is the error of both reads, in the words of the adapter.
func TestEngramReadsOfTheDoctorFailWithTheDatabaseThatIsNotThere(t *testing.T) {
	t.Setenv(engramDBEnvVar, filepath.Join(t.TempDir(), "absent", "engram.db"))

	if _, err := liveObservationIDs(queryGoldenProject); err == nil {
		t.Error("liveObservationIDs answered no error for a database that is not there")
	}
	if _, err := liveObservationEligibility(queryGoldenProject); err == nil {
		t.Error("liveObservationEligibility answered no error for a database that is not there")
	}
}
