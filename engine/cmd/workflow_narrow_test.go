package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/sha256hex"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

// memoryTestGoalV3JSON is memoryTestGoalJSON at Goal version 3, with directive as its
// memory_directive.
func memoryTestGoalV3JSON(projectID, goalID, directive string) string {
	return strings.Replace(strings.Replace(memoryTestGoalJSON(projectID, goalID), `"version": 2,`, `"version": 3,`, 1),
		`"memory_scope": "goal"`, `"memory_directive": `+directive, 1)
}

// workflowEvents is the events of the log of proj/wf, which must be owned.
func workflowEvents(t *testing.T, project, wf string) []workflow.WorkflowEvent {
	t.Helper()
	log, ok := workflowLog(t, project, wf)
	if !ok {
		t.Fatalf("there is no log of %s/%s", project, wf)
	}
	loaded := workflow.ClassifyLog(project, wf, []byte(log))
	if loaded.Classification != workflow.ClassificationOwned {
		t.Fatalf("the log is %s (%s), want owned", loaded.Classification, loaded.Detail)
	}
	return loaded.Events
}

// directiveJSONOf is the compact JSON of a directive's wire form, as the log writes it.
func directiveJSONOf(scope string, sources ...string) string {
	quoted := make([]string, len(sources))
	for i, s := range sources {
		quoted[i] = `"` + s + `"`
	}
	return `{"version":1,"scope":"` + scope + `","sources":[` + strings.Join(quoted, ",") + `],"write":"none"}`
}

// workflow create --handoff records the digest of the handoff file's bytes and its directive beside
// the Goal's (C23 M3, decision 3), and prints them in the state.
func TestWorkflowCreateRecordsTheShaperHandoffAndTheGoalDirective(t *testing.T) {
	workflowTestStateHome(t)
	dir := t.TempDir()
	goalPath := writeMemoryTestFile(t, dir, "goal.json", shaperTestGoalV3("standalone-shaper-handoff", `[]`))
	handoffPath := writeMemoryTestFile(t, dir, "handoff.json", shaperTestHandoffV4)

	r := runWorkflowTest([]string{"create", "--project", "standalone-shaper-handoff", "--workflow", "wf-1", "--goal", goalPath, "--profile", "odd", "--handoff", handoffPath}, dir)
	if r.code != 0 || r.stderr != "" {
		t.Fatalf("create: code=%d stderr=%q, want exit 0 and nothing on stderr", r.code, r.stderr)
	}
	created := workflowEvents(t, "standalone-shaper-handoff", "wf-1")[0]
	if want := sha256hex.Sum([]byte(shaperTestHandoffV4)); created.HandoffDigest != want {
		t.Errorf("handoff_digest = %q, want the SHA-256 of the file, %q", created.HandoffDigest, want)
	}
	log, _ := workflowLog(t, "standalone-shaper-handoff", "wf-1")
	for _, want := range []string{
		`"goal_directive":` + directiveJSONOf("project", "engram", "longterm-mem"),
		`"handoff_directive":` + directiveJSONOf("goal", "engram"),
		`{"version":3,`,
	} {
		if !strings.Contains(log, want) {
			t.Errorf("the log lacks %s:\n%s", want, log)
		}
	}
	var state map[string]json.RawMessage
	if err := json.Unmarshal([]byte(r.stdout), &state); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, r.stdout)
	}
	for _, field := range []string{"goal_directive", "handoff_digest", "handoff_directive"} {
		if _, ok := state[field]; !ok {
			t.Errorf("the printed state lacks %s:\n%s", field, r.stdout)
		}
	}
}

func TestWorkflowCreateRefusesAHandoffItCannotRecordAndWritesNothing(t *testing.T) {
	tests := []struct {
		name    string
		handoff string
		missing bool
		want    string
	}{
		{"a version 3 handoff", shaperTestHandoffV3, false,
			"error: workflow create: workflow lifecycle: cannot create: the shaper handoff does not fit this workflow: it is of version 3, and only a version 4 handoff carries a memory directive\n"},
		{"a handoff of another goal", strings.Replace(shaperTestHandoffV4, `"goal_id":"goal-alpha"`, `"goal_id":"goal-beta"`, 1), false,
			`error: workflow create: workflow lifecycle: cannot create: the shaper handoff does not fit this workflow: its goal_id is "goal-beta" and the goal's is "goal-alpha"` + "\n"},
		{"a handoff that does not parse", `{"version":4}`, false, "error: workflow create: parse --handoff: "},
		{"a handoff that is not there", "", true, "error: workflow create: read --handoff: "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workflowTestStateHome(t)
			dir := t.TempDir()
			goalPath := writeMemoryTestFile(t, dir, "goal.json", shaperTestGoalV3("standalone-shaper-handoff", `[]`))
			handoffPath := dir + "/absent.json"
			if !tt.missing {
				handoffPath = writeMemoryTestFile(t, dir, "handoff.json", tt.handoff)
			}
			r := runWorkflowTest([]string{"create", "--project", "standalone-shaper-handoff", "--workflow", "wf-1", "--goal", goalPath, "--profile", "odd", "--handoff", handoffPath}, dir)
			if r.code != 2 || r.stdout != "" || !strings.HasPrefix(r.stderr, tt.want) || strings.Count(r.stderr, "\n") != 1 {
				t.Fatalf("create: code=%d stdout=%q stderr=%q, want exit 2 and one line beginning %q", r.code, r.stdout, r.stderr, tt.want)
			}
			if _, ok := workflowLog(t, "standalone-shaper-handoff", "wf-1"); ok {
				t.Error("the refused create wrote a log")
			}
		})
	}
}

// appendRole appends one role handoff record to proj-1/goal-1/chain-1.
func appendRole(t *testing.T, record string) {
	t.Helper()
	if a := runRolesTest([]string{"append", "--project", "proj-1", "--goal", "goal-1", "--chain", "chain-1", "--stdin"}, record); a.code != 0 {
		t.Fatalf("roles append: code=%d stderr=%q, want 0", a.code, a.stderr)
	}
}

// workflow narrow appends one narrowed event per role handoff the chain received, refuses when
// there is none, and refuses a directive that widens, writing nothing; memory plan --project
// --workflow prints the plan the log makes after each.
func TestWorkflowNarrowRecordsEachLaterRoleHandoff(t *testing.T) {
	workflowTestStateHome(t)
	dir := t.TempDir()
	goalPath := writeMemoryTestFile(t, dir, "goal.json", memoryTestGoalV3JSON("proj-1", "goal-1", `{"version": 1, "scope": "project", "sources": ["engram", "longterm-mem"], "write": "none"}`))
	r1 := rolesHandoffJSON(1, "", "shaper", "estimator", "completed", "")
	appendRole(t, r1)
	flags := []string{"--project", "proj-1", "--workflow", "wf-1"}
	if r := runWorkflowTest(append([]string{"create", "--goal", goalPath, "--profile", "odd", "--role-chain", "chain-1"}, flags...), dir); r.code != 0 {
		t.Fatalf("create: code=%d stderr=%q, want 0", r.code, r.stderr)
	}
	plan := func() string {
		t.Helper()
		m := runMemoryTest(append([]string{"plan"}, flags...))
		if m.code != 0 || m.stderr != "" {
			t.Fatalf("memory plan: code=%d stderr=%q, want 0", m.code, m.stderr)
		}
		return m.stdout
	}
	if got := plan(); !strings.Contains(got, `"scope": "project"`) || !strings.Contains(got, `"longterm-mem"`) || strings.Contains(got, "procedural-skills") {
		t.Errorf("the plan after create is %s, want the goal's directive over the ceiling", got)
	}

	before, _ := workflowLog(t, "proj-1", "wf-1")
	nothing := runWorkflowTest(append([]string{"narrow"}, flags...), dir)
	if nothing.code != 2 || nothing.stdout != "" || !strings.HasPrefix(nothing.stderr, "error: workflow narrow: workflow lifecycle: narrow: the role chain has no handoff the workflow has not recorded: its head is ") {
		t.Fatalf("narrow with nothing new: code=%d stdout=%q stderr=%q, want exit 2 and the refusal", nothing.code, nothing.stdout, nothing.stderr)
	}
	if after, _ := workflowLog(t, "proj-1", "wf-1"); after != before {
		t.Fatalf("the refused narrow changed the log")
	}

	r2 := rolesV2HandoffJSON(2, sha256hex.Sum([]byte(r1)), "estimator", "builder", rolesGoalEngramDirective)
	appendRole(t, r2)
	n := runWorkflowTest(append([]string{"narrow"}, flags...), dir)
	if n.code != 0 || n.stderr != "" {
		t.Fatalf("narrow: code=%d stderr=%q, want 0", n.code, n.stderr)
	}
	head := sha256hex.Sum([]byte(r2))
	if !strings.Contains(n.stdout, `"narrowed_role_chain_head": "`+head+`"`) || !strings.Contains(n.stdout, `"status": "created"`) {
		t.Errorf("narrow printed %s, want the created state with the new head %s", n.stdout, head)
	}
	events := workflowEvents(t, "proj-1", "wf-1")
	last := events[len(events)-1]
	if len(events) != 2 || last.Kind != workflow.KindNarrowed || last.Narrowed.RoleChainHead != head || last.Narrowed.MemoryDirective == nil {
		t.Fatalf("the log is %+v, want created then one narrowed event of head %s with its directive", events, head)
	}
	if got := plan(); !strings.Contains(got, `"scope": "goal"`) || !strings.Contains(got, `"goal_id": "goal-1"`) || strings.Contains(got, "longterm-mem") {
		t.Errorf("the plan after narrow is %s, want the role's goal [engram]", got)
	}

	r3 := rolesV2HandoffJSON(3, sha256hex.Sum([]byte(r2)), "builder", "reviewer", `{"version": 1, "scope": "project", "sources": ["procedural-skills"], "write": "none"}`)
	appendRole(t, r3)
	before, _ = workflowLog(t, "proj-1", "wf-1")
	wide := runWorkflowTest(append([]string{"narrow"}, flags...), dir)
	want := "error: workflow narrow: workflow lifecycle: narrow: the role handoff memory directive widens the plan: adds source procedural-skills not present in the current scope\n"
	if wide.code != 2 || wide.stdout != "" || wide.stderr != want {
		t.Fatalf("widening narrow: code=%d stdout=%q stderr=%q, want exit 2 and %q", wide.code, wide.stdout, wide.stderr, want)
	}
	if after, _ := workflowLog(t, "proj-1", "wf-1"); after != before {
		t.Fatalf("the refused narrow changed the log")
	}
}

func TestWorkflowNarrowRefusesAVersion2LogAndBadUsage(t *testing.T) {
	workflowTestStateHome(t)
	usage := runWorkflowTest([]string{"narrow", "--project", "proj-1"}, "")
	if usage.code != 1 || usage.stderr != "error: workflow narrow: --workflow is required\n" {
		t.Fatalf("narrow without --workflow: code=%d stderr=%q, want exit 1 and the usage error", usage.code, usage.stderr)
	}
	absent := runWorkflowTest([]string{"narrow", "--project", "proj-1", "--workflow", "wf-1"}, "")
	if absent.code != 2 || !strings.HasPrefix(absent.stderr, "error: workflow narrow: workflow lifecycle: workflow state is not owned") {
		t.Fatalf("narrow of no workflow: code=%d stderr=%q, want exit 2", absent.code, absent.stderr)
	}
}

func TestMemoryPlanRefusesAWorkflowWithTheOtherFlags(t *testing.T) {
	for _, extra := range [][]string{
		{"--profile", "odd"},
		{"--goal", "g.json"},
		{"--goal-directive", "d.json"},
		{"--handoff-directive", "d.json"},
	} {
		args := append([]string{"plan", "--project", "proj-1", "--workflow", "wf-1"}, extra...)
		r := runMemoryTest(args)
		want := "error: memory plan: --project and --workflow read the plan from a workflow's log and do not combine with --profile, --goal, --goal-directive or --handoff-directive\n"
		if r.code != 1 || r.stdout != "" || r.stderr != want {
			t.Errorf("%v: code=%d stdout=%q stderr=%q, want exit 1 and %q", extra, r.code, r.stdout, r.stderr, want)
		}
	}
	for _, args := range [][]string{{"plan", "--project", "proj-1"}, {"plan", "--workflow", "wf-1"}} {
		r := runMemoryTest(args)
		if r.code != 1 || r.stderr != "error: memory plan: --project and --workflow go together\n" {
			t.Errorf("%v: code=%d stderr=%q, want exit 1 and that the two go together", args, r.code, r.stderr)
		}
	}
}

func TestMemoryPlanOfAWorkflowThatIsNotThereIsRefused(t *testing.T) {
	workflowTestStateHome(t)
	r := runMemoryTest([]string{"plan", "--project", "proj-1", "--workflow", "wf-1"})
	if r.code != 2 || r.stdout != "" || r.stderr != "error: memory plan: workflow proj-1/wf-1 is not owned: its log is absent\n" {
		t.Fatalf("code=%d stdout=%q stderr=%q, want exit 2 and that there is no workflow", r.code, r.stdout, r.stderr)
	}
}

// A workflow created before the narrowers (a version 2 log) is planned at its profile's ceiling,
// exactly as memory plan --profile plans it for its project and goal.
func TestMemoryPlanOfAVersion2WorkflowIsTheCeiling(t *testing.T) {
	workflowTestStateHome(t)
	dir := t.TempDir()
	goalPath := writeMemoryTestFile(t, dir, "goal.json", memoryTestGoalJSON("proj-1", "goal-1"))
	if r := runWorkflowTest([]string{"create", "--project", "proj-1", "--workflow", "wf-1", "--goal", goalPath, "--profile", "sdd"}, dir); r.code != 0 {
		t.Fatalf("create: code=%d stderr=%q", r.code, r.stderr)
	}
	// Rewrite the log as version 2: the same events, chained again.
	events := workflowEvents(t, "proj-1", "wf-1")
	events[0].Version = workflow.EventVersionSnapshot
	line, err := events[0].MarshalLine()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(os.Getenv("XDG_STATE_HOME"), "labdrian", "workflows", "proj-1", "wf-1.jsonl")
	if err := os.WriteFile(path, line, 0o600); err != nil {
		t.Fatal(err)
	}
	if v2 := workflowEvents(t, "proj-1", "wf-1"); v2[0].Version != workflow.EventVersionSnapshot {
		t.Fatalf("the rewritten log is of version %d, want 2", v2[0].Version)
	}
	narrow := runWorkflowTest([]string{"narrow", "--project", "proj-1", "--workflow", "wf-1"}, dir)
	if narrow.code != 2 || narrow.stderr != "error: workflow narrow: workflow lifecycle: narrow: narrowers need a workflow created by this version: the log is of version 2, and narrowers need version 3: create the workflow again\n" {
		t.Errorf("narrow of a version 2 log: code=%d stderr=%q, want exit 2 and to create it again", narrow.code, narrow.stderr)
	}
	status := runWorkflowTest([]string{"status", "--project", "proj-1", "--workflow", "wf-1"}, dir)
	if status.code != 0 || !strings.Contains(status.stdout, `"classification": "owned"`) || strings.Contains(status.stdout, "narrowed") {
		t.Errorf("status of a version 2 log: code=%d stdout=%q, want it owned as before", status.code, status.stdout)
	}

	byLog := runMemoryTest([]string{"plan", "--project", "proj-1", "--workflow", "wf-1"})
	byProfile := runMemoryTest([]string{"plan", "--profile", "sdd", "--goal", goalPath})
	if byLog.code != 0 || byProfile.code != 0 {
		t.Fatalf("plans: %d %q / %d %q, want both 0", byLog.code, byLog.stderr, byProfile.code, byProfile.stderr)
	}
	// The workflow's project is its recorded identity; the flag form names the Goal's project_id.
	if strings.Replace(byLog.stdout, events[0].ProjectIdentity, "proj-1", 1) != byProfile.stdout {
		t.Errorf("the plan of the version 2 log is\n%s\nwant the ceiling\n%s", byLog.stdout, byProfile.stdout)
	}
}
