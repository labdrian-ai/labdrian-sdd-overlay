package review_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
)

// firedFindings is a sweep in which every trigger fired, and cleanFindings one in which none did.
func firedFindings() review.SweepFindings {
	return review.SweepFindings{
		T1: review.SweepT1{
			DeletedOrRenamed: []string{"engine/old.go"},
			RemovedNames:     []review.RemovedName{{Name: "Old", Hits: []string{"docs/a.md"}}},
		},
		T2: []review.SweepT2{{Tool: "staticcheck", Module: "engine", New: []string{"a.go: unused x"}}},
		T3: []string{"?? stray.txt"},
	}
}

func decodeMap(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("setup: the record is not a JSON object: %v\n%s", err, data)
	}
	return m
}

// The sweep record holds the binding, the outcome and the three lists as S6 (c) writes them, and a list that found
// nothing is an empty array, never null.
func TestEncodeSweepWritesTheRecordOfS6c(t *testing.T) {
	c := candidate(t)
	data, err := review.EncodeSweep(c, firedFindings())
	if err != nil {
		t.Fatalf("EncodeSweep() error = %v, want nil", err)
	}
	m := decodeMap(t, data)
	for key, want := range map[string]any{"kind": "sweep", "base": c.Base, "head": c.Head, "diff_sha256": c.DiffSHA256, "outcome": "fired"} {
		if m[key] != want {
			t.Errorf("%s = %v, want %v", key, m[key], want)
		}
	}
	if len(m) != 8 {
		t.Errorf("the record has %d members, want 8 (kind, base, head, diff_sha256, outcome, t1, t2, t3): %v", len(m), m)
	}
	t1 := m["t1"].(map[string]any)
	if got := t1["removed_names"].([]any)[0].(map[string]any); got["name"] != "Old" || got["hits"].([]any)[0] != "docs/a.md" {
		t.Errorf("t1.removed_names[0] = %v, want name Old with hit docs/a.md", got)
	}
	if got := m["t2"].([]any)[0].(map[string]any); got["tool"] != "staticcheck" || got["module"] != "engine" || got["new"].([]any)[0] != "a.go: unused x" {
		t.Errorf("t2[0] = %v, want the tool, the module and the new line", got)
	}
	if got := m["t3"].([]any); len(got) != 1 || got[0] != "?? stray.txt" {
		t.Errorf("t3 = %v, want the porcelain line", got)
	}
	if !strings.HasSuffix(string(data), "\n") {
		t.Errorf("the record does not end in a newline")
	}
}

func TestEncodeSweepIsCleanExactlyWhenEveryListIsEmpty(t *testing.T) {
	c := candidate(t)
	data, err := review.EncodeSweep(c, review.SweepFindings{})
	if err != nil {
		t.Fatalf("EncodeSweep(empty) error = %v, want nil", err)
	}
	m := decodeMap(t, data)
	if m["outcome"] != "clean" {
		t.Errorf("outcome = %v, want clean", m["outcome"])
	}
	if !strings.Contains(string(data), `"t2": []`) || !strings.Contains(string(data), `"t3": []`) || !strings.Contains(string(data), `"deleted_or_renamed": []`) || !strings.Contains(string(data), `"removed_names": []`) {
		t.Errorf("an empty list is not an empty array:\n%s", data)
	}
	for name, f := range map[string]review.SweepFindings{
		"T1 paths": {T1: review.SweepT1{DeletedOrRenamed: []string{"a"}}},
		"T1 names": {T1: review.SweepT1{RemovedNames: []review.RemovedName{{Name: "N", Hits: []string{"a"}}}}},
		"T2":       {T2: []review.SweepT2{{Tool: "deadcode", Module: ".", New: []string{"x"}}}},
		"T3":       {T3: []string{"?? a"}},
	} {
		data, err := review.EncodeSweep(c, f)
		if err != nil {
			t.Fatalf("%s: EncodeSweep() error = %v, want nil", name, err)
		}
		if got := decodeMap(t, data)["outcome"]; got != "fired" {
			t.Errorf("%s alone: outcome = %v, want fired", name, got)
		}
	}
}

func TestEncodeSweepRefusesFindingsThatNameNothing(t *testing.T) {
	c := candidate(t)
	for name, f := range map[string]review.SweepFindings{
		"an empty path":        {T1: review.SweepT1{DeletedOrRenamed: []string{""}}},
		"a name with no text":  {T1: review.SweepT1{RemovedNames: []review.RemovedName{{Name: "", Hits: []string{"a"}}}}},
		"a name with no hit":   {T1: review.SweepT1{RemovedNames: []review.RemovedName{{Name: "N"}}}},
		"a hit with no path":   {T1: review.SweepT1{RemovedNames: []review.RemovedName{{Name: "N", Hits: []string{""}}}}},
		"a tool with no name":  {T2: []review.SweepT2{{Module: ".", New: []string{"x"}}}},
		"a module with none":   {T2: []review.SweepT2{{Tool: "deadcode", New: []string{"x"}}}},
		"a check with no line": {T2: []review.SweepT2{{Tool: "deadcode", Module: "."}}},
		"an empty line":        {T2: []review.SweepT2{{Tool: "deadcode", Module: ".", New: []string{""}}}},
		"an empty T3 line":     {T3: []string{""}},
	} {
		if _, err := review.EncodeSweep(c, f); !errors.Is(err, review.ErrInvalidSweepFindings) {
			t.Errorf("%s: EncodeSweep() error = %v, want one that is %v", name, err, review.ErrInvalidSweepFindings)
		}
	}
}

func TestDecodeSweepReadsBackWhatEncodeSweepWrote(t *testing.T) {
	c := candidate(t)
	for name, f := range map[string]review.SweepFindings{"fired": firedFindings(), "clean": {}} {
		data, err := review.EncodeSweep(c, f)
		if err != nil {
			t.Fatalf("%s: setup: %v", name, err)
		}
		got, err := review.DecodeSweep(data)
		if err != nil {
			t.Fatalf("%s: DecodeSweep() error = %v, want nil", name, err)
		}
		want := review.RecordBinding{Base: c.Base, Head: c.Head, DiffSHA256: c.DiffSHA256}
		if got.Binding != want {
			t.Errorf("%s: binding = %+v, want %+v", name, got.Binding, want)
		}
		if got.Fired() != f.Fired() || (got.Outcome == "fired") != f.Fired() {
			t.Errorf("%s: fired = %v (outcome %q), want %v", name, got.Fired(), got.Outcome, f.Fired())
		}
		if f.Fired() && (len(got.Findings.T1.RemovedNames) != 1 || got.Findings.T2[0].Tool != "staticcheck" || got.Findings.T3[0] != "?? stray.txt") {
			t.Errorf("%s: findings = %+v, want the ones stored", name, got.Findings)
		}
	}
}

func TestDecodeSweepRefusesWhatEncodeSweepDoesNotWrite(t *testing.T) {
	c := candidate(t)
	fired, _ := review.EncodeSweep(c, firedFindings())
	clean, _ := review.EncodeSweep(c, review.SweepFindings{})
	result := `{"version":1,"binding":{"base":"b","head":"h","diff_sha256":"d","paths":[]},"rows":["default"],"reply":{}}`
	for name, data := range map[string]string{
		"not JSON":                     "nope",
		"a result":                     result,
		"another kind":                 strings.Replace(string(fired), `"sweep"`, `"sweeper"`, 1),
		"an outcome nobody wrote":      strings.Replace(string(fired), `"fired"`, `"maybe"`, 1),
		"clean with findings":          strings.Replace(string(fired), `"fired"`, `"clean"`, 1),
		"fired with none":              strings.Replace(string(clean), `"clean"`, `"fired"`, 1),
		"a member it does not have":    strings.Replace(string(clean), `"outcome"`, `"extra": 1, "outcome"`, 1),
		"a key written twice":          strings.Replace(string(clean), `"kind": "sweep"`, `"kind": "sweep", "kind": "sweep"`, 1),
		"no base":                      strings.Replace(string(clean), `"base": "`+c.Base+`"`, `"base": ""`, 1),
		"a finding that names nothing": strings.Replace(string(fired), `"tool": "staticcheck"`, `"tool": ""`, 1),
	} {
		if _, err := review.DecodeSweep([]byte(data)); !errors.Is(err, review.ErrInvalidStored) {
			t.Errorf("%s: DecodeSweep() error = %v, want one that is %v", name, err, review.ErrInvalidStored)
		}
	}
}

// Every reader holds a record to git again with the same comparison as a result (CheckBinding): base, head and the
// digest of the diff; the paths follow from them.
func TestCheckRecordBindingHoldsTheRecordToTheCandidateGitGivesNow(t *testing.T) {
	c := candidate(t)
	b := review.RecordBinding{Base: c.Base, Head: c.Head, DiffSHA256: c.DiffSHA256}
	if err := review.CheckRecordBinding(b, c); err != nil {
		t.Errorf("CheckRecordBinding(same) error = %v, want nil", err)
	}
	for member, mutate := range map[string]func(*review.RecordBinding){
		"base":        func(b *review.RecordBinding) { b.Base = strings.Repeat("3", 40) },
		"head":        func(b *review.RecordBinding) { b.Head = strings.Repeat("3", 40) },
		"diff_sha256": func(b *review.RecordBinding) { b.DiffSHA256 = strings.Repeat("0", 64) },
	} {
		other := b
		mutate(&other)
		err := review.CheckRecordBinding(other, c)
		if !errors.Is(err, review.ErrBindingDiffers) || !strings.Contains(err.Error(), member) {
			t.Errorf("CheckRecordBinding(%s differs) error = %v, want one that is %v and names %s", member, err, review.ErrBindingDiffers, member)
		}
	}
}

// The candidates of a fired sweep are the ones the sweeper must answer for, each once.
func TestCandidatesOfAFiredSweepAreOneForEachFinding(t *testing.T) {
	c := candidate(t)
	data, _ := review.EncodeSweep(c, firedFindings())
	s, err := review.DecodeSweep(data)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	want := []string{"T1 path: engine/old.go", "T1 name: Old", "T2 staticcheck engine: a.go: unused x", "T3: ?? stray.txt"}
	got := s.Candidates()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("Candidates() = %q, want %q", got, want)
	}
}

const sweeperCommit = "3333333333333333333333333333333333333333"

func sweeperReport(t *testing.T, commit string, dispositions ...string) []byte {
	t.Helper()
	cands := []string{"T1 path: engine/old.go", "T1 name: Old", "T2 staticcheck engine: a.go: unused x", "T3: ?? stray.txt"}
	var rows []string
	for i, d := range dispositions {
		rows = append(rows, `{"candidate":`+quote(cands[i])+`,"disposition":"`+d+`","evidence":"grep finds no use"}`)
	}
	return []byte(`{"commit":` + commit + `,"candidates":[` + strings.Join(rows, ",") + `]}`)
}

func quote(s string) string { b, _ := json.Marshal(s); return string(b) }

func firedSweep(t *testing.T) review.StoredSweep {
	t.Helper()
	data, _ := review.EncodeSweep(candidate(t), firedFindings())
	s, err := review.DecodeSweep(data)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	return s
}

func TestParseSweeperReportReadsTheReportOfS6c(t *testing.T) {
	r, err := review.ParseSweeperReport(sweeperReport(t, `"`+sweeperCommit+`"`, "removed", "corrected", "live", "finding"))
	if err != nil {
		t.Fatalf("ParseSweeperReport() error = %v, want nil", err)
	}
	if r.Commit == nil || *r.Commit != sweeperCommit || len(r.Candidates) != 4 || r.Candidates[1].Disposition != "corrected" || r.Candidates[0].Evidence == "" {
		t.Errorf("ParseSweeperReport() = %+v, want the commit and four candidates", r)
	}
	r, err = review.ParseSweeperReport(sweeperReport(t, `null`, "live", "live", "finding", "live"))
	if err != nil || r.Commit != nil {
		t.Errorf("ParseSweeperReport(null commit) = %+v, %v, want a nil commit and no error", r, err)
	}
}

func TestParseSweeperReportRefusesWhatIsNotTheReport(t *testing.T) {
	good := string(sweeperReport(t, `null`, "live"))
	for name, data := range map[string]string{
		"not JSON":                                 "x",
		"not an object":                            "[]",
		"a key written twice":                      strings.Replace(good, `"commit":null`, `"commit":null,"commit":null`, 1),
		"a member it does not have":                strings.Replace(good, `"commit":null`, `"commit":null,"x":1`, 1),
		"no commit member":                         `{"candidates":[]}`,
		"a commit that is a name":                  strings.Replace(good, `null`, `"abc"`, 1),
		"a commit that is a number":                strings.Replace(good, `null`, `7`, 1),
		"no candidates":                            `{"commit":null}`,
		"a disposition outside the enum":           strings.Replace(good, `"live"`, `"gone"`, 1),
		"an empty evidence":                        strings.Replace(good, `"grep finds no use"`, `""`, 1),
		"no evidence":                              strings.Replace(good, `,"evidence":"grep finds no use"`, ``, 1),
		"an empty candidate":                       strings.Replace(good, `"T1 path: engine/old.go"`, `""`, 1),
		"a member of a candidate it does not have": strings.Replace(good, `"disposition"`, `"x":1,"disposition"`, 1),
		"over the size of a reply":                 `{"commit":null,"candidates":[{"candidate":"a","disposition":"live","evidence":"` + strings.Repeat("e", review.MaxReplyBytes) + `"}]}`,
	} {
		_, err := review.ParseSweeperReport([]byte(data))
		want := review.ErrInvalidSweeperReport
		if strings.HasPrefix(name, "over") {
			want = review.ErrReplyTooLarge
		}
		if !errors.Is(err, want) {
			t.Errorf("%s: ParseSweeperReport() error = %v, want one that is %v", name, err, want)
		}
	}
}

// The report answers for every candidate of the fired sweep: none missing, none twice, none that sweep did not name.
func TestCheckSweeperReportHoldsTheReportToTheSweep(t *testing.T) {
	s := firedSweep(t)
	commit := `"` + sweeperCommit + `"`
	ok := func(data []byte) review.SweeperReport {
		r, err := review.ParseSweeperReport(data)
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		return r
	}
	if err := review.CheckSweeperReport(ok(sweeperReport(t, commit, "removed", "corrected", "live", "finding")), s); err != nil {
		t.Errorf("CheckSweeperReport(complete) error = %v, want nil", err)
	}
	if err := review.CheckSweeperReport(ok(sweeperReport(t, `null`, "live", "live", "finding", "live")), s); err != nil {
		t.Errorf("CheckSweeperReport(null commit, nothing removed) error = %v, want nil", err)
	}
	for name, tc := range map[string]struct {
		report review.SweeperReport
		want   string
	}{
		"a candidate missing":      {ok(sweeperReport(t, `null`, "live", "live", "live")), "T3: ?? stray.txt"},
		"a candidate repeated":     {withCandidates(ok(sweeperReport(t, `null`, "live", "live", "live", "live")), 3, "T1 name: Old"), "T1 name: Old"},
		"a candidate unknown":      {withCandidates(ok(sweeperReport(t, `null`, "live", "live", "live", "live")), 3, "T1 name: Other"), "T1 name: Other"},
		"removed with no commit":   {ok(sweeperReport(t, `null`, "removed", "live", "live", "live")), "removed"},
		"corrected with no commit": {ok(sweeperReport(t, `null`, "live", "corrected", "live", "live")), "corrected"},
	} {
		err := review.CheckSweeperReport(tc.report, s)
		if !errors.Is(err, review.ErrInvalidSweeperReport) || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: CheckSweeperReport() error = %v, want one that is %v and names %q", name, err, review.ErrInvalidSweeperReport, tc.want)
		}
	}
	clean, _ := review.EncodeSweep(candidate(t), review.SweepFindings{})
	cs, _ := review.DecodeSweep(clean)
	if err := review.CheckSweeperReport(review.SweeperReport{Candidates: []review.SweeperCandidate{}}, cs); !errors.Is(err, review.ErrInvalidSweeperReport) {
		t.Errorf("CheckSweeperReport(clean sweep) error = %v, want one that is %v: a clean sweep has no sweeper", err, review.ErrInvalidSweeperReport)
	}
}

func withCandidates(r review.SweeperReport, at int, name string) review.SweeperReport {
	r.Candidates[at].Candidate = name
	return r
}

// The commit of the sweeper is the child of the head the sweep bound, with the subject of a sweep.
func TestCheckSweepCommitWantsTheChildOfTheHeadWithTheSubjectOfASweep(t *testing.T) {
	if err := review.CheckSweepCommit(headSHA, sweeperCommit, headSHA, "chore(sweep): remove Old"); err != nil {
		t.Errorf("CheckSweepCommit(valid) error = %v, want nil", err)
	}
	for name, tc := range map[string]struct{ parent, subject string }{
		"another first parent": {baseSHA, "chore(sweep): x"},
		"another subject":      {headSHA, "feat: x"},
		"the type only":        {headSHA, "chore: sweep"},
		"no subject":           {headSHA, ""},
	} {
		if err := review.CheckSweepCommit(headSHA, sweeperCommit, tc.parent, tc.subject); !errors.Is(err, review.ErrInvalidSweeperReport) || !strings.Contains(err.Error(), sweeperCommit) {
			t.Errorf("%s: CheckSweepCommit() error = %v, want one that is %v and names the commit", name, err, review.ErrInvalidSweeperReport)
		}
	}
}

func TestSweeperRecordIsStoredWithItsBindingAndTheSweepItAnswers(t *testing.T) {
	c := candidate(t)
	report, err := review.ParseSweeperReport(sweeperReport(t, `"`+sweeperCommit+`"`, "removed", "corrected", "live", "finding"))
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	data, err := review.EncodeSweeper(c, "222222222222-1.sweep.json", report)
	if err != nil {
		t.Fatalf("EncodeSweeper() error = %v, want nil", err)
	}
	m := decodeMap(t, data)
	for key, want := range map[string]any{"kind": "sweeper", "base": c.Base, "head": c.Head, "diff_sha256": c.DiffSHA256, "sweep": "222222222222-1.sweep.json", "commit": sweeperCommit} {
		if m[key] != want {
			t.Errorf("%s = %v, want %v", key, m[key], want)
		}
	}
	got, err := review.DecodeSweeper(data)
	if err != nil {
		t.Fatalf("DecodeSweeper() error = %v, want nil", err)
	}
	if got.Binding != (review.RecordBinding{Base: c.Base, Head: c.Head, DiffSHA256: c.DiffSHA256}) || got.Sweep != "222222222222-1.sweep.json" ||
		got.Report.Commit == nil || *got.Report.Commit != sweeperCommit || len(got.Report.Candidates) != 4 {
		t.Errorf("DecodeSweeper() = %+v, want what was stored", got)
	}
	null, _ := review.ParseSweeperReport(sweeperReport(t, `null`, "live", "live", "finding", "live"))
	data, _ = review.EncodeSweeper(c, "222222222222-1.sweep.json", null)
	if !strings.Contains(string(data), `"commit": null`) {
		t.Errorf("a sweeper that changed nothing stores \"commit\": null:\n%s", data)
	}
	if _, err := review.EncodeSweeper(c, "222222222222-1.json", report); !errors.Is(err, review.ErrInvalidSweepName) {
		t.Errorf("EncodeSweeper(a result as the sweep) error = %v, want one that is %v", err, review.ErrInvalidSweepName)
	}
	for name, bad := range map[string]string{
		"another kind":              strings.Replace(string(data), `"sweeper"`, `"sweep"`, 1),
		"a sweep that is not one":   strings.Replace(string(data), `222222222222-1.sweep.json`, `x.json`, 1),
		"a member it does not have": strings.Replace(string(data), `"commit"`, `"x":1,"commit"`, 1),
		"a report that is not one":  strings.Replace(string(data), `"live"`, `"gone"`, 1),
	} {
		if _, err := review.DecodeSweeper([]byte(bad)); !errors.Is(err, review.ErrInvalidStored) {
			t.Errorf("%s: DecodeSweeper() error = %v, want one that is %v", name, err, review.ErrInvalidStored)
		}
	}
}

const polisherFindings = `{"findings":[{"severity":"suggestion","evidence":["engine/a.go:10 repeats engine/b.go:20"],"note":"x"}]}`

func TestParsePolisherFindingsAcceptsOneJudgesObject(t *testing.T) {
	for name, data := range map[string]string{
		"a finding":     polisherFindings,
		"no finding":    `{"findings":[]}`,
		"other members": `{"judge":"A","findings":[]}`,
		"two evidences": `{"findings":[{"evidence":["a","b"]}]}`,
	} {
		got, err := review.ParsePolisherFindings([]byte(data))
		if err != nil || string(got) != data {
			t.Errorf("%s: ParsePolisherFindings() = %q, %v, want the object verbatim and no error", name, got, err)
		}
	}
}

func TestParsePolisherFindingsRefusesWhatIsNotTheObject(t *testing.T) {
	for name, data := range map[string]string{
		"not JSON":                      "x",
		"an array":                      `[]`,
		"a string":                      `"x"`,
		"a key written twice":           `{"findings":[],"findings":[]}`,
		"no findings":                   `{}`,
		"findings null":                 `{"findings":null}`,
		"findings an object":            `{"findings":{}}`,
		"a finding a string":            `{"findings":["x"]}`,
		"a finding null":                `{"findings":[null]}`,
		"no evidence":                   `{"findings":[{"severity":"x"}]}`,
		"evidence null":                 `{"findings":[{"evidence":null}]}`,
		"evidence empty":                `{"findings":[{"evidence":[]}]}`,
		"evidence a string":             `{"findings":[{"evidence":"a"}]}`,
		"evidence with a number":        `{"findings":[{"evidence":[1]}]}`,
		"evidence with an empty string": `{"findings":[{"evidence":["a",""]}]}`,
		"the second finding bad":        `{"findings":[{"evidence":["a"]},{"evidence":[]}]}`,
	} {
		if _, err := review.ParsePolisherFindings([]byte(data)); !errors.Is(err, review.ErrInvalidPolisherFindings) {
			t.Errorf("%s: ParsePolisherFindings() error = %v, want one that is %v", name, err, review.ErrInvalidPolisherFindings)
		}
	}
	big := `{"findings":[{"evidence":["` + strings.Repeat("e", review.MaxReplyBytes) + `"]}]}`
	if _, err := review.ParsePolisherFindings([]byte(big)); !errors.Is(err, review.ErrReplyTooLarge) {
		t.Errorf("ParsePolisherFindings(over the size of a reply) error = %v, want one that is %v", err, review.ErrReplyTooLarge)
	}
}

func TestPolisherRecordIsTheFindingsVerbatimWithTheBinding(t *testing.T) {
	c := candidate(t)
	data, err := review.EncodePolisher(c, []byte(polisherFindings))
	if err != nil {
		t.Fatalf("EncodePolisher() error = %v, want nil", err)
	}
	m := decodeMap(t, data)
	for key, want := range map[string]any{"kind": "polisher", "base": c.Base, "head": c.Head, "diff_sha256": c.DiffSHA256} {
		if m[key] != want {
			t.Errorf("%s = %v, want %v", key, m[key], want)
		}
	}
	if len(m) != 5 {
		t.Errorf("the record has %d members, want 5 (kind, base, head, diff_sha256, findings): %v", len(m), m)
	}
	got, err := review.DecodePolisher(data)
	if err != nil {
		t.Fatalf("DecodePolisher() error = %v, want nil", err)
	}
	var back, want any
	_ = json.Unmarshal(got.Findings, &back)
	_ = json.Unmarshal([]byte(polisherFindings), &want)
	if got.Binding != (review.RecordBinding{Base: c.Base, Head: c.Head, DiffSHA256: c.DiffSHA256}) || !jsonEqual(back, want) {
		t.Errorf("DecodePolisher() = %+v, want the binding and the findings as given", got)
	}
	if _, err := review.EncodePolisher(c, []byte(`{"findings":[{"evidence":[]}]}`)); !errors.Is(err, review.ErrInvalidPolisherFindings) {
		t.Errorf("EncodePolisher(bad findings) error = %v, want one that is %v", err, review.ErrInvalidPolisherFindings)
	}
	for name, bad := range map[string]string{
		"another kind":              strings.Replace(string(data), `"polisher"`, `"sweep"`, 1),
		"findings that fail":        strings.Replace(string(data), `"evidence": [`, `"evidence": [""],"x":[`, 1),
		"a member it does not have": strings.Replace(string(data), `"kind"`, `"x":1,"kind"`, 1),
		"not JSON":                  "x",
	} {
		if _, err := review.DecodePolisher([]byte(bad)); !errors.Is(err, review.ErrInvalidStored) {
			t.Errorf("%s: DecodePolisher() error = %v, want one that is %v", name, err, review.ErrInvalidStored)
		}
	}
}

func jsonEqual(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}
