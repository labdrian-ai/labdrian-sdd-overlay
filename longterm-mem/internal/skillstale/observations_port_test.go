package skillstale_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/skillstale"
)

// fakeObservations is the lister port skillstale owns (ObservationLister), implemented without a database: the detector
// depends on the memory model and on that port, not on the store that serves them.
type fakeObservations struct {
	asked        []string
	observations []memory.Observation
	err          error
}

func (f *fakeObservations) ListObservations(project string) ([]memory.Observation, error) {
	f.asked = append(f.asked, project)
	return f.observations, f.err
}

func TestDetect_ReadsTheCandidatesThroughItsOwnPortForTheProjectItWasGiven(t *testing.T) {
	fixture := makeFixture(t)
	fake := &fakeObservations{observations: []memory.Observation{{
		ID:       1,
		Title:    "quiet-skill",
		Type:     "pattern",
		TopicKey: "procedural/candidates/repeated-success/quiet-skill",
		Content:  "**Status**: registered\n**LastObserved**: 2000-01-01T00:00:00Z\n",
	}}}

	findings, err := skillstale.Detect(skillstale.Config{
		ProjectRoot:  fixture.root,
		Project:      fixtureProject,
		Observations: fake,
		Now:          fixture.now,
		PathEnv:      fixture.pathEnv,
	})
	if err != nil {
		t.Fatalf("skillstale.Detect: %v", err)
	}

	if len(fake.asked) != 1 || fake.asked[0] != fixtureProject {
		t.Fatalf("the port was asked for %v, want exactly the configured project %q once", fake.asked, fixtureProject)
	}
	quiet := findingByID(t, findings, "quiet-skill")
	if quiet.Status != "registered" {
		t.Errorf("quiet-skill status = %q, want registered, read from the observation the port returned", quiet.Status)
	}
	if signal := requireSignal(t, quiet, skillstale.SignalQuiet); signal.Since != "2000-01-01T00:00:00Z" {
		t.Errorf("quiet signal = %+v, want the LastObserved of the observation the port returned", signal)
	}
}

func TestDetect_ReportsAFailureOfItsPortAndWritesNothingElse(t *testing.T) {
	fixture := makeFixture(t)
	broken := errors.New("memory store offline")

	_, err := skillstale.Detect(skillstale.Config{
		ProjectRoot:  fixture.root,
		Project:      fixtureProject,
		Observations: &fakeObservations{err: broken},
		Now:          fixture.now,
		PathEnv:      fixture.pathEnv,
	})
	if !errors.Is(err, broken) {
		t.Fatalf("Detect over a failing port = %v, want an error that wraps the port's own", err)
	}
	if !strings.Contains(err.Error(), "list candidate observations") {
		t.Errorf("error = %q, want it to say the candidate observations could not be listed", err)
	}
}

func TestDetect_RequiresAnObservationsPort(t *testing.T) {
	fixture := makeFixture(t)

	_, err := skillstale.Detect(skillstale.Config{
		ProjectRoot: fixture.root,
		Project:     fixtureProject,
		Now:         fixture.now,
		PathEnv:     fixture.pathEnv,
	})
	if err == nil || !strings.Contains(err.Error(), "observation lister is required") {
		t.Fatalf("Detect without a port = %v, want an error that says the observation lister is required", err)
	}
}

// A store that failed to open and was assigned anyway is a typed nil inside the interface, which a plain
// comparison with nil does not see: it must be refused in the detector's own words, not panic inside the store.
func TestDetect_RefusesAnObservationsPortHoldingANilPointer(t *testing.T) {
	fixture := makeFixture(t)
	var notThere *fakeObservations

	_, err := skillstale.Detect(skillstale.Config{
		ProjectRoot:  fixture.root,
		Project:      fixtureProject,
		Observations: notThere,
		Now:          fixture.now,
		PathEnv:      fixture.pathEnv,
	})
	if err == nil || !strings.Contains(err.Error(), "observation lister is required") {
		t.Fatalf("Detect over a nil pointer = %v, want the same refusal as for no port", err)
	}
}
