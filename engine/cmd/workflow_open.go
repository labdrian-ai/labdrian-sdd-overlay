package main

// workflow subcommand verb 'open': 'workflow open --feature <f> --task <T> [--goal <path>]' opens the
// workflow of one task of a feature, from the repository (own-rdd S19 b). The decisions are the use case's
// (engine/workflow/app, Open); this file parses the command line, builds the use case over the file-backed
// stores and says what it answered.
//
// Exit codes: 0 opened, 2 refused before anything was written, 1 a usage error or a step that failed
// after an earlier one wrote (the error names the steps done and the step that failed).

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/oddtask"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow/app"
)

// openOpts are the parsed flags of 'workflow open'.
type openOpts struct{ feature, task, goal string }

// parseOpenArgs parses --feature, --task and --goal; --feature and --task are required. It fails loud on an
// unknown flag, a flag without its value or a positional argument.
func parseOpenArgs(args []string) (openOpts, error) {
	var o openOpts
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--feature", "--task", "--goal":
			if i+1 >= len(args) {
				return o, fmt.Errorf("%s requires a value", a)
			}
			i++
			switch a {
			case "--feature":
				o.feature = args[i]
			case "--task":
				o.task = args[i]
			case "--goal":
				o.goal = args[i]
			}
		default:
			if strings.HasPrefix(a, "-") {
				return o, fmt.Errorf("unknown flag %q", a)
			}
			return o, fmt.Errorf("unexpected argument %q", a)
		}
	}
	if o.feature == "" {
		return o, fmt.Errorf("--feature is required")
	}
	if o.task == "" {
		return o, fmt.Errorf("--task is required")
	}
	return o, nil
}

// repositorySources reads the goal and the feature document from the git repository that holds cwd.
type repositorySources struct{ cwd string }

func (s repositorySources) root() (string, error) {
	root := newRepoLocator().Provenance(s.cwd).WorktreeRoot
	if root == "" {
		return "", errors.New(errNoRepository)
	}
	return root, nil
}

func (s repositorySources) Goal(feature, override string) ([]byte, error) {
	path := override
	switch {
	case path == "":
		root, err := s.root()
		if err != nil {
			return nil, err
		}
		path = filepath.Join(root, "odd", "goals", feature+".json")
	case !filepath.IsAbs(path):
		path = filepath.Join(s.cwd, path)
	}
	return readBounded(path)
}

func (s repositorySources) Document(feature string) ([]byte, error) {
	root, err := s.root()
	if err != nil {
		return nil, err
	}
	return readBounded(filepath.Join(root, "odd", "tasks", feature+".md"))
}

// readBounded reads a file of at most oddtask.MaxDocumentBytes.
func readBounded(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() > oddtask.MaxDocumentBytes {
		return nil, fmt.Errorf("%s is larger than %d bytes", path, oddtask.MaxDocumentBytes)
	}
	return os.ReadFile(path)
}

// runWorkflowOpen implements 'workflow open --feature --task [--goal]'.
func runWorkflowOpen(d deps, args []string, cwd string, stdout, stderr io.Writer, exit func(int)) {
	o, err := parseOpenArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "error: workflow open: %v\n", err)
		exit(1)
		return
	}
	lc, err := newWorkflowLifecycle(d, cwd, "", stderr)
	if err != nil {
		fmt.Fprintf(stderr, "error: workflow open: %v\n", err)
		exit(2)
		return
	}
	chains, err := newRoleChainStore()
	if err != nil {
		fmt.Fprintf(stderr, "error: workflow open: %v\n", err)
		exit(2)
		return
	}
	opener := app.Open{
		Sources: repositorySources{cwd: cwd}, Chains: chains, Workflows: lc, Binder: d.bindWorkflow(),
		Identity: func(project string) string { return workflowProjectIdentity("open", cwd, project, stderr) },
	}
	opened, err := opener.Open(app.OpenRequest{Feature: o.feature, Task: o.task, GoalPath: o.goal, Dir: cwd})
	if err != nil {
		var step *app.StepError
		switch {
		case errors.As(err, &step):
			detail := step.Err.Error()
			if step.Step == app.StepBind {
				detail = bindingRefusal(step.Err)
			}
			fmt.Fprintf(stderr, "error: workflow open: %s failed (done: %s): %s\n", step.Step, doneSteps(step.Done), detail)
			if len(step.Done) == 0 {
				exit(2)
			} else {
				exit(1)
			}
		default:
			fmt.Fprintf(stderr, "error: workflow open: %v\n", err)
			exit(2)
		}
		return
	}
	data, err := json.Marshal(opened)
	if err == nil {
		_, err = stdout.Write(append(data, '\n'))
	}
	if err != nil {
		fmt.Fprintf(stderr, "error: workflow open: writing output: %v\n", err)
		exit(1)
		return
	}
	exit(0)
}

// doneSteps lists the steps of Open that wrote, or says there were none.
func doneSteps(done []string) string {
	if len(done) == 0 {
		return "none"
	}
	return strings.Join(done, ", ")
}
