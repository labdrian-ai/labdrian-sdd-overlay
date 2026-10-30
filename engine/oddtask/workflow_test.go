package oddtask_test

import (
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/oddtask"
)

// running is a view that passes the workflow check for the feature f.
func running() oddtask.WorkflowView {
	return oddtask.WorkflowView{
		BindingClass: "owned", Workflow: "f-t1", WorkflowClass: "owned", Status: "running",
		GoalID: "f", ChainID: "f-t1", Records: 2, Role: "builder",
	}
}

// S19 (a): the text of each reason, in the order the check gives them.
func TestDecideWorkflowGivesEachReasonItsText(t *testing.T) {
	cases := []struct {
		name string
		edit func(v *oddtask.WorkflowView)
		want oddtask.Reason
	}{
		{"allowed", func(*oddtask.WorkflowView) {}, ""},
		{"no binding", func(v *oddtask.WorkflowView) { *v = oddtask.WorkflowView{BindingClass: "absent"} },
			"the repository of /repo is bound to no workflow (labdrian workflow binding: absent)"},
		{"a binding that is not ours", func(v *oddtask.WorkflowView) { v.BindingClass = "foreign" },
			"the repository of /repo is bound to no workflow (labdrian workflow binding: foreign)"},
		{"a paused workflow", func(v *oddtask.WorkflowView) { v.Status = "paused" },
			"the workflow f-t1 bound to the repository is paused, not running"},
		{"a workflow that is not ours", func(v *oddtask.WorkflowView) { v.WorkflowClass, v.Status = "unavailable", "" },
			"the workflow f-t1 bound to the repository is unavailable, not running"},
		{"another goal", func(v *oddtask.WorkflowView) { v.GoalID = "g" },
			"the workflow f-t1 bound to the repository is of goal g, not of the feature f the prompt cites"},
		{"no role chain", func(v *oddtask.WorkflowView) { v.ChainID = "" },
			"the workflow f-t1 bound to the repository follows no role chain"},
		{"a chain that does not load", func(v *oddtask.WorkflowView) { v.ChainErr = "boom" },
			"the role chain f-t1 of the workflow f-t1 does not verify (boom)"},
		{"a chain with no records", func(v *oddtask.WorkflowView) { v.Records, v.Role = 0, "" },
			"the role chain f-t1 of the workflow f-t1 does not verify (it has no records)"},
		{"another role", func(v *oddtask.WorkflowView) { v.Role = "reviewer" },
			"the role chain f-t1 of the workflow f-t1 hands the work to reviewer, not to the builder"},
	}
	for _, tc := range cases {
		v := running()
		tc.edit(&v)
		if got := oddtask.DecideWorkflow("/repo", "f", v); got != tc.want {
			t.Errorf("%s: reason = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// S19 (a): the first failing check names the reason, whatever follows it.
func TestDecideWorkflowGivesTheFirstReasonOnly(t *testing.T) {
	v := oddtask.WorkflowView{BindingClass: "owned", Workflow: "f-t1", WorkflowClass: "owned", Status: "paused", GoalID: "g", Role: "reviewer"}
	want := oddtask.Reason("the workflow f-t1 bound to the repository is paused, not running")
	if got := oddtask.DecideWorkflow("/repo", "f", v); got != want {
		t.Fatalf("reason = %q, want %q", got, want)
	}
}

// S19 (c): the message, and the unreadable reason it also frames.
func TestWorkflowMessageIsTheExactTextOfS19(t *testing.T) {
	want := "labdrian builder guard: the workflow of the repository cannot be read (disk). Every feature runs inside its workflow: " +
		"run labdrian workflow open --feature f --task T1 (it records the shaper and estimator hops, then creates, starts and binds the workflow of the task), " +
		"then launch the builder again. This guard reads text and local state only, so it is a speed bump, not a security boundary."
	if got := oddtask.WorkflowMessage(oddtask.WorkflowUnreadableReason("disk"), "f", "T1"); got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
}

// S20 (b): the message of a chain that waits for a hop inside the right workflow.
func TestHopMessageIsTheExactTextOfS20(t *testing.T) {
	want := "labdrian builder guard: the role chain f-t1 of the workflow f-t1 hands the work to reviewer, not to the builder. " +
		"The repository is already inside the workflow f-t1 of this feature, so do not open another: append the hop its role chain f-t1 waits for, " +
		"then launch the builder again; after a clear review, the next subtask of the same task goes back to the builder " +
		"with a reviewer to builder hop that carries the review (skills/role-chain). " +
		"This guard reads text and local state only, so it is a speed bump, not a security boundary."
	r := oddtask.Reason("the role chain f-t1 of the workflow f-t1 hands the work to reviewer, not to the builder")
	if got := oddtask.HopMessage(r, "f-t1", "f-t1"); got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
}

// S20 (b): the hop remedy holds only when the role is the one reason that fails and the chain is not at delivery.
func TestWaitsForHopOnlyWhenTheChainIsTheOnlyReason(t *testing.T) {
	at := func(role string) oddtask.WorkflowView {
		v := running()
		v.Role = role
		return v
	}
	for _, role := range []string{"reviewer", "sweeper", "polisher", "estimator"} {
		if !oddtask.WaitsForHop("f", at(role)) {
			t.Errorf("role %s: WaitsForHop = false, want true", role)
		}
	}
	for _, role := range []string{"builder", "delivery", "", "bogus", "Reviewer"} {
		if oddtask.WaitsForHop("f", at(role)) {
			t.Errorf("role %s: WaitsForHop = true, want false", role)
		}
	}
	earlier := []struct {
		name string
		edit func(v *oddtask.WorkflowView)
	}{
		{"no binding", func(v *oddtask.WorkflowView) { v.BindingClass = "absent" }},
		{"a paused workflow", func(v *oddtask.WorkflowView) { v.Status = "paused" }},
		{"another goal", func(v *oddtask.WorkflowView) { v.GoalID = "g" }},
		{"no role chain", func(v *oddtask.WorkflowView) { v.ChainID = "" }},
		{"a chain error", func(v *oddtask.WorkflowView) { v.ChainErr = "boom" }},
		{"no records", func(v *oddtask.WorkflowView) { v.Records = 0 }},
	}
	for _, tc := range earlier {
		v := at("reviewer")
		tc.edit(&v)
		if oddtask.WaitsForHop("f", v) {
			t.Errorf("reviewer with %s: WaitsForHop = true, want false", tc.name)
		}
	}
}

func TestCitationNamesItsFeature(t *testing.T) {
	for _, path := range []string{"odd/tasks/f.md", "/repo/odd/tasks/f.md"} {
		if got := (oddtask.Citation{Path: path}).Feature(); got != "f" {
			t.Errorf("Feature(%q) = %q, want f", path, got)
		}
	}
}
