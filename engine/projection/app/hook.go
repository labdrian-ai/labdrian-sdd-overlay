package app

import "github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"

// HookService is the use case of the projection hook: what the session is told about the workflow
// its repository follows when a prompt is submitted, and whether a tool call is allowed while the
// workflow is paused. It decodes no input and encodes no answer: the command reads the hook's JSON
// into the requests below and writes the outcomes as the hook's JSON.
//
// It never fails. Whatever it cannot do ends in an outcome that says nothing, or in one warning,
// because a hook that fails must not fail the prompt or the tool call it serves.
type HookService struct {
	// Repositories finds the repository a directory belongs to.
	Repositories projection.RepoLocator
	// Bindings keeps the binding of each repository.
	Bindings projection.BindingStore
	// Workflows reads the workflow a binding names.
	Workflows WorkflowReader
	// EditTools are the names of the tools that edit a file. They decide which tools the gate looks
	// at (projection.GateRelevant: these and a longterm-mem query; any other tool is answered
	// without reading either store) and which it denies while the workflow is paused. The caller
	// says which; with none, the gate denies no edit.
	EditTools []string
	// Project and Gate are the policies the service asks. They are fields so that a test can wrap
	// them. A nil one is the domain's (projection.Project, projection.Gate): the service falls
	// back to it in its own methods project and gate, so the zero value of the field is the
	// production policy.
	Project func(projection.ProjectionInput) projection.ProjectionResult
	Gate    func(projection.GateInput) projection.GateResult
}

// PromptRequest is a prompt that was submitted: the working directory the hook input names, or ""
// when it names none, and the working directory of the hook process, used in its place.
type PromptRequest struct {
	InputDir, ProcessDir string
}

// PromptKind is what the hook says in answer to a prompt.
type PromptKind int

const (
	// PromptSilent: there is nothing to say. The repository is not bound, or the hook has no
	// repository to look in.
	PromptSilent PromptKind = iota
	// PromptWarning: one warning for the user and nothing projected, because the binding store
	// could not be opened or read and so the hook cannot tell whether the repository is bound.
	PromptWarning
	// PromptProjection: the domain's decision, with the context to put into the session and, for
	// a closed workflow, the note of what became of the binding.
	PromptProjection
)

// PromptOutcome is the answer to a prompt. Warning is set for PromptWarning, Result for
// PromptProjection.
type PromptOutcome struct {
	Kind    PromptKind
	Warning string
	Result  projection.ProjectionResult
}

// OnPrompt decides what to tell the session when a prompt is submitted. It reads the binding of
// the repository and, when the binding is owned, the workflow it names, and asks the domain what to
// project. It reads and never writes, with one exception: when the workflow is closed the domain
// asks for the binding to be removed, and the service removes it, best effort, only if it is still
// the binding it read (BindingStore.UnbindIfUnchanged), so a fresh binding made since stays. What
// the removal did goes into the note.
func (s HookService) OnPrompt(req PromptRequest) PromptOutcome {
	repoKey, ok := s.Repositories.RepoKey(directoryOf(req.InputDir, req.ProcessDir))
	if !ok {
		return PromptOutcome{Kind: PromptSilent}
	}
	binding, err := s.Bindings.Load(repoKey)
	if err != nil {
		return PromptOutcome{Kind: PromptWarning, Warning: projection.StoreWarning(err)}
	}

	input := projection.ProjectionInput{Binding: binding}
	if binding.Classification == projection.ClassificationOwned {
		w := s.Workflows.Load(binding.Binding.ProjectID, binding.Binding.WorkflowID)
		input.Workflow = &w
	}
	result := s.project(input)
	if result.Unbind && binding.Classification == projection.ClassificationOwned {
		// Only a binding of ours that was read is removed: the domain asks for a removal only for
		// one, and a policy that asks for it for another has read nothing to remove.
		//
		// The removal is best effort and compares first: a fresh binding made since the read stays,
		// and a failure to remove this one is not the prompt's problem (the next prompt sees the
		// closed workflow again and retries). What happened goes into the note, so it never claims
		// a removal that did not take place.
		removed, unbindErr := s.Bindings.UnbindIfUnchanged(repoKey, binding.Binding)
		result = result.AfterUnbind(removed, unbindErr)
	}
	return PromptOutcome{Kind: PromptProjection, Result: result}
}

// ToolCallRequest is a tool call about to run: the working directory the hook input names, or ""
// when it names none, the working directory of the hook process, used in its place, and the call.
type ToolCallRequest struct {
	InputDir, ProcessDir string
	Call                 projection.ToolCall
}

// ToolCallOutcome is the answer to a tool call. Decided is false when the gate has nothing to say:
// the tool is not one it checks, the hook has no repository to look in, or the binding or the
// workflow cannot be followed. The gate runs on every tool call and the prompt hook already warns
// about those, so it does not repeat the warning. Result is the domain's decision otherwise.
type ToolCallOutcome struct {
	Decided bool
	Result  projection.GateResult
}

// OnToolCall decides whether a tool call is allowed. It answers for every tool the gate never
// checks without reading the binding or the workflow: only a file-edit tool or a longterm-mem
// query needs them. It is strictly read-only.
func (s HookService) OnToolCall(req ToolCallRequest) ToolCallOutcome {
	if !projection.GateRelevant(s.EditTools, req.Call.Name) {
		return ToolCallOutcome{}
	}
	repoKey, ok := s.Repositories.RepoKey(directoryOf(req.InputDir, req.ProcessDir))
	if !ok {
		return ToolCallOutcome{}
	}
	binding, err := s.Bindings.Load(repoKey)
	if err != nil {
		return ToolCallOutcome{}
	}

	input := projection.GateInput{Binding: binding, EditTools: s.EditTools, Call: req.Call}
	if binding.Classification == projection.ClassificationOwned {
		w := s.Workflows.Load(binding.Binding.ProjectID, binding.Binding.WorkflowID)
		input.Workflow = &w
	}
	return ToolCallOutcome{Decided: true, Result: s.gate(input)}
}

// directoryOf is the directory the session works in: the one the hook input names when it names
// one, else the one the hook process runs in.
func directoryOf(inputDir, processDir string) string {
	if inputDir != "" {
		return inputDir
	}
	return processDir
}

func (s HookService) project(in projection.ProjectionInput) projection.ProjectionResult {
	if s.Project != nil {
		return s.Project(in)
	}
	return projection.Project(in)
}

func (s HookService) gate(in projection.GateInput) projection.GateResult {
	if s.Gate != nil {
		return s.Gate(in)
	}
	return projection.Gate(in)
}
