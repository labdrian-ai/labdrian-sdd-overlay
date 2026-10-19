package main

import (
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/shaper"
)

// shaperTestHandoffV4 is shaperTestHandoffV3 at version 4, with memory_scope replaced by a
// memory directive.
var shaperTestHandoffV4 = strings.Replace(
	strings.Replace(shaperTestHandoffV3, `{"version":3,`, `{"version":4,`, 1),
	`"memory_scope":"Project-scoped."`, `"memory_directive":{"version":1,"scope":"goal","sources":["engram"],"write":"none"}`, 1)

// shaperTestGoalV3 is shaperTestGoal at Goal version 3, with memory_scope replaced by a
// memory directive.
func shaperTestGoalV3(projectID, nonGoals string) string {
	return strings.Replace(
		strings.Replace(shaperTestGoal(projectID, nonGoals), `{"version":2,`, `{"version":3,`, 1),
		`"memory_scope":"Project-scoped."`, `"memory_directive":{"version":1,"scope":"project","sources":["engram","longterm-mem"],"write":"none"}`, 1)
}

// A version 4 handoff bound to a version 3 Goal reaches ready through the CLI, shows both
// directives in a view of version 4, and prints the disclosure that names memory_directive.
func TestShaperHandoffV4_ReachesReadyEndToEnd(t *testing.T) {
	root, _ := shaperWorktree(t, shaperTestHandoffV4, shaperTestGoalV3("standalone-shaper-handoff", `[]`))

	before := runShaperTest(assessArgs(root), "")
	if before.code != 3 {
		t.Fatalf("assess before clearance exit %d, want 3 (draft); stdout %q stderr %q", before.code, before.stdout, before.stderr)
	}
	a := decodeAssess(t, before)
	if got := assessReasons(a); len(got) != 1 || got[0] != string(shaper.ReasonClearanceMissing) {
		t.Fatalf("blockers = %v, want only %s", got, shaper.ReasonClearanceMissing)
	}

	viewRun := runShaperTest(assessArgs(root, "--view"), "")
	for _, want := range []string{
		"labdrian shaper clearance view 4\n",
		"memory_directive 48\nversion=1 scope=goal sources=[engram] write=none\n",
		"goal_memory_directive 64\nversion=1 scope=project sources=[engram,longterm-mem] write=none\n",
	} {
		if !strings.Contains(viewRun.stdout, want) {
			t.Errorf("presented view lacks %q:\n%s", want, viewRun.stdout)
		}
	}

	if r := runShaperTest(recordArgs(root, "--stdin"), recordFromAssess(t, a, "affirm", "tui", nil)); r.code != 0 {
		t.Fatalf("record exit %d; stdout %q stderr %q", r.code, r.stdout, r.stderr)
	}
	after := runShaperTest(assessArgs(root), "")
	if after.code != 0 {
		t.Fatalf("assess after clearance exit %d, want 0 (ready); stdout %q stderr %q", after.code, after.stdout, after.stderr)
	}
	if !strings.Contains(after.stdout, "memory_directive, and delivery_limit are present") || strings.Contains(after.stdout, "memory_scope") {
		t.Errorf("v4 ready JSON must carry the version 4 disclosure:\n%s", after.stdout)
	}
}

// A version 4 handoff bound to a version 2 Goal stays a draft: the Goal is refused for its
// version, so no view is presented.
func TestShaperHandoffV4_RefusesAVersion2Goal(t *testing.T) {
	root, _ := shaperWorktree(t, shaperTestHandoffV4, shaperTestGoal("standalone-shaper-handoff", `[]`))
	run := runShaperTest(assessArgs(root), "")
	if run.code != 3 {
		t.Fatalf("assess exit %d, want 3 (draft); stdout %q stderr %q", run.code, run.stdout, run.stderr)
	}
	if got := assessReasons(decodeAssess(t, run)); !containsString(got, string(shaper.ReasonGoalInvalid)) {
		t.Errorf("blockers = %v, want to include %s", got, shaper.ReasonGoalInvalid)
	}
	if !strings.Contains(run.stdout, "goal has version 2, want 3") {
		t.Errorf("assess output does not name the Goal version it wants:\n%s", run.stdout)
	}
}
