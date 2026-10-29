package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// openFeatureDoc is a feature document with a shaped task (T1, designed by the shaper in L1) and an
// unmarked one (T2).
const openFeatureDoc = `# f

## Specs

- **S1** a spec.

## Tasks

- [ ] T1 — S1 — inline — shaped: L1 — commit: -
- [ ] T2 — S1 — inline — commit: -

## Log

- **L1** (2026-10-29, shaper): the design of T1.
`

const openOpenedJSON = `{"workflow":"f-t1","chain":"f-t1","goal":"f","project":"proj-1","status":"running","bound":true}` + "\n"

// openEnv is one isolated world for 'workflow open': HOME, XDG_STATE_HOME and XDG_CONFIG_HOME under a
// temporary directory, and a repository holding odd/tasks/f.md and odd/goals/f.json.
type openEnv struct {
	state string
	repo  string
	d     deps
}

func newOpenEnv(t *testing.T) openEnv {
	t.Helper()
	base := t.TempDir()
	state := filepath.Join(base, "state")
	for _, dir := range []string{state, filepath.Join(base, "home"), filepath.Join(base, "config")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("XDG_STATE_HOME", state)
	t.Setenv("HOME", filepath.Join(base, "home"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(base, "config"))
	repo := fixtureRepo(t, "repo")
	for rel, content := range map[string]string{
		"odd/tasks/f.md":   openFeatureDoc,
		"odd/goals/f.json": memoryTestGoalJSON("proj-1", "f"),
	} {
		path := filepath.Join(repo, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return openEnv{state: state, repo: repo, d: testDeps()}
}

func (e openEnv) open(args ...string) workflowRun {
	return runWorkflowTestWith(e.d, append([]string{"open"}, args...), e.repo)
}

func (e openEnv) write(t *testing.T, rel, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(e.repo, filepath.FromSlash(rel)), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func sha256OfFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// S19 (b): one command opens the workflow of a task, and the stores say so.
func TestWorkflowOpenOpensTheTaskWorkflow(t *testing.T) {
	e := newOpenEnv(t)

	r := e.open("--feature", "f", "--task", "T1")
	if r.code != 0 || r.stdout != openOpenedJSON || r.stderr != "" {
		t.Fatalf("open: code=%d stdout=%q stderr=%q, want exit 0 and %q", r.code, r.stdout, r.stderr, openOpenedJSON)
	}

	binding := decodeBindingReport(t, runWorkflowTestWith(e.d, []string{"binding"}, e.repo))
	if binding.Classification != "owned" || binding.Binding == nil || binding.Binding.WorkflowID != "f-t1" || binding.Binding.ProjectID != "proj-1" {
		t.Errorf("binding = %+v, want owned and bound to proj-1/f-t1", binding)
	}
	if binding.Workflow == nil || binding.Workflow.Status != "running" {
		t.Errorf("bound workflow = %+v, want status running", binding.Workflow)
	}

	status := runWorkflowTestWith(e.d, []string{"status", "--project", "proj-1", "--workflow", "f-t1"}, e.repo)
	var state workflowStateJSON
	if err := json.Unmarshal([]byte(status.stdout), &state); err != nil || status.code != 0 {
		t.Fatalf("status: code=%d stdout=%q err=%v", status.code, status.stdout, err)
	}
	wantStages := []string{"authorize", "explore", "resolve-uncertainty", "classify", "track-if-substantial"}
	if state.Status != "running" || state.Profile != "odd" || state.GoalID != "f" || state.RoleChainID != "f-t1" || !reflect.DeepEqual(state.Stages, wantStages) {
		t.Errorf("status = %+v, want running, profile odd, goal f, chain f-t1, stages %v", state, wantStages)
	}

	resume := runRolesTest([]string{"resume", "--project", "proj-1", "--goal", "f", "--chain", "f-t1"}, "")
	if resume.code != 0 || !strings.Contains(resume.stdout, `"role": "builder"`) {
		t.Errorf("roles resume: code=%d stdout=%q, want role builder", resume.code, resume.stdout)
	}

	store, err := newRoleChainStore()
	if err != nil {
		t.Fatal(err)
	}
	records, err := store.LoadChain("proj-1", "f", "f-t1")
	if err != nil || len(records) != 2 {
		t.Fatalf("chain f-t1: %d records, err=%v, want 2", len(records), err)
	}
	doc := sha256OfFile(t, filepath.Join(e.repo, "odd", "tasks", "f.md"))
	first, second := records[0].Handoff, records[1].Handoff
	if first.FromRole != "shaper" || first.ToRole != "estimator" || second.FromRole != "estimator" || second.ToRole != "builder" {
		t.Errorf("hops = %s to %s, %s to %s, want shaper to estimator, estimator to builder", first.FromRole, first.ToRole, second.FromRole, second.ToRole)
	}
	for i, h := range []string{first.PayloadSHA256, second.PayloadSHA256} {
		if h != doc {
			t.Errorf("record %d payload_sha256 = %s, want the sha256 of odd/tasks/f.md, %s", i+1, h, doc)
		}
	}
	for i, h := range []string{first.PayloadKind, second.PayloadKind} {
		if h != "feature-doc" {
			t.Errorf("record %d payload_kind = %q, want feature-doc", i+1, h)
		}
	}
	for i, h := range []string{first.Context.Summary, second.Context.Summary} {
		if !strings.Contains(h, "T1") || !strings.Contains(h, "shaped: L1") {
			t.Errorf("record %d summary = %q, want it to name T1 and shaped: L1", i+1, h)
		}
	}
}

// S19 (b): each refusal comes before any write.
func TestWorkflowOpenRefusesBeforeWritingAnything(t *testing.T) {
	cases := []struct {
		name  string
		args  []string
		setup func(t *testing.T, e openEnv)
		want  string
	}{
		{"no goal file", []string{"--feature", "f", "--task", "T1"}, func(t *testing.T, e openEnv) {
			if err := os.Remove(filepath.Join(e.repo, "odd", "goals", "f.json")); err != nil {
				t.Fatal(err)
			}
		}, "read goal"},
		{"a goal file that is not a goal", []string{"--feature", "f", "--task", "T1"}, func(t *testing.T, e openEnv) {
			e.write(t, "odd/goals/f.json", "not json")
		}, "parse goal"},
		{"a goal of another goal id", []string{"--feature", "f", "--task", "T1"}, func(t *testing.T, e openEnv) {
			e.write(t, "odd/goals/f.json", memoryTestGoalJSON("proj-1", "g"))
		}, `goal_id "g"`},
		{"--goal naming a goal of another goal id", []string{"--feature", "f", "--task", "T1", "--goal", "other.json"}, func(t *testing.T, e openEnv) {
			e.write(t, "other.json", memoryTestGoalJSON("proj-1", "g"))
		}, `goal_id "g"`},
		{"a task that is not marked", []string{"--feature", "f", "--task", "T2"}, nil, "not marked as designed by the shaper"},
		{"a task that is not in the document", []string{"--feature", "f", "--task", "T9"}, nil, "task T9 is not in the ## Tasks section"},
		{"no feature document", []string{"--feature", "f", "--task", "T1"}, func(t *testing.T, e openEnv) {
			if err := os.Remove(filepath.Join(e.repo, "odd", "tasks", "f.md")); err != nil {
				t.Fatal(err)
			}
		}, "read feature document"},
		{"a feature name that is a path", []string{"--feature", "../f", "--task", "T1"}, nil, `feature "../f"`},
		{"a task id that is not one", []string{"--feature", "f", "--task", "t1"}, nil, `task "t1"`},
		{"a chain that already has records", []string{"--feature", "f", "--task", "T1"}, func(t *testing.T, e openEnv) {
			if r := e.open("--feature", "f", "--task", "T1"); r.code != 0 {
				t.Fatalf("first open: code=%d stderr=%q", r.code, r.stderr)
			}
			// Only the workflow and its binding go, so the chain's two records remain.
			for _, dir := range []string{"workflows", "bindings"} {
				if err := os.RemoveAll(filepath.Join(e.state, "labdrian", dir)); err != nil {
					t.Fatal(err)
				}
			}
		}, "role chain f-t1 already has 2 records"},
		{"a workflow that already exists", []string{"--feature", "f", "--task", "T1"}, func(t *testing.T, e openEnv) {
			goal := filepath.Join(e.repo, "odd", "goals", "f.json")
			phase6MustExitZeroWith(t, e.d, "create", []string{"create", "--project", "proj-1", "--workflow", "f-t1", "--goal", goal, "--profile", "odd"}, e.repo)
		}, "workflow f-t1 of project proj-1 already exists"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newOpenEnv(t)
			if tc.setup != nil {
				tc.setup(t, e)
			}
			before := snapshotContents(t, e.state)
			r := e.open(tc.args...)
			if r.code != 2 || r.stdout != "" || !strings.HasPrefix(r.stderr, "error: workflow open: ") || !strings.Contains(r.stderr, tc.want) {
				t.Errorf("open: code=%d stdout=%q stderr=%q, want exit 2, no stdout, and an error naming %q", r.code, r.stdout, r.stderr, tc.want)
			}
			if after := snapshotContents(t, e.state); !reflect.DeepEqual(before, after) {
				t.Errorf("the state home changed after a refusal:\nbefore %v\nafter  %v", before, after)
			}
		})
	}
}

// S19 (b): a step that fails after an earlier one wrote leaves what was written, and the error names the
// steps done and the one that failed.
func TestWorkflowOpenNamesTheStepsDoneWhenALaterStepFails(t *testing.T) {
	e := newOpenEnv(t)
	other := writeMemoryTestFile(t, t.TempDir(), "other.json", memoryTestGoalJSON("proj-1", "other"))
	phase6MustExitZeroWith(t, e.d, "create", []string{"create", "--project", "proj-1", "--workflow", "other", "--goal", other, "--profile", "odd"}, e.repo)
	phase6MustExitZeroWith(t, e.d, "bind", []string{"bind", "--project", "proj-1", "--workflow", "other"}, e.repo)

	r := e.open("--feature", "f", "--task", "T1")
	if r.code != 1 || r.stdout != "" {
		t.Fatalf("open: code=%d stdout=%q stderr=%q, want exit 1 and no stdout", r.code, r.stdout, r.stderr)
	}
	for _, want := range []string{"error: workflow open: ", "bind failed", "done: append the shaper record, append the estimator record, create, start, record the five stages", "already bound"} {
		if !strings.Contains(r.stderr, want) {
			t.Errorf("stderr = %q, want it to contain %q", r.stderr, want)
		}
	}
	status := runWorkflowTestWith(e.d, []string{"status", "--project", "proj-1", "--workflow", "f-t1"}, e.repo)
	if !strings.Contains(status.stdout, `"status": "running"`) {
		t.Errorf("the workflow f-t1 written before the failure is gone: %q", status.stdout)
	}
}

// S19 (b): usage errors exit 1 and write nothing.
func TestWorkflowOpenUsageErrors(t *testing.T) {
	cases := map[string][]string{
		"no --feature":         {"--task", "T1"},
		"no --task":            {"--feature", "f"},
		"an unknown option":    {"--feature", "f", "--task", "T1", "--bogus"},
		"a flag with no value": {"--feature", "f", "--task"},
		"a positional":         {"--feature", "f", "--task", "T1", "extra"},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			e := newOpenEnv(t)
			before := snapshotContents(t, e.state)
			r := e.open(args...)
			if r.code != 1 || r.stdout != "" || !strings.HasPrefix(r.stderr, "error: workflow open: ") {
				t.Errorf("open %v: code=%d stdout=%q stderr=%q, want exit 1 and an error", args, r.code, r.stdout, r.stderr)
			}
			if after := snapshotContents(t, e.state); !reflect.DeepEqual(before, after) {
				t.Errorf("the state home changed after a usage error")
			}
		})
	}
}
