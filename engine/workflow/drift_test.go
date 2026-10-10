package workflow

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflowprofile"
)

// stateOfSnapshot is the state of a workflow of version 2 whose profile is the snapshot.
func stateOfSnapshot(s workflowprofile.Snapshot) State {
	return State{Status: StatusRunning, Version: EventVersionSnapshot, Profile: s.Name, ProfileSnapshot: &s}
}

func TestAWorkflowWithoutASnapshotHasNothingToDriftFrom(t *testing.T) {
	// A workflow of version 1 is the catalog's profile of its name; there is no recorded profile for
	// the catalog to differ from, whatever the catalog says, and the catalog is not asked.
	asked := ProfileCatalogFunc(func(name string) (workflowprofile.WorkflowProfile, error) {
		t.Errorf("the catalog was asked for %q", name)
		return workflowprofile.WorkflowProfile{}, errors.New("not to be asked")
	})
	drift, err := DetectProfileDrift(State{Status: StatusRunning, Version: EventVersionNameOnly, Profile: "odd"}, asked)
	if err != nil || drift != nil {
		t.Errorf("DetectProfileDrift() = %+v, %v, want no finding", drift, err)
	}
}

func TestAWorkflowWhoseSnapshotIsTheCatalogsProfileHasNotDrifted(t *testing.T) {
	for _, name := range []string{"odd", "sdd", "standalone-minimal", "maintenance", "incident-recovery"} {
		t.Run(name, func(t *testing.T) {
			drift, err := DetectProfileDrift(stateOfSnapshot(snapshotOfBuiltIn(t, name)), builtInProfiles)
			if err != nil || drift != nil {
				t.Errorf("DetectProfileDrift() = %+v, %v, want no finding", drift, err)
			}
		})
	}
}

// The catalog has a profile of the recorded name, and it is not the recorded one: the finding names the
// profile, both digests and the fields that differ, in the order of the encoding.
func TestACatalogProfileThatDiffersFromTheSnapshotIsAChangedFinding(t *testing.T) {
	recorded := snapshotOfBuiltIn(t, "maintenance")
	for _, tc := range []struct {
		name string
		edit func(*workflowprofile.WorkflowProfile)
		want []string
	}{
		{"a stage", func(p *workflowprofile.WorkflowProfile) { p.Stages = p.Stages[:len(p.Stages)-1] }, []string{"stages"}},
		{"a check", func(p *workflowprofile.WorkflowProfile) { p.Checks = append(p.Checks, "a new check") }, []string{"checks"}},
		{"the memory default", func(p *workflowprofile.WorkflowProfile) {
			p.MemoryDefault = workflowprofile.MemoryDefault{Scope: workflowprofile.MemoryScopeNone, Sources: []workflowprofile.MemorySource{}}
		}, []string{"memory_default"}},
		{"the review dependency and a policy", func(p *workflowprofile.WorkflowProfile) {
			p.ReliesOnGentleReview = false
			p.DeliveryPolicy = "changed"
		}, []string{"delivery_policy", "relies_on_gentle_review"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			current := builtIn(t, "maintenance")
			tc.edit(&current)
			drift, err := DetectProfileDrift(stateOfSnapshot(recorded), catalogOf(current))
			if err != nil {
				t.Fatalf("DetectProfileDrift() = %v, want a finding", err)
			}
			want := &ProfileDrift{
				Kind:           ProfileChanged,
				Profile:        "maintenance",
				RecordedDigest: digestOf(t, recorded),
				CurrentDigest:  digestOf(t, current.Snapshot()),
				Fields:         tc.want,
			}
			if !reflect.DeepEqual(drift, want) {
				t.Errorf("DetectProfileDrift() = %+v, want %+v", drift, want)
			}
		})
	}
}

func TestACatalogThatHasNoSuchProfileIsARetiredFinding(t *testing.T) {
	recorded := snapshotOfBuiltIn(t, "odd")
	gone := ProfileCatalogFunc(func(name string) (workflowprofile.WorkflowProfile, error) {
		return workflowprofile.WorkflowProfile{}, fmt.Errorf("%w: %q", workflowprofile.ErrUnknownProfile, name)
	})
	drift, err := DetectProfileDrift(stateOfSnapshot(recorded), gone)
	if err != nil {
		t.Fatalf("DetectProfileDrift() = %v, want a finding", err)
	}
	want := &ProfileDrift{Kind: ProfileRetired, Profile: "odd", RecordedDigest: digestOf(t, recorded)}
	if !reflect.DeepEqual(drift, want) {
		t.Errorf("DetectProfileDrift() = %+v, want %+v", drift, want)
	}
}

// A catalog that fails for another reason has said nothing about the profile, so that is an error and
// not a finding: drift is never reported on a guess.
func TestACatalogThatFailsForAnotherReasonIsAnErrorAndNotAFinding(t *testing.T) {
	broken := errors.New("the catalog cannot be read")
	drift, err := DetectProfileDrift(stateOfSnapshot(snapshotOfBuiltIn(t, "odd")), ProfileCatalogFunc(func(string) (workflowprofile.WorkflowProfile, error) {
		return workflowprofile.WorkflowProfile{}, broken
	}))
	if !errors.Is(err, broken) || drift != nil {
		t.Errorf("DetectProfileDrift() = %+v, %v, want the catalog's error and no finding", drift, err)
	}
}

// The lifecycle reports the drift of a workflow it holds, and only reports it: nothing is appended, and
// a workflow that is not owned has no drift to report.
func TestLifecycleReportsTheProfileDriftOfAWorkflowAndAppendsNothing(t *testing.T) {
	w := newSnapshotWorld(t)
	if _, err := w.lifecycle(builtInProfiles).Create("proj-1", "wf-1", w.g, "sdd", ""); err != nil {
		t.Fatal(err)
	}
	appends := w.store.appends
	if drift, err := w.lifecycle(builtInProfiles).ProfileDrift("proj-1", "wf-1"); err != nil || drift != nil {
		t.Errorf("ProfileDrift() over the catalog it was created from = %+v, %v, want no finding", drift, err)
	}
	changed := builtIn(t, "sdd")
	changed.Roles = append(changed.Roles, "new role")
	drift, err := w.lifecycle(catalogOf(changed)).ProfileDrift("proj-1", "wf-1")
	if err != nil {
		t.Fatal(err)
	}
	if drift == nil || drift.Kind != ProfileChanged || !reflect.DeepEqual(drift.Fields, []string{"roles"}) {
		t.Errorf("ProfileDrift() = %+v, want a changed finding in roles", drift)
	}
	if w.store.appends != appends {
		t.Errorf("ProfileDrift() appended %d events, want none", w.store.appends-appends)
	}
	if _, err := w.lifecycle(builtInProfiles).ProfileDrift("proj-1", "no-such-workflow"); !errors.Is(err, ErrWorkflowNotOwned) {
		t.Errorf("ProfileDrift() of an absent workflow = %v, want ErrWorkflowNotOwned", err)
	}
}

// A drifted workflow goes on exactly as one that has not: the finding is a report, and changes no
// verb.
func TestADriftedWorkflowGoesOnFromItsSnapshot(t *testing.T) {
	w := newSnapshotWorld(t)
	if _, err := w.lifecycle(builtInProfiles).Create("proj-1", "wf-1", w.g, "standalone-minimal", ""); err != nil {
		t.Fatal(err)
	}
	lc := w.lifecycle(retiredCatalog)
	if drift, err := lc.ProfileDrift("proj-1", "wf-1"); err == nil {
		t.Fatalf("ProfileDrift() over a catalog that fails = %+v, nil, want its error", drift)
	}
	gone := ProfileCatalogFunc(func(name string) (workflowprofile.WorkflowProfile, error) {
		return workflowprofile.WorkflowProfile{}, fmt.Errorf("%w: %q", workflowprofile.ErrUnknownProfile, name)
	})
	lc = w.lifecycle(gone)
	if drift, err := lc.ProfileDrift("proj-1", "wf-1"); err != nil || drift == nil || drift.Kind != ProfileRetired {
		t.Fatalf("ProfileDrift() = %+v, %v, want a retired finding", drift, err)
	}
	if _, err := lc.Start("proj-1", "wf-1"); err != nil {
		t.Fatalf("Start() of a retired profile's workflow = %v, want nil", err)
	}
	if _, err := lc.RecordStage("proj-1", "wf-1", builtIn(t, "standalone-minimal").Stages[0].Name); err != nil {
		t.Fatalf("RecordStage() of a retired profile's workflow = %v, want nil", err)
	}
	if _, err := lc.Verify("proj-1", "wf-1"); err != nil {
		t.Fatalf("Verify() of a retired profile's workflow = %v, want nil", err)
	}
}
