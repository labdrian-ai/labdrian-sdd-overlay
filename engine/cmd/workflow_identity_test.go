package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// workflowLog is the text of the log of workflow wf of project under the state home the test set.
func workflowLog(t *testing.T, project, wf string) (string, bool) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(os.Getenv("XDG_STATE_HOME"), "labdrian", "workflows", project, wf+".jsonl"))
	if os.IsNotExist(err) {
		return "", false
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(raw), true
}

// gitRepository makes dir a repository by hand, with origin as its origin remote ("" for none).
func gitRepository(t *testing.T, dir, origin string) {
	t.Helper()
	config := "[core]\n\tbare = false\n"
	if origin != "" {
		config += "[remote \"origin\"]\n\turl = " + origin + "\n"
	}
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeMemoryTestFile(t, filepath.Join(dir, ".git"), "HEAD", "ref: refs/heads/main\n")
	writeMemoryTestFile(t, filepath.Join(dir, ".git"), "config", config)
}

// workflow create fills project_identity with the shared resolver over the directory it runs in
// (C23, decision 2 of M2), and the slug stays the key of the workflow's directory.
func TestWorkflowCreateRecordsTheSharedIdentityOfTheDirectory(t *testing.T) {
	for _, tt := range []struct {
		name     string
		build    func(t *testing.T, dir string) string
		identity string
	}{
		{"a repository with an origin", func(t *testing.T, dir string) string {
			repo := filepath.Join(dir, "checkout")
			gitRepository(t, repo, "git@github.com:acme/widgets.git")
			return repo
		}, "github.com/acme/widgets"},
		{"a declared name", func(t *testing.T, dir string) string {
			repo := filepath.Join(dir, "checkout")
			gitRepository(t, repo, "git@github.com:acme/widgets.git")
			writeMemoryTestFile(t, repo, ".longterm-mem-project", "acme-widgets\n")
			return repo
		}, "acme-widgets"},
		{"below the root of a repository with no origin", func(t *testing.T, dir string) string {
			repo := filepath.Join(dir, "widgets")
			gitRepository(t, repo, "")
			below := filepath.Join(repo, "sub")
			if err := os.MkdirAll(below, 0o755); err != nil {
				t.Fatal(err)
			}
			return below
		}, "widgets"},
		{"a directory that is no repository", func(t *testing.T, dir string) string {
			plain := filepath.Join(dir, "scratch")
			if err := os.MkdirAll(plain, 0o755); err != nil {
				t.Fatal(err)
			}
			return plain
		}, "scratch"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			workflowTestStateHome(t)
			dir := t.TempDir()
			cwd := tt.build(t, dir)
			goalPath := writeMemoryTestFile(t, dir, "goal.json", memoryTestGoalJSON("proj-1", "goal-1"))

			r := runWorkflowTest([]string{"create", "--project", "proj-1", "--workflow", "wf-1", "--goal", goalPath, "--profile", "odd"}, cwd)
			if r.code != 0 {
				t.Fatalf("create: code=%d stderr=%q, want exit 0", r.code, r.stderr)
			}
			log, ok := workflowLog(t, "proj-1", "wf-1")
			if !ok {
				t.Fatal("create wrote no log under workflows/proj-1/")
			}
			if want := `"project_identity":"` + tt.identity + `"`; !strings.Contains(log, want) || !strings.Contains(log, `"project_id":"proj-1"`) {
				t.Errorf("the log is %s, want the slug as project_id and %s", log, want)
			}
		})
	}
}

// workflow create refuses, writing nothing, when the Goal names another project than --project
// (C23, decision 5 of M2).
func TestWorkflowCreateRefusesAGoalOfAnotherProject(t *testing.T) {
	workflowTestStateHome(t)
	dir := t.TempDir()
	goalPath := writeMemoryTestFile(t, dir, "goal.json", memoryTestGoalJSON("proj-2", "goal-1"))

	r := runWorkflowTest([]string{"create", "--project", "proj-1", "--workflow", "wf-1", "--goal", goalPath, "--profile", "odd"}, dir)
	if r.code != 2 || r.stdout != "" {
		t.Fatalf("create: code=%d stdout=%q, want exit 2 and nothing on stdout", r.code, r.stdout)
	}
	want := `error: workflow create: workflow lifecycle: cannot create: the goal belongs to another project: its project_id is "proj-2" and the workflow's project is "proj-1"` + "\n"
	if r.stderr != want {
		t.Errorf("stderr = %q, want %q", r.stderr, want)
	}
	if _, ok := workflowLog(t, "proj-1", "wf-1"); ok {
		t.Error("the refused create wrote a log under workflows/proj-1/")
	}
	if _, ok := workflowLog(t, "proj-2", "wf-1"); ok {
		t.Error("the refused create wrote a log under workflows/proj-2/")
	}
}

// create never needed a repository it can read, and still does not: in one the shared resolver
// cannot read, it creates the workflow as before, records no identity (an unknown, which the gate
// allows) and says so in one line.
func TestWorkflowCreateStillCreatesWhenTheProjectIdentityCannotBeResolved(t *testing.T) {
	workflowTestStateHome(t)
	dir := t.TempDir()
	broken := filepath.Join(dir, "broken")
	if err := os.MkdirAll(broken, 0o755); err != nil {
		t.Fatal(err)
	}
	writeMemoryTestFile(t, broken, ".git", "this is not a git file\n")
	goalPath := writeMemoryTestFile(t, dir, "goal.json", memoryTestGoalJSON("proj-1", "goal-1"))

	r := runWorkflowTest([]string{"create", "--project", "proj-1", "--workflow", "wf-1", "--goal", goalPath, "--profile", "odd"}, broken)
	if r.code != 0 || !strings.Contains(r.stdout, `"status": "created"`) {
		t.Fatalf("create: code=%d stdout=%q stderr=%q, want exit 0 and the created state", r.code, r.stdout, r.stderr)
	}
	if !strings.HasPrefix(r.stderr, "warning: workflow create: the project identity could not be resolved (") ||
		!strings.Contains(r.stderr, "gitdir:") || !strings.HasSuffix(r.stderr, "so the workflow records its slug, proj-1, as its identity\n") || strings.Count(r.stderr, "\n") != 1 {
		t.Errorf("stderr = %q, want one warning line naming the resolution error", r.stderr)
	}
	log, ok := workflowLog(t, "proj-1", "wf-1")
	if !ok || !strings.Contains(log, `"project_identity":"proj-1"`) {
		t.Errorf("the log is %q (written %v), want a workflow whose project_identity is its slug, proj-1", log, ok)
	}
}

// An identity the resolver gives but a workflow log cannot hold (here a directory name with a tab,
// which the directory rule answers outside git) is treated like one that cannot be resolved: create
// still creates, records none and says so in one line. Decision 5 stays its only new refusal.
func TestWorkflowCreateStillCreatesWhenTheProjectIdentityCannotBeRecorded(t *testing.T) {
	workflowTestStateHome(t)
	dir := t.TempDir()
	odd := filepath.Join(dir, "a\tb")
	if err := os.MkdirAll(odd, 0o755); err != nil {
		t.Fatal(err)
	}
	goalPath := writeMemoryTestFile(t, dir, "goal.json", memoryTestGoalJSON("proj-1", "goal-1"))

	r := runWorkflowTest([]string{"create", "--project", "proj-1", "--workflow", "wf-1", "--goal", goalPath, "--profile", "odd"}, odd)
	if r.code != 0 || !strings.Contains(r.stdout, `"status": "created"`) {
		t.Fatalf("create: code=%d stdout=%q stderr=%q, want exit 0 and the created state", r.code, r.stdout, r.stderr)
	}
	if !strings.HasPrefix(r.stderr, "warning: workflow create: the project identity could not be resolved (") ||
		!strings.Contains(r.stderr, "control character") || !strings.HasSuffix(r.stderr, "so the workflow records its slug, proj-1, as its identity\n") || strings.Count(r.stderr, "\n") != 1 {
		t.Errorf("stderr = %q, want one warning line naming why the identity cannot be recorded", r.stderr)
	}
	log, ok := workflowLog(t, "proj-1", "wf-1")
	if !ok || !strings.Contains(log, `"project_identity":"proj-1"`) {
		t.Errorf("the log is %q (written %v), want a workflow whose project_identity is its slug, proj-1", log, ok)
	}
}
