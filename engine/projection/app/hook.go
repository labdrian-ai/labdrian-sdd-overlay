package app

import (
	"errors"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
)

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
	// at (projection.GateRelevant: these and the memory reads; any other tool is answered
	// without reading either store) and which it denies while the workflow is paused. The caller
	// says which; with none, the gate denies no edit.
	EditTools []string
	// EngramName is how Engram names the project of a shared identity, which the gate's Engram
	// project check compares a read's project with. The caller says how (identity.EngramName in the
	// engine); with none, the gate checks no Engram project and says so.
	EngramName func(identity string) string
	// ProceduralSkills reads the procedural skills lock, which the service does only for a call that
	// can use a skill (projection.CanUseASkill: the Skill tool, or a Read of a SKILL.md) in a
	// repository bound to a workflow of ours. With none, the gate knows of no procedural skill.
	ProceduralSkills ProceduralSkillsReader
	// Evidence is where the evidence of a memory read that ran is appended (C23 M7), and Clock
	// dates it. With either missing, a read that would be recorded gives the warning of one that
	// could not be.
	Evidence projection.ReadEvidenceLog
	Clock    Clock
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
	repoKey, input, stop := s.readBound(req)
	if stop != nil {
		return *stop
	}
	binding := input.Binding
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

// readBound reads what the repository of req is bound to: the repository's key, and the binding
// and, when the binding is owned, the workflow it names, as the domain takes them. stop is the
// outcome to answer with instead when there is no repository to look in (silence) or the binding
// store cannot be read (its warning).
func (s HookService) readBound(req PromptRequest) (repoKey string, input projection.ProjectionInput, stop *PromptOutcome) {
	repoKey, ok := s.Repositories.RepoKey(directoryOf(req.InputDir, req.ProcessDir))
	if !ok {
		return "", input, &PromptOutcome{Kind: PromptSilent}
	}
	binding, err := s.Bindings.Load(repoKey)
	if err != nil {
		return "", input, &PromptOutcome{Kind: PromptWarning, Warning: projection.StoreWarning(err)}
	}
	input.Binding = binding
	if binding.Classification == projection.ClassificationOwned {
		w := s.Workflows.Load(binding.Binding.ProjectID, binding.Binding.WorkflowID)
		input.Workflow = &w
	}
	return repoKey, input, nil
}

// InstructionsOutcome is the answer to a request for the memory instructions: Kind as for a
// prompt (silence, the warning of a store that cannot be read, or the domain's decision), with the
// warning or Result set as for PromptOutcome.
type InstructionsOutcome struct {
	Kind    PromptKind
	Warning string
	Result  projection.InstructionsResult
}

// OnInstructions decides the memory instructions of the workflow the repository of req follows
// (C23 M6): it reads the binding and the workflow as OnPrompt does and asks the domain
// (projection.Instructions) with the service's Engram naming rule, the one the gate checks with. The
// session-start hook, the gate-task rewrite and 'memory plan --instructions' ask it. It is strictly
// read-only: the binding of a closed workflow is left for the prompt hook to remove.
func (s HookService) OnInstructions(req PromptRequest) InstructionsOutcome {
	_, input, stop := s.readBound(req)
	if stop != nil {
		return InstructionsOutcome{Kind: stop.Kind, Warning: stop.Warning}
	}
	return InstructionsOutcome{Kind: PromptProjection, Result: projection.Instructions(input, s.EngramName)}
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
// checks without reading the binding or the workflow: only a file-edit tool, a memory read, the
// Skill tool or a Read of a SKILL.md needs them, and only the last two need the procedural skills
// lock, which is read for them alone and only for a binding of ours. It is strictly read-only.
func (s HookService) OnToolCall(req ToolCallRequest) ToolCallOutcome {
	call := readOnce(req.Call)
	if !projection.GateRelevantCall(s.EditTools, call) {
		return ToolCallOutcome{}
	}
	dir := directoryOf(req.InputDir, req.ProcessDir)
	repoKey, ok := s.Repositories.RepoKey(dir)
	if !ok {
		return ToolCallOutcome{}
	}
	binding, err := s.Bindings.Load(repoKey)
	if err != nil {
		return ToolCallOutcome{}
	}

	input := projection.GateInput{Binding: binding, EditTools: s.EditTools, EngramName: s.EngramName, Call: call}
	if binding.Classification == projection.ClassificationOwned {
		w := s.Workflows.Load(binding.Binding.ProjectID, binding.Binding.WorkflowID)
		input.Workflow = &w
		if s.ProceduralSkills != nil && projection.CanUseASkill(call) {
			input.Procedural = s.ProceduralSkills.Load(dir)
		}
	}
	return ToolCallOutcome{Decided: true, Result: s.gate(input)}
}

// ToolResultRequest is a tool call that has run: the working directories as for a ToolCallRequest,
// the call, and what the runtime says of it (its id and session) and of its response (a digest and
// a length).
type ToolResultRequest struct {
	InputDir, ProcessDir string
	Call                 projection.ToolCall
	CallID, SessionID    string
	ResponseSHA256       string
	ResponseBytes        int64
}

// ToolResultOutcome is the answer to a call that has run: Warning is the one line the agent is given
// when the evidence of its read could not be written, and empty otherwise.
type ToolResultOutcome struct {
	Warning string
}

// OnToolResult records the evidence of a memory read that ran (C23 M7): the reads the gate checks,
// in a repository bound to an open workflow of ours (projection.Evidence), are appended to Evidence.
// Every other tool is answered without reading the binding or the workflow, and every case where
// the hook cannot follow the binding is silent, as for OnToolCall. Its one write is the evidence;
// one it cannot make is a warning, never a failure.
func (s HookService) OnToolResult(req ToolResultRequest) ToolResultOutcome {
	call := readOnce(req.Call)
	if !projection.GateRelevantCall(nil, call) {
		return ToolResultOutcome{}
	}
	dir := directoryOf(req.InputDir, req.ProcessDir)
	repoKey, ok := s.Repositories.RepoKey(dir)
	if !ok {
		return ToolResultOutcome{}
	}
	binding, err := s.Bindings.Load(repoKey)
	if err != nil || binding.Classification != projection.ClassificationOwned {
		return ToolResultOutcome{}
	}
	w := s.Workflows.Load(binding.Binding.ProjectID, binding.Binding.WorkflowID)
	input := projection.GateInput{Binding: binding, Workflow: &w, Call: call}
	if s.ProceduralSkills != nil && projection.CanUseASkill(call) {
		input.Procedural = s.ProceduralSkills.Load(dir)
	}
	ev, ok := projection.Evidence(input)
	if !ok {
		return ToolResultOutcome{}
	}
	if s.Evidence == nil || s.Clock == nil {
		return ToolResultOutcome{Warning: projection.EvidenceWarning(ev, errNoEvidenceLog)}
	}
	rec := projection.NewReadRecord(ev, s.Clock.Now(), req.CallID, req.SessionID, req.ResponseSHA256, req.ResponseBytes)
	if err := s.Evidence.Append(rec); err != nil {
		return ToolResultOutcome{Warning: projection.EvidenceWarning(ev, err)}
	}
	return ToolResultOutcome{}
}

var errNoEvidenceLog = errors.New("no memory-read evidence log or clock is configured")

// readOnce makes the call read each of its skill and file arguments at most once, however many
// times the service and the gate ask: reading them is the caller's work.
func readOnce(call projection.ToolCall) projection.ToolCall {
	call.ReadSkill = onceString(call.ReadSkill)
	call.ReadFilePath = onceString(call.ReadFilePath)
	return call
}

func onceString(read func() string) func() string {
	if read == nil {
		return nil
	}
	var (
		done  bool
		value string
	)
	return func() string {
		if !done {
			value, done = read(), true
		}
		return value
	}
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
