package reviewresult_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewresult"
)

// The candidate every test reviews: two full commit names, the digest of the diff between them and the
// two files that diff changes. None of them is computed here; the caller of Parse computes them.
const (
	baseSHA = "1111111111111111111111111111111111111111"
	headSHA = "2222222222222222222222222222222222222222"
	diffSHA = "3333333333333333333333333333333333333333333333333333333333333333"
)

// binding is the candidate as the parent computed it before it launched the reviewer.
func binding() reviewresult.Binding {
	return reviewresult.Binding{
		Base:       baseSHA,
		Head:       headSHA,
		DiffSHA256: diffSHA,
		Paths:      []string{"engine/guard.go", "engine/guard_test.go"},
	}
}

// The four members of a reply that inspected the whole candidate and reports a BLOCKER with its proof and
// a SUGGESTION with none. Each is the JSON text of the member, so a test can swap one and keep the rest.
const (
	candidateJSON  = `{"base":"` + baseSHA + `","head":"` + headSHA + `","diff_sha256":"` + diffSHA + `"}`
	inspectionJSON = `{"status":"completed","paths":["engine/guard.go","engine/guard_test.go"],"reason":""}`
	blockerJSON    = `{"id":"F1","location":"engine/guard.go:10-14","severity":"BLOCKER","claim":"the guard lets a symlink out of the worktree","evidence_class":"deterministic","causal_disposition":"introduced","proof_refs":["engine/guard.go:12 is a line the candidate added"]}`
	suggestionJSON = `{"id":"F2","location":"engine/guard_test.go:7","severity":"SUGGESTION","claim":"name the case","evidence_class":"inferential","causal_disposition":"unknown","proof_refs":[]}`
	findingsJSON   = `[` + blockerJSON + `,` + suggestionJSON + `]`
	evidenceJSON   = `["read engine/guard.go","read engine/guard_test.go"]`
)

// validReply is the reply of that reviewer: one JSON object and no prose, the shape of agents/reviewer.md.
const validReply = `{"candidate":` + candidateJSON + `,"inspection":` + inspectionJSON + `,"findings":` + findingsJSON + `,"evidence":` + evidenceJSON + `}`

// reply is a reply made of the given members, each the JSON text of its value.
func reply(candidate, inspection, findings, evidence string) []byte {
	return []byte(`{"candidate":` + candidate + `,"inspection":` + inspection + `,"findings":` + findings + `,"evidence":` + evidence + `}`)
}

// replyFor is a reply that echoes the candidate b names, whatever it holds, and lists the paths of b as the
// paths it inspected: a reply that b cannot refuse for a difference, only for what b is.
func replyFor(t *testing.T, b reviewresult.Binding) []byte {
	t.Helper()
	quoted := func(value any) string {
		text, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		return string(text)
	}
	paths := b.Paths
	if paths == nil {
		paths = []string{}
	}
	return reply(
		`{"base":`+quoted(b.Base)+`,"head":`+quoted(b.Head)+`,"diff_sha256":`+quoted(b.DiffSHA256)+`}`,
		`{"status":"completed","paths":`+quoted(paths)+`,"reason":""}`,
		`[]`,
		`[]`,
	)
}

// edit returns validReply with the one place that holds old rewritten as replacement. It fails the test
// when old is not in validReply exactly once, so a test can never refuse a reply that was not changed.
func edit(t *testing.T, old, replacement string) []byte {
	t.Helper()
	if n := strings.Count(validReply, old); n != 1 {
		t.Fatalf("setup: %q occurs %d times in the valid reply, want exactly 1", old, n)
	}
	return []byte(strings.Replace(validReply, old, replacement, 1))
}

// refusals are the reasons Parse gives for turning a reply away. A refusal is exactly one of them.
var refusals = []error{
	reviewresult.ErrUnparseable,
	reviewresult.ErrUnknownMember,
	reviewresult.ErrInvalidBinding,
	reviewresult.ErrCandidateMismatch,
	reviewresult.ErrInspectionNotCompleted,
	reviewresult.ErrPathsMismatch,
	reviewresult.ErrOutsideEnum,
	reviewresult.ErrMissingID,
	reviewresult.ErrDuplicateID,
	reviewresult.ErrMissingProof,
}

// assertRefused fails the test unless Parse turned the reply away for the one reason want: the error is
// want and no other reason, and no part of the reply comes back as a result.
func assertRefused(t *testing.T, got reviewresult.Result, err error, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Errorf("Parse() error = %v, want one that is %v", err, want)
	}
	for _, other := range refusals {
		if other != want && errors.Is(err, other) {
			t.Errorf("Parse() error = %v is also %v, want only %v", err, other, want)
		}
	}
	if !reflect.DeepEqual(got, reviewresult.Result{}) {
		t.Errorf("Parse() returned %+v along with a refusal, want the zero Result", got)
	}
}
