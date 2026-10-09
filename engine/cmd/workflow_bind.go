package main

// workflow subcommand verbs 'bind', 'unbind', and 'binding': the session
// binding. A binding records which workflow the git repository containing the
// working directory follows. It lives outside the repository, in
// $XDG_STATE_HOME/labdrian/bindings/<repo-key>.json, where the repo key is the
// SHA-256 of the repository's git common directory (projection.RepoKeyOf, found
// by engine/gitfs), so every worktree of a repository, and every symlinked
// spelling of its path, shares one binding. The record and its store are
// engine/projection's.
//
// A binding is a pointer. These verbs never append to the workflow's log, and
// they run no subprocess: the repository is found by walking the filesystem, as
// the workflow verbs find the provenance they record. binding is strictly
// read-only.
//
// Exit codes are those of the other workflow verbs: 0 success, 2 refused or
// invalid, 1 usage error (including an unknown flag) or a failed write of the
// output. Exit 2 covers every refusal a binding verb makes:
//
//   - no repository to key on, or a workflow that cannot be bound;
//   - a binding file that is not ours or cannot be used: foreign, malformed, or
//     unavailable (never overwritten or removed);
//   - another bind or unbind in progress for the repository (busy), which can
//     be retried;
//   - a binding that another process changed while the verb was running, which
//     is reported and left alone: run 'workflow binding' and retry.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

// errNoRepository is why a binding verb refuses to run outside a repository:
// the binding is keyed by the repository, so without one there is nothing to
// key on, and guessing from the working directory would bind the wrong thing.
const errNoRepository = "binding needs a git repository to key on: no .git was found at or above the working directory"

// beforeStaleReplace is a test seam, nil outside tests. runWorkflowBind calls it
// after it judged the repository's binding stale and before it replaces it: the
// window in which another process can bind a live workflow, which
// projection.Store.BindIfUnchanged must then refuse to overwrite.
var beforeStaleReplace func()

// bindingReportJSON is the CLI's JSON view of a repository's binding, printed
// by bind (with the binding just made or kept) and by binding. Detail explains
// every classification other than absent and owned; Binding is set only when the
// classification is owned; Workflow, set only by binding, describes the
// workflow an owned binding names.
type bindingReportJSON struct {
	Classification string              `json:"classification"`
	Detail         string              `json:"detail,omitempty"`
	Binding        *projection.Binding `json:"binding,omitempty"`
	Workflow       *boundWorkflowJSON  `json:"workflow,omitempty"`
}

// boundWorkflowJSON is what binding reports about the bound workflow: its
// on-disk classification and, when owned, its status. An unreadable workflow
// store is reported here as data (classification unavailable), never as a
// failure of the verb.
type boundWorkflowJSON struct {
	Classification string `json:"classification"`
	Status         string `json:"status,omitempty"`
	Detail         string `json:"detail,omitempty"`
}

// parseBindingArgs parses the arguments of the binding verbs. bind
// (takesIDs) takes --project and --workflow, both required; unbind and
// binding take no arguments at all. Anything else is a usage error: an unknown
// flag, a positional argument, a flag without its value, or a missing required
// flag. The last occurrence of a repeated flag wins, as for the other workflow
// verbs.
func parseBindingArgs(args []string, takesIDs bool) (project, workflowID string, err error) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case takesIDs && (a == "--project" || a == "--workflow"):
			if i+1 >= len(args) {
				return "", "", fmt.Errorf("%s requires a value", a)
			}
			i++
			if a == "--project" {
				project = args[i]
			} else {
				workflowID = args[i]
			}
		case strings.HasPrefix(a, "-"):
			return "", "", fmt.Errorf("unknown flag %q", a)
		default:
			return "", "", fmt.Errorf("unexpected argument %q", a)
		}
	}
	if takesIDs {
		if project == "" {
			return "", "", fmt.Errorf("--project is required")
		}
		if workflowID == "" {
			return "", "", fmt.Errorf("--workflow is required")
		}
	}
	return project, workflowID, nil
}

// refuseBinding reports a refusal of a binding verb on stderr and exits 2. The
// caller returns right after it, because tests inject a non-terminating exit.
func refuseBinding(stderr io.Writer, exit func(int), verb, format string, args ...any) {
	fmt.Fprintf(stderr, "error: workflow %s: %s\n", verb, fmt.Sprintf(format, args...))
	exit(2)
}

// writeBindingJSON prints v as indented JSON on stdout and exits 0, or reports
// a marshal or write failure on stderr and exits 1, the way writeWorkflowState
// does for the other verbs.
func writeBindingJSON(verb string, v any, stdout, stderr io.Writer, exit func(int)) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "error: workflow %s: %v\n", verb, err)
		exit(1)
		return
	}
	if _, err := stdout.Write(append(data, '\n')); err != nil {
		fmt.Fprintf(stderr, "error: workflow %s: writing output: %v\n", verb, err)
		exit(1)
		return
	}
	exit(0)
}

// loadWorkflow loads one workflow through a store built from the environment.
// A store that cannot be built or read is reported as an unavailable workflow,
// with the reason as its detail, rather than as an error: binding must be able
// to describe a workflow it cannot read, and bind must be able to say why it
// cannot bind one. The identifiers must already be valid.
func loadWorkflow(projectID, workflowID string) workflow.Loaded {
	unavailable := func(err error) workflow.Loaded {
		return workflow.Loaded{Classification: workflow.ClassificationUnavailable, Detail: err.Error()}
	}
	store, err := newWorkflowStore()
	if err != nil {
		return unavailable(err)
	}
	loaded, err := store.Load(projectID, workflowID)
	if err != nil {
		return unavailable(err)
	}
	return loaded
}

// runWorkflowBind implements 'workflow bind --project --workflow'.
//
// The workflow must exist, be owned, and not be closed. If the repository is
// already bound to the same workflow, nothing changes. If it is bound to a
// different one, that binding is replaced only when it is stale: the bound
// workflow is closed, gone, or a log that can never be followed again because
// it is corrupt or not ours (drifted, malformed, foreign). A binding to a
// workflow that is still active (created, running, or paused), or whose log
// cannot be read right now (unavailable, so it may be active), is never
// replaced silently: bind refuses, names the bound workflow, and tells the
// user to unbind first. A stale binding is replaced only if it is still the
// exact binding that was judged stale (projection.Store.BindIfUnchanged): if
// another process changed it in between, bind exits 2 and leaves it alone.
func runWorkflowBind(args []string, cwd string, stdout, stderr io.Writer, exit func(int)) {
	project, workflowID, err := parseBindingArgs(args, true)
	if err != nil {
		fmt.Fprintf(stderr, "error: workflow bind: %v\n", err)
		exit(1)
		return
	}
	repoKey, ok := newRepoLocator().RepoKey(cwd)
	if !ok {
		refuseBinding(stderr, exit, "bind", "%s", errNoRepository)
		return
	}
	if err := workflow.ValidateIdentifier("project_id", project); err != nil {
		refuseBinding(stderr, exit, "bind", "%v", err)
		return
	}
	if err := workflow.ValidateIdentifier("workflow_id", workflowID); err != nil {
		refuseBinding(stderr, exit, "bind", "%v", err)
		return
	}

	target := loadWorkflow(project, workflowID)
	switch {
	case target.Classification == workflow.ClassificationAbsent:
		refuseBinding(stderr, exit, "bind", "workflow %q of project %q does not exist; create it first", workflowID, project)
		return
	case target.Classification != workflow.ClassificationOwned:
		refuseBinding(stderr, exit, "bind", "workflow %q of project %q is not owned (%s): %s", workflowID, project, target.Classification, target.Detail)
		return
	case target.State.Status == workflow.StatusClosed:
		refuseBinding(stderr, exit, "bind", "workflow %q of project %q is closed (%s); a closed workflow cannot be followed", workflowID, project, target.State.CloseOutcome)
		return
	}

	bindings, err := newBindingStore()
	if err != nil {
		refuseBinding(stderr, exit, "bind", "%v", err)
		return
	}
	// stale is the binding judged stale below, or nil when there is nothing to
	// replace. The judgment and the replacement are two steps, and the window
	// between them is closed by replacing only that exact binding.
	var stale *projection.Binding
	current, err := bindings.Load(repoKey)
	if err != nil {
		refuseBinding(stderr, exit, "bind", "%v", err)
		return
	}
	if current.Classification == projection.ClassificationOwned && (current.Binding.ProjectID != project || current.Binding.WorkflowID != workflowID) {
		bound := current.Binding
		previous := loadWorkflow(bound.ProjectID, bound.WorkflowID)
		switch {
		case previous.Classification == workflow.ClassificationOwned && previous.State.Status != workflow.StatusClosed:
			refuseBinding(stderr, exit, "bind", "this repository is already bound to workflow %q of project %q (status: %s); run 'workflow unbind' first to bind another", bound.WorkflowID, bound.ProjectID, previous.State.Status)
			return
		case previous.Classification == workflow.ClassificationUnavailable:
			refuseBinding(stderr, exit, "bind", "this repository is bound to workflow %q of project %q, whose log cannot be read (%s), so it may still be active; fix the problem, or run 'workflow unbind' first to bind another", bound.WorkflowID, bound.ProjectID, previous.Detail)
			return
		}
		stale = &bound
	}

	var bindErr error
	if stale != nil {
		if beforeStaleReplace != nil {
			beforeStaleReplace()
		}
		// Replaces the binding judged stale only if it is still that exact
		// binding: a live workflow another process bound since is left alone.
		bindErr = bindings.BindIfUnchanged(repoKey, project, workflowID, time.Now(), *stale)
	} else {
		bindErr = bindings.Bind(repoKey, project, workflowID, time.Now(), false)
	}
	if bindErr != nil {
		switch {
		case errors.Is(bindErr, projection.ErrBindingChanged):
			// The binding changed after it was judged stale; what is bound now is
			// not the caller's to overwrite.
			refuseBinding(stderr, exit, "bind", "%v; run 'workflow binding' to see what is bound now, then retry", bindErr)
		case errors.Is(bindErr, projection.ErrAlreadyBound):
			// The binding changed between the check above and the write.
			refuseBinding(stderr, exit, "bind", "%v; run 'workflow unbind' first to bind another", bindErr)
		default:
			refuseBinding(stderr, exit, "bind", "%v", bindErr)
		}
		return
	}

	// Print what is stored, which for an idempotent bind is the original
	// binding with its original bound_at.
	stored, err := bindings.Load(repoKey)
	if err != nil {
		refuseBinding(stderr, exit, "bind", "%v", err)
		return
	}
	if stored.Classification != projection.ClassificationOwned {
		refuseBinding(stderr, exit, "bind", "the binding could not be read back (%s): %s", stored.Classification, stored.Detail)
		return
	}
	if err := verifyStored(stored.Binding, project, workflowID); err != nil {
		refuseBinding(stderr, exit, "bind", "%v", err)
		return
	}
	writeBindingJSON("bind", bindingReportJSON{Classification: string(stored.Classification), Binding: &stored.Binding}, stdout, stderr, exit)
}

// verifyStored checks the binding read back after Bind: another process may have
// changed it meanwhile, and reporting that would name a workflow nobody asked for.
func verifyStored(b projection.Binding, project, workflowID string) error {
	if b.ProjectID != project || b.WorkflowID != workflowID {
		return fmt.Errorf("the binding was changed concurrently: it now names workflow %q of project %q, not the requested workflow %q of project %q", b.WorkflowID, b.ProjectID, workflowID, project)
	}
	return nil
}

// runWorkflowUnbind implements 'workflow unbind'. It removes the binding of the
// repository containing cwd and prints {"removed": true|false}. It is
// idempotent: unbinding a repository that is not bound removes nothing and
// succeeds. A binding file that is foreign or malformed is refused and left
// untouched.
func runWorkflowUnbind(args []string, cwd string, stdout, stderr io.Writer, exit func(int)) {
	if _, _, err := parseBindingArgs(args, false); err != nil {
		fmt.Fprintf(stderr, "error: workflow unbind: %v\n", err)
		exit(1)
		return
	}
	repoKey, ok := newRepoLocator().RepoKey(cwd)
	if !ok {
		refuseBinding(stderr, exit, "unbind", "%s", errNoRepository)
		return
	}
	bindings, err := newBindingStore()
	if err != nil {
		refuseBinding(stderr, exit, "unbind", "%v", err)
		return
	}
	removed, err := bindings.Unbind(repoKey)
	if err != nil {
		refuseBinding(stderr, exit, "unbind", "%v", err)
		return
	}
	writeBindingJSON("unbind", struct {
		Removed bool `json:"removed"`
	}{removed}, stdout, stderr, exit)
}

// runWorkflowBinding implements 'workflow binding'. It is strictly read-only:
// it prints the binding's classification and, only when the binding is owned,
// the binding and the classification and status of the workflow it names. A
// state that is not ours (foreign, malformed, unavailable) is reported with its
// detail and exit 0, as 'workflow status' reports a workflow it cannot own.
func runWorkflowBinding(args []string, cwd string, stdout, stderr io.Writer, exit func(int)) {
	if _, _, err := parseBindingArgs(args, false); err != nil {
		fmt.Fprintf(stderr, "error: workflow binding: %v\n", err)
		exit(1)
		return
	}
	repoKey, ok := newRepoLocator().RepoKey(cwd)
	if !ok {
		refuseBinding(stderr, exit, "binding", "%s", errNoRepository)
		return
	}
	bindings, err := newBindingStore()
	if err != nil {
		refuseBinding(stderr, exit, "binding", "%v", err)
		return
	}
	loaded, err := bindings.Load(repoKey)
	if err != nil {
		refuseBinding(stderr, exit, "binding", "%v", err)
		return
	}

	report := bindingReportJSON{Classification: string(loaded.Classification), Detail: loaded.Detail}
	if loaded.Classification == projection.ClassificationOwned {
		bound := loaded.Binding
		report.Binding = &bound
		w := loadWorkflow(bound.ProjectID, bound.WorkflowID)
		report.Workflow = &boundWorkflowJSON{Classification: string(w.Classification), Detail: w.Detail}
		if w.Classification == workflow.ClassificationOwned {
			report.Workflow.Status = string(w.State.Status)
		}
	}
	writeBindingJSON("binding", report, stdout, stderr, exit)
}
