package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/shaper"
)

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
	var out, errBuf bytes.Buffer
	code := -1
	exited := false
	runShaperCore(args, strings.NewReader(stdin), &out, &errBuf, func(c int) {
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
		name  string
		args  []string
		stdin string
		env   map[string]string
		match string
	}{
		{"no --stdin", recordArgs(root), good, nil, "--stdin"},
		{"record content in argv", recordArgs(root, "--stdin", good), good, nil, "unexpected argument"},
		{"unknown flag", recordArgs(root, "--stdin", "--record", "x"), good, nil, "unknown flag"},
		{"empty stdin", recordArgs(root, "--stdin"), "", nil, "empty"},
		{"oversized stdin", recordArgs(root, "--stdin"), strings.Repeat(" ", stdinSizeLimit+1), nil, "exceeds"},
		{"view digest mismatch", recordArgs(root, "--stdin"), badView, nil, "view_sha256"},
		{"missing flag resolution", recordArgs(root, "--stdin"), unresolved, nil, "unresolved"},
		{"rpc channel", recordArgs(root, "--stdin"), recordFromAssess(t, a, "affirm", "rpc", nil), nil, "tui"},
		{"gentle-pi child", recordArgs(root, "--stdin"), good, map[string]string{"GENTLE_PI_AGENTS_CHILD": "1"}, "GENTLE_PI_AGENTS_CHILD"},
		{"not json", recordArgs(root, "--stdin"), "{", nil, "parse"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			r := runShaperTest(tc.args, tc.stdin)
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
// guard reads the whole PreToolUse payload. The guard runs on every Bash and
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

func TestCheckShaperClearanceGuard(t *testing.T) {
	full := buildSettingsWithHooks("/x/gentle-ai-overlay")
	if c := checkShaperClearanceGuard(full, nil, "s.json"); !c.ok || c.degraded {
		t.Errorf("full guard: %+v", c)
	}
	delete(full, "permissions")
	c := checkShaperClearanceGuard(full, nil, "s.json")
	if !c.ok || !c.degraded || !strings.Contains(c.note, settings.ShaperClearanceDenyRule) || !strings.Contains(c.note, "speed bump") {
		t.Errorf("missing deny rule: %+v", c)
	}
	if c := checkShaperClearanceGuard(nil, nil, "s.json"); !c.degraded {
		t.Errorf("absent settings: %+v", c)
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
