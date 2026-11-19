package main

// workflow subcommand verb 'goal-base': 'workflow goal-base --feature <f> [--project <id>]' prints the four fields of a
// Goal v3 that the engine knows (labdrian-orchestrator-opencode T55, S3, L152), so the shaper copies them and writes only
// the judgment fields. It is read-only: it resolves a project identity and a profile's memory default, and writes
// nothing. The directive is the memory default of the profile 'workflow open' creates (memoryscope.DefaultFor), the
// ceiling a goal may only narrow; the project id is --project or the last '/'-separated part of the shared project
// identity 'workflow create' and 'workflow open' record for the memory gate (workflowProjectIdentity's resolver).
//
// Exit codes: 0 printed, 2 refused (not a feature name, a project id outside [A-Za-z0-9._-], an identity that cannot be
// read), 1 a usage error.

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/memoryscope"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/oddtask"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow/app"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflowprofile"
)

// goalBaseOpts are the parsed flags of 'workflow goal-base'.
type goalBaseOpts struct{ feature, project string }

// parseGoalBaseArgs parses --feature and --project; --feature is required. It fails loud on an unknown flag, a flag
// without its value or a positional argument, as parseOpenArgs does.
func parseGoalBaseArgs(args []string) (goalBaseOpts, error) {
	var o goalBaseOpts
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--feature", "--project":
			if i+1 >= len(args) {
				return o, fmt.Errorf("%s requires a value", a)
			}
			i++
			if a == "--feature" {
				o.feature = args[i]
			} else {
				o.project = args[i]
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
	return o, nil
}

// goalBase is the line the verb prints; its field order is the order of the Goal's keys.
type goalBase struct {
	Version         int                   `json:"version"`
	ProjectID       string                `json:"project_id"`
	GoalID          string                `json:"goal_id"`
	MemoryDirective memoryscope.Directive `json:"memory_directive"`
}

// goalBaseProject is the project id of the goal: the explicit one, else the last '/'-separated part of the shared
// identity (the resolver workflowProjectIdentity reads). A refusal is the message after "workflow goal-base: ".
func goalBaseProject(cwd, explicit string) (string, error) {
	if explicit != "" {
		if err := workflow.ValidateIdentifier("project_id", explicit); err != nil {
			return "", err
		}
		return explicit, nil
	}
	resolved, err := sharedProjectRules("").Resolve(cwd, "")
	if err != nil {
		return "", fmt.Errorf("the project identity could not be resolved (%s); pass --project <id>", strings.NewReplacer("\n", " ", "\r", " ").Replace(err.Error()))
	}
	project := resolved.Project
	if i := strings.LastIndex(project, "/"); i >= 0 {
		project = project[i+1:]
	}
	if err := workflow.ValidateIdentifier("project_id", project); err != nil {
		return "", fmt.Errorf("the project identity %q gives no project id (%s); pass --project <id>", resolved.Project, err)
	}
	return project, nil
}

// runWorkflowGoalBase implements 'workflow goal-base --feature <f> [--project <id>]'.
func runWorkflowGoalBase(d deps, args []string, cwd string, stdout, stderr io.Writer, exit func(int)) {
	o, err := parseGoalBaseArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "error: workflow goal-base: %v\n", err)
		exit(1)
		return
	}
	if !oddtask.ValidFeature(o.feature) {
		fmt.Fprintf(stderr, "error: workflow goal-base: feature %q is not a feature name\n", o.feature)
		exit(2)
		return
	}
	line, err := goalBaseLine(cwd, o)
	if err != nil {
		fmt.Fprintf(stderr, "error: workflow goal-base: %v\n", err)
		exit(2)
		return
	}
	fmt.Fprintln(stdout, line)
}

// goalBaseLine is the compact JSON of the four fields for feature o.feature.
func goalBaseLine(cwd string, o goalBaseOpts) (string, error) {
	profile, err := workflowprofile.Resolve(app.Profile)
	if err != nil {
		return "", err
	}
	directive, err := memoryscope.DefaultFor(profile)
	if err != nil {
		return "", err
	}
	project, err := goalBaseProject(cwd, o.project)
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(goalBase{Version: 3, ProjectID: project, GoalID: o.feature, MemoryDirective: directive})
	if err != nil {
		return "", err
	}
	return string(raw), nil
}
