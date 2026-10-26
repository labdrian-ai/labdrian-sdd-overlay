package reviewresult_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewresult"
)

// The reply of a reviewer that did what the contract asks is returned as the value it states, every member
// of it: the candidate it echoes, the inspection, each finding and the evidence. Every refusal below is
// meaningful only because this reply is accepted.
func TestParseReturnsWhatTheReviewerReported(t *testing.T) {
	got, err := reviewresult.Parse([]byte(validReply), binding())
	if err != nil {
		t.Fatalf("Parse(valid reply) error = %v, want nil", err)
	}
	want := reviewresult.Result{
		Candidate: reviewresult.Candidate{Base: baseSHA, Head: headSHA, DiffSHA256: diffSHA},
		Inspection: reviewresult.Inspection{
			Status: reviewresult.InspectionCompleted,
			Paths:  []string{"engine/guard.go", "engine/guard_test.go"},
			Reason: "",
		},
		Findings: []reviewresult.Finding{
			{
				ID:            "F1",
				Location:      "engine/guard.go:10-14",
				Severity:      reviewresult.SeverityBlocker,
				Claim:         "the guard lets a symlink out of the worktree",
				EvidenceClass: reviewresult.ClassDeterministic,
				Disposition:   reviewresult.DispositionIntroduced,
				ProofRefs:     []string{"engine/guard.go:12 is a line the candidate added"},
			},
			{
				ID:            "F2",
				Location:      "engine/guard_test.go:7",
				Severity:      reviewresult.SeveritySuggestion,
				Claim:         "name the case",
				EvidenceClass: reviewresult.ClassInferential,
				Disposition:   reviewresult.DispositionUnknown,
				ProofRefs:     []string{},
			},
		},
		Evidence: []string{"read engine/guard.go", "read engine/guard_test.go"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Parse(valid reply) =\n%+v\nwant\n%+v", got, want)
	}
}

// A clean review is `"findings":[]`: a completed inspection of the whole candidate that found nothing is a
// result, and it comes back with no finding.
func TestParseAcceptsACleanReview(t *testing.T) {
	got, err := reviewresult.Parse(reply(candidateJSON, inspectionJSON, `[]`, evidenceJSON), binding())
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}
	if len(got.Findings) != 0 {
		t.Errorf("Parse() findings = %+v, want none", got.Findings)
	}
	if got.Inspection.Status != reviewresult.InspectionCompleted || !reflect.DeepEqual(got.Inspection.Paths, binding().Paths) {
		t.Errorf("Parse() inspection = %+v, want a completed one of the paths of the diff", got.Inspection)
	}
}

// A reply is one JSON object with no prose, and white space around it is not prose: a reply that ends with
// a newline, as a model's does and as a stored file does, is the same reply.
func TestParseAcceptsWhiteSpaceAroundTheObject(t *testing.T) {
	want, err := reviewresult.Parse([]byte(validReply), binding())
	if err != nil {
		t.Fatalf("setup: Parse(valid reply) error = %v", err)
	}
	got, err := reviewresult.Parse([]byte(" \n\t"+validReply+"\r\n\n"), binding())
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Parse() =\n%+v\nwant the result of the same reply without white space\n%+v", got, want)
	}
}

// A reply that breaks several rules is refused for the first of them, in the order the contract lists
// them: the binding, then the shape of the reply and its members, the echo of the candidate, the status of
// the inspection, its paths, and the findings one by one, each by its id, then its values, then its proof.
// The same reply is always refused for the same reason, which is the one the parent records.
func TestParseRefusesForTheFirstRuleABrokenReplyBreaks(t *testing.T) {
	var (
		unfinished     = `{"status":"incomplete","paths":["engine/guard.go"],"reason":"engine/guard_test.go could not be read"}`
		otherBase      = `{"base":"` + otherSHA + `","head":"` + headSHA + `","diff_sha256":"` + diffSHA + `"}`
		otherPaths     = strings.Replace(inspectionJSON, `"engine/guard_test.go"`, `"engine/other.go"`, 1)
		repeatedID     = `[` + blockerJSON + `,` + strings.Replace(suggestionJSON, `"id":"F2"`, `"id":"F1"`, 1) + `]`
		noProofAndEnum = `[` + strings.Replace(strings.Replace(blockerJSON, `"deterministic"`, `"maybe"`, 1), `["engine/guard.go:12 is a line the candidate added"]`, `[]`, 1) + `]`
		invalidBinding = binding()
	)
	invalidBinding.Head = "HEAD"
	// The reply of another candidate that also has a member the contract does not name.
	unknownAndOtherBase := []byte(`{"schema":"x",` + string(reply(otherBase, inspectionJSON, findingsJSON, evidenceJSON))[1:])
	for _, tt := range []struct {
		name  string
		bound reviewresult.Binding
		reply []byte
		want  error
	}{
		{"an invalid binding before a reply nobody can read", invalidBinding, []byte("not json"), reviewresult.ErrInvalidBinding},
		{"an unknown member before another candidate", binding(), unknownAndOtherBase, reviewresult.ErrUnknownMember},
		{"another candidate before an unfinished inspection", binding(), reply(otherBase, unfinished, findingsJSON, evidenceJSON), reviewresult.ErrCandidateMismatch},
		{"an unfinished inspection before its paths", binding(), reply(candidateJSON, unfinished, findingsJSON, evidenceJSON), reviewresult.ErrInspectionNotCompleted},
		{"the paths before the findings", binding(), reply(candidateJSON, otherPaths, repeatedID, evidenceJSON), reviewresult.ErrPathsMismatch},
		{"a repeated id before the values of that finding", binding(), edit(t, `"id":"F2","location":"engine/guard_test.go:7","severity":"SUGGESTION"`, `"id":"F1","location":"engine/guard_test.go:7","severity":"HIGH"`), reviewresult.ErrDuplicateID},
		{"the values of a finding before its proof", binding(), reply(candidateJSON, inspectionJSON, noProofAndEnum, evidenceJSON), reviewresult.ErrOutsideEnum},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := reviewresult.Parse(tt.reply, tt.bound)
			assertRefused(t, got, err, tt.want)
		})
	}
}

// A reply that is not the one JSON object of the contract is not a result. The reviewer must reply with
// that object and no prose, with all four members and every member of each, and a reply that cannot be read
// that way is never read as "no findings": a findings member that is missing is not a clean review, and a
// member written twice would make a BLOCKER read as the SUGGESTION that follows it. Each case is a way the
// reply fails to be that object, and every one is refused for the same reason.
func TestParseRefusesAReplyThatIsNotTheObjectOfTheContract(t *testing.T) {
	notUTF8 := string([]byte{0xff})
	without := func(members ...string) []byte {
		parts := map[string]string{"candidate": candidateJSON, "inspection": inspectionJSON, "findings": findingsJSON, "evidence": evidenceJSON}
		for _, member := range members {
			delete(parts, member)
		}
		var kept []string
		for _, member := range []string{"candidate", "inspection", "findings", "evidence"} {
			if value, ok := parts[member]; ok {
				kept = append(kept, `"`+member+`":`+value)
			}
		}
		return []byte("{" + strings.Join(kept, ",") + "}")
	}
	for _, tt := range []struct {
		name  string
		reply []byte
	}{
		{"an empty reply", []byte("")},
		{"prose before the object", []byte("Here is my review: " + validReply)},
		{"a markdown fence around the object", []byte("```json\n" + validReply + "\n```")},
		{"an array", []byte("[" + validReply + "]")},
		{"null", []byte("null")},
		{"prose after the object", []byte(validReply + "\nDone.")},
		{"two objects", []byte(validReply + validReply)},
		{"a member twice at the top", edit(t, `"evidence":[`, `"findings":[],"evidence":[`)},
		{"a severity twice, the last a SUGGESTION", edit(t, `"severity":"BLOCKER"`, `"severity":"BLOCKER","severity":"SUGGESTION"`)},
		{"bytes that are not UTF-8", edit(t, "the guard lets a symlink", "the guard lets a "+notUTF8+" symlink")},

		{"a candidate that is a string", reply(`"abc"`, inspectionJSON, findingsJSON, evidenceJSON)},
		{"an inspection that is an array", reply(candidateJSON, `[]`, findingsJSON, evidenceJSON)},
		{"findings that is a string", reply(candidateJSON, inspectionJSON, `"none"`, evidenceJSON)},
		{"a finding that is null", reply(candidateJSON, inspectionJSON, `[null]`, evidenceJSON)},
		{"evidence that is a string", reply(candidateJSON, inspectionJSON, findingsJSON, `"read it"`)},
		{"a status that is a number", edit(t, `"status":"completed"`, `"status":7`)},
		{"paths that is a string", edit(t, `"paths":["engine/guard.go","engine/guard_test.go"]`, `"paths":"engine/guard.go"`)},
		{"a severity that is a number", edit(t, `"severity":"SUGGESTION"`, `"severity":3`)},
		{"proof_refs that is a string", edit(t, `"proof_refs":[]`, `"proof_refs":"none"`)},

		{"no candidate", without("candidate")},
		{"no inspection", without("inspection")},
		{"no findings", without("findings")},
		{"no evidence", without("evidence")},
		{"no head in the candidate", edit(t, `"head":"`+headSHA+`",`, ``)},
		{"no reason in the inspection", edit(t, `,"reason":""`, ``)},
		{"no claim in a finding", edit(t, `"claim":"name the case",`, ``)},
		{"no proof_refs in a finding", edit(t, `,"proof_refs":[]`, ``)},

		{"findings that is null", reply(candidateJSON, inspectionJSON, `null`, evidenceJSON)},
		{"evidence that is null", reply(candidateJSON, inspectionJSON, findingsJSON, `null`)},
		{"a base that is null", edit(t, `"base":"`+baseSHA+`"`, `"base":null`)},
		{"a reason that is null", edit(t, `"reason":""`, `"reason":null`)},
		{"paths that is null", edit(t, `"paths":["engine/guard.go","engine/guard_test.go"]`, `"paths":null`)},
		{"proof_refs that is null", edit(t, `"proof_refs":[]`, `"proof_refs":null`)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := reviewresult.Parse(tt.reply, binding())
			assertRefused(t, got, err, reviewresult.ErrUnparseable)
		})
	}
}

// A binding that names no candidate holds a reply to nothing, so Parse refuses to compare with it: with an
// empty binding, a reply that echoes empty values would be "the same candidate". The reply of each case
// echoes exactly what its binding holds, so only what the binding is can refuse it.
func TestParseRefusesABindingThatNamesNoCandidate(t *testing.T) {
	with := func(change func(*reviewresult.Binding)) reviewresult.Binding {
		b := binding()
		change(&b)
		return b
	}
	for _, tt := range []struct {
		name   string
		member string
		bound  reviewresult.Binding
	}{
		{"an empty binding", "base", reviewresult.Binding{}},
		{"a base that is empty", "base", with(func(b *reviewresult.Binding) { b.Base = "" })},
		{"a base that is an abbreviation", "base", with(func(b *reviewresult.Binding) { b.Base = baseSHA[:12] })},
		{"a head with an uppercase letter", "head", with(func(b *reviewresult.Binding) { b.Head = "22222222222222222222222222222222222222AB" })},
		{"a head that is a ref", "head", with(func(b *reviewresult.Binding) { b.Head = "HEAD" })},
		{"a diff_sha256 that is empty", "diff_sha256", with(func(b *reviewresult.Binding) { b.DiffSHA256 = "" })},
		{"a diff_sha256 of 63 characters", "diff_sha256", with(func(b *reviewresult.Binding) { b.DiffSHA256 = diffSHA[:63] })},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := reviewresult.Parse(replyFor(t, tt.bound), tt.bound)
			assertRefused(t, got, err, reviewresult.ErrInvalidBinding)
			if err != nil && !strings.Contains(err.Error(), tt.member) {
				t.Errorf("Parse() error = %v, want it to name the member %q", err, tt.member)
			}
		})
	}
}

// A result of ours is check evidence in the role chain, never review authority (odd/review-results/README.md):
// it carries no schema, lineage, receipt or GENTLE_AI_* member, nor any member the contract does not name,
// in the reply or in the candidate, the inspection and the findings it holds. The names are exact: Go reads
// a key into a member whatever its case and keeps the last, so "Severity" after "severity" would make a
// BLOCKER read as the SUGGESTION that follows it, and it is refused as the member it is not.
func TestParseRefusesAMemberTheContractDoesNotName(t *testing.T) {
	for _, tt := range []struct {
		name   string
		member string
		reply  []byte
	}{
		{"a schema in the reply", "schema", edit(t, `{"candidate":`, `{"schema":"gentle-ai.review-result/v1","candidate":`)},
		{"a gentle member in the reply", "GENTLE_AI_REVIEW_BINDING", edit(t, `,"evidence":[`, `,"GENTLE_AI_REVIEW_BINDING":"x","evidence":[`)},
		{"a subject_hash in the candidate", "subject_hash", edit(t, `"diff_sha256":"`+diffSHA+`"`, `"diff_sha256":"`+diffSHA+`","subject_hash":"`+diffSHA+`"`)},
		{"a lineage in the inspection", "lineage", edit(t, `"reason":""}`, `"reason":"","lineage":"x"}`)},
		{"a rationale in a finding", "rationale", edit(t, `"claim":"name the case",`, `"claim":"name the case","rationale":"x",`)},
		{"a member named in another case instead of the member", "Findings", edit(t, `"findings":`, `"Findings":`)},
		{"a severity written again in another case", "Severity", edit(t, `"severity":"BLOCKER"`, `"severity":"BLOCKER","Severity":"SUGGESTION"`)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := reviewresult.Parse(tt.reply, binding())
			assertRefused(t, got, err, reviewresult.ErrUnknownMember)
			if err != nil && !strings.Contains(err.Error(), `"`+tt.member+`"`) {
				t.Errorf("Parse() error = %v, want it to name the member %q", err, tt.member)
			}
		})
	}
}
