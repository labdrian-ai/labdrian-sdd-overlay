package roles

import "fmt"

// BoundWorkflow is the workflow a checkout is bound to, as far as the role-chain verbs need it: its id,
// its project, the goal and the role chain it names, and its status. The zero value is "no bound
// workflow". The caller fills it only from an owned binding of an owned workflow; every other state
// (no binding, a foreign or malformed one, an unreadable workflow) is the zero value.
type BoundWorkflow struct {
	ID, Project, Goal, Chain, Status string
}

// closedStatus is the status of a workflow that has ended (workflow.StatusClosed; this package does
// not import the workflow package).
const closedStatus = "closed"

// holds is whether the workflow binds the parent to its role chain: it is not closed and has one.
func (b BoundWorkflow) holds() bool {
	return b.ID != "" && b.Chain != "" && b.Status != closedStatus
}

// BoundAppendRefusal is the sentence that refuses an append of a record of project, goal and chain
// while the checkout is bound to an open workflow with a role chain, unless the record is of that
// workflow's project, goal and chain; "" when the append is allowed.
func BoundAppendRefusal(b BoundWorkflow, project, goal, chain string) string {
	if !b.holds() || (project == b.Project && goal == b.Goal && chain == b.Chain) {
		return ""
	}
	return fmt.Sprintf("this checkout is bound to workflow %s; append its hops with --project %s --goal %s --chain %s",
		b.ID, b.Project, b.Goal, b.Chain)
}

// FeatureEntryRefusal is the sentence that refuses the first record (seq 1) of the chain of a goal that is a
// feature of the checkout, whose document is odd/tasks/<goal>.md: such a chain starts with 'workflow open',
// which writes its first two records. "" for any other seq and for a goal with no document. The caller
// reads the disk and passes hasDocument; the rule does no I/O.
func FeatureEntryRefusal(seq int, goal string, hasDocument bool) string {
	if seq != 1 || !hasDocument {
		return ""
	}
	return fmt.Sprintf("odd/tasks/%s.md is a feature document, so its role chain starts with labdrian workflow open --feature %s --task <T>, "+
		"which writes its first two records; run it, then append the next hops with the --chain it prints", goal, goal)
}

// BoundReadRefusal is the sentence that refuses a read (resume or next) of the bound workflow's goal
// under a chain that is not its role chain; "" for any other goal, the workflow's own chain, or a
// workflow that does not hold the parent to a chain.
func BoundReadRefusal(b BoundWorkflow, goal, chain string) string {
	if !b.holds() || goal != b.Goal || chain == b.Chain {
		return ""
	}
	return fmt.Sprintf("this checkout is bound to workflow %s, whose role chain for goal %s is %s: use --chain %s",
		b.ID, b.Goal, b.Chain, b.Chain)
}
