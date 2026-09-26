package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/shaper"
)

const shaperTestHandoffV2 = `{"version":2,"project_id":"standalone-shaper-handoff","goal_id":"goal-alpha","architecture":"Layered CLI.",` +
	`"stages":["Extract jsonstrict.","Refactor goal.Parse."],` +
	`"acceptance":[{"criterion":"go test ./... passes.","verification":{"check":"cd engine && go test -count=1 ./..."}},` +
	`{"criterion":"The help reads clearly.","verification":{"adjudication":"A maintainer judges the help text."}}],` +
	`"out_of_scope":["Runtime clearance UI."]}`

// planIncompleteWants are the phrases every ready output must carry beyond
// the forgery limit: the full Phase 3 plan outcome is not met and which
// plan fields are absent.
var planIncompleteWants = []string{"Phase 3", "not yet met", "roles", "tests", "risks", "estimates", "memory_scope", "delivery_limit"}

func TestShaperAssessOutput_ReadyStatesPlanIncompleteness(t *testing.T) {
	ready := shaper.Assessment{State: shaper.StateReady}
	for _, view := range []bool{false, true} {
		var out, errBuf bytes.Buffer
		writeShaperAssessment(&out, &errBuf, ready, clearanceReport{Status: "verified"}, view, 2)
		text := out.String() + errBuf.String()
		for _, want := range append([]string{"not a signature", "same OS user", "any installed Pi extension"}, planIncompleteWants...) {
			if !strings.Contains(text, want) {
				t.Errorf("view=%v: ready output lacks %q:\n%s", view, want, text)
			}
		}
	}
}

func TestShaperAssessOutput_DraftOmitsReadyDisclosure(t *testing.T) {
	var out, errBuf bytes.Buffer
	writeShaperAssessment(&out, &errBuf, shaper.Assessment{State: shaper.StateDraft}, clearanceReport{Status: "missing"}, false, 2)
	if strings.Contains(out.String(), "delivery_limit") {
		t.Errorf("draft output claims the ready disclosure:\n%s", out.String())
	}
}

// TestShaperHandoffV2_ReachesReadyEndToEnd drives assess, clearance record,
// and assess again through the CLI core against a real git worktree, with
// the clearance store isolated under a temporary XDG_STATE_HOME.
func TestShaperHandoffV2_ReachesReadyEndToEnd(t *testing.T) {
	root, stateHome := shaperWorktree(t, shaperTestHandoffV2, shaperTestGoal("standalone-shaper-handoff", `[]`))

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
	sum := sha256.Sum256([]byte(viewRun.stdout))
	if hex.EncodeToString(sum[:]) != a.Subject.ViewSHA256 {
		t.Fatalf("printed view does not hash to the assessed view_sha256")
	}
	for _, want := range []string{
		"labdrian shaper clearance view 2\n",
		"criterion 21\ngo test ./... passes.\n",
		"verification_check 35\ncd engine && go test -count=1 ./...\n",
		"verification_adjudication 34\nA maintainer judges the help text.\n",
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
	if !strings.HasPrefix(r.stdout, "shaper clearance record: stored affirm record at "+filepath.Join(stateHome, "labdrian", "shaper-clearance")) {
		t.Errorf("record not stored under the isolated state home: %q", r.stdout)
	}

	after := runShaperTest(assessArgs(root), "")
	if after.code != 0 {
		t.Fatalf("assess after clearance exit %d, want 0 (ready); stdout %q stderr %q", after.code, after.stdout, after.stderr)
	}
	ready := decodeAssess(t, after)
	if ready.State != "ready" || len(ready.Blockers) != 0 {
		t.Fatalf("state %q blockers %v, want ready with none", ready.State, assessReasons(ready))
	}
	for _, want := range append([]string{"not a signature", "same OS user", "any installed Pi extension"}, planIncompleteWants...) {
		if !strings.Contains(after.stdout, want) {
			t.Errorf("ready JSON lacks %q:\n%s", want, after.stdout)
		}
	}

	viewReady := runShaperTest(assessArgs(root, "--view"), "")
	if viewReady.code != 0 || viewReady.stdout != viewRun.stdout {
		t.Errorf("ready --view exit %d or changed bytes", viewReady.code)
	}
	for _, want := range append([]string{"not a signature", "any installed Pi extension"}, planIncompleteWants...) {
		if !strings.Contains(viewReady.stderr, want) {
			t.Errorf("ready --view stderr lacks %q:\n%s", want, viewReady.stderr)
		}
	}
}

func TestShaperHandoffV1_StaysDraftAfterClearance(t *testing.T) {
	root, _ := shaperWorktree(t, shaperTestHandoff, shaperTestGoal("standalone-shaper-handoff", `[]`))
	a := decodeAssess(t, runShaperTest(assessArgs(root), ""))
	if r := runShaperTest(recordArgs(root, "--stdin"), recordFromAssess(t, a, "affirm", "tui", nil)); r.code != 0 {
		t.Fatalf("record exit %d; stderr %q", r.code, r.stderr)
	}
	after := runShaperTest(assessArgs(root), "")
	got := decodeAssess(t, after)
	if after.code != 3 || got.State != "draft" {
		t.Fatalf("v1 after clearance: exit %d state %q, want draft", after.code, got.State)
	}
	if reasons := assessReasons(got); len(reasons) != 1 || reasons[0] != string(shaper.ReasonAcceptanceVerificationUnrepresentable) {
		t.Errorf("blockers = %v, want only the v1 acceptance cap", reasons)
	}
}
