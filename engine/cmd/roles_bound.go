package main

// The adapter of the bound-chain rule of the role-chain verbs (labdrian-orchestrator-opencode S16 B5, T56): it
// reads what the checkout of the working directory is bound to with the one reading of the binding the builder
// guard uses (describeBinding) and hands the rule of engine/roles the workflow it found. It decides nothing.

import (
	"errors"
	"path"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/oddtask"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/oddtask/fsdoc"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
	projapp "github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection/app"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/roles"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

// boundWorkflowHere is the workflow the checkout of the working directory is bound to, in the form the rule of
// engine/roles reads: the zero value for a directory outside any repository and for every binding or workflow
// that is absent, foreign, malformed or not owned. The error is a binding that could not be read: a working
// directory that cannot be determined, a binding store that cannot be built or whose file is unavailable, or
// the log of an owned binding's workflow that is unavailable. The repository is resolved first, so outside any
// repository no binding is read.
func boundWorkflowHere(workingDir func() (string, error)) (roles.BoundWorkflow, error) {
	if workingDir == nil {
		return roles.BoundWorkflow{}, nil
	}
	cwd, err := workingDir()
	if err != nil {
		return roles.BoundWorkflow{}, err
	}
	if _, inRepository := newRepoLocator().RepoKey(cwd); !inRepository {
		return roles.BoundWorkflow{}, nil
	}
	described, err := describeBinding(cwd)
	var noRepository *projapp.NoRepositoryError
	if errors.As(err, &noRepository) {
		return roles.BoundWorkflow{}, nil
	}
	if err != nil {
		return roles.BoundWorkflow{}, err
	}
	if described.Binding.Classification == projection.ClassificationUnavailable {
		return roles.BoundWorkflow{}, errors.New(described.Binding.Detail)
	}
	if described.Binding.Classification != projection.ClassificationOwned || described.Workflow == nil {
		return roles.BoundWorkflow{}, nil
	}
	if described.Workflow.Classification == workflow.ClassificationUnavailable {
		return roles.BoundWorkflow{}, errors.New(described.Workflow.Detail)
	}
	if described.Workflow.Classification != workflow.ClassificationOwned {
		return roles.BoundWorkflow{}, nil
	}
	bound, state := described.Binding.Binding, described.Workflow.State
	return roles.BoundWorkflow{
		ID: bound.WorkflowID, Project: bound.ProjectID, Goal: state.GoalID, Chain: state.RoleChainID, Status: string(state.Status),
	}, nil
}

// hasFeatureDocument is whether goal names a feature of the checkout of the working directory: odd/tasks/<goal>.md
// at the worktree root, read through the documents port the builder guard reads with. It is false only when the
// goal is no feature name, or the read finds no file (oddtask.CauseNotFound) or no repository holds the working
// directory (oddtask.CauseNoRepository). Every other outcome is true, so the rule fails closed: a document
// read, a symbolic link, a file that is not regular or is too large, an open or read error, an error that is
// no ReadError, and a working directory that cannot be read.
func hasFeatureDocument(workingDir func() (string, error), goal string) bool {
	if workingDir == nil || !oddtask.ValidFeature(goal) {
		return false
	}
	cwd, err := workingDir()
	if err != nil {
		return true
	}
	_, err = fsdoc.Reader{Root: worktreeRoot}.Read(path.Join("odd", "tasks", goal+".md"), cwd)
	var refused *oddtask.ReadError
	if errors.As(err, &refused) {
		return refused.Cause != oddtask.CauseNotFound && refused.Cause != oddtask.CauseNoRepository
	}
	return true
}

// boundReadRefusal is the refusal of 'roles resume' and 'roles next' for a goal and chain, "" to go on. A binding
// that cannot be read refuses nothing here: the read verbs answer as they did.
func boundReadRefusal(workingDir func() (string, error), goal, chain string) string {
	bound, err := boundWorkflowHere(workingDir)
	if err != nil {
		return ""
	}
	return roles.BoundReadRefusal(bound, goal, chain)
}
