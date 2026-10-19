package workflow

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/goal"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/memoryscope"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/roles"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/sha256hex"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/shaper"
)

// goalV3 is validGoal at version 3, carrying directive.
func goalV3(projectID, goalID string, directive *memoryscope.Directive) goal.Goal {
	g := validGoal(projectID, goalID)
	g.Version = 3
	g.MemoryScope = ""
	g.MemoryDirective = directive
	return g
}

// handoffJSON is a shaper handoff document of version 3 for projectID/goalID; its memory member is
// memory, written verbatim.
func handoffJSON(version int, projectID, goalID, memory string) string {
	return `{"version":` + itoa(version) + `,"project_id":"` + projectID + `","goal_id":"` + goalID + `","architecture":"Layered CLI.",` +
		`"stages":["Build."],"acceptance":[{"criterion":"It works.","verification":{"check":"go test ./..."}}],` +
		`"out_of_scope":["Nothing else."],"roles":[{"role":"builder","responsibility":"Writes the code."}],` +
		`"tests":["go test ./..."],"risks":[{"risk":"Scope creep.","mitigation":"Stick to the design."}],` +
		`"estimates":[{"stage":"Build.","low_minutes":10,"high_minutes":20}],` + memory + `,"delivery_limit":"No delivery without human clearance."}`
}

// shaperHandoffOf parses a shaper handoff document and pairs it with the digest of its bytes, as the
// CLI does.
func shaperHandoffOf(t *testing.T, doc string) ShaperHandoff {
	t.Helper()
	h, err := shaper.Parse([]byte(doc))
	if err != nil {
		t.Fatalf("shaper.Parse() = %v, want nil", err)
	}
	return ShaperHandoff{Handoff: h, Digest: sha256hex.Sum([]byte(doc))}
}

// handoffV4Of is a version 4 shaper handoff for projectID/goalID carrying directive.
func handoffV4Of(t *testing.T, projectID, goalID string, directive *memoryscope.Directive) ShaperHandoff {
	t.Helper()
	data, err := json.Marshal(directive)
	if err != nil {
		t.Fatal(err)
	}
	return shaperHandoffOf(t, handoffJSON(4, projectID, goalID, `"memory_directive":`+string(data)))
}

// roleChain is a valid chain of version 2 records for proj-1/goal-1/chain-1 whose record i carries
// directives[i] (nil for none): prototyper hands off to shaper, then shaper to builder, then builder
// to reviewer, and so on.
func roleChain(t *testing.T, directives ...*memoryscope.Directive) []roles.ChainRecord {
	t.Helper()
	path := []roles.Role{roles.RolePrototyper, roles.RoleShaper, roles.RoleBuilder, roles.RoleReviewer, roles.RolePolisher}
	prev := roles.EmptyChainDigest
	var records []roles.ChainRecord
	for i, d := range directives {
		h := roles.RoleHandoff{
			Version: roles.HandoffVersionV2, ProjectID: "proj-1", GoalID: "goal-1", ChainID: "chain-1", Seq: i + 1,
			FromRole: path[i], ToRole: path[i+1], PrevSHA256: prev, PayloadKind: "plan", PayloadSHA256: strings.Repeat("a", 64),
			Evidence: []roles.Evidence{}, Context: roles.Context{Summary: "s", Decisions: []string{}, OpenQuestions: []string{}},
			Status: roles.StatusCompleted, MemoryDirective: d,
		}
		raw, err := json.Marshal(h)
		if err != nil {
			t.Fatal(err)
		}
		records = append(records, roles.ChainRecord{Raw: raw, Handoff: h})
		prev = sha256hex.Sum(raw)
	}
	if err := roles.VerifyChain(records); err != nil {
		t.Fatalf("the test chain does not verify: %v", err)
	}
	return records
}

// narrowWorld is a lifecycle over an in-memory log and chain store, for the odd profile.
type narrowWorld struct {
	store  *memEventLog
	chains *fakeChainReader
	lc     Lifecycle
}

func newNarrowWorld(t *testing.T) narrowWorld {
	t.Helper()
	store, chains := newMemEventLog(), newFakeChainReader()
	return narrowWorld{store: store, chains: chains, lc: newTestLifecycle(t, store, stepClock(), newFakeGoalReader(), chains, UnavailableProber{})}
}

func (w narrowWorld) log() string { return string(w.store.logs["proj-1/wf-1"]) }

func (w narrowWorld) events(t *testing.T) []WorkflowEvent {
	t.Helper()
	loaded := ClassifyLog("proj-1", "wf-1", w.store.logs["proj-1/wf-1"])
	if loaded.Classification != ClassificationOwned {
		t.Fatalf("the log is %s (%s), want owned", loaded.Classification, loaded.Detail)
	}
	return loaded.Events
}

var (
	eng  = memoryscope.SourceEngram
	ltm  = memoryscope.SourceLongtermMem
	proc = memoryscope.SourceProceduralSkills
)

func TestCreateRecordsTheGoalTheShaperHandoffAndTheRoleChainDirectives(t *testing.T) {
	w := newNarrowWorld(t)
	records := roleChain(t, directiveOf(memoryscope.ScopeGoal, eng))
	w.chains.set("proj-1", "goal-1", "chain-1", records)
	handoff := handoffV4Of(t, "proj-1", "goal-1", directiveOf(memoryscope.ScopeGoal, eng, ltm))

	state, err := w.lc.CreateWithHandoff("proj-1", "wf-1", testIdentity, goalV3("proj-1", "goal-1", directiveOf(memoryscope.ScopeProject, eng, ltm)), "odd", "chain-1", handoff)
	if err != nil {
		t.Fatalf("CreateWithHandoff() = %v, want nil", err)
	}
	events := w.events(t)
	if len(events) != 1 {
		t.Fatalf("the log holds %d events, want 1", len(events))
	}
	created := events[0]
	if created.Version != EventVersionNarrowers {
		t.Errorf("version = %d, want %d", created.Version, EventVersionNarrowers)
	}
	checks := []struct {
		name      string
		got, want any
	}{
		{"goal_directive", created.GoalDirective, directiveOf(memoryscope.ScopeProject, eng, ltm)},
		{"handoff_digest", created.HandoffDigest, handoff.Digest},
		{"handoff_directive", created.HandoffDirective, directiveOf(memoryscope.ScopeGoal, eng, ltm)},
		{"role_directive", created.RoleDirective, directiveOf(memoryscope.ScopeGoal, eng)},
		{"state goal directive", state.GoalDirective, created.GoalDirective},
		{"state handoff digest", state.HandoffDigest, handoff.Digest},
		{"state role directive", state.RoleDirective, created.RoleDirective},
	}
	for _, c := range checks {
		if !reflect.DeepEqual(c.got, c.want) {
			t.Errorf("%s = %+v, want %+v", c.name, c.got, c.want)
		}
	}
	plan, err := PlanOf(state, "proj-1", oddProfile(t))
	if err != nil {
		t.Fatalf("PlanOf() = %v, want nil", err)
	}
	if plan.Scope != memoryscope.ScopeGoal || !reflect.DeepEqual(plan.Sources, []memoryscope.Source{eng}) {
		t.Errorf("plan = %s %v, want goal [engram]", plan.Scope, plan.Sources)
	}
}

// Without a handoff and with Goals and role records that carry no directive, the created event has
// no narrower at all, and the plan is the ceiling.
func TestCreateRecordsNoNarrowerWhereThereIsNone(t *testing.T) {
	w := newNarrowWorld(t)
	w.chains.set("proj-1", "goal-1", "chain-1", roleChain(t, nil))
	if _, err := w.lc.Create("proj-1", "wf-1", testIdentity, validGoal("proj-1", "goal-1"), "odd", "chain-1"); err != nil {
		t.Fatalf("Create() = %v, want nil", err)
	}
	for _, field := range []string{"goal_directive", "handoff_digest", "handoff_directive", "role_directive"} {
		if strings.Contains(w.log(), field) {
			t.Errorf("the created event holds %s: %s", field, w.log())
		}
	}
	if !strings.HasPrefix(w.log(), `{"version":3,`) {
		t.Errorf("the created event is %.20s, want version 3", w.log())
	}
}

func TestCreateRefusesAShaperHandoffThatDoesNotFitAndAppendsNothing(t *testing.T) {
	tests := []struct {
		name    string
		goal    goal.Goal
		handoff func(*testing.T) ShaperHandoff
		wantErr string
	}{
		{"a version 3 handoff", goalV3("proj-1", "goal-1", directiveOf(memoryscope.ScopeProject, eng)),
			func(t *testing.T) ShaperHandoff {
				return shaperHandoffOf(t, handoffJSON(3, "proj-1", "goal-1", `"memory_scope":"Project-scoped."`))
			}, "it is of version 3, and only a version 4 handoff carries a memory directive"},
		{"a handoff of another project", goalV3("proj-1", "goal-1", directiveOf(memoryscope.ScopeProject, eng)),
			func(t *testing.T) ShaperHandoff {
				return handoffV4Of(t, "proj-2", "goal-1", directiveOf(memoryscope.ScopeNone))
			},
			`its project_id is "proj-2" and the goal's is "proj-1"`},
		{"a handoff of another goal", goalV3("proj-1", "goal-1", directiveOf(memoryscope.ScopeProject, eng)),
			func(t *testing.T) ShaperHandoff {
				return handoffV4Of(t, "proj-1", "goal-2", directiveOf(memoryscope.ScopeNone))
			},
			`its goal_id is "goal-2" and the goal's is "goal-1"`},
		{"a version 4 handoff with a version 2 Goal", validGoal("proj-1", "goal-1"),
			func(t *testing.T) ShaperHandoff {
				return handoffV4Of(t, "proj-1", "goal-1", directiveOf(memoryscope.ScopeNone))
			},
			"a version 4 handoff pairs with a Goal of version 3, and the goal is of version 2"},
		{"a digest that is not one", goalV3("proj-1", "goal-1", directiveOf(memoryscope.ScopeProject, eng)),
			func(t *testing.T) ShaperHandoff {
				h := handoffV4Of(t, "proj-1", "goal-1", directiveOf(memoryscope.ScopeNone))
				h.Digest = "abc"
				return h
			}, `its digest "abc" is not 64 lowercase hex characters`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newNarrowWorld(t)
			_, err := w.lc.CreateWithHandoff("proj-1", "wf-1", testIdentity, tt.goal, "odd", "", tt.handoff(t))
			if !errors.Is(err, ErrHandoffRefused) || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("CreateWithHandoff() = %v, want ErrHandoffRefused saying %q", err, tt.wantErr)
			}
			if !strings.HasPrefix(err.Error(), "workflow lifecycle: ") {
				t.Errorf("the refusal %q does not begin like the lifecycle's", err)
			}
			if w.store.appends != 0 || len(w.store.logs) != 0 {
				t.Errorf("the refused create appended %d events, want none", w.store.appends)
			}
		})
	}
}

func TestCreateRefusesAWideningDirectiveNamingItAndAppendsNothing(t *testing.T) {
	tests := []struct {
		name    string
		profile string
		goal    goal.Goal
		handoff *memoryscope.Directive
		role    *memoryscope.Directive
		wantErr string
	}{
		{"the goal", "sdd", goalV3("proj-1", "goal-1", directiveOf(memoryscope.ScopeProject, eng, ltm)), nil, nil,
			"workflow lifecycle: create: the goal memory directive widens the plan: adds source longterm-mem not present in the current scope"},
		{"the shaper handoff", "odd", goalV3("proj-1", "goal-1", directiveOf(memoryscope.ScopeGoal, eng)), directiveOf(memoryscope.ScopeProject, eng), nil,
			"workflow lifecycle: create: the shaper handoff memory directive widens the plan: widens scope from goal to project"},
		{"the role handoff", "odd", goalV3("proj-1", "goal-1", directiveOf(memoryscope.ScopeProject, eng)), nil, directiveOf(memoryscope.ScopeProject, eng, proc),
			"workflow lifecycle: create: the role handoff memory directive widens the plan: adds source procedural-skills not present in the current scope"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newNarrowWorld(t)
			w.chains.set("proj-1", "goal-1", "chain-1", roleChain(t, tt.role))
			var err error
			if tt.handoff != nil {
				_, err = w.lc.CreateWithHandoff("proj-1", "wf-1", testIdentity, tt.goal, tt.profile, "chain-1", handoffV4Of(t, "proj-1", "goal-1", tt.handoff))
			} else {
				_, err = w.lc.Create("proj-1", "wf-1", testIdentity, tt.goal, tt.profile, "chain-1")
			}
			if err == nil || err.Error() != tt.wantErr {
				t.Fatalf("create = %v, want %q", err, tt.wantErr)
			}
			if w.store.appends != 0 || len(w.store.logs) != 0 {
				t.Errorf("the refused create appended %d events, want none", w.store.appends)
			}
		})
	}
}

// createBound creates proj-1/wf-1 of the odd profile with a Goal v3 directive over the chain of
// records, and returns the world.
func createBound(t *testing.T, records []roles.ChainRecord) narrowWorld {
	t.Helper()
	w := newNarrowWorld(t)
	w.chains.set("proj-1", "goal-1", "chain-1", records)
	if _, err := w.lc.Create("proj-1", "wf-1", testIdentity, goalV3("proj-1", "goal-1", directiveOf(memoryscope.ScopeProject, eng, ltm)), "odd", "chain-1"); err != nil {
		t.Fatalf("Create() = %v, want nil", err)
	}
	return w
}

func TestNarrowAppendsTheNewHeadAndItsDirective(t *testing.T) {
	records := roleChain(t, nil, directiveOf(memoryscope.ScopeGoal, ltm), directiveOf(memoryscope.ScopeNone), directiveOf(memoryscope.ScopeProject, eng, ltm))
	w := createBound(t, records[:1])
	if _, err := w.lc.Start("proj-1", "wf-1"); err != nil {
		t.Fatal(err)
	}

	type step struct {
		chain       int
		wantScope   memoryscope.Scope
		wantSources []memoryscope.Source
	}
	steps := []step{
		{2, memoryscope.ScopeGoal, []memoryscope.Source{ltm}},         // the role narrows the base
		{3, memoryscope.ScopeNone, []memoryscope.Source{}},            // the next replaces it
		{4, memoryscope.ScopeProject, []memoryscope.Source{eng, ltm}}, // a later role gets back to the base
	}
	for _, s := range steps {
		w.chains.set("proj-1", "goal-1", "chain-1", records[:s.chain])
		state, err := w.lc.Narrow("proj-1", "wf-1")
		if err != nil {
			t.Fatalf("Narrow() with %d records = %v, want nil", s.chain, err)
		}
		events := w.events(t)
		last := events[len(events)-1]
		head := chainRecordDigest(records[s.chain-1])
		if last.Kind != KindNarrowed || last.Narrowed == nil || last.Narrowed.RoleChainHead != head ||
			!reflect.DeepEqual(last.Narrowed.MemoryDirective, records[s.chain-1].Handoff.MemoryDirective) {
			t.Fatalf("the last event is %+v, want a narrowed event of head %s", last, head)
		}
		if state.Status != StatusRunning || state.RoleChainHead != chainRecordDigest(records[0]) || state.NarrowedHead != head {
			t.Errorf("state = status %q bound head %q narrowed head %q, want running, the created head and %s", state.Status, state.RoleChainHead, state.NarrowedHead, head)
		}
		plan, err := PlanOf(state, "proj-1", oddProfile(t))
		if err != nil {
			t.Fatalf("PlanOf() = %v, want nil", err)
		}
		if plan.Scope != s.wantScope || !reflect.DeepEqual(plan.Sources, s.wantSources) {
			t.Errorf("after record %d the plan is %s %v, want %s %v", s.chain, plan.Scope, plan.Sources, s.wantScope, s.wantSources)
		}
	}
	// The chain still contains the head bound at creation, so verify passes, with its words.
	goals := newFakeGoalReader()
	goals.set("proj-1", "goal-1", goalV3("proj-1", "goal-1", directiveOf(memoryscope.ScopeProject, eng, ltm)))
	lc := newTestLifecycle(t, w.store, stepClock(), goals, w.chains, UnavailableProber{})
	if _, err := lc.Verify("proj-1", "wf-1"); err != nil {
		t.Fatalf("Verify() after narrowing = %v, want nil", err)
	}
}

func TestNarrowRefusesAndAppendsNothing(t *testing.T) {
	records := roleChain(t, nil, directiveOf(memoryscope.ScopeGoal, eng), directiveOf(memoryscope.ScopeProject, proc))
	tests := []struct {
		name    string
		setup   func(*testing.T) narrowWorld
		wantIs  error
		wantErr string
	}{
		{"a version 2 log", func(t *testing.T) narrowWorld {
			w := newNarrowWorld(t)
			created := validCreatedEventV2(t)
			created.RoleChainID, created.RoleChainHead = "chain-1", chainRecordDigest(records[0])
			w.store.logs["proj-1/wf-1"] = logOf(t, created)
			w.chains.set("proj-1", "goal-1", "chain-1", records[:2])
			return w
		}, ErrNarrowersNeedVersion3, "the log is of version 2, and narrowers need version 3: create the workflow again"},
		{"no role chain", func(t *testing.T) narrowWorld {
			w := newNarrowWorld(t)
			if _, err := w.lc.Create("proj-1", "wf-1", testIdentity, validGoal("proj-1", "goal-1"), "odd", ""); err != nil {
				t.Fatal(err)
			}
			return w
		}, ErrNoRoleChain, ""},
		{"a chain that no longer verifies", func(t *testing.T) narrowWorld {
			w := createBound(t, records[:1])
			w.chains.set("proj-1", "goal-1", "chain-1", nil)
			return w
		}, ErrRoleChainInvalid, "has no records"},
		{"a chain that lost the last recorded head", func(t *testing.T) narrowWorld {
			w := createBound(t, records[:1])
			w.chains.set("proj-1", "goal-1", "chain-1", roleChain(t, directiveOf(memoryscope.ScopeNone), nil))
			return w
		}, ErrRoleChainInvalid, "workflow lifecycle: narrow: workflow lifecycle: referenced role chain does not verify: role chain \"chain-1\" no longer contains the last head this workflow recorded"},
		{"nothing new to narrow", func(t *testing.T) narrowWorld {
			w := createBound(t, records[:1])
			return w
		}, ErrNothingToNarrow, ""},
		{"nothing new after a narrowed event", func(t *testing.T) narrowWorld {
			w := createBound(t, records[:1])
			w.chains.set("proj-1", "goal-1", "chain-1", records[:2])
			if _, err := w.lc.Narrow("proj-1", "wf-1"); err != nil {
				t.Fatal(err)
			}
			return w
		}, ErrNothingToNarrow, ""},
		{"a closed workflow", func(t *testing.T) narrowWorld {
			w := createBound(t, records[:1])
			if _, err := w.lc.Close("proj-1", "wf-1", OutcomeAbandoned, "stop"); err != nil {
				t.Fatal(err)
			}
			w.chains.set("proj-1", "goal-1", "chain-1", records[:2])
			return w
		}, ErrClosed, "workflow lifecycle: narrow: workflow: no event may follow a closed workflow"},
		{"a widening role directive", func(t *testing.T) narrowWorld {
			w := createBound(t, records[:1])
			w.chains.set("proj-1", "goal-1", "chain-1", records)
			return w
		}, nil, "workflow lifecycle: narrow: the role handoff memory directive widens the plan: adds source procedural-skills not present in the current scope"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := tt.setup(t)
			before := w.log()
			appends := w.store.appends
			_, err := w.lc.Narrow("proj-1", "wf-1")
			if err == nil {
				t.Fatalf("Narrow() = nil, want a refusal")
			}
			if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
				t.Errorf("Narrow() = %v, want %v", err, tt.wantIs)
			}
			if tt.wantIs == nil && err.Error() != tt.wantErr {
				t.Errorf("Narrow() = %q, want %q", err, tt.wantErr)
			}
			if tt.wantIs != nil && !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Narrow() = %q, want it to say %q", err, tt.wantErr)
			}
			if w.log() != before || w.store.appends != appends {
				t.Errorf("the refused narrow changed the log:\n%s\nwas\n%s", w.log(), before)
			}
		})
	}
}
