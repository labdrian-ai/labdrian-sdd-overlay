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
// The decisions are the use case's (engine/projection/app, BindWorkflow); this
// file parses the command line, calls it and says what it answered, and its
// refusals, in the words these verbs have always used.
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

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection/app"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

// errNoRepository is why a binding verb refuses to run outside a repository:
// the binding is keyed by the repository, so without one there is nothing to
// key on, and guessing from the working directory would bind the wrong thing.
const errNoRepository = "binding needs a git repository to key on: no .git was found at or above the working directory"

// beforeStaleReplace is a test seam, nil outside tests. The binding store of
// the verbs calls it (seamedBindings) after bind judged the repository's
// binding stale and before it replaces it: the window in which another
// process can bind a live workflow, which projection.Store.BindIfUnchanged
// must then refuse to overwrite.
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

// bindingRefusal says what a binding use case refused, in the words of the verbs: the typed
// refusals of engine/projection/app, each with the command that gets a person out of it, and
// anything else (what the binding store said) as it was said.
func bindingRefusal(err error) string {
	var (
		noRepository *app.NoRepositoryError
		absent       *app.WorkflowAbsentError
		notOwned     *app.WorkflowNotOwnedError
		closedFlow   *app.WorkflowClosedError
		live         *app.BoundToLiveWorkflowError
		unreadable   *app.BoundToUnreadableWorkflowError
		readBack     *app.ReadBackError
	)
	switch {
	case errors.As(err, &noRepository):
		return errNoRepository
	case errors.As(err, &absent):
		return fmt.Sprintf("workflow %q of project %q does not exist; create it first", absent.WorkflowID, absent.ProjectID)
	case errors.As(err, &notOwned):
		return fmt.Sprintf("workflow %q of project %q is not owned (%s): %s", notOwned.WorkflowID, notOwned.ProjectID, notOwned.Classification, notOwned.Detail)
	case errors.As(err, &closedFlow):
		return fmt.Sprintf("workflow %q of project %q is closed (%s); a closed workflow cannot be followed", closedFlow.WorkflowID, closedFlow.ProjectID, closedFlow.Outcome)
	case errors.As(err, &live):
		return fmt.Sprintf("this repository is already bound to workflow %q of project %q (status: %s); run 'workflow unbind' first to bind another", live.Bound.WorkflowID, live.Bound.ProjectID, live.Status)
	case errors.As(err, &unreadable):
		return fmt.Sprintf("this repository is bound to workflow %q of project %q, whose log cannot be read (%s), so it may still be active; fix the problem, or run 'workflow unbind' first to bind another", unreadable.Bound.WorkflowID, unreadable.Bound.ProjectID, unreadable.Detail)
	case errors.As(err, &readBack):
		return fmt.Sprintf("the binding could not be read back (%s): %s", readBack.Classification, readBack.Detail)
	case errors.Is(err, projection.ErrBindingChanged):
		// The binding changed after it was judged stale; what is bound now is
		// not the caller's to overwrite.
		return fmt.Sprintf("%v; run 'workflow binding' to see what is bound now, then retry", err)
	case errors.Is(err, projection.ErrAlreadyBound):
		// The binding changed between the check and the write.
		return fmt.Sprintf("%v; run 'workflow unbind' first to bind another", err)
	}
	// An invalid identifier, a binding changed concurrently and a store that failed say what they
	// are in their own words.
	return err.Error()
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
func runWorkflowBind(d deps, args []string, cwd string, stdout, stderr io.Writer, exit func(int)) {
	project, workflowID, err := parseBindingArgs(args, true)
	if err != nil {
		fmt.Fprintf(stderr, "error: workflow bind: %v\n", err)
		exit(1)
		return
	}
	stored, err := newBindWorkflow().Bind(app.BindRequest{Dir: cwd, ProjectID: project, WorkflowID: workflowID})
	if err != nil {
		refuseBinding(stderr, exit, "bind", "%s", bindingRefusal(err))
		return
	}
	// Print what is stored, which for an idempotent bind is the original
	// binding with its original bound_at.
	writeBindingJSON("bind", bindingReportJSON{Classification: string(projection.ClassificationOwned), Binding: &stored}, stdout, stderr, exit)
}

// runWorkflowUnbind implements 'workflow unbind'. It removes the binding of the
// repository containing cwd and prints {"removed": true|false}. It is
// idempotent: unbinding a repository that is not bound removes nothing and
// succeeds. A binding file that is foreign or malformed is refused and left
// untouched.
func runWorkflowUnbind(d deps, args []string, cwd string, stdout, stderr io.Writer, exit func(int)) {
	if _, _, err := parseBindingArgs(args, false); err != nil {
		fmt.Fprintf(stderr, "error: workflow unbind: %v\n", err)
		exit(1)
		return
	}
	removed, err := newBindWorkflow().Unbind(cwd)
	if err != nil {
		refuseBinding(stderr, exit, "unbind", "%s", bindingRefusal(err))
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
func runWorkflowBinding(d deps, args []string, cwd string, stdout, stderr io.Writer, exit func(int)) {
	if _, _, err := parseBindingArgs(args, false); err != nil {
		fmt.Fprintf(stderr, "error: workflow binding: %v\n", err)
		exit(1)
		return
	}
	view, err := newBindWorkflow().Describe(cwd)
	if err != nil {
		refuseBinding(stderr, exit, "binding", "%s", bindingRefusal(err))
		return
	}

	report := bindingReportJSON{Classification: string(view.Binding.Classification), Detail: view.Binding.Detail}
	if view.Workflow != nil {
		bound := view.Binding.Binding
		report.Binding = &bound
		report.Workflow = &boundWorkflowJSON{Classification: string(view.Workflow.Classification), Detail: view.Workflow.Detail}
		if view.Workflow.Classification == workflow.ClassificationOwned {
			report.Workflow.Status = string(view.Workflow.State.Status)
		}
	}
	writeBindingJSON("binding", report, stdout, stderr, exit)
}
