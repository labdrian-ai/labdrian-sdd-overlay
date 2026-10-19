package workflow

import (
	"errors"
	"fmt"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/memoryscope"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflowprofile"
)

// The names PlanOf gives the narrowers in what it says.
const (
	goalNarrowerName    = "goal"
	handoffNarrowerName = "shaper handoff"
	roleNarrowerName    = "role handoff"
)

// namedDirective is one narrower of a workflow's plan with the name of what it came from.
type namedDirective struct {
	name      string
	directive *memoryscope.Directive
}

// PlanOf is the memory plan of the workflow in state, computed from its log alone (C23 M3): the
// ceiling of profile (memoryscope.DefaultFor; profile is the workflow's, see ProfileOf), narrowed
// first by the Goal's directive and then by the shaper handoff's, which make the base, and then by
// the role handoff's directive in force (State.RoleDirectiveInForce: the latest narrowed event's, or
// before any the created event's), applied over the base, so a later role can get back to the base
// and never past it. A narrower that is absent narrows nothing; a log of version 1 or 2 has none, and
// its plan is the ceiling.
//
// The plan's project is the shared identity the workflow recorded (C23 M2), or, for a workflow
// created before it, projectID, the slug that keys the workflow's directory; its goal is the
// workflow's goal. A narrower that widens is refused with its name and what it widens, for example
// "the role handoff memory directive widens the plan: adds source longterm-mem not present in the
// current scope". The lifecycle asks the same question before it writes a narrower, so a log it wrote
// is never refused here; the projection's context and memory gate both call it, so they never
// disagree.
func PlanOf(state State, projectID string, profile workflowprofile.WorkflowProfile) (memoryscope.Plan, error) {
	ceiling, err := memoryscope.DefaultFor(profile)
	if err != nil {
		return memoryscope.Plan{}, err
	}
	candidates := []namedDirective{
		{goalNarrowerName, state.GoalDirective},
		{handoffNarrowerName, state.HandoffDirective},
		{roleNarrowerName, state.RoleDirectiveInForce()},
	}
	var names []string
	var narrowers []memoryscope.Directive
	for _, c := range candidates {
		if c.directive == nil {
			continue
		}
		names = append(names, c.name)
		narrowers = append(narrowers, *c.directive)
	}
	project := state.ProjectIdentity
	if project == "" {
		project = projectID
	}
	plan, err := memoryscope.Resolve(ceiling, project, state.GoalID, narrowers...)
	var refused *memoryscope.NarrowerError
	if errors.As(err, &refused) {
		if refused.Widens {
			return memoryscope.Plan{}, fmt.Errorf("the %s memory directive widens the plan: %w", names[refused.Index], refused.Err)
		}
		return memoryscope.Plan{}, fmt.Errorf("the %s memory directive: %w", names[refused.Index], refused.Err)
	}
	return plan, err
}
