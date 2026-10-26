package reviewresult_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewresult"
)

// Other values of the candidate, for the reply that echoes a candidate which is not the one it was sent.
const (
	otherSHA    = "4444444444444444444444444444444444444444"
	otherDigest = "5555555555555555555555555555555555555555555555555555555555555555"
)

// The status of the inspection is "incomplete" when any changed path could not be inspected, and only a
// "completed" inspection is a review: a result that says it did not read the whole candidate, or says
// something the contract has no word for, is never recorded as one. The error carries the reason the
// reviewer gave, which the parent records as the reason the review did not run, and it speaks before the
// paths do, since an incomplete inspection lists fewer of them.
func TestParseRefusesAnInspectionThatIsNotCompleted(t *testing.T) {
	const unread = `{"status":"incomplete","paths":["engine/guard.go"],"reason":"engine/guard_test.go could not be read"}`
	for _, tt := range []struct {
		name  string
		reply []byte
		names []string
	}{
		{"incomplete, with its reason and the paths it read", reply(candidateJSON, unread, findingsJSON, evidenceJSON), []string{`"incomplete"`, "engine/guard_test.go could not be read"}},
		{"incomplete, with no reason", edit(t, `"status":"completed"`, `"status":"incomplete"`), []string{`"incomplete"`}},
		{"a status in capitals", edit(t, `"status":"completed"`, `"status":"Completed"`), []string{`"Completed"`}},
		{"a status the contract has no word for", edit(t, `"status":"completed"`, `"status":"done"`), []string{`"done"`}},
		{"an empty status", edit(t, `"status":"completed"`, `"status":""`), []string{`""`}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := reviewresult.Parse(tt.reply, binding())
			assertRefused(t, got, err, reviewresult.ErrInspectionNotCompleted)
			if err == nil {
				return
			}
			for _, part := range tt.names {
				if !strings.Contains(err.Error(), part) {
					t.Errorf("Parse() error = %v, want it to name %s", err, part)
				}
			}
		})
	}
}

// A completed inspection lists every changed path it inspected, and the parent compares that list with the
// paths of the diff (git diff --name-only base head): a path of the diff the reviewer did not list is a file
// nobody read, and a path it lists that the diff does not change is a result about another candidate. The
// lists are compared as sets, so a reviewer need not sort its paths, and the error names every path on
// either side.
func TestParseRefusesPathsThatAreNotThePathsOfTheDiff(t *testing.T) {
	both := func(inspected string) []byte {
		return reply(candidateJSON, `{"status":"completed","paths":`+inspected+`,"reason":""}`, findingsJSON, evidenceJSON)
	}
	for _, tt := range []struct {
		name  string
		bound []string
		reply []byte
		names []string
	}{
		{"a changed path that was not inspected", binding().Paths, both(`["engine/guard.go"]`),
			[]string{`not inspected: "engine/guard_test.go"`}},
		{"a path that the diff does not change", binding().Paths, both(`["engine/guard.go","engine/guard_test.go","engine/other.go"]`),
			[]string{`not in the diff: "engine/other.go"`}},
		{"a path of the diff missing and another one added", binding().Paths, both(`["engine/guard.go","engine/other.go"]`),
			[]string{`not inspected: "engine/guard_test.go"`, `not in the diff: "engine/other.go"`}},
		{"no path inspected, the diff's paths in the order git did not give them", []string{"engine/guard_test.go", "engine/guard.go"}, both(`[]`),
			[]string{`not inspected: "engine/guard.go", "engine/guard_test.go"`}},
		{"paths for a diff that changes none", nil, both(`["engine/guard.go"]`),
			[]string{`not in the diff: "engine/guard.go"`}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			bound := binding()
			bound.Paths = tt.bound
			got, err := reviewresult.Parse(tt.reply, bound)
			assertRefused(t, got, err, reviewresult.ErrPathsMismatch)
			if err == nil {
				return
			}
			for _, part := range tt.names {
				if !strings.Contains(err.Error(), part) {
					t.Errorf("Parse() error = %v, want it to say %s", err, part)
				}
			}
		})
	}

	t.Run("the same paths in another order are the paths of the diff", func(t *testing.T) {
		got, err := reviewresult.Parse(both(`["engine/guard_test.go","engine/guard.go"]`), binding())
		if err != nil {
			t.Fatalf("Parse() error = %v, want nil", err)
		}
		if want := []string{"engine/guard_test.go", "engine/guard.go"}; !reflect.DeepEqual(got.Inspection.Paths, want) {
			t.Errorf("Parse() inspection paths = %v, want %v as the reviewer wrote them", got.Inspection.Paths, want)
		}
	})
}

// The severity, the class of evidence and the causal disposition of a finding are enumerations, and a value
// outside one is refused, never read as the nearest value: "blocker" is not BLOCKER, and "activated" is the
// word of the prose for "behavior-activated". The error names the finding, the member and the value, and
// lists the values the contract has.
func TestParseRefusesAValueOutsideItsEnumeration(t *testing.T) {
	for _, tt := range []struct {
		name        string
		finding     string
		member      string
		old, change string
		value       string
		allowed     string
	}{
		{"a severity in lower case", "F1", "severity", `"severity":"BLOCKER"`, `"severity":"blocker"`, "blocker", "BLOCKER, CRITICAL, WARNING, SUGGESTION"},
		{"a severity the contract has no word for", "F2", "severity", `"severity":"SUGGESTION"`, `"severity":"HIGH"`, "HIGH", "BLOCKER, CRITICAL, WARNING, SUGGESTION"},
		{"an empty severity", "F2", "severity", `"severity":"SUGGESTION"`, `"severity":""`, "", "BLOCKER, CRITICAL, WARNING, SUGGESTION"},
		{"a disposition as the prose shortens it", "F1", "causal_disposition", `"causal_disposition":"introduced"`, `"causal_disposition":"activated"`, "activated", "introduced, behavior-activated, worsened, pre-existing, base-only, unknown"},
		{"a disposition in capitals", "F2", "causal_disposition", `"causal_disposition":"unknown"`, `"causal_disposition":"Unknown"`, "Unknown", "introduced, behavior-activated, worsened, pre-existing, base-only, unknown"},
		{"an empty disposition", "F1", "causal_disposition", `"causal_disposition":"introduced"`, `"causal_disposition":""`, "", "introduced, behavior-activated, worsened, pre-existing, base-only, unknown"},
		{"a class of evidence the contract has no word for", "F2", "evidence_class", `"evidence_class":"inferential"`, `"evidence_class":"probable"`, "probable", "deterministic, inferential"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := reviewresult.Parse(edit(t, tt.old, tt.change), binding())
			assertRefused(t, got, err, reviewresult.ErrOutsideEnum)
			if err == nil {
				return
			}
			for _, part := range []string{`finding "` + tt.finding + `"`, tt.member + ` "` + tt.value + `"`, "want one of " + tt.allowed} {
				if !strings.Contains(err.Error(), part) {
					t.Errorf("Parse() error = %v, want it to say %s", err, part)
				}
			}
		})
	}
}

// Every value the reviewer's contract lists for the three enumerations is accepted, and comes back as the
// constant of the package that names it. The finding changed is the first of the valid reply, the BLOCKER
// that carries its proof, so a severe severity is accepted with the proof it needs.
func TestParseAcceptsEveryValueOfTheEnumerations(t *testing.T) {
	first := func(t *testing.T, member, old, value string) reviewresult.Finding {
		t.Helper()
		got, err := reviewresult.Parse(edit(t, old, `"`+member+`":"`+value+`"`), binding())
		if err != nil {
			t.Fatalf("Parse() error = %v, want nil", err)
		}
		return got.Findings[0]
	}
	for _, tt := range []struct {
		value string
		want  reviewresult.Severity
	}{
		{"BLOCKER", reviewresult.SeverityBlocker},
		{"CRITICAL", reviewresult.SeverityCritical},
		{"WARNING", reviewresult.SeverityWarning},
		{"SUGGESTION", reviewresult.SeveritySuggestion},
	} {
		t.Run("severity "+tt.value, func(t *testing.T) {
			if got := first(t, "severity", `"severity":"BLOCKER"`, tt.value).Severity; got != tt.want {
				t.Errorf("Parse() severity = %q, want %q", got, tt.want)
			}
		})
	}
	for _, tt := range []struct {
		value string
		want  reviewresult.EvidenceClass
	}{
		{"deterministic", reviewresult.ClassDeterministic},
		{"inferential", reviewresult.ClassInferential},
	} {
		t.Run("evidence_class "+tt.value, func(t *testing.T) {
			if got := first(t, "evidence_class", `"evidence_class":"deterministic"`, tt.value).EvidenceClass; got != tt.want {
				t.Errorf("Parse() evidence class = %q, want %q", got, tt.want)
			}
		})
	}
	for _, tt := range []struct {
		value string
		want  reviewresult.Disposition
	}{
		{"introduced", reviewresult.DispositionIntroduced},
		{"behavior-activated", reviewresult.DispositionBehaviorActivated},
		{"worsened", reviewresult.DispositionWorsened},
		{"pre-existing", reviewresult.DispositionPreExisting},
		{"base-only", reviewresult.DispositionBaseOnly},
		{"unknown", reviewresult.DispositionUnknown},
	} {
		t.Run("causal_disposition "+tt.value, func(t *testing.T) {
			if got := first(t, "causal_disposition", `"causal_disposition":"introduced"`, tt.value).Disposition; got != tt.want {
				t.Errorf("Parse() disposition = %q, want %q", got, tt.want)
			}
		})
	}
}

// Every finding carries an id, because the verifier answers per finding id and the parent follows a finding
// by it: a finding with no id, or one that is only blank, cannot be answered, so the result is refused. The
// error names the finding by its place in the findings, the one thing it has.
func TestParseRefusesAFindingWithoutAnID(t *testing.T) {
	for _, tt := range []struct {
		name    string
		finding string
		reply   []byte
	}{
		{"no id member", "findings[1]", edit(t, `{"id":"F2","location"`, `{"location"`)},
		{"an id that is null", "findings[1]", edit(t, `"id":"F2"`, `"id":null`)},
		{"an empty id", "findings[1]", edit(t, `"id":"F2"`, `"id":""`)},
		{"an id that is only white space", "findings[0]", edit(t, `"id":"F1"`, `"id":" \t"`)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := reviewresult.Parse(tt.reply, binding())
			assertRefused(t, got, err, reviewresult.ErrMissingID)
			if err != nil && !strings.Contains(err.Error(), tt.finding) {
				t.Errorf("Parse() error = %v, want it to name %s", err, tt.finding)
			}
		})
	}
}

// An id names one finding of the result, so the result that gives two findings the same id is refused: the
// verifier's answer for that id would belong to both. The error names the id and the two places it is in.
func TestParseRefusesARepeatedID(t *testing.T) {
	third := strings.Replace(suggestionJSON, `"id":"F2"`, `"id":"F3"`, 1)
	repeated := strings.Replace(blockerJSON, `"claim":"the guard lets a symlink out of the worktree"`, `"claim":"another defect"`, 1)
	for _, tt := range []struct {
		name   string
		reply  []byte
		id     string
		places []string
	}{
		{"the second finding repeats the first", edit(t, `"id":"F2"`, `"id":"F1"`), "F1", []string{"findings[0]", "findings[1]"}},
		{"the third finding repeats the first, with another claim", reply(candidateJSON, inspectionJSON, `[`+blockerJSON+`,`+third+`,`+repeated+`]`, evidenceJSON), "F1", []string{"findings[0]", "findings[2]"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := reviewresult.Parse(tt.reply, binding())
			assertRefused(t, got, err, reviewresult.ErrDuplicateID)
			if err == nil {
				return
			}
			for _, part := range append([]string{`"` + tt.id + `"`}, tt.places...) {
				if !strings.Contains(err.Error(), part) {
					t.Errorf("Parse() error = %v, want it to name %s", err, part)
				}
			}
		})
	}
}

// BLOCKER and CRITICAL are the severe severities, the two that can block a review; WARNING and SUGGESTION
// are not, and a value that is not a severity is not severe either.
func TestOnlyBlockerAndCriticalAreSevere(t *testing.T) {
	for _, tt := range []struct {
		severity reviewresult.Severity
		severe   bool
	}{
		{reviewresult.SeverityBlocker, true},
		{reviewresult.SeverityCritical, true},
		{reviewresult.SeverityWarning, false},
		{reviewresult.SeveritySuggestion, false},
		{reviewresult.Severity("HIGH"), false},
	} {
		if got := tt.severity.Severe(); got != tt.severe {
			t.Errorf("Severity(%q).Severe() = %v, want %v", tt.severity, got, tt.severe)
		}
	}
}

// A BLOCKER or a CRITICAL finding blocks the review, so it must say what proves it: the contract asks for
// changed-hunk, created-path, differential or before/after proof, and the proof_refs member lists it. A
// severe finding that lists no proof reference, or only blank ones, is refused, whatever else it says. A
// WARNING or a SUGGESTION needs none, and a blank reference beside a real one is not a missing proof.
func TestParseRefusesASevereFindingWithoutAProofReference(t *testing.T) {
	const real = `"engine/guard.go:12 is a line the candidate added"`
	proof := `"proof_refs":[` + real + `]`
	for _, tt := range []struct {
		name     string
		finding  string
		severity string
		reply    []byte
	}{
		{"a BLOCKER with an empty list", "F1", "BLOCKER", edit(t, proof, `"proof_refs":[]`)},
		{"a CRITICAL with an empty list", "F2", "CRITICAL", edit(t, `"severity":"SUGGESTION"`, `"severity":"CRITICAL"`)},
		{"a BLOCKER with a blank reference", "F1", "BLOCKER", edit(t, proof, `"proof_refs":[""]`)},
		{"a BLOCKER with white space only", "F1", "BLOCKER", edit(t, proof, `"proof_refs":["  ","\t"]`)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := reviewresult.Parse(tt.reply, binding())
			assertRefused(t, got, err, reviewresult.ErrMissingProof)
			if err == nil {
				return
			}
			for _, part := range []string{`finding "` + tt.finding + `"`, tt.severity} {
				if !strings.Contains(err.Error(), part) {
					t.Errorf("Parse() error = %v, want it to say %s", err, part)
				}
			}
		})
	}

	for _, tt := range []struct {
		name  string
		reply []byte
	}{
		{"a WARNING with no reference", edit(t, `"severity":"SUGGESTION"`, `"severity":"WARNING"`)},
		{"a BLOCKER with a blank reference beside a real one", edit(t, proof, `"proof_refs":["",`+real+`]`)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := reviewresult.Parse(tt.reply, binding()); err != nil {
				t.Errorf("Parse() error = %v, want nil", err)
			}
		})
	}
}

// The reviewer copies base, head and diff_sha256 from its prompt and never computes them, and the parent
// refuses a result whose echo differs from what it sent: a result applies to the bytes it was launched on
// and to no other, so a difference in any one of the three means it does not apply. The error names the
// member and both values, which is the reason the parent records for the review that did not run.
func TestParseRefusesAnEchoOfAnotherCandidate(t *testing.T) {
	for _, tt := range []struct {
		name   string
		member string
		echoed string
		bound  string
		reply  []byte
	}{
		{"another base", "base", otherSHA, baseSHA, edit(t, `"base":"`+baseSHA+`"`, `"base":"`+otherSHA+`"`)},
		{"another head", "head", otherSHA, headSHA, edit(t, `"head":"`+headSHA+`"`, `"head":"`+otherSHA+`"`)},
		{"another diff_sha256", "diff_sha256", otherDigest, diffSHA, edit(t, `"diff_sha256":"`+diffSHA+`"`, `"diff_sha256":"`+otherDigest+`"`)},
		{"a head that is an abbreviation of the head", "head", headSHA[:12], headSHA, edit(t, `"head":"`+headSHA+`"`, `"head":"`+headSHA[:12]+`"`)},
		{"an empty diff_sha256", "diff_sha256", "", diffSHA, edit(t, `"diff_sha256":"`+diffSHA+`"`, `"diff_sha256":""`)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := reviewresult.Parse(tt.reply, binding())
			assertRefused(t, got, err, reviewresult.ErrCandidateMismatch)
			if err == nil {
				return
			}
			for _, part := range []string{tt.member, `"` + tt.echoed + `"`, `"` + tt.bound + `"`} {
				if !strings.Contains(err.Error(), part) {
					t.Errorf("Parse() error = %v, want it to name %s", err, part)
				}
			}
		})
	}
}
