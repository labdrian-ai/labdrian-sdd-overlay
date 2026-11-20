package main

// The adapter of the bound-chain rule of the role-chain verbs (labdrian-orchestrator-opencode S16 B5, T56): it
// reads what the checkout of the working directory is bound to with the one reading of the binding the builder
// guard uses (describeBinding) and hands the rule of engine/roles the workflow it found. It decides nothing.

import (
	"errors"

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

// boundReadRefusal is the refusal of 'roles resume' and 'roles next' for a goal and chain, "" to go on. A binding
// that cannot be read refuses nothing here: the read verbs answer as they did.
func boundReadRefusal(workingDir func() (string, error), goal, chain string) string {
	bound, err := boundWorkflowHere(workingDir)
	if err != nil {
		return ""
	}
	return roles.BoundReadRefusal(bound, goal, chain)
}
