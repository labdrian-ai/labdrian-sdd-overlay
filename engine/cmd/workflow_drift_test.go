package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/goal"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflowprofile"
)

// Catalog drift, as the program reports it: a workflow whose log records a snapshot of its profile
// (version 2) is reported when the built-in profile of that name is no longer the recorded one.
// `workflow status` gains an optional profile_drift object (kind, profile, the two digests and the
// fields that differ); `workflow verify` prints one warning line on stderr and succeeds as before;
// no other verb and no hook says anything. A workflow of version 1 recorded a name and no profile,
// so it has nothing to drift from.

// driftWorld is a profileWorld that holds a version 2 workflow log whose snapshot is the profile
// given, made by hand (the built-in catalog cannot be changed from outside the program).
type driftWorld struct {
	*profileWorld
	goal string
}

// driftProfileChanged is the built-in odd profile with a check added and the review dependency
// dropped: what odd would be if it had been edited since the workflow was created.
func driftProfileChanged(t *testing.T) workflowprofile.WorkflowProfile {
	t.Helper()
	p, err := workflowprofile.Resolve("odd")
	if err != nil {
		t.Fatal(err)
	}
	p.Checks = append(p.Checks, "a check odd did not have")
	p.ReliesOnGentleReview = false
	return p
}

// driftProfileRetired is a profile that is in no catalog, standing for a built-in one that was removed.
func driftProfileRetired() workflowprofile.WorkflowProfile {
	return workflowprofile.WorkflowProfile{
		Name:           "retired-profile",
		Stages:         []workflowprofile.Stage{{Name: "first", DependsOn: []string{}}, {Name: "second", DependsOn: []string{"first"}}},
		Roles:          []string{"role"},
		Checks:         []string{"check"},
		MemoryPolicy:   "memory",
		ReviewPolicy:   "review",
		DeliveryPolicy: "delivery",
		MemoryDefault:  workflowprofile.MemoryDefault{Scope: workflowprofile.MemoryScopeNone, Sources: []workflowprofile.MemorySource{}},
	}
}

// newDriftWorld makes a world holding the workflow proj-1/wf-1, created and started, whose created
// event records the snapshot of profile.
func newDriftWorld(t *testing.T, bin string, profile workflowprofile.WorkflowProfile) *driftWorld {
	t.Helper()
	w := &driftWorld{profileWorld: newProfileWorld(t, bin, false)}
	w.goal = writeMemoryTestFile(t, w.dir, "goal.json", memoryTestGoalJSON("proj-1", "goal-1"))
	w.places[w.goal] = "<GOAL>"
	data, err := os.ReadFile(w.goal)
	if err != nil {
		t.Fatal(err)
	}
	g, err := goal.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	marshaled, err := g.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	goalSum := sha256.Sum256(marshaled)
	snapshot := profile.Snapshot()
	digest, err := snapshot.Digest()
	if err != nil {
		t.Fatal(err)
	}
	created := workflow.WorkflowEvent{
		Version: workflow.EventVersionSnapshot, WorkflowID: "wf-1", ProjectID: "proj-1", Kind: workflow.KindCreated,
		At: "2026-10-10T09:00:00Z", Observations: []workflow.Observation{},
		GoalID: "goal-1", GoalDigest: hex.EncodeToString(goalSum[:]),
		Profile: profile.Name, ProfileSnapshot: &snapshot, ProfileDigest: digest,
	}
	started := workflow.WorkflowEvent{
		Version: workflow.EventVersionSnapshot, WorkflowID: "wf-1", ProjectID: "proj-1", Kind: workflow.KindStarted,
		Seq: 1, At: "2026-10-10T09:01:00Z", Observations: []workflow.Observation{},
	}
	started.PrevDigest, err = workflow.EventDigest(created)
	if err != nil {
		t.Fatal(err)
	}
	var log []byte
	for _, e := range []workflow.WorkflowEvent{created, started} {
		line, err := e.MarshalLine()
		if err != nil {
			t.Fatal(err)
		}
		log = append(log, line...)
	}
	w.put(w.workflowLogPath(), string(log), 0o600)
	return w
}

// verb runs a workflow verb on proj-1/wf-1 and returns its exit code and streams.
func (w *driftWorld) verb(t *testing.T, verb string, more ...string) (int, string, string) {
	t.Helper()
	return w.profileWorld.exec(append([]string{"workflow", verb, "--project", "proj-1", "--workflow", "wf-1"}, more...)...)
}

// statusFields decodes the JSON a status prints into its top-level fields.
func statusFields(t *testing.T, out string) map[string]json.RawMessage {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &fields); err != nil {
		t.Fatalf("status printed %q, which is not a JSON object: %v", out, err)
	}
	return fields
}

func TestStatusReportsAChangedProfileOfAVersion2Workflow(t *testing.T) {
	recorded := driftProfileChanged(t)
	current, _ := workflowprofile.Resolve("odd")
	w := newDriftWorld(t, engineBinary(t), recorded)
	code, out, errOut := w.verb(t, "status")
	if code != 0 || errOut != "" {
		t.Fatalf("status = exit %d, stderr %q, want 0 and nothing", code, errOut)
	}
	var drift struct {
		Kind           string   `json:"kind"`
		Profile        string   `json:"profile"`
		RecordedDigest string   `json:"recorded_digest"`
		CurrentDigest  string   `json:"current_digest"`
		Fields         []string `json:"fields"`
	}
	if err := json.Unmarshal(statusFields(t, out)["profile_drift"], &drift); err != nil {
		t.Fatalf("status = %s: no profile_drift object: %v", out, err)
	}
	recordedDigest, _ := recorded.Snapshot().Digest()
	currentDigest, _ := current.Snapshot().Digest()
	if drift.Kind != "changed" || drift.Profile != "odd" || drift.RecordedDigest != recordedDigest || drift.CurrentDigest != currentDigest ||
		!reflect.DeepEqual(drift.Fields, []string{"checks", "relies_on_gentle_review"}) {
		t.Errorf("profile_drift = %+v, want changed odd, recorded %s, current %s, fields checks and relies_on_gentle_review", drift, recordedDigest, currentDigest)
	}
}

func TestStatusReportsARetiredProfileOfAVersion2Workflow(t *testing.T) {
	retired := driftProfileRetired()
	w := newDriftWorld(t, engineBinary(t), retired)
	code, out, errOut := w.verb(t, "status")
	if code != 0 || errOut != "" {
		t.Fatalf("status = exit %d, stderr %q, want 0 and nothing", code, errOut)
	}
	drift := statusFields(t, out)["profile_drift"]
	digest, _ := retired.Snapshot().Digest()
	want := `{"kind":"retired","profile":"retired-profile","recorded_digest":"` + digest + `"}`
	var compact bytes.Buffer
	if err := json.Compact(&compact, drift); err != nil {
		t.Fatal(err)
	}
	if compact.String() != want {
		t.Errorf("profile_drift = %s, want %s", compact.String(), want)
	}
}

// Without drift a status has no profile_drift field at all, so every status that was printed before
// keeps its bytes: a version 2 workflow made by the program, and the version 1 logs of every profile.
func TestStatusWithoutDriftHasNoProfileDriftField(t *testing.T) {
	bin := engineBinary(t)
	t.Run("a version 2 workflow made by the program", func(t *testing.T) {
		for _, p := range profileStages {
			w := newProfileWorld(t, bin, false)
			goalFile := writeMemoryTestFile(t, w.dir, "goal.json", memoryTestGoalJSON("proj-1", "goal-1"))
			flags := []string{"--project", "proj-1", "--workflow", "wf-1"}
			if code, _, errOut := w.exec(append([]string{"workflow", "create", "--goal", goalFile, "--profile", p.profile}, flags...)...); code != 0 {
				t.Fatalf("%s: create = exit %d, %s", p.profile, code, errOut)
			}
			code, out, errOut := w.exec(append([]string{"workflow", "status"}, flags...)...)
			if code != 0 || errOut != "" || strings.Contains(out, "drift") {
				t.Errorf("%s: status = exit %d, stderr %q, stdout %s, want 0, nothing and no drift", p.profile, code, errOut, out)
			}
		}
	})
	t.Run("the version 1 logs", func(t *testing.T) {
		for _, p := range profileStages {
			for _, shape := range []string{"created", "running"} {
				w := newProfileWorld(t, bin, false)
				installV1Log(t, w.state, p.profile, shape)
				code, out, errOut := w.exec("workflow", "status", "--project", "proj-1", "--workflow", "wf-1")
				if code != 0 || errOut != "" || strings.Contains(out, "drift") {
					t.Errorf("%s %s: status = exit %d, stderr %q, stdout %s, want 0, nothing and no drift", p.profile, shape, code, errOut, out)
				}
			}
		}
	})
}

// verify warns once, on stderr, and succeeds as it did: the state it prints has no drift field
// (that is status's), and the workflow goes on from its snapshot.
func TestVerifyOfADriftedWorkflowWarnsOnStderrAndSucceeds(t *testing.T) {
	bin := engineBinary(t)
	for name, profile := range map[string]workflowprofile.WorkflowProfile{"changed": driftProfileChanged(t), "retired": driftProfileRetired()} {
		t.Run(name, func(t *testing.T) {
			w := newDriftWorld(t, bin, profile)
			code, out, errOut := w.verb(t, "verify", "--goal", w.goal)
			if code != 0 {
				t.Fatalf("verify = exit %d, stderr %q, want 0", code, errOut)
			}
			lines := strings.Split(strings.TrimSuffix(errOut, "\n"), "\n")
			if len(lines) != 1 || !strings.HasPrefix(lines[0], "warning: workflow verify: profile drift: ") ||
				!strings.Contains(lines[0], `"`+profile.Name+`"`) || !strings.Contains(lines[0], "("+name) {
				t.Errorf("stderr = %q, want one warning line naming the profile and the kind (%s)", errOut, name)
			}
			if strings.Contains(out, "drift") {
				t.Errorf("verify printed %s: the drift field is status's", out)
			}
			if _, status, _ := w.verb(t, "status"); !strings.Contains(status, `"last_verified_seq": 2`) {
				t.Errorf("status after verify = %s, want the verified event recorded at seq 2", status)
			}
		})
	}
}

// Nothing else says anything: not the verbs that move the workflow, and not a verify without drift.
func TestOnlyVerifyWarnsOfDriftAndOnlyWhenThereIsSome(t *testing.T) {
	bin := engineBinary(t)
	w := newDriftWorld(t, bin, driftProfileRetired())
	for _, step := range [][]string{{"pause"}, {"resume"}, {"stage", "--stage", "first"}} {
		if code, _, errOut := w.verb(t, step[0], step[1:]...); code != 0 || errOut != "" {
			t.Errorf("%v on a drifted workflow = exit %d, stderr %q, want 0 and nothing", step, code, errOut)
		}
	}
	clean := newProfileWorld(t, bin, false)
	goalFile := writeMemoryTestFile(t, clean.dir, "goal.json", memoryTestGoalJSON("proj-1", "goal-1"))
	flags := []string{"--project", "proj-1", "--workflow", "wf-1"}
	clean.exec(append([]string{"workflow", "create", "--goal", goalFile, "--profile", "odd"}, flags...)...)
	if code, _, errOut := clean.exec(append([]string{"workflow", "verify", "--goal", goalFile}, flags...)...); code != 0 || errOut != "" {
		t.Errorf("verify without drift = exit %d, stderr %q, want 0 and nothing", code, errOut)
	}
	cleanV1 := newProfileWorld(t, bin, false)
	installV1Log(t, cleanV1.state, "odd", "running")
	g := writeMemoryTestFile(t, cleanV1.dir, "goal.json", memoryTestGoalJSON("proj-1", "goal-1"))
	if code, _, errOut := cleanV1.exec(append([]string{"workflow", "verify", "--goal", g}, flags...)...); code != 0 || errOut != "" {
		t.Errorf("verify of a version 1 workflow = exit %d, stderr %q, want 0 and nothing", code, errOut)
	}
}
