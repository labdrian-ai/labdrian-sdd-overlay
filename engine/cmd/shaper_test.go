package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/gitprov"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/hookwire"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/shaper"
)

// The shaper binds the three worktree paths and nothing else. The composition root
// maps the git adapter's observation into that value, so HEAD (informational) and
// Linked (derivable from the paths) never reach the shaper: two observations that
// differ only in them are the same provenance.
func TestWorktreeProvenanceFromCarriesOnlyTheBoundPaths(t *testing.T) {
	obs := gitprov.Observation{
		Toplevel:  "/work/tree",
		GitDir:    "/work/main/.git/worktrees/tree",
		CommonDir: "/work/main/.git",
		Head:      "0123456789abcdef0123456789abcdef01234567",
		Linked:    true,
	}
	want := shaper.WorktreeProvenance{
		Toplevel:  "/work/tree",
		GitDir:    "/work/main/.git/worktrees/tree",
		CommonDir: "/work/main/.git",
	}
	if got := worktreeProvenanceFrom(obs); got != want {
		t.Fatalf("worktreeProvenanceFrom = %#v, want %#v", got, want)
	}

	moved := obs
	moved.Head = "fedcba9876543210fedcba9876543210fedcba98"
	moved.Linked = false
	if got := worktreeProvenanceFrom(moved); got != want {
		t.Errorf("provenance changed with HEAD and Linked alone: %#v, want %#v", got, want)
	}
}

const shaperTestHandoff = `{"version":1,"project_id":"standalone-shaper-handoff","goal_id":"goal-alpha","architecture":"Layered CLI.","stages":["Extract jsonstrict.","Refactor goal.Parse."],"acceptance":["go test ./... passes."],"out_of_scope":["Runtime clearance UI."]}`

func shaperTestGoal(projectID, nonGoals string) string {
	return `{"version":2,"project_id":"` + projectID + `","goal_id":"goal-alpha",` +
		`"objective":"Bind the handoff to real intent.","scope":"One project.",` +
		`"constraints":[],"non_goals":` + nonGoals + `,"acceptance_criteria":["Binding succeeds."],` +
		`"memory_scope":"Project-scoped.","runtime_scope":"Deferred.","delivery_boundary":"No delivery."}`
}

// shaperWorktree creates a committed git worktree holding handoff.json and
// goal.json and isolates the clearance store in a fresh XDG_STATE_HOME. It
// returns the symlink-resolved root and the state home.
func shaperWorktree(t *testing.T, handoff, goal string) (root, stateHome string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git unavailable: %v", err)
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	stateHome = t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateHome)
	for _, args := range [][]string{
		{"init", "-q"},
		{"-c", "user.name=t", "-c", "user.email=t@example.invalid", "commit", "-q", "--allow-empty", "-m", "init"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "handoff.json"), []byte(handoff), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "goal.json"), []byte(goal), 0o600); err != nil {
		t.Fatal(err)
	}
	return root, stateHome
}

type shaperRun struct {
	code   int
	stdout string
	stderr string
}

func runShaperTest(args []string, stdin string) shaperRun {
	return runShaperTestWith(testDeps(), args, stdin)
}

func runShaperTestWith(d deps, args []string, stdin string) shaperRun {
	var out, errBuf bytes.Buffer
	code := -1
	exited := false
	runShaperCore(d, args, strings.NewReader(stdin), &out, &errBuf, func(c int) {
		if !exited {
			code = c
			exited = true
		}
	})
	if !exited {
		code = 0
	}
	return shaperRun{code: code, stdout: out.String(), stderr: errBuf.String()}
}

func assessArgs(root string, extra ...string) []string {
	return append([]string{"assess", "--root", root, "--handoff", "handoff.json", "--goal", "goal.json"}, extra...)
}

func recordArgs(root string, extra ...string) []string {
	return append([]string{"clearance", "record", "--root", root, "--handoff", "handoff.json", "--goal", "goal.json"}, extra...)
}

type assessOutput struct {
	State    string `json:"state"`
	Blockers []struct {
		Reason string `json:"reason"`
		Kind   string `json:"kind"`
		Detail string `json:"detail"`
	} `json:"blockers"`
	Flags []struct {
		ID string `json:"id"`
	} `json:"flags"`
	Subject *struct {
		ProjectID        string `json:"project_id"`
		GoalID           string `json:"goal_id"`
		GoalSHA256       string `json:"goal_sha256"`
		HandoffSHA256    string `json:"handoff_sha256"`
		ProvenanceSHA256 string `json:"provenance_sha256"`
		ViewSHA256       string `json:"view_sha256"`
	} `json:"subject"`
	Clearance struct {
		Status string `json:"status"`
		Detail string `json:"detail"`
	} `json:"clearance"`
	Authority  string  `json:"authority"`
	Disclosure *string `json:"disclosure"`
}

func decodeAssess(t *testing.T, r shaperRun) assessOutput {
	t.Helper()
	var out assessOutput
	dec := json.NewDecoder(strings.NewReader(r.stdout))
	if err := dec.Decode(&out); err != nil {
		t.Fatalf("decode assess output %q: %v (stderr %q)", r.stdout, err, r.stderr)
	}
	return out
}

func assessReasons(a assessOutput) []string {
	var out []string
	for _, b := range a.Blockers {
		out = append(out, b.Reason)
	}
	return out
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// listFiles returns every regular file under dir.
func listFiles(t *testing.T, dir string) []string {
	t.Helper()
	var files []string
	filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			files = append(files, p)
		}
		return nil
	})
	return files
}

// recordFromAssess builds a clearance record JSON bound to the subject and
// flags assess reports, as the Pi dialog will.
func recordFromAssess(t *testing.T, a assessOutput, decision, mode string, extra map[string]any) string {
	t.Helper()
	if a.Subject == nil {
		t.Fatalf("assess reported no subject")
	}
	resolutions := []map[string]string{}
	for _, f := range a.Flags {
		resolutions = append(resolutions, map[string]string{"flag_id": f.ID, "reason": "Intended.", "evidence": "Read the view."})
	}
	rec := map[string]any{
		"version": 1,
		"subject": map[string]string{
			"project_id":        a.Subject.ProjectID,
			"goal_id":           a.Subject.GoalID,
			"goal_sha256":       a.Subject.GoalSHA256,
			"handoff_sha256":    a.Subject.HandoffSHA256,
			"provenance_sha256": a.Subject.ProvenanceSHA256,
			"view_sha256":       a.Subject.ViewSHA256,
		},
		"flag_resolutions": resolutions,
		"decision":         decision,
		"channel":          map[string]any{"runtime": "pi", "mode": mode, "verified": false},
	}
	for k, v := range extra {
		rec[k] = v
	}
	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestShaperAssess_DraftWithoutClearanceIsReadOnly(t *testing.T) {
	root, stateHome := shaperWorktree(t, shaperTestHandoff, shaperTestGoal("standalone-shaper-handoff", `["Extract jsonstrict."]`))
	r := runShaperTest(assessArgs(root), "")
	if r.code != 3 {
		t.Fatalf("exit %d, want 3 (draft); stdout %q stderr %q", r.code, r.stdout, r.stderr)
	}
	a := decodeAssess(t, r)
	if a.State != "draft" {
		t.Errorf("state %q, want draft", a.State)
	}
	reasons := assessReasons(a)
	for _, want := range []string{"flags_unresolved", "clearance_missing", "acceptance_verification_unrepresentable"} {
		if !containsString(reasons, want) {
			t.Errorf("blockers %v lack %q", reasons, want)
		}
	}
	if len(a.Flags) != 1 || a.Flags[0].ID != "goal_non_goal_overlap:stages:1" {
		t.Errorf("flags %+v", a.Flags)
	}
	if a.Subject == nil || a.Subject.ViewSHA256 == "" {
		t.Fatalf("subject missing: %+v", a.Subject)
	}
	if a.Clearance.Status != "absent" {
		t.Errorf("clearance status %q, want absent", a.Clearance.Status)
	}
	if a.Disclosure != nil {
		t.Errorf("draft output carries the ready disclosure")
	}
	if !strings.Contains(a.Authority, "no execution authority") {
		t.Errorf("authority %q", a.Authority)
	}
	if files := listFiles(t, stateHome); len(files) != 0 {
		t.Errorf("assess wrote %v", files)
	}
}

func TestShaperAssess_ViewPrintsExactRenderedBytes(t *testing.T) {
	root, _ := shaperWorktree(t, shaperTestHandoff, shaperTestGoal("standalone-shaper-handoff", `[]`))
	a := decodeAssess(t, runShaperTest(assessArgs(root), ""))
	r := runShaperTest(assessArgs(root, "--view"), "")
	if r.code != 3 {
		t.Fatalf("exit %d, want 3; stderr %q", r.code, r.stderr)
	}
	if !strings.HasPrefix(r.stdout, "labdrian shaper clearance view 1\n") {
		t.Fatalf("--view stdout is not a rendered view: %q", r.stdout)
	}
	if got := shaper.ViewDigest([]byte(r.stdout)); got != a.Subject.ViewSHA256 {
		t.Errorf("--view bytes digest %s, subject view_sha256 %s", got, a.Subject.ViewSHA256)
	}
}

func TestShaperAssess_InvalidHandoffExits2(t *testing.T) {
	root, _ := shaperWorktree(t, `{"version":1}`, shaperTestGoal("standalone-shaper-handoff", `[]`))
	r := runShaperTest(assessArgs(root), "")
	if r.code != 2 {
		t.Fatalf("exit %d, want 2; stdout %q stderr %q", r.code, r.stdout, r.stderr)
	}
	a := decodeAssess(t, r)
	if a.State != "invalid" || !containsString(assessReasons(a), "handoff_invalid") {
		t.Errorf("assess = %+v", a)
	}
}

func TestShaperAssess_GoalIdentityMismatchIsARefusedBlocker(t *testing.T) {
	root, _ := shaperWorktree(t, shaperTestHandoff, shaperTestGoal("other-project", `[]`))
	r := runShaperTest(assessArgs(root), "")
	if r.code != 3 {
		t.Fatalf("exit %d, want 3; stdout %q stderr %q", r.code, r.stdout, r.stderr)
	}
	a := decodeAssess(t, r)
	if !containsString(assessReasons(a), "goal_identity_mismatch") || a.Subject != nil {
		t.Errorf("assess = %+v", a)
	}
}

func TestShaperAssess_UnobservedWorktreeIsDraft(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	os.WriteFile(filepath.Join(root, "handoff.json"), []byte(shaperTestHandoff), 0o600)
	os.WriteFile(filepath.Join(root, "goal.json"), []byte(shaperTestGoal("standalone-shaper-handoff", `[]`)), 0o600)
	r := runShaperTest(assessArgs(root), "")
	if r.code != 3 {
		t.Fatalf("exit %d, want 3; stderr %q", r.code, r.stderr)
	}
	if a := decodeAssess(t, r); !containsString(assessReasons(a), "worktree_provenance_unobserved") {
		t.Errorf("blockers %v", assessReasons(a))
	}
}

func TestShaperAssess_UsageErrorsExit1(t *testing.T) {
	root, _ := shaperWorktree(t, shaperTestHandoff, shaperTestGoal("standalone-shaper-handoff", `[]`))
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"no verb", nil},
		{"unknown verb", []string{"bless"}},
		{"missing root", []string{"assess", "--handoff", "handoff.json", "--goal", "goal.json"}},
		{"relative root", []string{"assess", "--root", "rel", "--handoff", "handoff.json", "--goal", "goal.json"}},
		{"missing goal", []string{"assess", "--root", root, "--handoff", "handoff.json"}},
		{"flag without value", []string{"assess", "--root", root, "--handoff", "handoff.json", "--goal"}},
		{"unknown flag", assessArgs(root, "--force")},
		{"positional arg", assessArgs(root, "extra")},
		{"missing handoff file", []string{"assess", "--root", root, "--handoff", "nope.json", "--goal", "goal.json"}},
		{"traversing handoff", []string{"assess", "--root", root, "--handoff", "../handoff.json", "--goal", "goal.json"}},
		{"stdin on assess", assessArgs(root, "--stdin")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := runShaperTest(tc.args, "")
			if r.code != 1 {
				t.Errorf("exit %d, want 1; stdout %q stderr %q", r.code, r.stdout, r.stderr)
			}
			if r.stderr == "" {
				t.Errorf("no error message")
			}
		})
	}
}

func TestShaperAssessOutput_ReadyStatesForgeryDisclosure(t *testing.T) {
	ready := shaper.Assessment{State: shaper.StateReady}
	for _, view := range []bool{false, true} {
		var out, errBuf bytes.Buffer
		code := writeShaperAssessment(&out, &errBuf, ready, clearanceReport{Status: "verified"}, view, 1)
		if code != 0 {
			t.Errorf("view=%v: exit %d, want 0 for ready", view, code)
		}
		text := out.String() + errBuf.String()
		for _, want := range []string{"not a signature", "same OS user", "any installed Pi extension", "Phase 3"} {
			if !strings.Contains(text, want) {
				t.Errorf("view=%v: ready output lacks %q:\n%s", view, want, text)
			}
		}
	}
}

func TestShaperClearanceRecord_StoresAffirmAndAssessVerifiesIt(t *testing.T) {
	root, stateHome := shaperWorktree(t, shaperTestHandoff, shaperTestGoal("standalone-shaper-handoff", `["Extract jsonstrict."]`))
	a := decodeAssess(t, runShaperTest(assessArgs(root), ""))
	rec := recordFromAssess(t, a, "affirm", "tui", nil)

	r := runShaperTest(recordArgs(root, "--stdin"), rec)
	if r.code != 0 {
		t.Fatalf("record exit %d; stdout %q stderr %q", r.code, r.stdout, r.stderr)
	}
	want := filepath.Join(stateHome, "labdrian", "shaper-clearance", "standalone-shaper-handoff", "goal-alpha", a.Subject.HandoffSHA256+".json")
	data, err := os.ReadFile(want)
	if err != nil {
		t.Fatalf("record not at %s: %v (stdout %q)", want, err, r.stdout)
	}
	if string(data) != rec {
		t.Errorf("stored bytes differ from stdin bytes")
	}
	if info, _ := os.Stat(want); info.Mode().Perm() != 0o600 {
		t.Errorf("record mode %v", info.Mode().Perm())
	}
	if info, _ := os.Stat(filepath.Dir(want)); info.Mode().Perm() != 0o700 {
		t.Errorf("dir mode %v", info.Mode().Perm())
	}
	if !strings.Contains(r.stdout, want) || !strings.Contains(r.stdout, "not a signature") {
		t.Errorf("record stdout %q", r.stdout)
	}

	if again := runShaperTest(recordArgs(root, "--stdin"), rec); again.code != 0 {
		t.Errorf("identical re-record exit %d; stderr %q", again.code, again.stderr)
	}
	informational := true
	different := recordFromAssess(t, a, "affirm", "tui", map[string]any{"host_time": map[string]any{"value": "t", "informational": informational}})
	if diff := runShaperTest(recordArgs(root, "--stdin"), different); diff.code != 1 || !strings.Contains(diff.stderr, "immutable") {
		t.Errorf("different bytes: exit %d stderr %q", diff.code, diff.stderr)
	}

	after := runShaperTest(assessArgs(root), "")
	if after.code != 3 {
		t.Fatalf("assess after record exit %d, want 3 (handoff v1 stays draft)", after.code)
	}
	got := decodeAssess(t, after)
	if got.Clearance.Status != "verified" {
		t.Errorf("clearance %+v, want verified", got.Clearance)
	}
	reasons := assessReasons(got)
	if containsString(reasons, "clearance_missing") || containsString(reasons, "flags_unresolved") {
		t.Errorf("verified clearance still blocked: %v", reasons)
	}
	if !containsString(reasons, "acceptance_verification_unrepresentable") || got.State != "draft" {
		t.Errorf("handoff v1 reached %q with %v", got.State, reasons)
	}
}

func TestShaperClearanceRecord_DeclineIsStoredAndNeverClears(t *testing.T) {
	root, _ := shaperWorktree(t, shaperTestHandoff, shaperTestGoal("standalone-shaper-handoff", `[]`))
	a := decodeAssess(t, runShaperTest(assessArgs(root), ""))
	if r := runShaperTest(recordArgs(root, "--stdin"), recordFromAssess(t, a, "decline", "tui", nil)); r.code != 0 {
		t.Fatalf("decline record exit %d; stderr %q", r.code, r.stderr)
	}
	got := decodeAssess(t, runShaperTest(assessArgs(root), ""))
	if got.Clearance.Status != "refused" || !strings.Contains(got.Clearance.Detail, "decline") {
		t.Errorf("clearance %+v, want refused decline", got.Clearance)
	}
	if !containsString(assessReasons(got), "clearance_missing") {
		t.Errorf("blockers %v", assessReasons(got))
	}
}

func TestShaperClearanceRecord_Refusals(t *testing.T) {
	root, stateHome := shaperWorktree(t, shaperTestHandoff, shaperTestGoal("standalone-shaper-handoff", `["Extract jsonstrict."]`))
	a := decodeAssess(t, runShaperTest(assessArgs(root), ""))
	good := recordFromAssess(t, a, "affirm", "tui", nil)
	badView := strings.Replace(good, a.Subject.ViewSHA256, strings.Repeat("0", 64), 1)
	unresolved := strings.Replace(good, `"flag_resolutions":[{"evidence":"Read the view.","flag_id":"goal_non_goal_overlap:stages:1","reason":"Intended."}]`, `"flag_resolutions":[]`, 1)
	if unresolved == good {
		t.Fatalf("fixture did not drop the resolution: %s", good)
	}
	for _, tc := range []struct {
		name       string
		args       []string
		stdin      string
		agentChild bool
		match      string
	}{
		{"no --stdin", recordArgs(root), good, false, "--stdin"},
		{"record content in argv", recordArgs(root, "--stdin", good), good, false, "unexpected argument"},
		{"unknown flag", recordArgs(root, "--stdin", "--record", "x"), good, false, "unknown flag"},
		{"empty stdin", recordArgs(root, "--stdin"), "", false, "empty"},
		{"oversized stdin", recordArgs(root, "--stdin"), strings.Repeat(" ", shaper.MaxRecordBytes+1), false, "exceeds"},
		{"view digest mismatch", recordArgs(root, "--stdin"), badView, false, "view_sha256"},
		{"missing flag resolution", recordArgs(root, "--stdin"), unresolved, false, "unresolved"},
		{"rpc channel", recordArgs(root, "--stdin"), recordFromAssess(t, a, "affirm", "rpc", nil), false, "tui"},
		{"gentle-pi child", recordArgs(root, "--stdin"), good, true, "GENTLE_PI_AGENTS_CHILD"},
		{"not json", recordArgs(root, "--stdin"), "{", false, "parse"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := testDeps()
			d.agentChild = tc.agentChild
			r := runShaperTestWith(d, tc.args, tc.stdin)
			if r.code != 1 {
				t.Fatalf("exit %d, want 1; stdout %q stderr %q", r.code, r.stdout, r.stderr)
			}
			if !strings.Contains(r.stderr, tc.match) {
				t.Errorf("stderr %q does not name %q", r.stderr, tc.match)
			}
			if files := listFiles(t, stateHome); len(files) != 0 {
				t.Errorf("refused record wrote %v", files)
			}
		})
	}
}

// Whether the command runs inside an agent child is main's to say (deps.agentChild, resolved from
// the environment once): the record verb does not look at the variable itself.
func TestShaperClearanceRecord_DoesNotReadTheEnvironmentForTheAgentChild(t *testing.T) {
	root, _ := shaperWorktree(t, shaperTestHandoff, shaperTestGoal("standalone-shaper-handoff", `["Extract jsonstrict."]`))
	a := decodeAssess(t, runShaperTest(assessArgs(root), ""))
	t.Setenv("GENTLE_PI_AGENTS_CHILD", "1")
	d := testDeps()
	d.agentChild = false

	r := runShaperTestWith(d, recordArgs(root, "--stdin"), recordFromAssess(t, a, "affirm", "tui", nil))

	if r.code != 0 {
		t.Errorf("exit %d with deps.agentChild false and the variable set, want 0 (the record is stored); stderr %q", r.code, r.stderr)
	}
}

// The refusal names the variable and the value of the agent child by the constants main reads them
// with, so the message cannot drift from what is read.
func TestShaperRefusalInAnAgentChildNamesTheVariableAndValueOfTheConstants(t *testing.T) {
	root, _ := shaperWorktree(t, shaperTestHandoff, shaperTestGoal("standalone-shaper-handoff", `[]`))
	d := testDeps()
	d.agentChild = true

	r := runShaperTestWith(d, recordArgs(root, "--stdin"), "{}")

	want := "error: shaper clearance record: refusing inside a gentle-pi agent child (" + agentChildVariable + "=" + agentChildValue + "): no human answers its dialogs\n"
	if r.stderr != want || r.code != 1 {
		t.Errorf("exit %d, stderr %q, want 1 and %q", r.code, r.stderr, want)
	}
	if !strings.Contains(r.stderr, "(GENTLE_PI_AGENTS_CHILD=1)") {
		t.Errorf("stderr %q no longer names GENTLE_PI_AGENTS_CHILD=1, the variable the gentle-pi extension sets", r.stderr)
	}
}

// TestShaperAssess_RefusesRPCRecordPlacedInStore proves the read side, not
// only the record CLI, refuses an RPC-captured affirm: a record written
// straight into the store (bypassing 'clearance record') must not verify.
func TestShaperAssess_RefusesRPCRecordPlacedInStore(t *testing.T) {
	root, stateHome := shaperWorktree(t, shaperTestHandoff, shaperTestGoal("standalone-shaper-handoff", `["Extract jsonstrict."]`))
	a := decodeAssess(t, runShaperTest(assessArgs(root), ""))
	path := filepath.Join(stateHome, "labdrian", "shaper-clearance", "standalone-shaper-handoff", "goal-alpha", a.Subject.HandoffSHA256+".json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(recordFromAssess(t, a, "affirm", "rpc", nil)), 0o600); err != nil {
		t.Fatal(err)
	}
	got := decodeAssess(t, runShaperTest(assessArgs(root), ""))
	if got.Clearance.Status != "refused" || !strings.Contains(got.Clearance.Detail, "tui") {
		t.Errorf("clearance %+v, want refused naming tui", got.Clearance)
	}
	reasons := assessReasons(got)
	if !containsString(reasons, "clearance_missing") || !containsString(reasons, "flags_unresolved") {
		t.Errorf("rpc record cleared blockers: %v", reasons)
	}
}

func TestShaperClearanceRecord_RefusesAfterSourceDrift(t *testing.T) {
	root, stateHome := shaperWorktree(t, shaperTestHandoff, shaperTestGoal("standalone-shaper-handoff", `[]`))
	a := decodeAssess(t, runShaperTest(assessArgs(root), ""))
	rec := recordFromAssess(t, a, "affirm", "tui", nil)
	if err := os.WriteFile(filepath.Join(root, "goal.json"), []byte(shaperTestGoal("standalone-shaper-handoff", `["x"]`)), 0o600); err != nil {
		t.Fatal(err)
	}
	r := runShaperTest(recordArgs(root, "--stdin"), rec)
	if r.code != 1 || !strings.Contains(r.stderr, "goal_sha256") {
		t.Errorf("drift: exit %d stderr %q", r.code, r.stderr)
	}
	if files := listFiles(t, stateHome); len(files) != 0 {
		t.Errorf("drifted record wrote %v", files)
	}
}

func TestShaperGuardHook_DeniesRecordAndAllowsOthers(t *testing.T) {
	deny := runShaperTest([]string{"guard-hook"}, `{"tool_name":"Bash","tool_input":{"command":"gentle-ai-overlay shaper clearance record --stdin"}}`)
	if deny.code != 2 || !strings.Contains(deny.stderr, "speed bump") {
		t.Errorf("deny: exit %d stderr %q", deny.code, deny.stderr)
	}
	allow := runShaperTest([]string{"guard-hook"}, `{"tool_name":"Bash","tool_input":{"command":"ls"}}`)
	if allow.code != 0 || allow.stderr != "" {
		t.Errorf("allow: exit %d stderr %q", allow.code, allow.stderr)
	}
}

// TestShaperGuardHook_JudgesPayloadsLargerThanTheStdinLimit pins that the
// guard reads the whole PreToolUse payload, up to its bound (the one of every
// guard, hookwire.MaxToolCallBytes). The guard runs on every Bash and
// Write/Edit call, so truncating a large unrelated payload would deny it on a
// JSON decode failure instead of judging it; a large payload that does carry
// the marker must still be denied.
func TestShaperGuardHook_JudgesPayloadsLargerThanTheStdinLimit(t *testing.T) {
	big := strings.Repeat("a", stdinSizeLimit+1024)
	allow := runShaperTest([]string{"guard-hook"}, `{"tool_name":"Write","tool_input":{"file_path":"/tmp/big.txt","content":"`+big+`"}}`)
	if allow.code != 0 || allow.stderr != "" {
		t.Errorf("large unrelated write: exit %d stderr %q", allow.code, allow.stderr)
	}
	deny := runShaperTest([]string{"guard-hook"}, `{"tool_name":"Bash","tool_input":{"command":"echo `+big+` && labdrian shaper clearance record --stdin"}}`)
	if deny.code != 2 {
		t.Errorf("large payload carrying the marker: exit %d, want 2", deny.code)
	}
}

// TestShaperGuardHook_DeniesWhatIsOverTheBound pins the other side of the bound. The guard fails
// closed: a call it cannot read it cannot vouch for, and a call over the bound is not read, so it
// is denied with the words that say why, as a call that is not JSON is. The bound is inclusive:
// a call of exactly that many bytes is judged as any other.
func TestShaperGuardHook_DeniesWhatIsOverTheBound(t *testing.T) {
	padded := func(prefix string, size int) string {
		const suffix = `"}}`
		return prefix + strings.Repeat("x", size-len(prefix)-len(suffix)) + suffix
	}
	const write = `{"tool_name":"Write","tool_input":{"file_path":"/tmp/big.txt","content":"`
	at := runShaperTest([]string{"guard-hook"}, padded(write, hookwire.MaxToolCallBytes))
	if at.code != 0 || at.stderr != "" {
		t.Errorf("an unrelated write of exactly the bound: exit %d stderr %q, want it judged and allowed", at.code, at.stderr)
	}
	atMarked := runShaperTest([]string{"guard-hook"}, padded(`{"tool_name":"Bash","tool_input":{"command":"shaper clearance record `, hookwire.MaxToolCallBytes))
	if atMarked.code != 2 || strings.Contains(atMarked.stderr, "too large") {
		t.Errorf("a command naming the record verb, of exactly the bound: exit %d stderr %q, want it judged and denied for the command", atMarked.code, atMarked.stderr)
	}
	over := runShaperTest([]string{"guard-hook"}, padded(write, hookwire.MaxToolCallBytes+1))
	if over.code != 2 {
		t.Fatalf("an unrelated write one byte over the bound: exit %d, want 2: the guard fails closed", over.code)
	}
	wantEnding := " (this tool call is larger than the " + strconv.Itoa(hookwire.MaxToolCallBytes) + " bytes the guard reads, so the guard denied it; send a smaller call)\n"
	if !strings.HasPrefix(over.stderr, shaperGuardDenial) || !strings.HasSuffix(over.stderr, wantEnding) {
		t.Errorf("the denial is %q, want the guard's message and then %q", over.stderr, wantEnding)
	}
}

// shaperGuardDenial is the start of every denial of the clearance guard, the same for a call that
// names the record and for one the guard could not read.
const shaperGuardDenial = "labdrian shaper clearance guard: recording a clearance or touching the clearance store is reserved for the human"

// The denial for a call the guard cannot read is one stable sentence, the same for every shape
// the call can have: it says what the guard could not read and what to do, and it carries
// nothing of the JSON library or of the type the call is read into, which are not the model's to
// depend on and change with a Go release or a rename.
func TestShaperGuardHook_TheDenialOfAShapeItCannotReadIsOneStableSentence(t *testing.T) {
	const wantEnding = " (this tool call could not be read as a command or a file path, so the guard denied it; send it again as a well-formed tool call)\n"
	for name, in := range map[string]string{
		"empty":                    ``,
		"not JSON":                 `hello`,
		"a truncated object":       `{"tool_name":"Bash","tool_input":{"command":"ls"`,
		"two objects":              `{"tool_name":"Bash"}{"tool_name":"Bash"}`,
		"an array":                 `[1]`,
		"a string":                 `"Bash"`,
		"tool_name a number":       `{"tool_name":7}`,
		"tool_input a string":      `{"tool_input":"ls"}`,
		"tool_input an array":      `{"tool_input":[1]}`,
		"command a number":         `{"tool_input":{"command":5}}`,
		"file_path an object":      `{"tool_input":{"file_path":{}}}`,
		"notebook_path a boolean":  `{"tool_input":{"notebook_path":false}}`,
		"two fields of wrong type": `{"tool_name":7,"tool_input":{"command":5}}`,
	} {
		t.Run(name, func(t *testing.T) {
			r := runShaperTest([]string{"guard-hook"}, in)
			if r.code != 2 || !strings.HasPrefix(r.stderr, shaperGuardDenial) || !strings.HasSuffix(r.stderr, wantEnding) || strings.Count(r.stderr, "\n") != 1 {
				t.Errorf("exit %d, stderr %q: want exit 2 and the guard's message, then %q", r.code, r.stderr, wantEnding)
			}
			for _, leak := range []string{"json", "JSON", "Go ", "guardHookInput", "unmarshal", "struct", "hookwire", "shaper."} {
				if strings.Contains(r.stderr, leak) {
					t.Errorf("the denial %q carries %q, a word of the implementation", r.stderr, leak)
				}
			}
		})
	}
}

// TestShaperGuardHook_ReadsAtMostTheBoundPlusOneByte: input that never ends is never read to the
// end, nor held in memory whole. One byte past the bound is enough to know the input is over it.
func TestShaperGuardHook_ReadsAtMostTheBoundPlusOneByte(t *testing.T) {
	src := &endlessReader{}
	var out, errBuf bytes.Buffer
	var codes []int
	runShaperCore(testDeps(), []string{"guard-hook"}, src, &out, &errBuf, func(c int) { codes = append(codes, c) })
	if !reflect.DeepEqual(codes, []int{2}) || !strings.Contains(errBuf.String(), "is larger than the") {
		t.Errorf("exits %v, stderr %q, want a denial that names the bound", codes, errBuf.String())
	}
	// endlessReader (projection_hook_test.go) counts the bytes it was asked for.
	if limit := hookwire.MaxToolCallBytes + 1; src.read != limit {
		t.Errorf("read %d bytes, want exactly %d: up to one byte past the bound", src.read, limit)
	}
}

func TestShaperTestSha(t *testing.T) {
	// Guards the fixture helper against drift from the shaper digest.
	sum := sha256.Sum256([]byte("x"))
	if shaper.SourceSHA256([]byte("x")) != hex.EncodeToString(sum[:]) {
		t.Fatal("SourceSHA256 is not lowercase hex SHA-256")
	}
}

// TestShaperAssess_RefusesUnpresentableView proves a Goal scope that decodes
// to a terminal control sequence is refused on every path: assess reports the
// refused blocker, --view prints nothing and exits non-zero naming it, and
// clearance record refuses before writing the store.
func TestShaperAssess_RefusesUnpresentableView(t *testing.T) {
	goal := strings.Replace(shaperTestGoal("standalone-shaper-handoff", `[]`), `"scope":"One project."`, `"scope":"One\u001b[8m hidden project."`, 1)
	root, stateHome := shaperWorktree(t, shaperTestHandoff, goal)

	a := decodeAssess(t, runShaperTest(assessArgs(root), ""))
	if !containsString(assessReasons(a), "view_unpresentable") {
		t.Fatalf("blockers %v lack view_unpresentable", assessReasons(a))
	}
	if a.Subject != nil {
		t.Errorf("subject %+v present for an unpresentable view", a.Subject)
	}

	v := runShaperTest(assessArgs(root, "--view"), "")
	if v.code == 0 {
		t.Errorf("--view exit 0, want non-zero")
	}
	if v.stdout != "" {
		t.Errorf("--view printed %q for an unpresentable view", v.stdout)
	}
	if !strings.Contains(v.stderr, "view_unpresentable") || !strings.Contains(v.stderr, "goal_scope") {
		t.Errorf("--view stderr %q does not name view_unpresentable and the section", v.stderr)
	}

	r := runShaperTest(recordArgs(root, "--stdin"), `{"version":1}`)
	if r.code != 1 || !strings.Contains(r.stderr, "view_unpresentable") {
		t.Errorf("record exit %d stderr %q, want 1 naming view_unpresentable", r.code, r.stderr)
	}
	if files := listFiles(t, stateHome); len(files) != 0 {
		t.Errorf("refused record wrote %v", files)
	}
}
