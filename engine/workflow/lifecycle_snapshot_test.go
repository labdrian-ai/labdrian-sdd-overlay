package workflow

import (
	"errors"
	"reflect"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/goal"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflowprofile"
)

// retiredCatalog is a catalog from which the profile has gone.
var retiredCatalog = ProfileCatalogFunc(func(name string) (workflowprofile.WorkflowProfile, error) {
	return workflowprofile.WorkflowProfile{}, errors.New("profile retired")
})

// catalogOf is a catalog that gives profile for any name it is asked.
func catalogOf(profile workflowprofile.WorkflowProfile) ProfileCatalog {
	return ProfileCatalogFunc(func(string) (workflowprofile.WorkflowProfile, error) { return profile, nil })
}

// createVersion1Workflow puts a workflow of the profile into the log as the version of the program
// before the snapshot wrote it: a created event of version 1 that names the profile and nothing else.
func createVersion1Workflow(t *testing.T, log EventLog, projectID, workflowID string, g goal.Goal, profile string) {
	t.Helper()
	digest, err := goalDigest(g)
	if err != nil {
		t.Fatal(err)
	}
	created := WorkflowEvent{
		Version: EventVersionNameOnly, WorkflowID: workflowID, ProjectID: projectID, Kind: KindCreated,
		At: "2026-09-28T09:00:00Z", Provenance: testProvenance(), Observations: []Observation{},
		GoalID: g.GoalID, GoalDigest: digest, Profile: profile,
	}
	if err := log.Append(projectID, workflowID, created); err != nil {
		t.Fatalf("Append(created, version 1) = %v, want nil", err)
	}
}

// snapshotWorld is a log, a goal and a lifecycle over a catalog that may be changed under it.
type snapshotWorld struct {
	t     *testing.T
	store *memEventLog
	goals *fakeGoalReader
	g     goal.Goal
}

func newSnapshotWorld(t *testing.T) *snapshotWorld {
	t.Helper()
	w := &snapshotWorld{t: t, store: newMemEventLog(), goals: newFakeGoalReader(), g: validGoal("proj-1", "goal-1")}
	w.goals.set("proj-1", "goal-1", w.g)
	return w
}

// lifecycle is a lifecycle over the catalog; its clock runs on from the one it was last given.
func (w *snapshotWorld) lifecycle(catalog ProfileCatalog) Lifecycle {
	w.t.Helper()
	lc := newTestLifecycle(w.t, w.store, stepClock(), w.goals, newFakeChainReader(), UnavailableProber{})
	lc.profiles = catalog
	return lc
}

func (w *snapshotWorld) events() []WorkflowEvent {
	w.t.Helper()
	loaded, err := w.store.Load("proj-1", "wf-1")
	if err != nil {
		w.t.Fatal(err)
	}
	return loaded.Events
}

func (w *snapshotWorld) state() State {
	w.t.Helper()
	_, state, err := w.lifecycle(builtInProfiles).Status("proj-1", "wf-1")
	if err != nil {
		w.t.Fatal(err)
	}
	return state
}

func builtIn(t *testing.T, name string) workflowprofile.WorkflowProfile {
	t.Helper()
	p, err := workflowprofile.Resolve(name)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// A workflow is created in version 2 with a snapshot of the profile the catalog gave, whole, and the
// digest of it; the profile is recorded as it was, whatever the catalog says later.
func TestCreateRecordsTheSnapshotOfTheProfileTheCatalogGave(t *testing.T) {
	for _, name := range []string{"odd", "sdd", "standalone-minimal", "maintenance", "incident-recovery"} {
		t.Run(name, func(t *testing.T) {
			w := newSnapshotWorld(t)
			profile := builtIn(t, name)
			if _, err := w.lifecycle(builtInProfiles).Create("proj-1", "wf-1", w.g, name, ""); err != nil {
				t.Fatalf("Create() = %v, want nil", err)
			}
			created := w.events()[0]
			if created.Version != EventVersionSnapshot {
				t.Errorf("version = %d, want %d", created.Version, EventVersionSnapshot)
			}
			if created.ProfileSnapshot == nil || !reflect.DeepEqual(*created.ProfileSnapshot, profile.Snapshot()) {
				t.Errorf("profile_snapshot = %+v, want %+v", created.ProfileSnapshot, profile.Snapshot())
			}
			if want := digestOf(t, profile.Snapshot()); created.ProfileDigest != want {
				t.Errorf("profile_digest = %s, want %s", created.ProfileDigest, want)
			}
			if created.Profile != name {
				t.Errorf("profile = %q, want %q", created.Profile, name)
			}
		})
	}
}

// What is recorded is what the catalog returned, so a catalog that gives a profile of its own is
// recorded as it gave it.
func TestCreateRecordsTheProfileTheCatalogGaveWhateverItDeclares(t *testing.T) {
	w := newSnapshotWorld(t)
	custom := builtIn(t, "maintenance")
	custom.Stages = append([]workflowprofile.Stage{{Name: "custom-first"}}, custom.Stages...)
	custom.Stages[1].DependsOn = []string{"custom-first"}
	custom.MemoryDefault.Sources = []workflowprofile.MemorySource{workflowprofile.MemorySourceEngram}
	custom.ReliesOnGentleReview = false
	if _, err := w.lifecycle(catalogOf(custom)).Create("proj-1", "wf-1", w.g, "maintenance", ""); err != nil {
		t.Fatalf("Create() = %v, want nil", err)
	}
	if got := w.events()[0].ProfileSnapshot; got == nil || !reflect.DeepEqual(*got, custom.Snapshot()) {
		t.Errorf("profile_snapshot = %+v, want %+v", got, custom.Snapshot())
	}
}

func TestCreateRefusesACatalogThatGivesAProfileOfAnotherName(t *testing.T) {
	w := newSnapshotWorld(t)
	_, err := w.lifecycle(catalogOf(builtIn(t, "sdd"))).Create("proj-1", "wf-1", w.g, "odd", "")
	if err == nil {
		t.Fatal("Create() = nil, want an error: the catalog gave sdd for odd")
	}
	if n := len(w.events()); n != 0 {
		t.Errorf("%d events were appended, want none", n)
	}
}

func TestCreateAppendsNothingWhenTheProfileTheCatalogGaveIsMalformed(t *testing.T) {
	w := newSnapshotWorld(t)
	malformed := builtIn(t, "odd")
	malformed.Roles = nil
	if _, err := w.lifecycle(catalogOf(malformed)).Create("proj-1", "wf-1", w.g, "odd", ""); err == nil {
		t.Fatal("Create() = nil, want an error")
	}
	// The append is attempted and refused (the store counts attempts), so what must be none is what
	// the log holds.
	if n := len(w.events()); n != 0 {
		t.Errorf("the log holds %d events, want none", n)
	}
	if w.store.appends == 0 {
		t.Error("no append was attempted, so the refusal was not the created event's own validation")
	}
}

// A workflow of version 2 goes on from its snapshot: the catalog is not asked for its profile, so a
// catalog that has dropped it, changed it or replaced it makes no difference to any verb.
func TestAVersion2WorkflowGoesOnFromItsSnapshotWhateverTheCatalogSays(t *testing.T) {
	changed := builtIn(t, "maintenance")
	changed.Stages = []workflowprofile.Stage{{Name: "only-stage"}}
	changed.Checks = []string{"other"}
	changed.MemoryDefault = workflowprofile.MemoryDefault{Scope: workflowprofile.MemoryScopeNone, Sources: []workflowprofile.MemorySource{}}
	changed.ReliesOnGentleReview = false
	for name, catalogFor := range map[string]func(t *testing.T) ProfileCatalog{
		"a catalog that has retired the profile": func(*testing.T) ProfileCatalog { return retiredCatalog },
		"a catalog that has changed the profile": func(*testing.T) ProfileCatalog { return catalogOf(changed) },
		"a catalog that gives another profile":   func(t *testing.T) ProfileCatalog { return catalogOf(builtIn(t, "odd")) },
		"the catalog it was created from":        func(*testing.T) ProfileCatalog { return builtInProfiles },
		"a catalog that is asked for nothing": func(t *testing.T) ProfileCatalog {
			return ProfileCatalogFunc(func(n string) (workflowprofile.WorkflowProfile, error) {
				t.Errorf("the catalog was asked for %q", n)
				return workflowprofile.WorkflowProfile{}, errors.New("not to be asked")
			})
		},
	} {
		t.Run(name, func(t *testing.T) {
			w := newSnapshotWorld(t)
			if _, err := w.lifecycle(builtInProfiles).Create("proj-1", "wf-1", w.g, "maintenance", ""); err != nil {
				t.Fatalf("Create() = %v, want nil", err)
			}
			lc := w.lifecycle(catalogFor(t))
			if _, err := lc.Start("proj-1", "wf-1"); err != nil {
				t.Fatalf("Start() = %v, want nil", err)
			}
			declared := builtIn(t, "maintenance").Stages
			for i, stage := range declared {
				// A stage the snapshot does not declare next is refused.
				if _, err := lc.RecordStage("proj-1", "wf-1", "only-stage"); !errors.Is(err, ErrStageOutOfOrder) {
					t.Fatalf("stage %d: RecordStage(only-stage) = %v, want ErrStageOutOfOrder", i, err)
				}
				if _, err := lc.RecordStage("proj-1", "wf-1", stage.Name); err != nil {
					t.Fatalf("RecordStage(%q) = %v, want nil: the snapshot declares it next", stage.Name, err)
				}
			}
			if _, err := lc.RecordStage("proj-1", "wf-1", declared[0].Name); !errors.Is(err, ErrStageOutOfOrder) {
				t.Fatalf("RecordStage() past the last stage = %v, want ErrStageOutOfOrder", err)
			}
			if _, err := lc.Pause("proj-1", "wf-1"); err != nil {
				t.Fatalf("Pause() = %v, want nil", err)
			}
			if _, err := lc.Resume("proj-1", "wf-1"); err != nil {
				t.Fatalf("Resume() = %v, want nil", err)
			}
			state, err := lc.Verify("proj-1", "wf-1")
			if err != nil {
				t.Fatalf("Verify() = %v, want nil", err)
			}
			if state.LastVerifiedSeq < 0 {
				t.Errorf("LastVerifiedSeq = %d, want a verified event", state.LastVerifiedSeq)
			}
			if _, err := lc.Close("proj-1", "wf-1", OutcomeCompleted, ""); err != nil {
				t.Fatalf("Close(completed) = %v, want nil", err)
			}
		})
	}
}

// The dependencies a version 2 event records are those of the snapshot, not the catalog's.
func TestAVersion2WorkflowObservesTheDependenciesOfItsSnapshot(t *testing.T) {
	w := newSnapshotWorld(t)
	if _, err := w.lifecycle(builtInProfiles).Create("proj-1", "wf-1", w.g, "maintenance", ""); err != nil {
		t.Fatal(err)
	}
	none := builtIn(t, "maintenance")
	none.MemoryDefault = workflowprofile.MemoryDefault{Scope: workflowprofile.MemoryScopeNone, Sources: []workflowprofile.MemorySource{}}
	none.ReliesOnGentleReview = false
	lc := w.lifecycle(catalogOf(none))
	if _, err := lc.Start("proj-1", "wf-1"); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, o := range w.events()[1].Observations {
		got = append(got, o.Capability)
	}
	want := []string{"memory:engram", "memory:longterm-mem", "memory:procedural-skills", gentleAIReviewCapability}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("observed %v, want %v: those of the snapshot", got, want)
	}
}

// An abandoned close needs no fallback in version 2: the profile is in the log, so the close records
// the dependencies as any other event does.
func TestAnAbandonedCloseOfAVersion2WorkflowObservesItsDependenciesWhenTheCatalogHasRetiredTheProfile(t *testing.T) {
	w := newSnapshotWorld(t)
	if _, err := w.lifecycle(builtInProfiles).Create("proj-1", "wf-1", w.g, "odd", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := w.lifecycle(retiredCatalog).Close("proj-1", "wf-1", OutcomeAbandoned, "done with it"); err != nil {
		t.Fatalf("Close(abandoned) = %v, want nil", err)
	}
	closed := w.events()[1]
	for _, o := range closed.Observations {
		if o.Capability == "profile" {
			t.Errorf("observation %+v: a version 2 close has no profile observation to make", o)
		}
	}
	if len(closed.Observations) != 4 {
		t.Errorf("observations = %+v, want the four dependencies of odd", closed.Observations)
	}
}

// Verify of a version 2 workflow still checks that the stages recorded follow the order the snapshot
// declares: a log edited by hand to record a stage the profile does not have is not verified.
func TestVerifyOfAVersion2WorkflowChecksTheRecordedStagesAgainstTheSnapshot(t *testing.T) {
	w := newSnapshotWorld(t)
	created := withSnapshot(t, validCreatedEvent(), snapshotOfBuiltIn(t, "odd"))
	digest, err := goalDigest(w.g)
	if err != nil {
		t.Fatal(err)
	}
	created.GoalDigest, created.GoalID = digest, w.g.GoalID
	started := asV2(seq(KindStarted))
	stage := asV2(validPayloadEvent(KindStageRecorded))
	stage.Stage = "not-a-stage-of-odd"
	for _, e := range withDigestChain([]WorkflowEvent{created, started, stage}) {
		if err := w.store.Append("proj-1", "wf-1", e); err != nil {
			t.Fatalf("Append(%s) = %v, want nil", e.Kind, err)
		}
	}
	if _, err := w.lifecycle(builtInProfiles).Verify("proj-1", "wf-1"); !errors.Is(err, ErrStageOrderInvalid) {
		t.Fatalf("Verify() = %v, want ErrStageOrderInvalid", err)
	}
}

// A workflow of version 1 is carried on as it always was: its events are version 1, with no snapshot,
// and its profile is the catalog's, asked for by name each time.
func TestAVersion1WorkflowKeepsItsVersionAndResolvesItsProfileByName(t *testing.T) {
	w := newSnapshotWorld(t)
	createVersion1Workflow(t, w.store, "proj-1", "wf-1", w.g, "maintenance")
	lc := w.lifecycle(builtInProfiles)
	if _, err := lc.Start("proj-1", "wf-1"); err != nil {
		t.Fatal(err)
	}
	for _, stage := range builtIn(t, "maintenance").Stages {
		if _, err := lc.RecordStage("proj-1", "wf-1", stage.Name); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := lc.Verify("proj-1", "wf-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := lc.Close("proj-1", "wf-1", OutcomeCompleted, ""); err != nil {
		t.Fatal(err)
	}
	for i, e := range w.events() {
		if e.Version != EventVersionNameOnly || e.ProfileSnapshot != nil || e.ProfileDigest != "" {
			t.Errorf("event %d (%s): version %d, snapshot %v, digest %q, want version 1 and neither", i, e.Kind, e.Version, e.ProfileSnapshot, e.ProfileDigest)
		}
	}
	if state := w.state(); state.Version != EventVersionNameOnly || state.ProfileSnapshot != nil {
		t.Errorf("state = version %d, snapshot %v, want version 1 and none", state.Version, state.ProfileSnapshot)
	}
}
