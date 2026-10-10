package main

import (
	"testing"
)

// The golden files under testdata/workflow-drift-golden record what 'workflow status' and 'workflow
// verify' say of a workflow, with and without catalog drift: a version 2 workflow whose recorded odd
// profile differs from the built-in one (changed), one whose recorded profile is no longer built in
// (retired), a version 2 workflow made by the program (no drift), and a version 1 workflow (which
// recorded no profile, so has none to drift from). Each case is a status, a verify and a status
// again. They were recorded from the program as it is with the drift report (Phase 9 unit H33, the
// owner's decision of 2026-10-10), and the cases without drift show the bytes of a status and of a
// verify that did not report any. Rewrite them deliberately with
//
//	go test ./cmd -run TestWorkflowDriftGolden -update-profile-golden
//
// and read the diff before committing it.

// driftTranscript runs status, verify and status on proj-1/wf-1 in the world.
func driftTranscript(w *profileWorld, goal string) string {
	flags := []string{"--project", "proj-1", "--workflow", "wf-1"}
	with := func(verb string, more ...string) []string {
		return append(append([]string{"workflow", verb}, flags...), more...)
	}
	w.run(with("status")...)
	w.run(with("verify", "--goal", goal)...)
	w.run(with("status")...)
	return w.text()
}

func TestWorkflowDriftGolden(t *testing.T) {
	bin := engineBinary(t)
	cases := []struct {
		name  string
		world func(t *testing.T) (*profileWorld, string)
	}{
		{"a-changed-profile", func(t *testing.T) (*profileWorld, string) {
			w := newDriftWorld(t, bin, driftProfileChanged(t))
			return w.profileWorld, w.goal
		}},
		{"a-retired-profile", func(t *testing.T) (*profileWorld, string) {
			w := newDriftWorld(t, bin, driftProfileRetired())
			return w.profileWorld, w.goal
		}},
		{"a-version-2-workflow-without-drift", func(t *testing.T) (*profileWorld, string) {
			w := newProfileWorld(t, bin, false)
			goal := writeMemoryTestFile(t, w.dir, "goal.json", memoryTestGoalJSON("proj-1", "goal-1"))
			w.places[goal] = "<GOAL>"
			w.run("workflow", "create", "--project", "proj-1", "--workflow", "wf-1", "--goal", goal, "--profile", "odd")
			w.run("workflow", "start", "--project", "proj-1", "--workflow", "wf-1")
			return w, goal
		}},
		{"a-version-1-workflow", func(t *testing.T) (*profileWorld, string) {
			w := newProfileWorld(t, bin, false)
			goal := writeMemoryTestFile(t, w.dir, "goal.json", memoryTestGoalJSON("proj-1", "goal-1"))
			w.places[goal] = "<GOAL>"
			installV1Log(t, w.state, "odd", "running")
			return w, goal
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w, goal := c.world(t)
			checkProfileGolden(t, "workflow-drift-golden", c.name, driftTranscript(w, goal))
		})
	}
}
