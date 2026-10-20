package projection

// The projection: the pure decision of what a session is told about the
// workflow its repository is bound to. Everything here is a function of its
// arguments. Nothing reads a file, an environment variable, or the clock, and
// nothing keeps state between calls, so the same binding and the same workflow
// state give the same bytes whichever process, session, or prompt asks. That is
// what lets a new session, or a restarted one, be told exactly what the last
// one was told.
//
// The caller (the hook command in engine/cmd) does the reading: it loads the
// binding and the workflow, calls Project, removes the binding when Project
// asks for that, and says the result to the runtime (engine/hookwire). Nothing
// here knows the format that is spoken in.

import (
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/memoryscope"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflowprofile"
)

// ProjectionInput is everything Project decides from: the classification of the
// repository's binding as the binding store reported it, and, only when the
// binding is owned, the workflow it names as the workflow store reported it. A
// nil Workflow with an owned binding means the workflow could not be loaded.
type ProjectionInput struct {
	Binding  Loaded
	Workflow *workflow.Loaded
}

// ProjectionResult is what Project decides.
//
// Context is text for the session, or empty. Warning is one line for the user,
// or empty. Unbind asks the caller to remove the repository's binding, which
// Project does only for a workflow it read and understood to be closed. The
// three are independent of the session and the prompt. When Unbind is set the
// caller tries the removal and then calls AfterUnbind, so the note says what
// really happened to the binding.
type ProjectionResult struct {
	Context string
	Warning string
	Unbind  bool

	// closedNote is the part of a closed workflow's Context that does not depend
	// on the removal, kept so that AfterUnbind can complete it.
	closedNote string
}

// Project decides what a session is told about the workflow its repository is
// bound to.
//
//   - No binding: nothing. An unbound repository is not the projection's
//     business, so it says nothing at all.
//   - A binding that is foreign, malformed, or unavailable: one warning, and
//     nothing is projected or removed. The store never touches a file that is
//     not ours, so neither does this.
//   - An owned binding to a workflow that is absent, foreign, malformed,
//     drifted, or unavailable, or that has a status this package does not know:
//     one warning, nothing is projected, and Unbind is never set. An unknown
//     state is not a closed workflow, and only a closed workflow is unbound.
//   - An owned binding to a closed workflow: a one-line context saying so, and
//     Unbind, so the next prompt is silent.
//   - An owned binding to a created, running, or paused workflow: the context,
//     bounded to MaxContextBytes.
//
// It is pure: it reads nothing but its argument, and it never fails.
func Project(in ProjectionInput) ProjectionResult {
	switch in.Binding.Classification {
	case ClassificationAbsent:
		return ProjectionResult{}
	case ClassificationOwned:
		return projectOwned(in.Binding.Binding, in.Workflow)
	default:
		return ProjectionResult{Warning: bindingWarning(in.Binding)}
	}
}

func projectOwned(b Binding, w *workflow.Loaded) ProjectionResult {
	if w == nil || w.Classification != workflow.ClassificationOwned {
		return ProjectionResult{Warning: workflowWarning(b, w)}
	}
	switch w.State.Status {
	case workflow.StatusClosed:
		note := closedNote(b, w.State)
		return ProjectionResult{Context: note, Unbind: true, closedNote: note}
	case workflow.StatusCreated, workflow.StatusRunning, workflow.StatusPaused:
		return ProjectionResult{Context: boundContext(buildContext(b, *w))}
	default:
		return ProjectionResult{Warning: statusWarning(b, w.State.Status)}
	}
}

// InstructionsResult is what Instructions decides: the memory instructions
// block to give a session or a subagent, or a warning that says why there is
// none. Both are empty for a repository nothing is bound to.
type InstructionsResult struct {
	Block   string
	Warning string
}

// Instructions decides whether the workflow a repository is bound to gives
// memory instructions (C23 M6): the block (InstructionsBlock) for a created,
// running or paused workflow of ours, nothing without a binding, and otherwise
// the warning Project gives for the same input, or the note of a closed
// workflow, which no longer has a plan to follow. It never asks for an unbind:
// only the prompt hook removes the binding of a closed workflow. engramName is
// the caller's Engram naming rule, as for the gate, and probe (C23 M8) the session-time probe of the
// plan's sources, nil for a caller that has none. It is pure: the probe is the caller's.
func Instructions(in ProjectionInput, engramName func(identity string) string, probe SourceProber) InstructionsResult {
	switch in.Binding.Classification {
	case ClassificationAbsent:
		return InstructionsResult{}
	case ClassificationOwned:
	default:
		return InstructionsResult{Warning: bindingWarning(in.Binding)}
	}
	b, w := in.Binding.Binding, in.Workflow
	if w == nil || w.Classification != workflow.ClassificationOwned {
		return InstructionsResult{Warning: workflowWarning(b, w)}
	}
	switch w.State.Status {
	case workflow.StatusClosed:
		return InstructionsResult{Warning: closedNote(b, w.State)}
	case workflow.StatusCreated, workflow.StatusRunning, workflow.StatusPaused:
		return InstructionsResult{Block: InstructionsBlock(b, w.State, engramName, probe)}
	default:
		return InstructionsResult{Warning: statusWarning(b, w.State.Status)}
	}
}

// builtInWorkflowProfile is the profile the workflow in state goes on from, the one the lifecycle
// goes on from: workflow.ProfileOf over the built-in catalog, that is the snapshot its log
// recorded, or, for a log that recorded only a name, the built-in profile of that name.
func builtInWorkflowProfile(state workflow.State) (workflowprofile.WorkflowProfile, error) {
	return workflow.ProfileOf(state, workflow.ProfileCatalogFunc(workflowprofile.Resolve))
}

// resolvePlan is the one place the projection computes the memory plan of a
// workflow: workflow.PlanOf over the workflow's profile, that is the profile's
// ceiling narrowed by the directives its log stored (C23 M3: the Goal's, the
// shaper handoff's, then the role directive in force), resolved for the project
// and goal. The context states this plan and the PreToolUse gate enforces it, so
// both call this function, and can never disagree about which project is
// permitted.
//
// The plan's project is the shared identity the workflow recorded at create
// (C23 M2: a declared name, a normalized origin remote or the main checkout's
// name), the name longterm-mem gives the project; projectID, the slug that keys
// the workflow's directory, stands in only for a workflow created before it,
// whose identity is unknown (and whose queries the gate does not check).
func resolvePlan(state workflow.State, projectID string) (memoryscope.Plan, error) {
	profile, err := builtInWorkflowProfile(state)
	if err != nil {
		return memoryscope.Plan{}, err
	}
	return workflow.PlanOf(state, projectID, profile)
}
