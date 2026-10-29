package oddtask

import (
	"fmt"
	"strings"
)

// The workflow check of the builder guard (own-rdd S19 a, c): after every check of S14 passes, a builder
// launch must be made inside the workflow the repository is bound to, which must be the running workflow of
// the cited feature whose role chain hands the work to the builder. The decision is pure: the caller reads the
// binding, the workflow and the chain and hands this package what it saw.

// BuilderRole is the role the chain must hand the work to for a builder to be launched.
const BuilderRole = "builder"

// The classifications and statuses the check tells apart.
const (
	// BindingOwned is the classification of a binding the engine wrote and can follow.
	BindingOwned = "owned"
	// WorkflowOwned is the classification of a workflow log the engine wrote and can follow.
	WorkflowOwned = "owned"
	// StatusRunning is the status of a workflow that is being worked on.
	StatusRunning = "running"
)

// WorkflowView is what the caller saw of the workflow of the repository of a session.
type WorkflowView struct {
	// BindingClass is the classification of the binding of the repository ("absent" when it has none).
	BindingClass string
	// Workflow is the id of the bound workflow; the fields below are meaningful only for an owned binding.
	Workflow string
	// WorkflowClass is the classification of the bound workflow, and Status its status when it is owned.
	WorkflowClass string
	Status        string
	// GoalID and ChainID are the goal and the role chain the workflow names at its creation.
	GoalID  string
	ChainID string
	// ChainErr is why the chain could not be loaded and verified, "" when it was. Records is how many records
	// the chain has, and Role the role of its latest handoff (roles.Resume), when it was loaded.
	ChainErr string
	Records  int
	Role     string
}

// Feature is the name of the feature document the citation names: its file name without ".md".
func (c Citation) Feature() string {
	name := c.Path[strings.LastIndex(c.Path, "/")+1:]
	return strings.TrimSuffix(name, ".md")
}

// WorkflowUnreadableReason refuses a launch when the binding, the workflow or the chain could not be read at
// all, as every failure of a builder launch is refused (S14 e).
func WorkflowUnreadableReason(cause string) Reason {
	return Reason(fmt.Sprintf("the workflow of the repository cannot be read (%s)", cause))
}

// DecideWorkflow says whether a builder may be launched in the session at cwd, citing the feature, given what
// was seen of its workflow. It returns the first reason of S19 (a), in order, or "" when the launch may go on.
func DecideWorkflow(cwd, feature string, v WorkflowView) Reason {
	switch {
	case v.BindingClass != BindingOwned:
		return Reason(fmt.Sprintf("the repository of %s is bound to no workflow (labdrian workflow binding: %s)", cwd, v.BindingClass))
	case v.WorkflowClass != WorkflowOwned || v.Status != StatusRunning:
		state := v.Status
		if v.WorkflowClass != WorkflowOwned {
			state = v.WorkflowClass
		}
		return Reason(fmt.Sprintf("the workflow %s bound to the repository is %s, not running", v.Workflow, state))
	case v.GoalID != feature:
		return Reason(fmt.Sprintf("the workflow %s bound to the repository is of goal %s, not of the feature %s the prompt cites", v.Workflow, v.GoalID, feature))
	case v.ChainID == "":
		return Reason(fmt.Sprintf("the workflow %s bound to the repository follows no role chain", v.Workflow))
	case v.ChainErr != "":
		return Reason(fmt.Sprintf("the role chain %s of the workflow %s does not verify (%s)", v.ChainID, v.Workflow, v.ChainErr))
	case v.Records == 0:
		return Reason(fmt.Sprintf("the role chain %s of the workflow %s does not verify (it has no records)", v.ChainID, v.Workflow))
	case v.Role != BuilderRole:
		return Reason(fmt.Sprintf("the role chain %s of the workflow %s hands the work to %s, not to the builder", v.ChainID, v.Workflow, v.Role))
	}
	return ""
}

// DeliveryRole is the terminal role: a chain handed to it has ended, so no hop can follow.
const DeliveryRole = "delivery"

// WaitsForHop says whether the only reason DecideWorkflow gives is the role of S19 (a) and the chain's role is not
// delivery (own-rdd S20 b): the repository is inside the right workflow and its chain waits for a hop, so the
// remedy is to append that hop and not to open another workflow.
func WaitsForHop(feature string, v WorkflowView) bool {
	if v.Role == BuilderRole || v.Role == DeliveryRole {
		return false
	}
	atBuilder := v
	atBuilder.Role = BuilderRole
	return DecideWorkflow("", feature, atBuilder) == ""
}

// HopMessage is the whole text of a denial for a chain that waits for a hop: the reason, and the remedy of
// appending the hop of the workflow's chain instead of opening another workflow.
func HopMessage(r Reason, workflow, chain string) string {
	return fmt.Sprintf("%s%s. The repository is already inside the workflow %s of this feature, so do not open another: "+
		"append the hop its role chain %s waits for, then launch the builder again; "+
		"after a clear review, the next subtask of the same task goes back to the builder "+
		"with a reviewer to builder hop that carries the review (skills/role-chain). "+
		"This guard reads text and local state only, so it is a speed bump, not a security boundary.",
		guardPrefix, string(r), workflow, chain)
}

// WorkflowMessage is the whole text of a denial for a reason of the workflow check: the reason, and the
// command that opens the workflow of the first task the prompt cites.
func WorkflowMessage(r Reason, feature, task string) string {
	return fmt.Sprintf("%s%s. Every feature runs inside its workflow: run labdrian workflow open --feature %s --task %s "+
		"(it records the shaper and estimator hops, then creates, starts and binds the workflow of the task), "+
		"then launch the builder again. This guard reads text and local state only, so it is a speed bump, not a security boundary.",
		guardPrefix, string(r), feature, task)
}
