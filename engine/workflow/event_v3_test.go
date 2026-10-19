package workflow

import (
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/memoryscope"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflowprofile"
)

// directiveOf is a valid MemoryDirective v1 of scope and sources.
func directiveOf(scope memoryscope.Scope, sources ...memoryscope.Source) *memoryscope.Directive {
	if sources == nil {
		sources = []memoryscope.Source{}
	}
	return &memoryscope.Directive{Version: memoryscope.DirectiveVersion, Scope: scope, Sources: sources, Write: "none"}
}

// headOf is a role chain head digest made of one repeated hex character.
func headOf(c string) string { return strings.Repeat(c, 64) }

// validCreatedEventV3 is a version 3 created event of the odd profile, bound to a role chain, with
// no narrower.
func validCreatedEventV3(t *testing.T) WorkflowEvent {
	t.Helper()
	e := withSnapshot(t, validCreatedEvent(), snapshotOfBuiltIn(t, "odd"))
	e.Version = EventVersionNarrowers
	e.RoleChainID = "chain-1"
	e.RoleChainHead = headOf("1")
	return e
}

// narrowedEventV3 is a version 3 narrowed event recording head and directive.
func narrowedEventV3(head string, directive *memoryscope.Directive) WorkflowEvent {
	e := newEventOfKind(KindNarrowed)
	e.Version = EventVersionNarrowers
	e.Narrowed = &Narrowed{RoleChainHead: head, MemoryDirective: directive}
	return e
}

// v3 is e as an event of a version 3 log.
func v3(e WorkflowEvent) WorkflowEvent {
	e.Version = EventVersionNarrowers
	return e
}

func TestTheProgramWritesVersion3AndAcceptsVersionsOneToThree(t *testing.T) {
	if EventVersion != EventVersionNarrowers || EventVersionNarrowers != 3 {
		t.Fatalf("EventVersion = %d, EventVersionNarrowers = %d, want both 3", EventVersion, EventVersionNarrowers)
	}
	for _, version := range []int{-1, 0, 4, 99} {
		e := validCreatedEventV3(t)
		e.Version = version
		if err := e.Validate(); err == nil || err.Error() != "version must be 1, 2, or 3, got "+itoa(version) {
			t.Errorf("version %d: Validate() = %v, want the version refusal", version, err)
		}
	}
	if err := validCreatedEventV3(t).Validate(); err != nil {
		t.Fatalf("a version 3 created event: Validate() = %v, want nil", err)
	}
}

func itoa(n int) string { return strconv.Itoa(n) }

// A version 3 created event carries the narrowers in a fixed order after the role chain, and parses
// back to the same event.
func TestAVersion3CreatedEventCarriesItsNarrowers(t *testing.T) {
	e := validCreatedEventV3(t)
	e.GoalDirective = directiveOf(memoryscope.ScopeProject, memoryscope.SourceEngram, memoryscope.SourceLongtermMem)
	e.HandoffDigest = headOf("d")
	e.HandoffDirective = directiveOf(memoryscope.ScopeGoal, memoryscope.SourceEngram)
	e.RoleDirective = directiveOf(memoryscope.ScopeNone)
	line, err := e.MarshalLine()
	if err != nil {
		t.Fatalf("MarshalLine() = %v, want nil", err)
	}
	const tail = `"role_chain_id":"chain-1","role_chain_head":"1111111111111111111111111111111111111111111111111111111111111111",` +
		`"goal_directive":{"version":1,"scope":"project","sources":["engram","longterm-mem"],"write":"none"},` +
		`"handoff_digest":"dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",` +
		`"handoff_directive":{"version":1,"scope":"goal","sources":["engram"],"write":"none"},` +
		`"role_directive":{"version":1,"scope":"none","sources":[],"write":"none"}}` + "\n"
	if !strings.HasSuffix(string(line), tail) || !strings.HasPrefix(string(line), `{"version":3,`) {
		t.Fatalf("MarshalLine() = %s, want version 3 ending in %s", line, tail)
	}
	parsed, err := ParseWorkflowEvent(line[:len(line)-1])
	if err != nil {
		t.Fatalf("ParseWorkflowEvent() = %v, want nil", err)
	}
	if !reflect.DeepEqual(parsed, e) {
		t.Errorf("ParseWorkflowEvent() = %+v, want %+v", parsed, e)
	}
}

func TestTheNarrowersOfAVersion3CreatedEventAreWellFormed(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*WorkflowEvent)
		wantErr string
	}{
		{"a handoff digest without its directive", func(e *WorkflowEvent) { e.HandoffDigest = headOf("d") }, "handoff_digest and handoff_directive must be present together"},
		{"a handoff directive without its digest", func(e *WorkflowEvent) { e.HandoffDirective = directiveOf(memoryscope.ScopeNone) }, "handoff_digest and handoff_directive must be present together"},
		{"a handoff digest that is not one", func(e *WorkflowEvent) {
			e.HandoffDigest, e.HandoffDirective = "abc", directiveOf(memoryscope.ScopeNone)
		}, `handoff_digest must be 64 lowercase hex characters, got "abc"`},
		{"an invalid goal directive", func(e *WorkflowEvent) {
			e.GoalDirective = &memoryscope.Directive{Version: 1, Scope: "all", Sources: []memoryscope.Source{}, Write: "none"}
		}, `goal_directive: scope must be "none", "goal", or "project", got "all"`},
		{"an invalid handoff directive", func(e *WorkflowEvent) {
			e.HandoffDigest, e.HandoffDirective = headOf("d"), &memoryscope.Directive{Version: 1, Scope: "none", Sources: []memoryscope.Source{}, Write: "all"}
		}, `handoff_directive: write must be exactly "none", got "all"`},
		{"an invalid role directive", func(e *WorkflowEvent) {
			e.RoleDirective = &memoryscope.Directive{Version: 2, Scope: "none", Sources: []memoryscope.Source{}, Write: "none"}
		}, "role_directive: version must be 1, got 2"},
		{"a role directive with no role chain", func(e *WorkflowEvent) {
			e.RoleChainID, e.RoleChainHead, e.RoleDirective = "", "", directiveOf(memoryscope.ScopeNone)
		}, "role_directive must be empty when role_chain_id is not set"},
		{"no profile snapshot", func(e *WorkflowEvent) { e.ProfileSnapshot, e.ProfileDigest = nil, "" }, "profile_snapshot must be present in a version 3 created event"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := validCreatedEventV3(t)
			tt.mutate(&e)
			if err := e.Validate(); err == nil || err.Error() != tt.wantErr {
				t.Fatalf("Validate() = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

// Versions 1 and 2 know none of the narrowers: a created event of those versions carrying any of
// them, and a narrowed event of those versions, are refused.
func TestTheNarrowersAreRefusedInVersionsOneAndTwo(t *testing.T) {
	fields := map[string]func(*WorkflowEvent){
		"goal_directive":    func(e *WorkflowEvent) { e.GoalDirective = directiveOf(memoryscope.ScopeNone) },
		"handoff_digest":    func(e *WorkflowEvent) { e.HandoffDigest = headOf("d") },
		"handoff_directive": func(e *WorkflowEvent) { e.HandoffDirective = directiveOf(memoryscope.ScopeNone) },
		"role_directive":    func(e *WorkflowEvent) { e.RoleDirective = directiveOf(memoryscope.ScopeNone) },
	}
	for _, version := range []int{EventVersionNameOnly, EventVersionSnapshot} {
		for field, set := range fields {
			t.Run(itoa(version)+"/"+field, func(t *testing.T) {
				e := validCreatedEvent()
				if version == EventVersionSnapshot {
					e = validCreatedEventV2(t)
				}
				e.RoleChainID, e.RoleChainHead = "chain-1", headOf("1")
				set(&e)
				want := "goal_directive, handoff_digest, handoff_directive and role_directive are not part of version " + itoa(version)
				if err := e.Validate(); err == nil || err.Error() != want {
					t.Fatalf("Validate() = %v, want %q", err, want)
				}
			})
		}
		e := narrowedEventV3(headOf("2"), nil)
		e.Version = version
		want := `kind "narrowed" is not part of version ` + itoa(version)
		if err := e.Validate(); err == nil || err.Error() != want {
			t.Errorf("a version %d narrowed event: Validate() = %v, want %q", version, err, want)
		}
	}
}

func TestTheNarrowedPayloadBelongsToTheNarrowedKindAlone(t *testing.T) {
	tests := []struct {
		name    string
		event   func(*testing.T) WorkflowEvent
		wantErr string
	}{
		{"a narrowed event with no payload", func(*testing.T) WorkflowEvent {
			e := narrowedEventV3(headOf("2"), nil)
			e.Narrowed = nil
			return e
		}, `narrowed must be present for kind "narrowed"`},
		{"a malformed head", func(*testing.T) WorkflowEvent { return narrowedEventV3("2", nil) }, `narrowed.role_chain_head must be 64 lowercase hex characters, got "2"`},
		{"an invalid directive", func(*testing.T) WorkflowEvent {
			return narrowedEventV3(headOf("2"), &memoryscope.Directive{Version: 1, Scope: "none", Sources: []memoryscope.Source{memoryscope.SourceEngram}, Write: "none"})
		}, `narrowed.memory_directive: scope "none" requires empty sources, got [engram]`},
		{"a narrowed event with a created field", func(*testing.T) WorkflowEvent {
			e := narrowedEventV3(headOf("2"), nil)
			e.GoalID = "goal-1"
			return e
		}, `kind "narrowed" must not carry created, stage, checked, or closed payload fields`},
		{"a started event with the narrowed payload", func(*testing.T) WorkflowEvent {
			e := v3(newEventOfKind(KindStarted))
			e.Narrowed = &Narrowed{RoleChainHead: headOf("2")}
			return e
		}, `kind "started" must not carry the narrowed payload`},
		{"a created event with the narrowed payload", func(t *testing.T) WorkflowEvent {
			e := validCreatedEventV3(t)
			e.Narrowed = &Narrowed{RoleChainHead: headOf("2")}
			return e
		}, `kind "created" must not carry the narrowed payload`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.event(t).Validate(); err == nil || err.Error() != tt.wantErr {
				t.Fatalf("Validate() = %v, want %q", err, tt.wantErr)
			}
		})
	}
	for _, d := range []*memoryscope.Directive{nil, directiveOf(memoryscope.ScopeGoal, memoryscope.SourceEngram)} {
		e := narrowedEventV3(headOf("2"), d)
		line, err := e.MarshalLine()
		if err != nil {
			t.Fatalf("MarshalLine() = %v, want nil", err)
		}
		parsed, err := ParseWorkflowEvent(line[:len(line)-1])
		if err != nil {
			t.Fatalf("ParseWorkflowEvent(%s) = %v, want nil", line, err)
		}
		if !reflect.DeepEqual(parsed, e) {
			t.Errorf("ParseWorkflowEvent() = %+v, want %+v", parsed, e)
		}
	}
}

// Every nested directive, and the narrowed object, is read with exact-case keys: a key that differs
// only in case would be decoded by encoding/json, and is refused instead.
func TestNestedNarrowerKeysAreReadExactly(t *testing.T) {
	created, err := func() ([]byte, error) {
		e := validCreatedEventV3(t)
		e.GoalDirective = directiveOf(memoryscope.ScopeNone)
		e.HandoffDigest, e.HandoffDirective = headOf("d"), directiveOf(memoryscope.ScopeNone)
		e.RoleDirective = directiveOf(memoryscope.ScopeNone)
		return e.MarshalLine()
	}()
	if err != nil {
		t.Fatal(err)
	}
	narrowed, err := narrowedEventV3(headOf("2"), directiveOf(memoryscope.ScopeNone)).MarshalLine()
	if err != nil {
		t.Fatal(err)
	}
	directive := `{"version":1,"scope":"none","sources":[],"write":"none"}`
	tests := []struct {
		name, line, old, new, wantErr string
	}{
		{"goal directive", string(created), `"goal_directive":` + directive, `"goal_directive":{"version":1,"Scope":"none","sources":[],"write":"none"}`, `goal_directive: parse memory directive: unknown memory directive field "Scope"`},
		{"handoff directive", string(created), `"handoff_directive":` + directive, `"handoff_directive":{"version":1,"scope":"none","sources":[],"write":"none","x":1}`, `handoff_directive: parse memory directive: unknown memory directive field "x"`},
		{"role directive", string(created), `"role_directive":` + directive, `"role_directive":{"version":1,"scope":"none","Sources":[],"write":"none"}`, `role_directive: parse memory directive: unknown memory directive field "Sources"`},
		{"narrowed object", string(narrowed), `"narrowed":{"role_chain_head"`, `"narrowed":{"Role_chain_head"`, `narrowed: unknown workflow event narrowed field "Role_chain_head"`},
		{"narrowed directive", string(narrowed), `"memory_directive":` + directive, `"memory_directive":{"version":1,"scope":"none","sources":[],"WRITE":"none"}`, `narrowed: memory_directive: parse memory directive: unknown memory directive field "WRITE"`},
		{"duplicate nested key", string(narrowed), `"memory_directive":` + directive, `"memory_directive":{"version":1,"scope":"none","scope":"none","sources":[],"write":"none"}`, `duplicate`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(tt.line, tt.old) {
				t.Fatalf("the line %s does not hold %s", tt.line, tt.old)
			}
			line := strings.Replace(strings.TrimSuffix(tt.line, "\n"), tt.old, tt.new, 1)
			_, err := ParseWorkflowEvent([]byte(line))
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("ParseWorkflowEvent() = %v, want an error containing %q", err, tt.wantErr)
			}
		})
	}
}

// narrowedLog is a version 3 log: created (bound to a role chain), then the given kinds, chained.
func narrowedLog(t *testing.T, created WorkflowEvent, rest ...WorkflowEvent) []WorkflowEvent {
	t.Helper()
	return withDigestChain(append([]WorkflowEvent{created}, rest...))
}

func TestANarrowedEventIsLegalWhileTheWorkflowIsOpenAndChangesNoStatus(t *testing.T) {
	created := validCreatedEventV3(t)
	narrowed := narrowedEventV3(headOf("2"), directiveOf(memoryscope.ScopeGoal, memoryscope.SourceEngram))
	tests := []struct {
		name   string
		before []WorkflowEvent
		want   Status
	}{
		{"created", nil, StatusCreated},
		{"running", []WorkflowEvent{v3(seq(KindStarted))}, StatusRunning},
		{"paused", []WorkflowEvent{v3(seq(KindStarted)), v3(seq(KindPaused))}, StatusPaused},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events := append(append([]WorkflowEvent{}, tt.before...), narrowed)
			state, err := Replay(narrowedLog(t, created, events...))
			if err != nil {
				t.Fatalf("Replay() = %v, want nil", err)
			}
			if state.Status != tt.want {
				t.Errorf("status = %q, want %q", state.Status, tt.want)
			}
			if state.NarrowedHead != headOf("2") || !reflect.DeepEqual(state.NarrowedDirective, narrowed.Narrowed.MemoryDirective) {
				t.Errorf("narrowed head %q directive %+v, want %q and %+v", state.NarrowedHead, state.NarrowedDirective, headOf("2"), narrowed.Narrowed.MemoryDirective)
			}
			if state.RoleChainHead != headOf("1") {
				t.Errorf("RoleChainHead = %q, want the head bound at creation, %q", state.RoleChainHead, headOf("1"))
			}
			if state.LastRoleChainHead() != headOf("2") {
				t.Errorf("LastRoleChainHead() = %q, want %q", state.LastRoleChainHead(), headOf("2"))
			}
		})
	}
}

func TestANarrowedEventIsRefusedAfterCloseAndWithoutARoleChain(t *testing.T) {
	created := validCreatedEventV3(t)
	closed := v3(seq(KindClosed))
	closed.Outcome, closed.Reason = string(OutcomeAbandoned), "stop"
	_, err := Replay(narrowedLog(t, created, closed, narrowedEventV3(headOf("2"), nil)))
	if !errors.Is(err, ErrClosed) {
		t.Errorf("narrowed after closed: Replay() = %v, want ErrClosed", err)
	}

	unbound := validCreatedEventV3(t)
	unbound.RoleChainID, unbound.RoleChainHead = "", ""
	_, err = Replay(narrowedLog(t, unbound, narrowedEventV3(headOf("2"), nil)))
	if !errors.Is(err, ErrInvalidTransition) || !strings.Contains(err.Error(), "narrowed needs a workflow bound to a role chain, and this one has none") {
		t.Errorf("narrowed without a role chain: Replay() = %v, want ErrInvalidTransition naming the role chain", err)
	}
}

func TestACompletedCloseStillNeedsAVerifiedEventImmediatelyBefore(t *testing.T) {
	created := validCreatedEventV3(t)
	verified := v3(validPayloadEvent(KindVerified))
	completed := v3(seq(KindClosed))
	completed.Outcome = string(OutcomeCompleted)
	_, err := Replay(narrowedLog(t, created, v3(seq(KindStarted)), verified, narrowedEventV3(headOf("2"), nil), completed))
	if !errors.Is(err, ErrCompletedRequiresVerify) {
		t.Fatalf("completed close after verified then narrowed: Replay() = %v, want ErrCompletedRequiresVerify", err)
	}
	if _, err := Replay(narrowedLog(t, created, v3(seq(KindStarted)), narrowedEventV3(headOf("2"), nil), verified, completed)); err != nil {
		t.Fatalf("completed close after narrowed then verified: Replay() = %v, want nil", err)
	}
}

// Version 3 keeps the rules of version 2: a verification checked the workflow's own profile.
func TestAVersion3VerifiedEventChecksTheWorkflowsProfile(t *testing.T) {
	verified := v3(validPayloadEvent(KindVerified))
	verified.Checked.Profile = "sdd"
	_, err := Replay(narrowedLog(t, validCreatedEventV3(t), verified))
	if !errors.Is(err, ErrInvalidTransition) || !strings.Contains(err.Error(), `verified checked profile "sdd", but the workflow's profile is "odd"`) {
		t.Fatalf("Replay() = %v, want the profile refusal", err)
	}
}

// planState is the state of a version 3 log of the odd profile: created with the given narrowers,
// then the given narrowed events.
func planState(t *testing.T, goal, handoff, role *memoryscope.Directive, narrowed ...WorkflowEvent) State {
	t.Helper()
	created := validCreatedEventV3(t)
	created.ProjectIdentity = "github.com/acme/proj"
	created.GoalDirective = goal
	if handoff != nil {
		created.HandoffDigest, created.HandoffDirective = headOf("d"), handoff
	}
	created.RoleDirective = role
	state, err := Replay(narrowedLog(t, created, narrowed...))
	if err != nil {
		t.Fatalf("Replay() = %v, want nil", err)
	}
	return state
}

func oddProfile(t *testing.T) workflowprofile.WorkflowProfile {
	t.Helper()
	p, err := workflowprofile.Resolve("odd")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPlanOfAppliesTheStoredNarrowersInOrderAndTheLatestRoleOverTheBase(t *testing.T) {
	all := []memoryscope.Source{memoryscope.SourceEngram, memoryscope.SourceLongtermMem, memoryscope.SourceProceduralSkills}
	eng, ltm := memoryscope.SourceEngram, memoryscope.SourceLongtermMem
	tests := []struct {
		name        string
		state       func(*testing.T) State
		wantScope   memoryscope.Scope
		wantSources []memoryscope.Source
		wantGoal    string
	}{
		{"no narrower: the ceiling", func(t *testing.T) State { return planState(t, nil, nil, nil) }, memoryscope.ScopeProject, all, ""},
		{"the goal narrows", func(t *testing.T) State {
			return planState(t, directiveOf(memoryscope.ScopeProject, eng, ltm), nil, nil)
		}, memoryscope.ScopeProject, []memoryscope.Source{eng, ltm}, ""},
		{"the shaper handoff narrows the goal", func(t *testing.T) State {
			return planState(t, directiveOf(memoryscope.ScopeProject, eng, ltm), directiveOf(memoryscope.ScopeGoal, eng), nil)
		}, memoryscope.ScopeGoal, []memoryscope.Source{eng}, "goal-1"},
		{"the created role directive narrows the base", func(t *testing.T) State {
			return planState(t, directiveOf(memoryscope.ScopeProject, eng, ltm), nil, directiveOf(memoryscope.ScopeProject, ltm))
		}, memoryscope.ScopeProject, []memoryscope.Source{ltm}, ""},
		{"a narrowed event replaces the created role directive", func(t *testing.T) State {
			return planState(t, directiveOf(memoryscope.ScopeProject, eng, ltm), nil, directiveOf(memoryscope.ScopeProject, ltm),
				narrowedEventV3(headOf("2"), directiveOf(memoryscope.ScopeGoal, eng)))
		}, memoryscope.ScopeGoal, []memoryscope.Source{eng}, "goal-1"},
		{"the latest narrowed event rules, and gets back to the base", func(t *testing.T) State {
			return planState(t, directiveOf(memoryscope.ScopeProject, eng, ltm), nil, nil,
				narrowedEventV3(headOf("2"), directiveOf(memoryscope.ScopeNone)),
				narrowedEventV3(headOf("3"), directiveOf(memoryscope.ScopeProject, eng, ltm)))
		}, memoryscope.ScopeProject, []memoryscope.Source{eng, ltm}, ""},
		{"a narrowed event with no directive is the base", func(t *testing.T) State {
			return planState(t, directiveOf(memoryscope.ScopeProject, eng, ltm), nil, directiveOf(memoryscope.ScopeNone),
				narrowedEventV3(headOf("2"), nil))
		}, memoryscope.ScopeProject, []memoryscope.Source{eng, ltm}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := PlanOf(tt.state(t), "proj-1", oddProfile(t))
			if err != nil {
				t.Fatalf("PlanOf() = %v, want nil", err)
			}
			if plan.Scope != tt.wantScope || !reflect.DeepEqual(plan.Sources, tt.wantSources) {
				t.Errorf("plan = %s %v, want %s %v", plan.Scope, plan.Sources, tt.wantScope, tt.wantSources)
			}
			if plan.Filters.ProjectID != "github.com/acme/proj" || plan.Filters.GoalID != tt.wantGoal {
				t.Errorf("filters = %+v, want project github.com/acme/proj and goal %q", plan.Filters, tt.wantGoal)
			}
		})
	}
}

func TestPlanOfNamesTheDirectiveThatWidens(t *testing.T) {
	eng, ltm, proc := memoryscope.SourceEngram, memoryscope.SourceLongtermMem, memoryscope.SourceProceduralSkills
	sdd, err := workflowprofile.Resolve("sdd")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		state   func(*testing.T) State
		wantErr string
	}{
		{"the goal", func(t *testing.T) State {
			return planState(t, directiveOf(memoryscope.ScopeProject, eng, ltm), nil, nil)
		}, "the goal memory directive widens the plan: adds source longterm-mem not present in the current scope"},
		{"the shaper handoff", func(t *testing.T) State {
			return planState(t, directiveOf(memoryscope.ScopeGoal, eng), directiveOf(memoryscope.ScopeProject, eng), nil)
		}, "the shaper handoff memory directive widens the plan: widens scope from goal to project"},
		{"the created role handoff", func(t *testing.T) State {
			return planState(t, nil, nil, directiveOf(memoryscope.ScopeProject, eng, proc))
		}, "the role handoff memory directive widens the plan: adds source procedural-skills not present in the current scope"},
		{"a narrowed role handoff", func(t *testing.T) State {
			return planState(t, nil, nil, nil, narrowedEventV3(headOf("2"), directiveOf(memoryscope.ScopeProject, ltm)))
		}, "the role handoff memory directive widens the plan: adds source longterm-mem not present in the current scope"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := PlanOf(tt.state(t), "proj-1", sdd)
			if err == nil || err.Error() != tt.wantErr {
				t.Fatalf("PlanOf() = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

// A workflow that recorded no project identity is planned for its slug, as before.
func TestPlanOfFallsBackToTheSlugWithoutAnIdentity(t *testing.T) {
	state := planState(t, nil, nil, nil)
	state.ProjectIdentity = ""
	plan, err := PlanOf(state, "proj-1", oddProfile(t))
	if err != nil {
		t.Fatalf("PlanOf() = %v, want nil", err)
	}
	if plan.Filters.ProjectID != "proj-1" {
		t.Errorf("project = %q, want the slug proj-1", plan.Filters.ProjectID)
	}
}

// A version 2 log replays as it did, and its plan is the profile's ceiling.
func TestAVersion2LogStillReplaysAndIsPlannedAtTheCeiling(t *testing.T) {
	created := validCreatedEventV2(t)
	state, err := Replay(withDigestChain([]WorkflowEvent{created, asV2(seq(KindStarted)), asV2(validPayloadEvent(KindVerified))}))
	if err != nil {
		t.Fatalf("Replay() = %v, want nil", err)
	}
	if state.Version != EventVersionSnapshot || state.Status != StatusRunning || state.LastVerifiedSeq != 2 {
		t.Errorf("state = version %d status %q last verified %d, want 2 running 2", state.Version, state.Status, state.LastVerifiedSeq)
	}
	plan, err := PlanOf(state, "proj-1", oddProfile(t))
	if err != nil {
		t.Fatalf("PlanOf() = %v, want nil", err)
	}
	ceiling, err := memoryscope.DefaultFor(oddProfile(t))
	if err != nil {
		t.Fatal(err)
	}
	want, err := memoryscope.Resolve(ceiling, "proj-1", "goal-1")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(plan, want) {
		t.Errorf("PlanOf() = %+v, want the ceiling %+v", plan, want)
	}
}
