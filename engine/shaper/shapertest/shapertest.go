// Package shapertest holds the documents the shaper's tests share: the valid handoff and
// the matching Goal, written as the wire text the program reads, and the helpers that vary
// them. It exists so that the domain's own tests (engine/shaper) and the file-backed
// adapter's tests (engine/shaper/fsadapter) are exercised on one copy of what a valid
// document looks like. A change to what the domain accepts then fails in
// shapertest_test.go, in one place, instead of leaving one package's copy stale while both
// still compile.
//
// It is test support: only _test.go files import it, and it is not part of any binary. It
// imports no package of the shaper on purpose. The domain's tests are in package shaper,
// and a package that imports the shaper could not be imported by them (an import cycle), so
// everything here is text and generic helpers; a test that needs the typed value parses
// the text with shaper.Parse.
package shapertest

import (
	"encoding/json"
	"reflect"
	"testing"
)

// ValidHandoffJSON is a structurally valid handoff version 1 whose project and goal match
// GoalV2JSON("standalone-shaper-handoff", "goal-alpha").
const ValidHandoffJSON = `{"version":1,"project_id":"standalone-shaper-handoff","goal_id":"goal-alpha","architecture":"Layered CLI with a shared jsonstrict wire gate.","stages":["Extract jsonstrict.","Refactor goal.Parse onto jsonstrict."],"acceptance":["go test ./... passes."],"out_of_scope":["Runtime clearance UI."]}`

// DocumentWith returns ValidHandoffJSON with the top-level fields in changes replaced (or
// added), re-encoded.
func DocumentWith(t testing.TB, changes map[string]any) []byte {
	t.Helper()
	var fields map[string]any
	if err := json.Unmarshal([]byte(ValidHandoffJSON), &fields); err != nil {
		t.Fatalf("decode valid test document: %v", err)
	}
	for key, value := range changes {
		fields[key] = value
	}
	data, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("encode test document: %v", err)
	}
	return data
}

// GoalV2JSON is a Goal version 2 for the given project and goal, with no non-goals.
func GoalV2JSON(projectID, goalID string) string {
	return `{"version":2,"project_id":"` + projectID + `","goal_id":"` + goalID + `",` +
		`"objective":"Bind the handoff to real intent.","scope":"One project.",` +
		`"constraints":[],"non_goals":[],"acceptance_criteria":["Binding succeeds."],` +
		`"memory_scope":"Project-scoped.","runtime_scope":"Deferred.","delivery_boundary":"No delivery."}`
}

// AssertRejectedWithoutPartialState fails unless operation returned an error together with
// the zero value of its result, so no rejection leaks partial state. what names the input
// that should have been rejected.
func AssertRejectedWithoutPartialState[T any](t testing.TB, operation string, got T, err error, what string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s accepted %s", operation, what)
	}
	var zero T
	if !reflect.DeepEqual(got, zero) {
		t.Errorf("%s returned a partial %T alongside error %v: %#v", operation, got, err, got)
	}
}
