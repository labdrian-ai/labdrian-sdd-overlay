package main

import (
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/shaper"
)

const shaperTestHandoffV3 = `{"version":3,"project_id":"standalone-shaper-handoff","goal_id":"goal-alpha","architecture":"Layered CLI.",` +
	`"stages":["Extract jsonstrict.","Refactor goal.Parse."],` +
	`"acceptance":[{"criterion":"go test ./... passes.","verification":{"check":"cd engine && go test -count=1 ./..."}},` +
	`{"criterion":"The help reads clearly.","verification":{"adjudication":"A maintainer judges the help text."}}],` +
	`"out_of_scope":["Runtime clearance UI."],` +
	`"roles":[{"role":"implementer","responsibility":"Writes the code."},{"role":"reviewer","responsibility":"Reviews the diff."}],` +
	`"tests":["go test ./... passes."],` +
	`"risks":[{"risk":"Scope creep.","mitigation":"Stick to the approved design."}],` +
	`"estimates":[{"stage":"Extract jsonstrict.","low_minutes":10,"high_minutes":20},{"stage":"Refactor goal.Parse.","low_minutes":15,"high_minutes":30}],` +
	`"memory_scope":"Project-scoped.","delivery_limit":"No delivery without human clearance."}`

// planCompleteWants are the phrases a v3 ready output must carry beyond the
// forgery limit: the full Phase 3 plan is present, not absent.
var planCompleteWants = []string{"Phase 3", "roles", "tests", "risks", "estimates", "memory_scope", "delivery_limit"}

// TestShaperHandoffV3_ReachesReadyEndToEnd drives assess, clearance record,
// and assess again through the CLI core against a real git worktree, proving
// a version 3 handoff reaches ready and states plan completeness, not
// absence.
func TestShaperHandoffV3_ReachesReadyEndToEnd(t *testing.T) {
	root, stateHome := shaperWorktree(t, shaperTestHandoffV3, shaperTestGoal("standalone-shaper-handoff", `[]`))

	before := runShaperTest(assessArgs(root), "")
	if before.code != 3 {
		t.Fatalf("assess before clearance exit %d, want 3 (draft); stdout %q stderr %q", before.code, before.stdout, before.stderr)
	}
	a := decodeAssess(t, before)
	if got := assessReasons(a); len(got) != 1 || got[0] != string(shaper.ReasonClearanceMissing) {
		t.Fatalf("blockers = %v, want only %s", got, shaper.ReasonClearanceMissing)
	}

	viewRun := runShaperTest(assessArgs(root, "--view"), "")
	if viewRun.code != 3 {
		t.Fatalf("assess --view exit %d; stderr %q", viewRun.code, viewRun.stderr)
	}
	for _, want := range []string{
		"labdrian shaper clearance view 3\n",
		"role 11\nimplementer\n",
		"role 8\nreviewer\n",
		"stage 19\nExtract jsonstrict.\n",
	} {
		if !strings.Contains(viewRun.stdout, want) {
			t.Errorf("presented view lacks %q:\n%s", want, viewRun.stdout)
		}
	}

	rec := recordFromAssess(t, a, "affirm", "tui", nil)
	r := runShaperTest(recordArgs(root, "--stdin"), rec)
	if r.code != 0 {
		t.Fatalf("record exit %d; stdout %q stderr %q", r.code, r.stdout, r.stderr)
	}
	_ = stateHome

	after := runShaperTest(assessArgs(root), "")
	if after.code != 0 {
		t.Fatalf("assess after clearance exit %d, want 0 (ready); stdout %q stderr %q", after.code, after.stdout, after.stderr)
	}
	ready := decodeAssess(t, after)
	if ready.State != "ready" || len(ready.Blockers) != 0 {
		t.Fatalf("state %q blockers %v, want ready with none", ready.State, assessReasons(ready))
	}
	for _, want := range append([]string{"not a signature", "same OS user", "any installed Pi extension"}, planCompleteWants...) {
		if !strings.Contains(after.stdout, want) {
			t.Errorf("ready JSON lacks %q:\n%s", want, after.stdout)
		}
	}
	if strings.Contains(after.stdout, "are absent") {
		t.Errorf("v3 ready JSON must not claim missing plan fields:\n%s", after.stdout)
	}

	viewReady := runShaperTest(assessArgs(root, "--view"), "")
	if viewReady.code != 0 || viewReady.stdout != viewRun.stdout {
		t.Errorf("ready --view exit %d or changed bytes", viewReady.code)
	}
	for _, want := range append([]string{"not a signature", "any installed Pi extension"}, planCompleteWants...) {
		if !strings.Contains(viewReady.stderr, want) {
			t.Errorf("ready --view stderr lacks %q:\n%s", want, viewReady.stderr)
		}
	}
}

// TestShaperHandoffV3_OD6OverlapIsRefusedNotFlagged proves that a v3 stage
// byte-identical to a Goal non_goal blocks readiness with the refused
// goal_non_goal_overlap reason and raises no human-review flag, even after an
// affirmative clearance covering zero flags.
func TestShaperHandoffV3_OD6OverlapIsRefusedNotFlagged(t *testing.T) {
	root, _ := shaperWorktree(t, shaperTestHandoffV3, shaperTestGoal("standalone-shaper-handoff", `["Extract jsonstrict."]`))
	a := decodeAssess(t, runShaperTest(assessArgs(root), ""))
	if len(a.Flags) != 0 {
		t.Fatalf("Flags = %v, want none: v3 overlap is refused, not flagged", a.Flags)
	}
	got := assessReasons(a)
	if !containsString(got, string(shaper.ReasonGoalNonGoalOverlap)) {
		t.Fatalf("blockers = %v, want to include %s", got, shaper.ReasonGoalNonGoalOverlap)
	}
}
