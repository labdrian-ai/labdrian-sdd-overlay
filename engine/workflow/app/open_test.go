package app_test

// Tests for the use case 'workflow open' (own-rdd S19 b) at the seams the CLI tests cannot reach: a stage
// that fails partway, and two opens racing for one chain. The rest of its behavior is pinned through the CLI
// (engine/cmd/workflow_open_test.go).

import (
	"errors"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/goal"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
	projapp "github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection/app"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/roles"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/roles/filechain"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow/app"
)

const openGoal = `{
  "version": 2, "project_id": "p", "goal_id": "f", "objective": "o", "scope": "s", "constraints": [],
  "non_goals": [], "acceptance_criteria": ["a"], "memory_scope": "goal", "runtime_scope": "r", "delivery_boundary": "d"
}`

const openDoc = "## Tasks\n\n- [ ] T1 — S1 — inline — x — shaped: L1 — commit: -\n\n## Log\n\n- **L1** (2026-10-29, shaper): design\n"

type fakeSources struct{ doc string }

func (fakeSources) Goal(string, string) ([]byte, error) { return []byte(openGoal), nil }
func (f fakeSources) Document(string) ([]byte, error)   { return []byte(f.doc), nil }

// fakeWorkflows records what Open asks of the workflow and fails the stage named failOn.
type fakeWorkflows struct {
	failOn string
	stages []string
}

func (fakeWorkflows) Create(string, string, string, goal.Goal, string, string) (workflow.State, error) {
	return workflow.State{}, nil
}
func (fakeWorkflows) Start(string, string) (workflow.State, error) { return workflow.State{}, nil }
func (f *fakeWorkflows) RecordStage(_, _, stage string) (workflow.State, error) {
	if stage == f.failOn {
		return workflow.State{}, errors.New("disk full")
	}
	f.stages = append(f.stages, stage)
	return workflow.State{}, nil
}
func (fakeWorkflows) Status(string, string) (workflow.Classification, workflow.State, error) {
	return workflow.ClassificationAbsent, workflow.State{}, nil
}

type fakeBinder struct{ binds int }

func (b *fakeBinder) Bind(projapp.BindRequest) (projection.Binding, error) {
	b.binds++
	return projection.Binding{}, nil
}

// racyChains hides the records of the chain from LoadChain, as a second open sees them in the window before
// the first one appends, and appends through the real store.
type racyChains struct{ roles.ChainLog }

func (racyChains) LoadChain(string, string, string) ([]roles.ChainRecord, error) { return nil, nil }

func newOpen(t *testing.T, chains roles.ChainLog, wf *fakeWorkflows, doc string) (app.Open, *fakeBinder) {
	t.Helper()
	binder := &fakeBinder{}
	return app.Open{
		Sources: fakeSources{doc: doc}, Chains: chains, Workflows: wf, Binder: binder,
		Identity: func(project string) string { return project },
	}, binder
}

// S19 (b): a stage that fails partway says which stages were recorded and which failed, after the steps done.
func TestOpenNamesTheStagesRecordedWhenOneFails(t *testing.T) {
	store, err := filechain.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	wf := &fakeWorkflows{failOn: "classify"}
	open, binder := newOpen(t, store, wf, openDoc)

	_, err = open.Open(app.OpenRequest{Feature: "f", Task: "T1", Dir: "/repo"})
	var step *app.StepError
	if !errors.As(err, &step) || step.Step != app.StepStages {
		t.Fatalf("Open error = %v, want a *StepError of step %q", err, app.StepStages)
	}
	want := "record the five stages failed (done: append the shaper record, append the estimator record, create, start): " +
		"stage classify failed after recording authorize, explore, resolve-uncertainty: disk full"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
	if binder.binds != 0 {
		t.Errorf("bind was called %d times after a stage failed, want none", binder.binds)
	}

	wf = &fakeWorkflows{failOn: "authorize"}
	open, _ = newOpen(t, filechainOf(t), wf, openDoc)
	_, err = open.Open(app.OpenRequest{Feature: "f", Task: "T1", Dir: "/repo"})
	if err == nil || !strings.Contains(err.Error(), "stage authorize failed after recording none: disk full") {
		t.Errorf("error = %v, want it to say no stage was recorded", err)
	}
}

func filechainOf(t *testing.T) roles.ChainLog {
	t.Helper()
	store, err := filechain.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return store
}

// S19 (b): two opens racing for one chain. The role-chain store publishes a record by hard link, so a second
// record at a seq that is taken is refused when its bytes differ and is a no-op when they are identical: the
// loser of a race with a different document appends nothing and nothing after the append runs.
func TestOpenLosesARaceForTheChainWhenTheFirstRecordIsTaken(t *testing.T) {
	store := filechainOf(t)
	first, _ := newOpen(t, store, &fakeWorkflows{}, openDoc)
	if _, err := first.Open(app.OpenRequest{Feature: "f", Task: "T1", Dir: "/repo"}); err != nil {
		t.Fatalf("first open: %v", err)
	}

	// The second open sees an empty chain (the window before the first appended) and a document that changed.
	wf := &fakeWorkflows{}
	second, binder := newOpen(t, racyChains{store}, wf, openDoc+"\nchanged\n")
	_, err := second.Open(app.OpenRequest{Feature: "f", Task: "T1", Dir: "/repo"})
	var step *app.StepError
	if !errors.As(err, &step) || step.Step != app.StepShaperRecord || len(step.Done) != 0 {
		t.Fatalf("second open error = %v, want a *StepError of the shaper record with no step done", err)
	}
	if len(wf.stages) != 0 || binder.binds != 0 {
		t.Errorf("the loser went on: stages %v, binds %d", wf.stages, binder.binds)
	}
	records, err := store.LoadChain("p", "f", "f-t1")
	if err != nil || len(records) != 2 {
		t.Fatalf("chain f-t1 after the race: %d records, err=%v, want the winner's 2", len(records), err)
	}

	// The same document again: the identical bytes are a no-op at both seqs, so a loser of that race runs on
	// to its own create, which the workflow log (not the chain store) then refuses when the workflow exists.
	third, _ := newOpen(t, racyChains{store}, &fakeWorkflows{}, openDoc)
	if _, err := third.Open(app.OpenRequest{Feature: "f", Task: "T1", Dir: "/repo"}); err != nil {
		t.Fatalf("an open of identical records: %v, want the store to accept identical bytes again", err)
	}
}
