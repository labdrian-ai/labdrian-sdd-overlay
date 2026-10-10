package main

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflowprofile"
)

// A workflow created by the program is a log of version 2: its created event records a snapshot of the
// built-in profile whole, and the digest of it, and every event after it, whatever the verb, is of
// version 2 and carries neither. The profile transcripts under testdata/profile-golden show that
// nothing the verbs print differs; this is what the log holds.
func TestWorkflowCreateWritesAVersion2LogOfTheProfileWhateverTheVerbs(t *testing.T) {
	bin := engineBinary(t)
	for _, p := range profileStages {
		t.Run(p.profile, func(t *testing.T) {
			w := newProfileWorld(t, bin, false)
			goal := writeMemoryTestFile(t, w.dir, "goal.json", memoryTestGoalJSON("proj-1", "goal-1"))
			flags := []string{"--project", "proj-1", "--workflow", "wf-1"}
			with := func(verb string, more ...string) []string {
				return append(append([]string{"workflow", verb}, flags...), more...)
			}
			w.run(with("create", "--goal", goal, "--profile", p.profile)...)
			w.run(with("start")...)
			w.run(with("pause")...)
			w.run(with("resume")...)
			for _, stage := range p.stages {
				w.run(with("stage", "--stage", stage)...)
			}
			w.run(with("verify", "--goal", goal)...)
			w.run(with("close", "--outcome", "completed")...)

			data, err := os.ReadFile(w.workflowLogPath())
			if err != nil {
				t.Fatal(err)
			}
			loaded := workflow.ClassifyLog("proj-1", "wf-1", data)
			if loaded.Classification != workflow.ClassificationOwned {
				t.Fatalf("the log is %q (%s), want owned", loaded.Classification, loaded.Detail)
			}
			// created, started, paused, resumed, the stages, verified and closed.
			if want := 4 + len(p.stages) + 2; len(loaded.Events) != want {
				t.Fatalf("the log has %d events, want %d", len(loaded.Events), want)
			}
			for i, e := range loaded.Events {
				if e.Version != workflow.EventVersionSnapshot {
					t.Errorf("event %d (%s) is of version %d, want %d", i, e.Kind, e.Version, workflow.EventVersionSnapshot)
				}
				if i > 0 && (e.ProfileSnapshot != nil || e.ProfileDigest != "") {
					t.Errorf("event %d (%s) carries the snapshot, which belongs to the created event alone", i, e.Kind)
				}
			}
			profile, err := workflowprofile.Resolve(p.profile)
			if err != nil {
				t.Fatal(err)
			}
			created := loaded.Events[0]
			if created.ProfileSnapshot == nil || !reflect.DeepEqual(*created.ProfileSnapshot, profile.Snapshot()) {
				t.Errorf("the created event records %+v, want the snapshot of the built-in %s", created.ProfileSnapshot, p.profile)
			}
			if digest, _ := profile.Snapshot().Digest(); created.ProfileDigest != digest {
				t.Errorf("the created event records the digest %q, want %q", created.ProfileDigest, digest)
			}
			// The line is the compact encoding the format has always had: one object per line, the
			// snapshot after the profile's name.
			first := strings.SplitN(string(data), "\n", 2)[0]
			var fields []string
			dec := json.NewDecoder(strings.NewReader(first))
			if _, err := dec.Token(); err != nil {
				t.Fatal(err)
			}
			for dec.More() {
				key, err := dec.Token()
				if err != nil {
					t.Fatal(err)
				}
				fields = append(fields, key.(string))
				var skip json.RawMessage
				if err := dec.Decode(&skip); err != nil {
					t.Fatal(err)
				}
			}
			want := []string{"version", "workflow_id", "project_id", "seq", "prev_digest", "kind", "at", "provenance", "observations", "goal_id", "goal_digest", "profile", "profile_snapshot", "profile_digest"}
			if !reflect.DeepEqual(fields, want) {
				t.Errorf("the created event's fields are %v, want %v", fields, want)
			}
		})
	}
}
