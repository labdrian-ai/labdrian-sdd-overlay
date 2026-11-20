package roles

import "testing"

// boundSample is an open workflow that follows a role chain: the case the rule applies to.
var boundSample = BoundWorkflow{ID: "f-t1", Project: "proj-1", Goal: "f", Chain: "f-t1", Status: "running"}

func TestBoundAppendRefusal(t *testing.T) {
	const refusal = "this checkout is bound to workflow f-t1; append its hops with --project proj-1 --goal f --chain f-t1"
	for _, c := range []struct {
		name                 string
		bound                BoundWorkflow
		project, goal, chain string
		want                 string
	}{
		{"the workflow's own chain", boundSample, "proj-1", "f", "f-t1", ""},
		{"the feature named as the chain", boundSample, "proj-1", "f", "f", refusal},
		{"another goal", boundSample, "proj-1", "g", "f-t1", refusal},
		{"another project", boundSample, "other", "f", "f-t1", refusal},
		{"a paused workflow still holds the parent to its chain", withStatus(boundSample, "paused"), "proj-1", "f", "x", refusal},
		{"a closed workflow", withStatus(boundSample, "closed"), "proj-1", "f", "x", ""},
		{"a workflow with no role chain", withChain(boundSample, ""), "proj-1", "f", "x", ""},
		{"no bound workflow", BoundWorkflow{}, "proj-1", "f", "x", ""},
	} {
		if got := BoundAppendRefusal(c.bound, c.project, c.goal, c.chain); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestFeatureChainRefusal(t *testing.T) {
	const refusal = "odd/tasks/f.md is a feature document: from the shaper on, its role chain is the one " +
		"labdrian workflow open --feature f --task <T> writes, with the shaper's and the estimator's records; run it, then append the next hops " +
		"with the --chain it prints (chain f takes only the prototyper's hops)"
	closed := withStatus(boundSample, "closed")
	for _, c := range []struct {
		name        string
		bound       BoundWorkflow
		goal, chain string
		from        Role
		hasDocument bool
		want        string
	}{
		{"a shaper hop, no workflow", BoundWorkflow{}, "f", "f", RoleShaper, true, refusal},
		{"an estimator hop, no workflow", BoundWorkflow{}, "f", "f", RoleEstimator, true, refusal},
		{"a builder hop, no workflow", BoundWorkflow{}, "f", "f", RoleBuilder, true, refusal},
		{"a reviewer hop, a closed workflow", closed, "f", "f", RoleReviewer, true, refusal},
		{"a prototyper hop", BoundWorkflow{}, "f", "f", RolePrototyper, true, ""},
		{"another chain of the feature", BoundWorkflow{}, "f", "f-t1", RoleShaper, true, ""},
		{"no document", BoundWorkflow{}, "f", "f", RoleShaper, false, ""},
		{"an open workflow, chain of the feature", boundSample, "f", "f", RoleShaper, true, ""},
		{"an open workflow, another chain", boundSample, "f", "f-t1", RoleShaper, true, ""},
	} {
		if got := FeatureChainRefusal(c.bound, c.goal, c.chain, c.from, c.hasDocument); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestBoundReadRefusal(t *testing.T) {
	const refusal = "this checkout is bound to workflow f-t1, whose role chain for goal f is f-t1: use --chain f-t1"
	for _, c := range []struct {
		name        string
		bound       BoundWorkflow
		goal, chain string
		want        string
	}{
		{"the workflow's own chain", boundSample, "f", "f-t1", ""},
		{"the workflow's goal under another chain", boundSample, "f", "f", refusal},
		{"another goal under another chain", boundSample, "g", "g", ""},
		{"another goal under the workflow's chain", boundSample, "g", "f-t1", ""},
		{"a closed workflow", withStatus(boundSample, "closed"), "f", "x", ""},
		{"a workflow with no role chain", withChain(boundSample, ""), "f", "x", ""},
		{"no bound workflow", BoundWorkflow{}, "f", "x", ""},
	} {
		if got := BoundReadRefusal(c.bound, c.goal, c.chain); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func withStatus(b BoundWorkflow, status string) BoundWorkflow { b.Status = status; return b }
func withChain(b BoundWorkflow, chain string) BoundWorkflow   { b.Chain = chain; return b }
