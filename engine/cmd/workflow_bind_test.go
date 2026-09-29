package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

// bindEnv is one isolated world for the binding verbs: a state home under a
// temporary directory (XDG_STATE_HOME), a hand-made repository, and a scratch
// directory for Goal files. Repositories are plain fixture directories, never
// a real git repository and never the git binary.
type bindEnv struct {
	state string
	repo  string
	dir   string
}

func newBindEnv(t *testing.T) bindEnv {
	t.Helper()
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)
	return bindEnv{state: state, repo: fixtureRepo(t, "repo"), dir: t.TempDir()}
}

// bindingFile is where the binding of the repository at cwd is stored.
func (e bindEnv) bindingFile(t *testing.T, cwd string) string {
	t.Helper()
	return filepath.Join(e.state, "labdrian", "bindings", mustRepoKey(t, cwd)+".json")
}

// createWorkflow creates workflow wf of project through the CLI (status
// created). Every workflow uses the standalone-minimal profile.
func (e bindEnv) createWorkflow(t *testing.T, project, wf string) {
	t.Helper()
	goal := writeMemoryTestFile(t, e.dir, project+"-"+wf+"-goal.json", memoryTestGoalJSON(project, "goal-1"))
	phase6MustExitZero(t, "create", []string{"create", "--project", project, "--workflow", wf, "--goal", goal, "--profile", "standalone-minimal"}, e.dir)
}

// workflowInStatus creates a workflow and moves it to status: created,
// running, paused, or closed (abandoned).
func (e bindEnv) workflowInStatus(t *testing.T, project, wf, status string) {
	t.Helper()
	e.createWorkflow(t, project, wf)
	steps := map[string][][]string{
		"created": {},
		"running": {{"start"}},
		"paused":  {{"start"}, {"pause"}},
		"closed":  {{"close", "--outcome", "abandoned", "--reason", "test"}},
	}[status]
	for _, step := range steps {
		phase6MustExitZero(t, step[0], append(step, "--project", project, "--workflow", wf), e.dir)
	}
}

func (e bindEnv) workflowLog(project, wf string) string {
	return phase6WorkflowLogPath(e.state, project, wf)
}

func decodeBindingReport(t *testing.T, r workflowRun) bindingReportJSON {
	t.Helper()
	var report bindingReportJSON
	if err := json.Unmarshal([]byte(r.stdout), &report); err != nil {
		t.Fatalf("stdout is not a binding report: %v\n%s", err, r.stdout)
	}
	return report
}

// snapshotTree lists every path under root with its mode, size, and modification
// time, so a test can prove a verb changed nothing.
func snapshotTree(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		out = append(out, fmt.Sprintf("%s %v %d %d", rel, info.Mode(), info.Size(), info.ModTime().UnixNano()))
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return out
}

func mustBindOK(t *testing.T, cwd, project, wf string) bindingReportJSON {
	t.Helper()
	r := runWorkflowTest([]string{"bind", "--project", project, "--workflow", wf}, cwd)
	if r.code != 0 {
		t.Fatalf("bind %s/%s: code=%d stderr=%q, want exit 0", project, wf, r.code, r.stderr)
	}
	return decodeBindingReport(t, r)
}

// --- command line ----------------------------------------------------------

func TestWorkflowVerbErrorsNameTheBindingVerbs(t *testing.T) {
	if r := runWorkflowTest(nil, ""); r.code != 1 || !strings.Contains(r.stderr, "status, bind, unbind, binding") {
		t.Errorf("no verb: code=%d stderr=%q, want exit 1 listing bind, unbind, and binding", r.code, r.stderr)
	}
	if r := runWorkflowTest([]string{"astronaut"}, ""); r.code != 1 || !strings.Contains(r.stderr, "status, bind, unbind, or binding") {
		t.Errorf("unknown verb: code=%d stderr=%q, want exit 1 listing bind, unbind, and binding", r.code, r.stderr)
	}
}

// TestWorkflowBindingVerbsRejectBadCommandLines runs every case from a
// directory that is not a repository: a bad command line is a usage error
// (exit 1) and is reported before anything else is looked at.
func TestWorkflowBindingVerbsRejectBadCommandLines(t *testing.T) {
	newBindEnv(t)
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"bind without flags", []string{"bind"}, "--project is required"},
		{"bind without a workflow", []string{"bind", "--project", "p"}, "--workflow is required"},
		{"bind without a project", []string{"bind", "--workflow", "w"}, "--project is required"},
		{"bind with a flag missing its value", []string{"bind", "--project"}, "--project requires a value"},
		{"bind with a flag of another verb", []string{"bind", "--project", "p", "--workflow", "w", "--goal", "g.json"}, `unknown flag "--goal"`},
		{"bind with an unknown flag", []string{"bind", "--project", "p", "--workflow", "w", "--bogus"}, `unknown flag "--bogus"`},
		{"bind with a stray argument", []string{"bind", "--project", "p", "--workflow", "w", "stray"}, `unexpected argument "stray"`},
		{"unbind with a project", []string{"unbind", "--project", "p"}, `unknown flag "--project"`},
		{"unbind with a stray argument", []string{"unbind", "stray"}, `unexpected argument "stray"`},
		{"unbind with an unknown flag", []string{"unbind", "--bogus"}, `unknown flag "--bogus"`},
		{"binding with a workflow", []string{"binding", "--workflow", "w"}, `unknown flag "--workflow"`},
		{"binding with a stray argument", []string{"binding", "stray"}, `unexpected argument "stray"`},
		{"binding with an unknown flag", []string{"binding", "--json"}, `unknown flag "--json"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := runWorkflowTest(tt.args, t.TempDir())
			if r.code != 1 || !strings.Contains(r.stderr, tt.want) || r.stdout != "" {
				t.Fatalf("code=%d stdout=%q stderr=%q, want exit 1, no stdout, and stderr containing %q", r.code, r.stdout, r.stderr, tt.want)
			}
		})
	}
}

func TestWorkflowBindingVerbsNeedARepository(t *testing.T) {
	e := newBindEnv(t)
	e.workflowInStatus(t, "proj-1", "wf-1", "running")
	before := snapshotTree(t, e.state)

	for _, args := range [][]string{
		{"bind", "--project", "proj-1", "--workflow", "wf-1"},
		{"unbind"},
		{"binding"},
	} {
		r := runWorkflowTest(args, t.TempDir()) // a directory with no .git above it
		if r.code != 2 || !strings.Contains(r.stderr, "needs a git repository to key on") || r.stdout != "" {
			t.Errorf("%v: code=%d stdout=%q stderr=%q, want exit 2 saying binding needs a git repository to key on", args, r.code, r.stdout, r.stderr)
		}
	}
	// An unknown working directory (os.Getwd failed) is the same refusal.
	if r := runWorkflowTest([]string{"binding"}, ""); r.code != 2 || !strings.Contains(r.stderr, "needs a git repository to key on") {
		t.Errorf("empty cwd: code=%d stderr=%q, want exit 2 saying binding needs a git repository to key on", r.code, r.stderr)
	}
	if after := snapshotTree(t, e.state); !reflect.DeepEqual(after, before) {
		t.Errorf("a refused verb changed the state home:\nbefore %v\nafter  %v", before, after)
	}
}

// --- bind ------------------------------------------------------------------

func TestWorkflowBindRefusesAWorkflowThatCannotBeBound(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(t *testing.T, e bindEnv)
		args    []string
		want    string
		notWant string // a fragment that would mislabel the problem
	}{
		{"a workflow that does not exist", func(t *testing.T, e bindEnv) {}, []string{"--project", "proj-1", "--workflow", "wf-1"}, "does not exist", ""},
		{"a closed workflow", func(t *testing.T, e bindEnv) { e.workflowInStatus(t, "proj-1", "wf-1", "closed") }, []string{"--project", "proj-1", "--workflow", "wf-1"}, "closed", ""},
		{"a foreign workflow log", func(t *testing.T, e bindEnv) {
			writeFixtureFile(t, e.workflowLog("proj-1", "wf-1"), `{"hello":"world"}`+"\n")
		}, []string{"--project", "proj-1", "--workflow", "wf-1"}, "foreign", ""},
		{"a malformed workflow log", func(t *testing.T, e bindEnv) {
			writeFixtureFile(t, e.workflowLog("proj-1", "wf-1"), "not json\n")
		}, []string{"--project", "proj-1", "--workflow", "wf-1"}, "malformed", ""},
		{"a drifted workflow log", func(t *testing.T, e bindEnv) {
			e.workflowInStatus(t, "proj-1", "wf-1", "running")
			path := e.workflowLog("proj-1", "wf-1")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			// Change the first event, so the second one's prev_digest no longer matches.
			tampered := strings.Replace(string(data), `"at":"20`, `"at":"19`, 1)
			if tampered == string(data) {
				t.Fatal("test bug: the first event has no timestamp to change")
			}
			if err := os.WriteFile(path, []byte(tampered), 0o600); err != nil {
				t.Fatal(err)
			}
		}, []string{"--project", "proj-1", "--workflow", "wf-1"}, "drifted", ""},
		{"an unsafe project id", func(t *testing.T, e bindEnv) {}, []string{"--project", "../escape", "--workflow", "wf-1"}, "project_id", "not owned"},
		{"an unsafe workflow id", func(t *testing.T, e bindEnv) {}, []string{"--project", "proj-1", "--workflow", ".hidden"}, "workflow_id", "not owned"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newBindEnv(t)
			tt.prepare(t, e)
			r := runWorkflowTest(append([]string{"bind"}, tt.args...), e.repo)
			if r.code != 2 || !strings.Contains(r.stderr, tt.want) || r.stdout != "" {
				t.Fatalf("code=%d stdout=%q stderr=%q, want exit 2 and stderr containing %q", r.code, r.stdout, r.stderr, tt.want)
			}
			if tt.notWant != "" && strings.Contains(r.stderr, tt.notWant) {
				t.Errorf("stderr = %q, want it not to say %q: an unsafe identifier is invalid input, not an unreadable workflow", r.stderr, tt.notWant)
			}
			if _, err := os.Lstat(filepath.Join(e.state, "labdrian", "bindings")); err == nil {
				t.Errorf("a refused bind created the bindings directory")
			}
		})
	}
}

func TestWorkflowBindBindsAnActiveWorkflow(t *testing.T) {
	for _, status := range []string{"created", "running", "paused"} {
		t.Run(status, func(t *testing.T) {
			e := newBindEnv(t)
			e.workflowInStatus(t, "proj-1", "wf-1", status)
			before := time.Now().UTC().Add(-2 * time.Second)

			r := runWorkflowTest([]string{"bind", "--project", "proj-1", "--workflow", "wf-1"}, e.repo)
			if r.code != 0 || r.stderr != "" {
				t.Fatalf("code=%d stderr=%q, want exit 0 and no stderr", r.code, r.stderr)
			}
			report := decodeBindingReport(t, r)
			if report.Classification != "owned" || report.Binding == nil || report.Workflow != nil {
				t.Fatalf("report = %+v, want classification owned with a binding and no workflow section", report)
			}
			b := *report.Binding
			if b.Version != 1 || b.RepoKey != mustRepoKey(t, e.repo) || b.ProjectID != "proj-1" || b.WorkflowID != "wf-1" {
				t.Errorf("binding = %+v, want version 1 for this repository and proj-1/wf-1", b)
			}
			boundAt, err := time.Parse(time.RFC3339, b.BoundAt)
			if err != nil || !strings.HasSuffix(b.BoundAt, "Z") || boundAt.Before(before) || boundAt.After(time.Now().Add(2*time.Second)) {
				t.Errorf("bound_at = %q (%v), want a UTC timestamp for about now", b.BoundAt, err)
			}

			// What was printed is what was stored.
			want, err := b.Marshal()
			if err != nil {
				t.Fatal(err)
			}
			if got, err := os.ReadFile(e.bindingFile(t, e.repo)); err != nil || !bytes.Equal(got, want) {
				t.Errorf("stored binding = %q (%v), want the printed binding %q", got, err, want)
			}
		})
	}
}

func TestWorkflowBindWorksFromASubdirectory(t *testing.T) {
	e := newBindEnv(t)
	e.workflowInStatus(t, "proj-1", "wf-1", "running")
	sub := filepath.Join(e.repo, "deep", "er")
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	report := mustBindOK(t, sub, "proj-1", "wf-1")
	if report.Binding == nil || report.Binding.RepoKey != mustRepoKey(t, e.repo) {
		t.Fatalf("report = %+v, want the repository's key", report)
	}
}

func TestWorkflowBindIsIdempotent(t *testing.T) {
	e := newBindEnv(t)
	e.workflowInStatus(t, "proj-1", "wf-1", "running")
	first := mustBindOK(t, e.repo, "proj-1", "wf-1")
	stored, err := os.ReadFile(e.bindingFile(t, e.repo))
	if err != nil {
		t.Fatal(err)
	}

	second := mustBindOK(t, e.repo, "proj-1", "wf-1")
	if !reflect.DeepEqual(first, second) {
		t.Errorf("binding the same workflow again printed %+v, want the original %+v", second, first)
	}
	if again, err := os.ReadFile(e.bindingFile(t, e.repo)); err != nil || !bytes.Equal(again, stored) {
		t.Errorf("binding the same workflow again rewrote the file: %q (%v) vs %q", again, err, stored)
	}
}

func TestWorkflowBindRefusesAnotherWorkflowWhileTheBoundOneIsActive(t *testing.T) {
	for _, status := range []string{"created", "running", "paused"} {
		t.Run(status, func(t *testing.T) {
			e := newBindEnv(t)
			e.workflowInStatus(t, "proj-1", "wf-1", status)
			e.workflowInStatus(t, "proj-1", "wf-2", "running")
			mustBindOK(t, e.repo, "proj-1", "wf-1")
			stored, err := os.ReadFile(e.bindingFile(t, e.repo))
			if err != nil {
				t.Fatal(err)
			}

			r := runWorkflowTest([]string{"bind", "--project", "proj-1", "--workflow", "wf-2"}, e.repo)
			if r.code != 2 || r.stdout != "" {
				t.Fatalf("code=%d stdout=%q, want exit 2 and no stdout", r.code, r.stdout)
			}
			for _, want := range []string{"proj-1", "wf-1", status, "unbind"} {
				if !strings.Contains(r.stderr, want) {
					t.Errorf("stderr = %q, want it to name the bound workflow and how to free it (missing %q)", r.stderr, want)
				}
			}
			if again, err := os.ReadFile(e.bindingFile(t, e.repo)); err != nil || !bytes.Equal(again, stored) {
				t.Errorf("a refused bind changed the binding: %q (%v)", again, err)
			}
		})
	}
}

// TestWorkflowBindReplacesAStaleBinding pins when replacing is allowed without
// an unbind: only when the workflow the repository is bound to can no longer
// be followed, because it is closed, gone, or not a workflow log of ours.
func TestWorkflowBindReplacesAStaleBinding(t *testing.T) {
	tests := []struct {
		name  string
		stale func(t *testing.T, e bindEnv)
	}{
		{"the bound workflow is closed", func(t *testing.T, e bindEnv) {
			phase6MustExitZero(t, "close", []string{"close", "--project", "proj-1", "--workflow", "wf-1", "--outcome", "abandoned", "--reason", "done with it"}, e.dir)
		}},
		{"the bound workflow no longer exists", func(t *testing.T, e bindEnv) {
			if err := os.Remove(e.workflowLog("proj-1", "wf-1")); err != nil {
				t.Fatal(err)
			}
		}},
		{"the bound workflow log became foreign", func(t *testing.T, e bindEnv) {
			writeFixtureFile(t, e.workflowLog("proj-1", "wf-1"), `{"hello":"world"}`+"\n")
		}},
		{"the bound workflow log became malformed", func(t *testing.T, e bindEnv) {
			writeFixtureFile(t, e.workflowLog("proj-1", "wf-1"), "not json\n")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newBindEnv(t)
			e.workflowInStatus(t, "proj-1", "wf-1", "running")
			e.workflowInStatus(t, "proj-2", "wf-9", "running")
			mustBindOK(t, e.repo, "proj-1", "wf-1")
			tt.stale(t, e)

			report := mustBindOK(t, e.repo, "proj-2", "wf-9")
			if report.Binding == nil || report.Binding.ProjectID != "proj-2" || report.Binding.WorkflowID != "wf-9" {
				t.Fatalf("report = %+v, want the repository bound to proj-2/wf-9", report)
			}
			if loaded := loadStoredBinding(t, e.repo); loaded.Binding.WorkflowID != "wf-9" {
				t.Fatalf("stored binding = %+v, want wf-9", loaded.Binding)
			}
		})
	}
}

// TestWorkflowBindDoesNotReplaceABindingWhoseWorkflowCannotBeRead pins the
// safe side of "stale". A bound workflow whose log cannot be read, for example
// after a transient permission error, may still be active, so bind refuses and
// asks for an explicit unbind instead of moving the repository's sessions to
// another workflow. (A log that is corrupt or not ours can never be followed,
// and is replaced; see TestWorkflowBindReplacesAStaleBinding.)
func TestWorkflowBindDoesNotReplaceABindingWhoseWorkflowCannotBeRead(t *testing.T) {
	e := newBindEnv(t)
	e.workflowInStatus(t, "proj-1", "wf-1", "running")
	e.workflowInStatus(t, "proj-2", "wf-9", "running")
	mustBindOK(t, e.repo, "proj-1", "wf-1")

	log := e.workflowLog("proj-1", "wf-1")
	if err := os.Chmod(log, 0); err != nil {
		t.Fatalf("make %s unreadable: %v", log, err)
	}
	t.Cleanup(func() { _ = os.Chmod(log, 0o600) })
	if f, err := os.Open(log); err == nil {
		_ = f.Close()
		t.Skip("chmod 0 is not enforced for this process (root or an equivalent capability); the unreadable-log fault cannot be injected")
	}

	before := loadStoredBinding(t, e.repo)
	r := runWorkflowTest([]string{"bind", "--project", "proj-2", "--workflow", "wf-9"}, e.repo)
	if r.code != 2 {
		t.Fatalf("code=%d stderr=%q, want exit 2: the bound workflow cannot be read, so it may still be active", r.code, r.stderr)
	}
	for _, want := range []string{"wf-1", "proj-1", "unbind"} {
		if !strings.Contains(r.stderr, want) {
			t.Errorf("stderr = %q, want it to mention %q", r.stderr, want)
		}
	}
	after := loadStoredBinding(t, e.repo)
	if after.Classification != before.Classification || after.Binding != before.Binding {
		t.Fatalf("stored binding changed from %+v to %+v, want it untouched", before, after)
	}
}

// loadStoredBinding reads the stored binding of the repository at cwd through
// the projection store, independently of the CLI.
func loadStoredBinding(t *testing.T, cwd string) projection.Loaded {
	t.Helper()
	store, err := projection.NewStore()
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(mustRepoKey(t, cwd))
	if err != nil {
		t.Fatal(err)
	}
	return loaded
}

func TestWorkflowBindRefusesToOverwriteForeignAndMalformedBindingFiles(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"an unrelated JSON file", `{"hello":"world"}` + "\n", "foreign"},
		{"a file that is not JSON", "hand-written notes\n", "malformed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newBindEnv(t)
			e.workflowInStatus(t, "proj-1", "wf-1", "running")
			path := e.bindingFile(t, e.repo)
			writeFixtureFile(t, path, tt.content)

			r := runWorkflowTest([]string{"bind", "--project", "proj-1", "--workflow", "wf-1"}, e.repo)
			if r.code != 2 || !strings.Contains(r.stderr, tt.want) || r.stdout != "" {
				t.Fatalf("code=%d stdout=%q stderr=%q, want exit 2 naming %q", r.code, r.stdout, r.stderr, tt.want)
			}
			if got, err := os.ReadFile(path); err != nil || string(got) != tt.content {
				t.Errorf("the file was changed: %q (%v)", got, err)
			}
		})
	}
}

// --- unbind ----------------------------------------------------------------

func TestWorkflowUnbind(t *testing.T) {
	e := newBindEnv(t)
	e.workflowInStatus(t, "proj-1", "wf-1", "running")
	otherRepo := fixtureRepo(t, "other")

	removedOf := func(cwd string) bool {
		t.Helper()
		r := runWorkflowTest([]string{"unbind"}, cwd)
		if r.code != 0 || r.stderr != "" {
			t.Fatalf("unbind: code=%d stderr=%q, want exit 0 and no stderr", r.code, r.stderr)
		}
		var out struct {
			Removed *bool `json:"removed"`
		}
		if err := json.Unmarshal([]byte(r.stdout), &out); err != nil || out.Removed == nil {
			t.Fatalf("stdout = %q (%v), want a JSON object with a removed field", r.stdout, err)
		}
		return *out.Removed
	}

	if removedOf(e.repo) {
		t.Fatal("unbind with no binding reported removed=true")
	}
	if _, err := os.Lstat(filepath.Join(e.state, "labdrian", "bindings")); err == nil {
		t.Fatal("unbind with no binding created the bindings directory")
	}

	mustBindOK(t, e.repo, "proj-1", "wf-1")
	mustBindOK(t, otherRepo, "proj-1", "wf-1")

	if !removedOf(e.repo) {
		t.Fatal("unbind of a bound repository reported removed=false")
	}
	if removedOf(e.repo) {
		t.Fatal("a second unbind reported removed=true: unbind must be idempotent")
	}
	if loaded := loadStoredBinding(t, otherRepo); loaded.Classification != projection.ClassificationOwned {
		t.Fatalf("unbinding one repository unbound another: %+v", loaded)
	}
	// Freed, the repository can bind a different workflow.
	e.workflowInStatus(t, "proj-1", "wf-2", "running")
	mustBindOK(t, e.repo, "proj-1", "wf-2")
}

func TestWorkflowUnbindRefusesForeignAndMalformedBindingFiles(t *testing.T) {
	for name, tt := range map[string]struct{ content, want string }{
		"foreign":   {`{"hello":"world"}` + "\n", "foreign"},
		"malformed": {"hand-written notes\n", "malformed"},
	} {
		t.Run(name, func(t *testing.T) {
			e := newBindEnv(t)
			path := e.bindingFile(t, e.repo)
			writeFixtureFile(t, path, tt.content)

			r := runWorkflowTest([]string{"unbind"}, e.repo)
			if r.code != 2 || !strings.Contains(r.stderr, tt.want) || r.stdout != "" {
				t.Fatalf("code=%d stdout=%q stderr=%q, want exit 2 naming %q", r.code, r.stdout, r.stderr, tt.want)
			}
			if got, err := os.ReadFile(path); err != nil || string(got) != tt.content {
				t.Errorf("the file was changed: %q (%v)", got, err)
			}
		})
	}
}

// --- binding ---------------------------------------------------------------

func TestWorkflowBindingWhenNothingIsBound(t *testing.T) {
	e := newBindEnv(t)
	r := runWorkflowTest([]string{"binding"}, e.repo)
	if r.code != 0 || r.stderr != "" {
		t.Fatalf("code=%d stderr=%q, want exit 0 and no stderr", r.code, r.stderr)
	}
	report := decodeBindingReport(t, r)
	if report.Classification != "absent" || report.Binding != nil || report.Workflow != nil {
		t.Fatalf("report = %+v, want just the classification absent", report)
	}
}

func TestWorkflowBindingReportsTheBoundWorkflow(t *testing.T) {
	tests := []struct {
		name       string
		prepare    func(t *testing.T, e bindEnv)
		wantClass  string
		wantStatus string
		wantDetail bool
	}{
		{"a running workflow", func(t *testing.T, e bindEnv) {}, "owned", "running", false},
		{"a paused workflow", func(t *testing.T, e bindEnv) {
			phase6MustExitZero(t, "pause", []string{"pause", "--project", "proj-1", "--workflow", "wf-1"}, e.dir)
		}, "owned", "paused", false},
		{"a closed workflow", func(t *testing.T, e bindEnv) {
			phase6MustExitZero(t, "close", []string{"close", "--project", "proj-1", "--workflow", "wf-1", "--outcome", "abandoned", "--reason", "test"}, e.dir)
		}, "owned", "closed", false},
		{"a workflow whose log is gone", func(t *testing.T, e bindEnv) {
			if err := os.Remove(e.workflowLog("proj-1", "wf-1")); err != nil {
				t.Fatal(err)
			}
		}, "absent", "", false},
		{"a workflow log that became foreign", func(t *testing.T, e bindEnv) {
			writeFixtureFile(t, e.workflowLog("proj-1", "wf-1"), `{"hello":"world"}`+"\n")
		}, "foreign", "", true},
		{"a workflow log that became malformed", func(t *testing.T, e bindEnv) {
			writeFixtureFile(t, e.workflowLog("proj-1", "wf-1"), "not json\n")
		}, "malformed", "", true},
		{"a workflow store that cannot be read", func(t *testing.T, e bindEnv) {
			// The workflows directory becomes a symlink, which the workflow
			// store refuses to follow. The binding store is unaffected.
			workflows := filepath.Join(e.state, "labdrian", "workflows")
			moved := workflows + ".moved"
			if err := os.Rename(workflows, moved); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(moved, workflows); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
		}, "unavailable", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newBindEnv(t)
			e.workflowInStatus(t, "proj-1", "wf-1", "running")
			mustBindOK(t, e.repo, "proj-1", "wf-1")
			tt.prepare(t, e)
			before := snapshotTree(t, e.state)

			r := runWorkflowTest([]string{"binding"}, e.repo)
			if r.code != 0 || r.stderr != "" {
				t.Fatalf("code=%d stderr=%q, want exit 0 and no stderr: an unreadable workflow is reported as data", r.code, r.stderr)
			}
			report := decodeBindingReport(t, r)
			if report.Classification != "owned" || report.Binding == nil || report.Binding.WorkflowID != "wf-1" || report.Binding.ProjectID != "proj-1" {
				t.Fatalf("report = %+v, want an owned binding to proj-1/wf-1", report)
			}
			if report.Workflow == nil || report.Workflow.Classification != tt.wantClass || report.Workflow.Status != tt.wantStatus {
				t.Fatalf("workflow section = %+v, want classification %q and status %q", report.Workflow, tt.wantClass, tt.wantStatus)
			}
			if tt.wantDetail && report.Workflow.Detail == "" {
				t.Errorf("workflow section has no detail for a %s workflow, want the reason", tt.wantClass)
			}
			if after := snapshotTree(t, e.state); !reflect.DeepEqual(after, before) {
				t.Errorf("binding changed the state home:\nbefore %v\nafter  %v", before, after)
			}
		})
	}
}

func TestWorkflowBindingReportsAStateItCannotOwn(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, path string)
		want  string
	}{
		{"a foreign file", func(t *testing.T, path string) { writeFixtureFile(t, path, `{"hello":"world"}`+"\n") }, "foreign"},
		{"a malformed file", func(t *testing.T, path string) { writeFixtureFile(t, path, "not json\n") }, "malformed"},
		{"a directory in its place", func(t *testing.T, path string) {
			if err := os.MkdirAll(path, 0o700); err != nil {
				t.Fatal(err)
			}
		}, "unavailable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newBindEnv(t)
			tt.setup(t, e.bindingFile(t, e.repo))
			before := snapshotTree(t, e.state)

			r := runWorkflowTest([]string{"binding"}, e.repo)
			if r.code != 0 || r.stderr != "" {
				t.Fatalf("code=%d stderr=%q, want exit 0 and no stderr: a state that cannot be owned is reported, not refused", r.code, r.stderr)
			}
			var raw map[string]any
			if err := json.Unmarshal([]byte(r.stdout), &raw); err != nil {
				t.Fatal(err)
			}
			if raw["classification"] != tt.want || raw["detail"] == nil || raw["detail"] == "" {
				t.Fatalf("report = %v, want classification %q with a detail", raw, tt.want)
			}
			if _, has := raw["binding"]; has {
				t.Errorf("report carries a binding for a state that is not owned: %v", raw)
			}
			if _, has := raw["workflow"]; has {
				t.Errorf("report carries a workflow section without an owned binding: %v", raw)
			}
			if after := snapshotTree(t, e.state); !reflect.DeepEqual(after, before) {
				t.Errorf("binding changed the state home:\nbefore %v\nafter  %v", before, after)
			}
		})
	}
}

// --- worktrees and restarts ------------------------------------------------

// TestWorkflowBindingIsSharedAcrossWorktreesAndInvocations is the persistence
// guarantee at the command line. Every step is its own invocation (as each
// would be its own process), and the binding made in one worktree is the one
// every other worktree of the repository sees and can remove.
func TestWorkflowBindingIsSharedAcrossWorktreesAndInvocations(t *testing.T) {
	e := newBindEnv(t)
	e.workflowInStatus(t, "proj-1", "wf-1", "running")
	first := fixtureLinkedWorktree(t, e.repo, "wt1")
	second := fixtureLinkedWorktree(t, e.repo, "wt2")
	subdir := filepath.Join(second, "some", "dir")
	if err := os.MkdirAll(subdir, 0o700); err != nil {
		t.Fatal(err)
	}

	bound := mustBindOK(t, first, "proj-1", "wf-1")

	for name, cwd := range map[string]string{"the main checkout": e.repo, "the second worktree": second, "a subdirectory of it": subdir, "the first worktree": first} {
		r := runWorkflowTest([]string{"binding"}, cwd)
		if r.code != 0 {
			t.Fatalf("%s: binding code=%d stderr=%q", name, r.code, r.stderr)
		}
		report := decodeBindingReport(t, r)
		if report.Classification != "owned" || report.Binding == nil || *report.Binding != *bound.Binding {
			t.Errorf("%s: report = %+v, want the binding made from the first worktree, %+v", name, report, bound.Binding)
		}
		if report.Workflow == nil || report.Workflow.Status != "running" {
			t.Errorf("%s: workflow section = %+v, want the running workflow", name, report.Workflow)
		}
	}

	if r := runWorkflowTest([]string{"unbind"}, second); r.code != 0 || !strings.Contains(r.stdout, "true") {
		t.Fatalf("unbind from the second worktree: code=%d stdout=%q", r.code, r.stdout)
	}
	if report := decodeBindingReport(t, runWorkflowTest([]string{"binding"}, first)); report.Classification != "absent" {
		t.Fatalf("the first worktree still sees %+v after the second unbound", report)
	}
}

func TestWorkflowBindingKeepsRepositoriesApart(t *testing.T) {
	e := newBindEnv(t)
	e.workflowInStatus(t, "proj-1", "wf-1", "running")
	e.workflowInStatus(t, "proj-1", "wf-2", "running")
	other := fixtureRepo(t, "other")

	mustBindOK(t, e.repo, "proj-1", "wf-1")
	mustBindOK(t, other, "proj-1", "wf-2")

	if report := decodeBindingReport(t, runWorkflowTest([]string{"binding"}, e.repo)); report.Binding.WorkflowID != "wf-1" {
		t.Errorf("first repository is bound to %q, want wf-1", report.Binding.WorkflowID)
	}
	if report := decodeBindingReport(t, runWorkflowTest([]string{"binding"}, other)); report.Binding.WorkflowID != "wf-2" {
		t.Errorf("second repository is bound to %q, want wf-2", report.Binding.WorkflowID)
	}
}

// --- output failures -------------------------------------------------------

func TestWorkflowBindingVerbsReportAFailedStdoutWrite(t *testing.T) {
	e := newBindEnv(t)
	e.workflowInStatus(t, "proj-1", "wf-1", "running")
	for _, args := range [][]string{
		{"bind", "--project", "proj-1", "--workflow", "wf-1"},
		{"unbind"},
		{"binding"},
	} {
		var errBuf bytes.Buffer
		var codes []int
		runWorkflowCore(args, e.repo, failingMemoryWriter{}, &errBuf, func(c int) { codes = append(codes, c) })
		if !reflect.DeepEqual(codes, []int{1}) {
			t.Errorf("%v: exit calls = %v, want exactly [1]", args, codes)
		}
		if !strings.Contains(errBuf.String(), "broken pipe") || !strings.Contains(errBuf.String(), "workflow "+args[0]) {
			t.Errorf("%v: stderr = %q, want it to name the verb and the failed write", args, errBuf.String())
		}
	}
}

// --- what a binding is not -------------------------------------------------

// TestWorkflowBindingLeavesTheWorkflowLogAlone pins that a binding is a
// pointer: binding, rebinding, and unbinding never append to the workflow.
func TestWorkflowBindingLeavesTheWorkflowLogAlone(t *testing.T) {
	e := newBindEnv(t)
	e.workflowInStatus(t, "proj-1", "wf-1", "running")
	before, err := os.ReadFile(e.workflowLog("proj-1", "wf-1"))
	if err != nil {
		t.Fatal(err)
	}
	mustBindOK(t, e.repo, "proj-1", "wf-1")
	runWorkflowTest([]string{"binding"}, e.repo)
	runWorkflowTest([]string{"unbind"}, e.repo)

	if after, err := os.ReadFile(e.workflowLog("proj-1", "wf-1")); err != nil || !bytes.Equal(after, before) {
		t.Fatalf("the workflow log changed (%v):\n%q\nvs\n%q", err, after, before)
	}
	status := phase6MustUnmarshal(t, []byte(phase6MustExitZero(t, "status", []string{"status", "--project", "proj-1", "--workflow", "wf-1"}, e.dir).stdout))
	if status.Classification != string(workflow.ClassificationOwned) || status.Status != "running" {
		t.Fatalf("workflow status = %+v, want owned and running", status)
	}
}

// --- help ------------------------------------------------------------------

// TestUsageDocumentsTheBindingVerbs pins that the engine's help lists the three
// binding verbs with their arguments and says what their exit codes mean.
func TestUsageDocumentsTheBindingVerbs(t *testing.T) {
	text := captureUsage(t)
	for _, want := range []string{
		"engine workflow bind --project <id> --workflow <id>",
		"engine workflow unbind",
		"engine workflow binding",
		"$XDG_STATE_HOME/labdrian/bindings/",
		"SHA-256 of the git common directory",
		`prints {"removed": true|false}`,
		"exit 0 success, 2 refused/invalid (no git repository, a workflow that cannot be bound, a binding file that is not ours), 1 usage error",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("usage does not contain %q", want)
		}
	}
}
