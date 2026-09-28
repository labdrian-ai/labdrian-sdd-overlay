package workflow

import (
	"encoding/json"
	"strings"
	"testing"
)

func validCreatedEvent() WorkflowEvent {
	return WorkflowEvent{
		Version:    1,
		WorkflowID: "wf-1",
		ProjectID:  "proj-1",
		Seq:        0,
		PrevDigest: "",
		Kind:       KindCreated,
		At:         "2026-09-28T10:00:00Z",
		Provenance: Provenance{
			WorktreeRoot: "/home/labdrian/labdrian-sdd-overlay",
			GitHead:      "e1218c2f00000000000000000000000000000000",
		},
		Observations: []Observation{},
		GoalID:       "goal-1",
		GoalDigest:   strings.Repeat("a", 64),
		Profile:      "odd",
	}
}

func validStartedEvent() WorkflowEvent {
	created := validCreatedEvent()
	prevDigest, err := EventDigest(created)
	if err != nil {
		panic(err)
	}
	e := created
	e.Seq = 1
	e.PrevDigest = prevDigest
	e.Kind = KindStarted
	e.GoalID, e.GoalDigest, e.Profile = "", "", ""
	return e
}

func TestParseWorkflowEventValidDocument(t *testing.T) {
	e := validCreatedEvent()
	data, err := e.Marshal()
	if err != nil {
		t.Fatalf("Marshal() = %v, want nil", err)
	}
	parsed, err := ParseWorkflowEvent(data)
	if err != nil {
		t.Fatalf("ParseWorkflowEvent() = %v, want nil", err)
	}
	if parsed.WorkflowID != e.WorkflowID || parsed.Kind != e.Kind {
		t.Fatalf("ParseWorkflowEvent() = %+v, want %+v", parsed, e)
	}
}

func TestParseWorkflowEventRejectsUnknownField(t *testing.T) {
	e := validCreatedEvent()
	data, err := e.Marshal()
	if err != nil {
		t.Fatalf("Marshal() = %v, want nil", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("json.Unmarshal() = %v, want nil", err)
	}
	raw["bogus"] = json.RawMessage(`"x"`)
	mutated, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("json.Marshal() = %v, want nil", err)
	}
	if _, err := ParseWorkflowEvent(mutated); err == nil {
		t.Fatalf("ParseWorkflowEvent() = nil, want error for unknown field")
	}
}

func TestParseWorkflowEventRejectsUnknownNestedField(t *testing.T) {
	e := validCreatedEvent()
	data, err := e.Marshal()
	if err != nil {
		t.Fatalf("Marshal() = %v, want nil", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("json.Unmarshal() = %v, want nil", err)
	}
	raw["provenance"] = json.RawMessage(`{"worktree_root":"/x","git_head":"","bogus":"x"}`)
	mutated, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("json.Marshal() = %v, want nil", err)
	}
	if _, err := ParseWorkflowEvent(mutated); err == nil {
		t.Fatalf("ParseWorkflowEvent() = nil, want error for unknown nested field")
	}
}

func TestParseWorkflowEventRejectsDuplicateKeys(t *testing.T) {
	data := []byte(`{"version":1,"version":1}`)
	if _, err := ParseWorkflowEvent(data); err == nil {
		t.Fatalf("ParseWorkflowEvent() = nil, want error for duplicate keys")
	}
}

func TestObservationsSerializeAsEmptyArrayNeverNull(t *testing.T) {
	e := validCreatedEvent()
	e.Observations = []Observation{}
	data, err := e.Marshal()
	if err != nil {
		t.Fatalf("Marshal() = %v, want nil", err)
	}
	if !strings.Contains(string(data), `"observations": []`) && !strings.Contains(string(data), `"observations":[]`) {
		t.Fatalf("Marshal() = %s, want observations serialized as []", data)
	}
}

func TestWorkflowEventValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(e *WorkflowEvent)
		wantErr bool
	}{
		{name: "valid created", mutate: func(e *WorkflowEvent) {}, wantErr: false},
		{name: "bad version", mutate: func(e *WorkflowEvent) { e.Version = 2 }, wantErr: true},
		{name: "blank workflow_id", mutate: func(e *WorkflowEvent) { e.WorkflowID = "" }, wantErr: true},
		{name: "workflow_id with path separator", mutate: func(e *WorkflowEvent) { e.WorkflowID = "a/b" }, wantErr: true},
		{name: "workflow_id with dotdot", mutate: func(e *WorkflowEvent) { e.WorkflowID = ".." }, wantErr: true},
		{name: "workflow_id too long", mutate: func(e *WorkflowEvent) { e.WorkflowID = strings.Repeat("a", MaxIdentifierLength+1) }, wantErr: true},
		{name: "blank project_id", mutate: func(e *WorkflowEvent) { e.ProjectID = "" }, wantErr: true},
		{name: "negative seq", mutate: func(e *WorkflowEvent) { e.Seq = -1 }, wantErr: true},
		{name: "prev_digest not empty at seq 0", mutate: func(e *WorkflowEvent) { e.PrevDigest = strings.Repeat("a", 64) }, wantErr: true},
		{name: "unknown kind", mutate: func(e *WorkflowEvent) { e.Kind = "bogus" }, wantErr: true},
		{name: "bad timestamp", mutate: func(e *WorkflowEvent) { e.At = "not-a-time" }, wantErr: true},
		{name: "non-utc timestamp", mutate: func(e *WorkflowEvent) { e.At = "2026-09-28T10:00:00+02:00" }, wantErr: true},
		{name: "relative worktree root", mutate: func(e *WorkflowEvent) { e.Provenance.WorktreeRoot = "relative/path" }, wantErr: true},
		{name: "non-hex git head", mutate: func(e *WorkflowEvent) { e.Provenance.GitHead = "not-hex!" }, wantErr: true},
		{name: "nil observations", mutate: func(e *WorkflowEvent) { e.Observations = nil }, wantErr: true},
		{name: "observation blank capability", mutate: func(e *WorkflowEvent) {
			e.Observations = []Observation{{Capability: "", Status: ObservationAvailable}}
		}, wantErr: true},
		{name: "observation bad status", mutate: func(e *WorkflowEvent) {
			e.Observations = []Observation{{Capability: "memory", Status: "maybe"}}
		}, wantErr: true},
		{name: "created missing goal_id", mutate: func(e *WorkflowEvent) { e.GoalID = "" }, wantErr: true},
		{name: "created bad goal_digest", mutate: func(e *WorkflowEvent) { e.GoalDigest = "short" }, wantErr: true},
		{name: "created unknown profile", mutate: func(e *WorkflowEvent) { e.Profile = "no-such-profile" }, wantErr: true},
		{name: "created with stray stage field", mutate: func(e *WorkflowEvent) { e.Stage = "explore" }, wantErr: true},
		{name: "created with stray outcome field", mutate: func(e *WorkflowEvent) { e.Outcome = string(OutcomeCompleted) }, wantErr: true},
		{name: "created with valid role_chain_id", mutate: func(e *WorkflowEvent) { e.RoleChainID = "chain-1" }, wantErr: false},
		{name: "created with unsafe role_chain_id", mutate: func(e *WorkflowEvent) { e.RoleChainID = "../etc" }, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := validCreatedEvent()
			tt.mutate(&e)
			err := e.Validate()
			if tt.wantErr && err == nil {
				t.Fatalf("Validate() = nil, want error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
		})
	}
}

func TestWorkflowEventValidateStageRecordedPayload(t *testing.T) {
	e := validStartedEvent()
	e.Kind = KindStageRecorded
	e.Stage = "explore"
	if err := e.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}

	e.Stage = ""
	if err := e.Validate(); err == nil {
		t.Fatalf("Validate() = nil, want error for blank stage")
	}

	e.Stage = "explore"
	e.GoalID = "leaked"
	if err := e.Validate(); err == nil {
		t.Fatalf("Validate() = nil, want error for stray created field on stage_recorded")
	}
}

func TestWorkflowEventValidateVerifiedPayload(t *testing.T) {
	e := validStartedEvent()
	e.Kind = KindVerified
	e.Checked = &Checked{
		ChainDigest: strings.Repeat("b", 64),
		GoalDigest:  strings.Repeat("a", 64),
		Profile:     "odd",
	}
	if err := e.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}

	e.Checked = nil
	if err := e.Validate(); err == nil {
		t.Fatalf("Validate() = nil, want error for missing checked payload")
	}

	e.Checked = &Checked{ChainDigest: "short", GoalDigest: strings.Repeat("a", 64), Profile: "odd"}
	if err := e.Validate(); err == nil {
		t.Fatalf("Validate() = nil, want error for bad chain_digest")
	}

	e.Checked = &Checked{ChainDigest: strings.Repeat("b", 64), GoalDigest: strings.Repeat("a", 64), Profile: "no-such"}
	if err := e.Validate(); err == nil {
		t.Fatalf("Validate() = nil, want error for unknown checked profile")
	}
}

func TestWorkflowEventValidateClosedPayload(t *testing.T) {
	e := validStartedEvent()
	e.Kind = KindClosed
	e.Outcome = string(OutcomeCompleted)
	if err := e.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}

	e.Outcome = string(OutcomeAbandoned)
	if err := e.Validate(); err == nil {
		t.Fatalf("Validate() = nil, want error for abandoned close without reason")
	}
	e.Reason = "user cancelled"
	if err := e.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}

	e.Outcome = "bogus"
	if err := e.Validate(); err == nil {
		t.Fatalf("Validate() = nil, want error for unknown outcome")
	}
}

func TestEventDigestStableAndSensitive(t *testing.T) {
	e := validCreatedEvent()
	d1, err := EventDigest(e)
	if err != nil {
		t.Fatalf("EventDigest() = %v, want nil", err)
	}
	d2, err := EventDigest(e)
	if err != nil {
		t.Fatalf("EventDigest() = %v, want nil", err)
	}
	if d1 != d2 {
		t.Fatalf("EventDigest() not stable: %q != %q", d1, d2)
	}

	mutated := e
	mutated.GoalID = "goal-2"
	d3, err := EventDigest(mutated)
	if err != nil {
		t.Fatalf("EventDigest() = %v, want nil", err)
	}
	if d1 == d3 {
		t.Fatalf("EventDigest() did not change after field mutation")
	}
}

func chainOf(t *testing.T, n int) []WorkflowEvent {
	t.Helper()
	events := make([]WorkflowEvent, 0, n)
	created := validCreatedEvent()
	events = append(events, created)
	prevDigest, err := EventDigest(created)
	if err != nil {
		t.Fatalf("EventDigest() = %v, want nil", err)
	}
	for i := 1; i < n; i++ {
		next := validStartedEvent()
		next.Seq = i
		next.PrevDigest = prevDigest
		if i > 1 {
			next.Kind = KindStageRecorded
			next.Stage = "explore"
		}
		events = append(events, next)
		prevDigest, err = EventDigest(next)
		if err != nil {
			t.Fatalf("EventDigest() = %v, want nil", err)
		}
	}
	return events
}

func TestVerifyEventsValidChain(t *testing.T) {
	events := chainOf(t, 3)
	if err := VerifyEvents(events); err != nil {
		t.Fatalf("VerifyEvents() = %v, want nil", err)
	}
}

func TestVerifyEventsRejectsWrongPrevDigest(t *testing.T) {
	events := chainOf(t, 2)
	events[1].PrevDigest = strings.Repeat("f", 64)
	if err := VerifyEvents(events); err == nil {
		t.Fatalf("VerifyEvents() = nil, want error for wrong prev_digest")
	}
}

func TestVerifyEventsRejectsSeqGap(t *testing.T) {
	events := chainOf(t, 2)
	events[1].Seq = 5
	if err := VerifyEvents(events); err == nil {
		t.Fatalf("VerifyEvents() = nil, want error for seq gap")
	}
}

func TestVerifyEventsRejectsMixedWorkflowID(t *testing.T) {
	events := chainOf(t, 2)
	events[1].WorkflowID = "other-workflow"
	if err := VerifyEvents(events); err == nil {
		t.Fatalf("VerifyEvents() = nil, want error for mixed workflow_id")
	}
}

func TestVerifyEventsRejectsMixedProjectID(t *testing.T) {
	events := chainOf(t, 2)
	events[1].ProjectID = "other-project"
	if err := VerifyEvents(events); err == nil {
		t.Fatalf("VerifyEvents() = nil, want error for mixed project_id")
	}
}

func TestVerifyEventsRejectsEmptyChain(t *testing.T) {
	if err := VerifyEvents(nil); err == nil {
		t.Fatalf("VerifyEvents() = nil, want error for empty chain")
	}
}

func TestVerifyEventsRejectsInvalidEvent(t *testing.T) {
	events := chainOf(t, 2)
	events[1].At = "not-a-time"
	if err := VerifyEvents(events); err == nil {
		t.Fatalf("VerifyEvents() = nil, want error for invalid event")
	}
}
