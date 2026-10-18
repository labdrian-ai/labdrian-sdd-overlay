package workflow

import (
	"errors"
	"strings"
	"testing"
)

const testIdentity = "github.com/acme/proj-1"

func memLifecycle(t *testing.T, store *memEventLog) Lifecycle {
	t.Helper()
	return newTestLifecycle(t, store, stepClock(), newFakeGoalReader(), newFakeChainReader(), UnavailableProber{})
}

// The workflow's slug stays the key of its directory, and the created event records the shared
// identity of the project beside it (C23, decision 2 of M2): the log carries it, and every State
// replayed from the log has it.
func TestCreateRecordsTheProjectIdentityInTheCreatedEvent(t *testing.T) {
	store := newMemEventLog()
	state, err := memLifecycle(t, store).Create("proj-1", "wf-1", testIdentity, validGoal("proj-1", "goal-1"), "odd", "")
	if err != nil {
		t.Fatalf("Create() = %v, want nil", err)
	}
	if state.ProjectIdentity != testIdentity {
		t.Errorf("Create() state.ProjectIdentity = %q, want %q", state.ProjectIdentity, testIdentity)
	}
	raw := string(store.logs["proj-1/wf-1"])
	if !strings.Contains(raw, `"project_id":"proj-1"`) || !strings.Contains(raw, `"project_identity":"github.com/acme/proj-1"`) {
		t.Errorf("the created event is %s, want the slug as project_id and the identity as project_identity", raw)
	}

	started, err := memLifecycle(t, store).Start("proj-1", "wf-1")
	if err != nil {
		t.Fatalf("Start() = %v, want nil", err)
	}
	if started.ProjectIdentity != testIdentity {
		t.Errorf("a State replayed from the log has ProjectIdentity %q, want %q", started.ProjectIdentity, testIdentity)
	}
	if lines := strings.Count(string(store.logs["proj-1/wf-1"]), `"project_identity"`); lines != 1 {
		t.Errorf("project_identity appears in %d events, want only the created one", lines)
	}
}

// workflow create refuses, appending nothing, when the Goal names another project than the
// workflow's slug (C23, decision 5 of M2).
func TestCreateRefusesAGoalOfAnotherProjectAndAppendsNothing(t *testing.T) {
	store := newMemEventLog()
	_, err := memLifecycle(t, store).Create("proj-1", "wf-1", testIdentity, validGoal("proj-2", "goal-1"), "odd", "")
	if !errors.Is(err, ErrGoalProjectMismatch) {
		t.Fatalf("Create() = %v, want ErrGoalProjectMismatch", err)
	}
	for _, want := range []string{`"proj-2"`, `"proj-1"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal %q does not name %s", err, want)
		}
	}
	if store.appends != 0 || len(store.logs) != 0 {
		t.Errorf("the refused create appended %d events (%v), want none", store.appends, store.logs)
	}
}

// An identity the caller could not resolve is empty, and the workflow records none: an unknown,
// which the memory gate allows. One that is present but blank is refused before anything is
// appended.
func TestCreateRecordsNoIdentityWhenTheCallerHasNoneAndRefusesABlankOne(t *testing.T) {
	store := newMemEventLog()
	state, err := memLifecycle(t, store).Create("proj-1", "wf-1", "", validGoal("proj-1", "goal-1"), "odd", "")
	if err != nil || state.ProjectIdentity != "" {
		t.Fatalf("Create(no identity) = %+v, %v, want a workflow with no identity", state, err)
	}
	if raw := string(store.logs["proj-1/wf-1"]); strings.Contains(raw, "project_identity") {
		t.Errorf("the created event is %s, want no project_identity field", raw)
	}

	blank := newMemEventLog()
	_, err = memLifecycle(t, blank).Create("proj-1", "wf-1", "   ", validGoal("proj-1", "goal-1"), "odd", "")
	if err == nil || !strings.Contains(err.Error(), "project_identity") {
		t.Errorf("Create(blank identity) = %v, want a refusal naming project_identity", err)
	}
	if blank.appends != 0 {
		t.Errorf("Create(blank identity) appended %d events, want none", blank.appends)
	}
}

// The identity is free text a repository chose (a declared name, a normalized remote), not a path
// component: a slash is allowed, a line break or another control character is not.
func TestTheProjectIdentityOfACreatedEventIsOneLineOfBoundedText(t *testing.T) {
	base := validCreatedEventForIdentity(t)
	for _, tt := range []struct {
		name     string
		identity string
		ok       bool
	}{
		{"absent, as in a log from before it existed", "", true},
		{"a normalized remote", "gitlab.com/acme/platform/widgets", true},
		{"a declared name with a space", "acme widgets", true},
		{"a line break", "acme\nwidgets", false},
		{"a tab", "acme\twidgets", false},
		{"blank", "   ", false},
		{"invalid UTF-8", "acme\xffwidgets", false},
		{"too long", strings.Repeat("a", MaxProjectIdentityLength+1), false},
		{"as long as allowed", strings.Repeat("a", MaxProjectIdentityLength), true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			e := base
			e.ProjectIdentity = tt.identity
			err := e.Validate()
			if tt.ok != (err == nil) {
				t.Errorf("Validate() = %v, want ok=%v", err, tt.ok)
			}
			if err != nil && !strings.Contains(err.Error(), "project_identity") {
				t.Errorf("the refusal %q does not name project_identity", err)
			}
		})
	}
}

// project_identity is a created-only payload field.
func TestOnlyACreatedEventCarriesAProjectIdentity(t *testing.T) {
	store := newMemEventLog()
	if _, err := memLifecycle(t, store).Create("proj-1", "wf-1", testIdentity, validGoal("proj-1", "goal-1"), "odd", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := memLifecycle(t, store).Start("proj-1", "wf-1"); err != nil {
		t.Fatal(err)
	}
	loaded := store.classify("proj-1", "wf-1")
	if len(loaded.Events) < 2 {
		t.Fatalf("loaded %d events, want 2", len(loaded.Events))
	}
	e := loaded.Events[1]
	e.ProjectIdentity = testIdentity
	if err := e.Validate(); err == nil {
		t.Errorf("a started event with a project_identity validated, want it refused")
	}
}

// A log written before the field existed parses and replays: its State has no identity, which the
// memory gate reads as unknown.
func TestALogWithoutAProjectIdentityStillReplays(t *testing.T) {
	e := validCreatedEventForIdentity(t)
	line, err := e.MarshalLine()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(line), "project_identity") {
		t.Fatalf("an event with no identity wrote the field: %s", line)
	}
	loaded := ClassifyLog("proj-1", "wf-1", line)
	if loaded.Classification != ClassificationOwned || loaded.State.ProjectIdentity != "" {
		t.Errorf("ClassifyLog = %q, identity %q, want owned with no identity", loaded.Classification, loaded.State.ProjectIdentity)
	}
}

// validCreatedEventForIdentity is the created event of a fresh workflow, made by Create.
func validCreatedEventForIdentity(t *testing.T) WorkflowEvent {
	t.Helper()
	store := newMemEventLog()
	if _, err := memLifecycle(t, store).Create("proj-1", "wf-1", testIdentity, validGoal("proj-1", "goal-1"), "odd", ""); err != nil {
		t.Fatal(err)
	}
	loaded := store.classify("proj-1", "wf-1")
	if len(loaded.Events) != 1 {
		t.Fatalf("loaded %d events, want 1", len(loaded.Events))
	}
	e := loaded.Events[0]
	e.ProjectIdentity = ""
	return e
}
